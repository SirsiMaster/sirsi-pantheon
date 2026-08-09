package liveness

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// TestPlistContent_LeverShape enforces the LaunchAgent's load-bearing shape: it
// invokes `sirsi liveness-watch run`, fires at load AND on the interval, and does
// NOT set KeepAlive (a run-and-exit job under KeepAlive would tight-loop).
func TestPlistContent_LeverShape(t *testing.T) {
	got := PlistContent("/usr/local/bin/sirsi", "/repo/root")
	for _, want := range []string{
		"<string>ai.sirsi.liveness-watch</string>",
		"<string>/usr/local/bin/sirsi</string>",
		"<string>liveness-watch</string>",
		"<string>run</string>",
		"<key>RunAtLoad</key>",
		"<key>StartInterval</key>",
		"<string>/repo/root</string>", // WorkingDirectory pins the router root
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist missing %q\n---\n%s", want, got)
		}
	}
	if strings.Contains(got, "<key>KeepAlive</key>") {
		t.Error("plist must NOT set KeepAlive — the watch runs and exits; KeepAlive would tight-loop it")
	}
}

// TestRun_RoutesOnceAndDedups is the flood guard: a persistent blocker must
// route exactly ONE open item to `user`, not one every tick. We simulate a
// down broker (no port file) on a clean router root, then assert the second Run
// adds no duplicate.
func TestRun_RoutesOnceAndDedups(t *testing.T) {
	root := t.TempDir()
	writeLivenessTestAgents(t, root)
	// Ensure the gemma probe reads "down": point HOME at a dir with no
	// gemma-server.port, so probeGemma returns down deterministically.
	home := t.TempDir()
	t.Setenv("HOME", home)
	// A down broker only alarms when it is INSTALLED — an absent plist is a
	// deliberate retirement/quarantine and is suppressed (suppressGemmaDown).
	installFakeBrokerPlist(t, home)
	// Own store per test: Run dispatches through the router store now, and the
	// package-level db would let a sibling test's alert dedupe this one away.
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "router.db"))

	var buf bytes.Buffer
	if err := Run(root, &buf); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	first := openCount(t, root, "claude-pantheon")
	if first != 1 {
		t.Fatalf("first Run routed %d items to claude-pantheon, want exactly 1\n%s", first, buf.String())
	}

	buf.Reset()
	if err := Run(root, &buf); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if second := openCount(t, root, "claude-pantheon"); second != 1 {
		t.Errorf("second Run left %d open items, want 1 (dedup failed)\n%s", second, buf.String())
	}
	if !strings.Contains(buf.String(), "skip") {
		t.Errorf("second Run should report a dedup skip, got:\n%s", buf.String())
	}
}

// TestProbeSessionLeak_Threshold confirms the leaked-session probe stays OK
// below the threshold and alerts (alert-only, never reaps) at/above it.
func TestProbeSessionLeak_Threshold(t *testing.T) {
	old := getSessionLeakCount()
	t.Cleanup(func() { setSessionLeakCount(old) })

	setSessionLeakCount(func() (int, int) { return sessionLeakThreshold - 1, 100 })
	if f := probeSessionLeak(); !f.OK {
		t.Errorf("below threshold should be OK, got %+v", f)
	}
	setSessionLeakCount(func() (int, int) { return sessionLeakThreshold, 900 })
	f := probeSessionLeak()
	if f.OK {
		t.Errorf("at threshold should alert, got OK: %+v", f)
	}
	if !f.Fixable || f.Title == "" {
		t.Errorf("alert must be a routable fixable finding, got %+v", f)
	}
}

// TestPickWorst_SeverityOrder confirms the broker outranks memory outranks
// menubar, and that an OK or non-fixable finding is never routed.
func TestPickWorst_SeverityOrder(t *testing.T) {
	broker := Finding{Check: "gemma-broker", Fixable: true, Title: "b"}
	mem := Finding{Check: "memory-death", Fixable: true, Title: "m"}
	if w := pickWorst([]Finding{broker, mem}); w == nil || w.Check != "gemma-broker" {
		t.Errorf("broker should outrank memory, got %+v", w)
	}
	okFinding := Finding{Check: "gemma-broker", OK: true, Fixable: true}
	nonFixable := Finding{Check: "menubar", OK: false, Fixable: false}
	if w := pickWorst([]Finding{okFinding, nonFixable}); w != nil {
		t.Errorf("no fixable non-OK finding → nil, got %+v", w)
	}
}

// TestProbeGemmaState_RetryOnTimeout is the busy-vs-wedged regression guard
// (recurring alerts 20260724). The broker serializes requests, so a probe firing
// while the triage loop / gemma-worker is mid-generation queues behind it and
// times out on a HEALTHY broker. A timed-out probe must retry once: a busy
// broker answers on retry (healthy); a truly wedged one fails both (wedged).
func TestProbeGemmaState_RetryOnTimeout(t *testing.T) {
	// Shrink the timing so the timeout path runs fast.
	oldT, oldP := probeTimeout, probeRetryPause
	probeTimeout, probeRetryPause = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { probeTimeout, probeRetryPause = oldT, oldP })
	oldRunner := getRunnerWorkerActive()
	t.Cleanup(func() { setRunnerWorkerActive(oldRunner) })

	healthy := `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"completion_tokens":1}}`

	t.Run("busy_then_answers_is_healthy", func(t *testing.T) {
		var n int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if atomic.AddInt32(&n, 1) == 1 {
				time.Sleep(400 * time.Millisecond) // first attempt: busy → client times out
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(healthy)) // retry: answers
		}))
		defer srv.Close()
		got, detail := ProbeGemmaState(homeWithPort(t, srv.URL))
		if got != GemmaHealthy {
			t.Errorf("busy-then-answers = %v (%s), want GemmaHealthy", got, detail)
		}
	})

	t.Run("always_hangs_is_wedged", func(t *testing.T) {
		setRunnerWorkerActive(func() bool { return false })
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(400 * time.Millisecond) // every attempt times out
		}))
		defer srv.Close()
		got, detail := ProbeGemmaState(homeWithPort(t, srv.URL))
		if got != GemmaWedged {
			t.Errorf("always-hangs = %v (%s), want GemmaWedged", got, detail)
		}
	})

	t.Run("always_hangs_with_runner_is_busy_not_wedged", func(t *testing.T) {
		setRunnerWorkerActive(func() bool { return true })
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(400 * time.Millisecond) // every attempt times out
		}))
		defer srv.Close()
		got, detail := ProbeGemmaState(homeWithPort(t, srv.URL))
		if got != GemmaBusy {
			t.Errorf("always-hangs-with-runner = %v (%s), want GemmaBusy", got, detail)
		}
	})
}

// homeWithPort writes a gemma-server.port pointing at srvURL's port under a temp
// HOME, returning that HOME for ProbeGemmaState.
func homeWithPort(t *testing.T, srvURL string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	port := strings.TrimPrefix(srvURL, "http://127.0.0.1:")
	if err := os.WriteFile(filepath.Join(home, ".sirsi/gemma-server.port"), []byte(port), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// TestProbeGemmaState_Classification is the false-positive regression guard.
// A reasoning model (gemma-4) spends a small token budget in `message.reasoning`
// and legitimately leaves `message.content` empty on a HEALTHY broker — the old
// "empty content ⇒ wedged" rule paged the owner every cycle. The probe now
// asserts transport truth (normal finish_reason + tokens produced), so:
//   - reasoning-shaped response (empty content, non-empty reasoning) ⇒ healthy
//   - genuinely empty response (no tokens, no finish) ⇒ wedged
func TestProbeGemmaState_Classification(t *testing.T) {
	cases := []struct {
		name string
		body string
		want GemmaStatus
	}{
		{"reasoning_model_empty_content", `{"choices":[{"message":{"content":"","reasoning":"The user wants me to"},"finish_reason":"length"}],"usage":{"completion_tokens":32}}`, GemmaHealthy},
		{"plain_content", `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"completion_tokens":1}}`, GemmaHealthy},
		{"no_tokens_generated", `{"choices":[{"message":{"content":""},"finish_reason":""}],"usage":{"completion_tokens":0}}`, GemmaWedged},
		{"no_choices", `{"choices":[],"usage":{"completion_tokens":0}}`, GemmaWedged},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			home := t.TempDir()
			if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
				t.Fatal(err)
			}
			// httptest binds 127.0.0.1:PORT; hand the probe that port.
			port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")
			if err := os.WriteFile(filepath.Join(home, ".sirsi/gemma-server.port"), []byte(port), 0o644); err != nil {
				t.Fatal(err)
			}

			got, detail := ProbeGemmaState(home)
			if got != tc.want {
				t.Errorf("ProbeGemmaState = %v (%s), want %v", got, detail, tc.want)
			}
		})
	}
}

func openCount(t *testing.T, root, agent string) int {
	t.Helper()
	items, err := work.ListInbox(root, agent)
	if err != nil {
		// A missing items dir means zero — not a failure.
		if os.IsNotExist(err) || !dirExists(filepath.Join(root, "items")) {
			return 0
		}
		t.Fatalf("ListInbox: %v", err)
	}
	return len(items)
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func writeLivenessTestAgents(t *testing.T, root string) {
	t.Helper()
	registry := `{
		"agents": {
			"horus": {"id":"horus","type":"service","repo":"/tmp","workstream":"pantheon","wake":{"mechanism":"none"}},
			"owner": {"id":"owner","type":"human","repo":"/tmp","workstream":"portfolio","wake":{"mechanism":"owner-surface"}},
			"claude-pantheon": {"id":"claude-pantheon","type":"claude","cwd":"/tmp","workstream":"pantheon","wake":{"mechanism":"launchagent"}}
		}
	}`
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(registry), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestProbeGemmaState_RSSFloor is the weightless-broker regression guard for
// the FALLBACK path: /health is unreachable, so the check falls back to RSS.
// A broker with RSS below 1 GB cannot have model weights loaded — this is the
// class that /health passes but generation probes see as wedged, and that a
// restart cannot fix when the HF model cache is absent (2026-08-05 incident).
//
// The floor fires AFTER the generation probe, not before (2026-08-09 fix,
// router item 20260809-060146): weights load lazily on first request, so an
// idle-but-healthy broker legitimately has RSS/mlx_active_bytes near zero and
// must not be misdiagnosed as weightless. A genuinely weightless broker can't
// generate either, so the test server here returns a wedged (no-tokens)
// response — matching what a real weights-absent broker actually does — and
// the floor's job is to upgrade that generic wedge into the specific
// "weights likely absent" diagnosis.
func TestProbeGemmaState_RSSFloor(t *testing.T) {
	old := getBrokerRSSFn()
	t.Cleanup(func() { setBrokerRSSFn(old) })
	oldMLX := getBrokerMLXBytesFn()
	t.Cleanup(func() { setBrokerMLXBytesFn(oldMLX) })

	// /health unreachable — forces the RSS fallback path.
	setBrokerMLXBytesFn(func() (int64, bool) { return 0, false })

	// Stub a tiny RSS (100 MB — well below the 1 GB floor).
	const tinyRSSKB = 100 * 1024 // 100 MB
	setBrokerRSSFn(func(_ string) int64 { return tinyRSSKB })

	// A weightless broker cannot generate — the probe is called and fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[],"usage":{"completion_tokens":0}}`))
	}))
	defer srv.Close()

	got, detail := ProbeGemmaState(homeWithPort(t, srv.URL))
	if got != GemmaWedged {
		t.Errorf("low RSS = %v (%s), want GemmaWedged", got, detail)
	}
	if !strings.Contains(detail, "weight floor") {
		t.Errorf("detail should mention weight floor, got: %s", detail)
	}
	if !strings.Contains(detail, "restart will not fix") {
		t.Errorf("detail should say restart will not fix, got: %s", detail)
	}
	// The bind that makes the const load-bearing: the self-healing duty in
	// internal/router/gemmaliveness.go short-circuits on WeightsAbsentSentinel.
	// Without this assertion, rewording the detail above leaves every test green
	// while the production short-circuit silently stops firing (A35).
	if !strings.Contains(detail, WeightsAbsentSentinel) {
		t.Errorf("detail must embed WeightsAbsentSentinel (%q) or the router short-circuit breaks, got: %s",
			WeightsAbsentSentinel, detail)
	}
}

// TestProbeGemmaState_IdleBrokerIsNeverMisreadAsWeightless is the lazy-init
// regression guard (2026-08-09, router item 20260809-060146): a freshly
// restarted, idle broker reports mlx_active_bytes==0 legitimately (weights
// load on first request, not at process start). It must classify as healthy
// off the functional probe alone, never as "weights absent" off the floor.
func TestProbeGemmaState_IdleBrokerIsNeverMisreadAsWeightless(t *testing.T) {
	oldMLX := getBrokerMLXBytesFn()
	t.Cleanup(func() { setBrokerMLXBytesFn(oldMLX) })
	// Idle broker, not yet asked to generate: /health reports zero active bytes.
	setBrokerMLXBytesFn(func() (int64, bool) { return 0, true })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"completion_tokens":1}}`))
	}))
	defer srv.Close()

	got, detail := ProbeGemmaState(homeWithPort(t, srv.URL))
	if got != GemmaHealthy {
		t.Errorf("idle broker (mlx_active_bytes=0, answers fine) = %v (%s), want GemmaHealthy", got, detail)
	}
	if strings.Contains(detail, WeightsAbsentSentinel) {
		t.Errorf("idle-but-answering broker must never carry the weights-absent sentinel, got: %s", detail)
	}
}

// TestProbeGemmaState_RSSFloor_ZeroSkips verifies that a zero RSS (PID file
// absent or ps unavailable) does not falsely classify a healthy broker as
// weightless — fail-open is required (never falsely reap a live broker).
func TestProbeGemmaState_RSSFloor_ZeroSkips(t *testing.T) {
	old := getBrokerRSSFn()
	t.Cleanup(func() { setBrokerRSSFn(old) })
	oldMLX := getBrokerMLXBytesFn()
	t.Cleanup(func() { setBrokerMLXBytesFn(oldMLX) })

	setBrokerMLXBytesFn(func() (int64, bool) { return 0, false }) // /health unreachable too
	setBrokerRSSFn(func(_ string) int64 { return 0 })             // pid file absent

	healthy := `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"completion_tokens":1}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(healthy))
	}))
	defer srv.Close()

	got, detail := ProbeGemmaState(homeWithPort(t, srv.URL))
	if got != GemmaHealthy {
		t.Errorf("zero RSS (pid absent) with healthy server = %v (%s), want GemmaHealthy (fail-open)", got, detail)
	}
}

// TestProbeGemmaState_MLXFloor_RecognizesLargeAllocation is the R6 regression
// guard (PRD SNE_HETEROGENEOUS_COMPUTE.md, sirsi-pantheon-fabric): a 30 GB+
// Metal allocation, reported via mlx_active_bytes, MUST be recognized as a
// loaded model — even though RSS for the same broker was measured at 4.2 GB
// (a 27 GB gap). Before this fix, a check that floored on RSS could not tell
// this state apart from "weights absent" at all; this proves the mlx_active_bytes
// signal is read and is authoritative over the (here, misleadingly tiny) RSS.
func TestProbeGemmaState_MLXFloor_RecognizesLargeAllocation(t *testing.T) {
	old := getBrokerRSSFn()
	t.Cleanup(func() { setBrokerRSSFn(old) })
	oldMLX := getBrokerMLXBytesFn()
	t.Cleanup(func() { setBrokerMLXBytesFn(oldMLX) })

	// Measured 2026-08-06: mlx_active_bytes 24.9-31.4 GB. Use a value on the
	// high end of that range, in bytes.
	const measuredMLXBytes = int64(31_400_000_000) // ~31.4 GB
	setBrokerMLXBytesFn(func() (int64, bool) { return measuredMLXBytes, true })
	// RSS for the SAME broker, measured the same day: 4.2 GB. If the check were
	// still RSS-floored this would be misread as comfortably "loaded" only by
	// accident (4.2 GB > 1 GB floor) — the point of this test is that the
	// mlx_active_bytes path is what's actually consulted, never RSS, when
	// /health is reachable.
	const measuredRSSKB = int64(4_200_000) // ~4.2 GB in KB
	setBrokerRSSFn(func(_ string) int64 { return measuredRSSKB })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"completion_tokens":1}}`))
	}))
	defer srv.Close()

	got, detail := ProbeGemmaState(homeWithPort(t, srv.URL))
	if got != GemmaHealthy {
		t.Errorf("30GB+ mlx_active_bytes = %v (%s), want GemmaHealthy — a large Metal allocation must not read as weights-absent", got, detail)
	}
	if strings.Contains(detail, "weight floor") {
		t.Errorf("detail should not mention the weight floor for a 30GB+ allocation, got: %s", detail)
	}
}

// TestProbeGemmaState_MLXFloor_FiresOnSmallAllocation mirrors
// TestProbeGemmaState_RSSFloor but for the PRIMARY path: /health IS reachable
// and reports a small mlx_active_bytes — the weight-floor check must fire from
// that number, not from RSS, once the generation probe has already failed
// (see TestProbeGemmaState_IdleBrokerIsNeverMisreadAsWeightless for the case
// where it must NOT fire despite a small mlx_active_bytes reading).
func TestProbeGemmaState_MLXFloor_FiresOnSmallAllocation(t *testing.T) {
	oldMLX := getBrokerMLXBytesFn()
	t.Cleanup(func() { setBrokerMLXBytesFn(oldMLX) })

	const tinyMLXBytes = int64(50 * 1024 * 1024) // 50 MB — well below the 1 GB floor
	setBrokerMLXBytesFn(func() (int64, bool) { return tinyMLXBytes, true })

	// A weightless broker cannot generate — the probe fails, then the floor
	// upgrades the generic wedge into the specific diagnosis.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[],"usage":{"completion_tokens":0}}`))
	}))
	defer srv.Close()

	got, detail := ProbeGemmaState(homeWithPort(t, srv.URL))
	if got != GemmaWedged {
		t.Errorf("tiny mlx_active_bytes = %v (%s), want GemmaWedged", got, detail)
	}
	if !strings.Contains(detail, "mlx_active_bytes") {
		t.Errorf("detail should cite mlx_active_bytes, got: %s", detail)
	}
}

// TestRecipientFor pins the ownership routing: machine-health conditions go to
// claude-pantheon (which remediates them under A32/ADR-040), unclassified
// conditions fall through to the owner so nothing is silently misrouted.
func TestRecipientFor(t *testing.T) {
	pantheon := []string{"gemma-broker", "memory-death", "session-leak", "menubar"}
	for _, c := range pantheon {
		if got := recipientFor(c); got != "claude-pantheon" {
			t.Errorf("recipientFor(%q) = %q, want claude-pantheon", c, got)
		}
	}
	if got := recipientFor("some-future-owner-only-condition"); got != "owner" {
		t.Errorf("unclassified condition = %q, want owner (fail-safe to owner)", got)
	}
}

// TestRun_DedupsUnderStoreCutover is the post-cutover half of the flood guard
// and the fix for owner item 20260724-210246. Run used to send with
// internal/work (files) while every consumer had moved to the store: the alert
// never reached the wake path, and the file-reading dedupe could not see its own
// previous send, so a persistent blocker re-alerted every 15 minutes. Both sides
// now go through the dispatch facade, so the second Run must skip.
func TestRun_DedupsUnderStoreCutover(t *testing.T) {
	root := t.TempDir()
	writeLivenessTestAgents(t, root)
	home := t.TempDir() // no gemma-server.port → probeGemma reads down
	t.Setenv("HOME", home)
	installFakeBrokerPlist(t, home) // installed, so the down broker alarms
	t.Setenv(routercfg.StoreWakeEnv, "1")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "router.db"))

	var buf bytes.Buffer
	if err := Run(root, &buf); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if n := storeOpenCount(t, root, "claude-pantheon"); n != 1 {
		t.Fatalf("first Run left %d open store items, want 1\n%s", n, buf.String())
	}
	// Post-cutover the store row IS the record — no audit file is written.
	if dirExists(filepath.Join(root, "items")) {
		if entries, _ := os.ReadDir(filepath.Join(root, "items")); len(entries) != 0 {
			t.Errorf("cutover Run wrote %d file(s) into items/, want none", len(entries))
		}
	}

	buf.Reset()
	if err := Run(root, &buf); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if n := storeOpenCount(t, root, "claude-pantheon"); n != 1 {
		t.Errorf("second Run left %d open store items, want 1 (dedup blind to its own send)\n%s", n, buf.String())
	}
	if !strings.Contains(buf.String(), "skip") {
		t.Errorf("second Run should report a dedup skip, got:\n%s", buf.String())
	}
}

func storeOpenCount(t *testing.T, root, agent string) int {
	t.Helper()
	f, err := dispatch.OpenRoot(root)
	if err != nil {
		t.Fatalf("open dispatch: %v", err)
	}
	defer func() { _ = f.Close() }()
	items, err := f.Inbox(agent)
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	return len(items)
}

// TestLaunchAgentPlistPresent_IgnoresRetired guards the retired-label filter in
// probeLaunchdDisabled. Retiring a service renames its plist rather than
// deleting it, and launchd parks the orphaned label as "disabled" forever. If
// this predicate ever matched a retired suffix, the probe would keep
// prescribing `launchctl bootstrap …/<label>.plist` against a file that does
// not exist — an instruction that cannot succeed.
func TestLaunchAgentPlistPresent_IgnoresRetired(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	agents := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(agents, name), []byte("<plist/>"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("ai.sirsi.live.plist")
	write("ai.sirsi.retired.plist.retired-dupe-of-menubar-20260806")
	write("ai.sirsi.superseded.plist.superseded-by-go-20260806")

	for _, tc := range []struct {
		label string
		want  bool
	}{
		{"ai.sirsi.live", true},
		{"ai.sirsi.retired", false},
		{"ai.sirsi.superseded", false},
		{"ai.sirsi.never-existed", false},
	} {
		if got := launchAgentPlistPresent(tc.label); got != tc.want {
			t.Errorf("launchAgentPlistPresent(%q) = %v, want %v", tc.label, got, tc.want)
		}
	}
}

func TestApproximateModelGB(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want float64
	}{
		// 4bit (explicit) — no multiplier; "bf16-4bit" contains "4bit" so ×1 wins.
		{"mlx-community/gemma-2-27b-it-bf16-4bit", 14.0},
		{"mlx-community/gemma-2-9b-it-4bit", 5.0},
		{"mlx-community/gemma-2-2b-it-4bit", 1.5},
		{"mlx-community/gemma-2-12b-it-4bit", 7.0},
		// 8bit — ×2 the 4-bit base
		{"mlx-community/gemma-4-12B-it-8bit", 14.0}, // 7.0 × 2 = 14.0
		{"mlx-community/gemma-2-9b-it-8bit", 10.0},  // 5.0 × 2 = 10.0
		// bf16 (no 4bit suffix) — ×4 the 4-bit base
		{"mlx-community/gemma-2-27b-it-bf16", 56.0}, // 14.0 × 4 = 56.0
		// unknown
		{"mlx-community/some-unknown-model", 0},
	} {
		if got := approximateModelGB(tc.id); got != tc.want {
			t.Errorf("approximateModelGB(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

// TestApproximateModelGB_Gemma4_8bit pins the live machine's model to ~14 GB.
// Before the quantizer fix this returned 7 GB, causing rightSizeAdvice to emit
// "~7 GB" and at 18 GB available produce empty advice (false safe: 2×7+4=18 ≤ 18).
func TestApproximateModelGB_Gemma4_8bit(t *testing.T) {
	got := approximateModelGB("mlx-community/gemma-4-12B-it-8bit")
	if got != 14.0 {
		t.Errorf("approximateModelGB(gemma-4-12B-it-8bit) = %v, want 14.0", got)
	}
}

// homeWithModel writes gemma-model.conf under a temp home and returns the home path.
func homeWithModel(t *testing.T, modelID string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".sirsi/gemma-model.conf"), []byte(modelID), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func stubMLX(t *testing.T, activeBytes int64) {
	t.Helper()
	old := getBrokerMLXActive()
	t.Cleanup(func() { setBrokerMLXActive(old) })
	setBrokerMLXActive(func(_ int) (int64, error) { return activeBytes, nil })
}

func TestRightSizeAdvice_FitsAlready(t *testing.T) {
	// 2b model: estimated path 1.4×1.5+4=6.1 ≤ 20 → no advice regardless of source.
	home := homeWithModel(t, "mlx-community/gemma-2-2b-it-4bit")
	t.Setenv("GEMMA_MODEL", "")
	stubMLX(t, 0) // broker not running
	if got := rightSizeAdvice(home, 20.0); got != "" {
		t.Errorf("expected empty advice when model fits, got %q", got)
	}
}

// TestRightSizeAdvice_HealthEndpointUsed verifies that /health mlx_active_bytes
// is the primary size authority. The stub returns ~37 GB (the real measurement
// on the live SNE broker), not ~185 MB (ps RSS). With 9.68 GB available and
// 37 GB in use, advice must fire using the /health value.
func TestRightSizeAdvice_HealthEndpointUsed(t *testing.T) {
	home := homeWithModel(t, "mlx-community/gemma-4-12B-it-8bit")
	t.Setenv("GEMMA_MODEL", "")
	// Write a port file so rightSizeAdvice tries /health.
	if err := os.WriteFile(filepath.Join(home, ".sirsi/gemma-server.port"), []byte("8477"), 0o644); err != nil {
		t.Fatal(err)
	}
	const thirtySevenGB = int64(37) * 1024 * 1024 * 1024
	stubMLX(t, thirtySevenGB)

	got := rightSizeAdvice(home, 9.68)
	if got == "" {
		t.Fatal("expected non-empty advice: 37 GB model does not fit in 9.68 GB")
	}
	// The advice must reflect the /health figure (~37 GB), not the name estimate.
	if !strings.Contains(got, "37") && !strings.Contains(got, "~37") {
		t.Errorf("advice does not reflect /health size (~37 GB): %q", got)
	}
}

// TestRightSizeAdvice_HealthUnavailableFallsBackToName is the regression guard
// for "0.18 GB reading is NOT accepted as the model size". When the broker is
// not running (no port file) or /health is unreachable, rightSizeAdvice must use
// approximateModelGB, not any RSS-derived value.
//
// The old path: getBrokerRSSFn()(pidFile) returned 185 MB (ps RSS for an mmap'd
// model), which was accepted as modelGB=0.177. 2×0.177+4=4.35 ≤ 9.68 → empty
// advice. The operator received silence while the machine was dying.
//
// This test proves the fix: with /health unavailable and no pidfile, we get
// approximateModelGB("gemma-4-12B-it-8bit") = 14 GB → 1.4×14+4=23.6 > 9.68 → advice fires.
func TestRightSizeAdvice_HealthUnavailableFallsBackToName(t *testing.T) {
	home := homeWithModel(t, "mlx-community/gemma-4-12B-it-8bit")
	t.Setenv("GEMMA_MODEL", "")
	// No port file — /health path is skipped.
	stubMLX(t, 0)

	got := rightSizeAdvice(home, 9.68)
	if got == "" {
		t.Fatal("expected advice: approximateModelGB(gemma-4-12B-it-8bit)=14 GB, threshold 1.4×14+4=23.6 > 9.68; " +
			"a 0.18 GB reading (ps RSS) must not suppress this")
	}
}

func TestRightSizeAdvice_27bReturnsSmaller(t *testing.T) {
	// 27b model ("bf16-4bit" → ×1 quantizer → 14 GB): 1.4×14+4=23.6 > 12 → advice.
	// 9b-4bit tier: 1.4×5+4=11 ≤ 12 → first-fit tier offered.
	// (At ≤10 GB the 9b tier also exceeds threshold and the 2b tier would be offered instead.)
	home := homeWithModel(t, "mlx-community/gemma-2-27b-it-bf16-4bit")
	t.Setenv("GEMMA_MODEL", "")
	stubMLX(t, 0) // broker not running, fall through to approximateModelGB

	got := rightSizeAdvice(home, 12.0)
	if got == "" {
		t.Fatal("expected non-empty advice for oversized 27b model")
	}
	if !strings.Contains(got, "gemma-2-9b-it-4bit") {
		t.Errorf("expected advice to suggest 9b tier (first-fit at 12 GB available, 1.4×5+4=11 ≤ 12), got: %q", got)
	}
}

func TestRightSizeAdvice_NothingFits_StopOnly(t *testing.T) {
	// 9b model: 1.4×5+4=11 > 0.5; 2b tier: 1.4×1.5+4=6.1 > 0.5 — nothing fits.
	home := homeWithModel(t, "mlx-community/gemma-2-9b-it-4bit")
	t.Setenv("GEMMA_MODEL", "")
	stubMLX(t, 0)

	got := rightSizeAdvice(home, 0.5)
	if got == "" {
		t.Fatal("expected non-empty advice when nothing fits")
	}
	if strings.Contains(got, "echo '") {
		t.Errorf("expected stop-only advice (no model switch), got: %q", got)
	}
	if !strings.Contains(got, "serve --stop") {
		t.Errorf("expected stop command in fallback advice, got: %q", got)
	}
}

// TestRightSizeAdvice_Gemma4_NoGenDowngrade verifies that a gemma-4 node does
// not receive an echo-to-conf command pointing at a gemma-2 tier.
func TestRightSizeAdvice_Gemma4_NoGenDowngrade(t *testing.T) {
	home := homeWithModel(t, "mlx-community/gemma-4-12B-it-8bit")
	t.Setenv("GEMMA_MODEL", "")
	stubMLX(t, 0) // broker not running

	// 13 GB available: approximateModelGB=14 GB → threshold 1.4×14+4=23.6 > 13 → advice fires.
	// Cross-gen guard skips all gemma-2 tiers even though 9b-4bit (1.4×5+4=11 ≤ 13) would fit;
	// falls through to stop-only.
	got := rightSizeAdvice(home, 13.0)
	if got == "" {
		t.Fatal("expected advice: gemma-4-12B-it-8bit (~14 GB) does not fit in 13 GB (1.4×14+4=23.6)")
	}
	if strings.Contains(got, "gemma-2") {
		t.Errorf("advice must not offer a gemma-2 tier to a gemma-4 node: %q", got)
	}
	if !strings.Contains(got, "serve --stop") {
		t.Errorf("expected stop command (no same-gen tier fits), got: %q", got)
	}
}

// TestRightSizeAdvice_EstimatedPath_NoQuantizerDoubleCount verifies that the
// 8bit quantizer scale inside approximateModelGB (×2 on 4-bit base) is NOT
// double-counted by the kvScale factor in rightSizeAdvice.
//
// approximateModelGB("gemma-4-12B-it-8bit") = 7×2 = 14 GB (weights).
// kvScale on the estimated path is 1.4 (KV reserve, separate from quantizer).
// Threshold: 1.4×14+4 = 23.6 GB.
//
//	Scenario: 25 GB available → 23.6 ≤ 25 → no advice (model fits).
//	If the quantizer scale were double-counted (2×1.4=2.8): 2.8×14+4=43.2 > 25
//	→ advice would fire, a false alarm on a well-provisioned 25 GB node.
func TestRightSizeAdvice_EstimatedPath_NoQuantizerDoubleCount(t *testing.T) {
	home := homeWithModel(t, "mlx-community/gemma-4-12B-it-8bit")
	t.Setenv("GEMMA_MODEL", "")
	stubMLX(t, 0) // no /health → estimated path

	if got := rightSizeAdvice(home, 25.0); got != "" {
		t.Errorf("estimated path must not double-count quantizer scale (1.4×14+4=23.6 ≤ 25): got advice %q", got)
	}
}

// TestRightSizeAdvice_EstimatedPath_FalseFitsAt18GB is the regression guard for
// the death-spiral false-fits. With /health unreachable and ~18.5 GB available,
// a node running gemma-4-12B-it-8bit should receive advice — the allocator pool
// is ~20 GB (KV-inclusive) and will OOM before the next context window.
//
//	approximateModelGB = 14 GB (weights only); KV reserve: 1.4×14+4 = 23.6 GB.
//	18.5 < 23.6 → advice fires (correct — model does not fit with KV overhead).
//	Old formula (gb+4 = 18): 18.5 ≥ 18 → no advice (false-fits; silent OOM risk).
func TestRightSizeAdvice_EstimatedPath_FalseFitsAt18GB(t *testing.T) {
	home := homeWithModel(t, "mlx-community/gemma-4-12B-it-8bit")
	t.Setenv("GEMMA_MODEL", "")
	stubMLX(t, 0) // no /health → estimated path

	got := rightSizeAdvice(home, 18.5)
	if got == "" {
		t.Fatal("expected advice at 18.5 GB available: estimated threshold 1.4×14+4=23.6 > 18.5; " +
			"gb+4=18 would silently pass (false-fits)")
	}
}

// TestRightSizeAdvice_MeasuredPathNoDoubling pins the headroom formula for the
// measured (/health) path: modelGB+4. mlx_active_bytes already includes KV cache
// pages, so the measured path uses modelGB+4 (OS headroom only — no ×1.4 factor).
//
// Scenario: 14 GB measured active (mlx_active_bytes, KV-inclusive), 20 GB available.
//
//	Threshold: 14+4=18 ≤ 20 → no advice (model fits with 2 GB to spare).
func TestRightSizeAdvice_MeasuredPathNoDoubling(t *testing.T) {
	home := homeWithModel(t, "mlx-community/gemma-4-12B-it-8bit")
	t.Setenv("GEMMA_MODEL", "")
	if err := os.WriteFile(filepath.Join(home, ".sirsi/gemma-server.port"), []byte("8477"), 0o644); err != nil {
		t.Fatal(err)
	}
	const fourteenGB = int64(14) * 1024 * 1024 * 1024
	stubMLX(t, fourteenGB)

	// 20 GB available: 14+4=18 ≤ 20 → model fits, no advice.
	if got := rightSizeAdvice(home, 20.0); got != "" {
		t.Errorf("measured path must use modelGB+4 (18 ≤ 20): got advice %q", got)
	}
}

// TestSuppressGemmaDown pins A35 scoping: a GemmaDown probe raises a finding
// only when the broker is actually installed. Both directions are exercised —
// the suppression AND the regression where a real outage still alarms.
func TestSuppressGemmaDown(t *testing.T) {
	cases := []struct {
		name         string
		status       GemmaStatus
		plistPresent bool
		want         bool
	}{
		{"down and uninstalled — deliberate, suppress", GemmaDown, false, true},
		{"down but installed — real outage, must alarm", GemmaDown, true, false},
		{"wedged and uninstalled — process runs, must alarm", GemmaWedged, false, false},
		{"wedged and installed — must alarm", GemmaWedged, true, false},
		{"healthy and uninstalled — never a finding either way", GemmaHealthy, false, false},
		{"busy and uninstalled — never a finding either way", GemmaBusy, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := suppressGemmaDown(tc.status, tc.plistPresent); got != tc.want {
				t.Errorf("suppressGemmaDown(%v, plist=%v) = %v, want %v",
					tc.status, tc.plistPresent, got, tc.want)
			}
		})
	}
}

func TestSuppressMenubarDown(t *testing.T) {
	cases := []struct {
		name         string
		running      bool
		plistPresent bool
		want         bool
	}{
		{"down and uninstalled — owner-quarantined, suppress", false, false, true},
		{"down but installed — real outage, must alarm", false, true, false},
		{"running and uninstalled — never a finding either way", true, false, false},
		{"running and installed — never a finding either way", true, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := suppressMenubarDown(tc.running, tc.plistPresent); got != tc.want {
				t.Errorf("suppressMenubarDown(running=%v, plist=%v) = %v, want %v",
					tc.running, tc.plistPresent, got, tc.want)
			}
		})
	}
}

// TestProbeMenubar_QuarantinedIsSuppressed guards the ledger row
// menubar-liveness-not-quarantine-aware: a menubar the owner deliberately
// took offline (plist removed from LaunchAgents, same shape as gemma-broker
// quarantine) must not page the owner every StartInterval about a state they
// chose. probeMenubar's pgrep check is real (not injectable), so on a
// machine where the SwiftUI menubar happens to be running this only exercises
// the "running" branch, not the new suppress branch — either is a legitimate
// OK result (never a finding either way); TestSuppressMenubarDown above
// covers the suppress logic itself directly and unconditionally.
func TestProbeMenubar_QuarantinedIsSuppressed(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("menubar probe is darwin-only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	f := probeMenubar()
	if !f.OK {
		t.Errorf("probeMenubar() with no plist installed = %+v, want OK=true "+
			"(deliberately quarantined, or the real menubar happens to be running)", f)
	}
	if f.Detail != "running" && f.Fixable {
		t.Errorf("probeMenubar() suppress branch = %+v, want Fixable=false", f)
	}
}

// installFakeBrokerPlist puts a gemma-broker LaunchAgent plist under home so
// probeGemma treats a down broker as a real outage. Without it the probe reads
// the absence as a deliberate retirement and suppresses the finding, which is
// the correct production behavior but makes an outage test vacuous.
func installFakeBrokerPlist(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir LaunchAgents: %v", err)
	}
	p := filepath.Join(dir, BrokerLabel+".plist")
	if err := os.WriteFile(p, []byte("<plist/>"), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

// TestProbeGemmaFindingNamesTheBinary guards the ledger row
// broker-label-misnames-sne. The LaunchAgent label says "gemma-broker" but the
// process is sne-server (Go). An agent read the label, concluded the service
// was the legacy python/mlx_lm gemma path, and issued a directive to
// permanently down it — taking the Tier-0 substrate offline. The label cannot
// be renamed without bootout/bootstrap of a live load-bearing server, so the
// standing repair is that no owner-facing report about it ever says only
// "gemma". If someone strips the disclaimer, this fails.
func TestProbeGemmaFindingNamesTheBinary(t *testing.T) {
	f := probeGemma(t.TempDir())
	if !strings.Contains(f.Title, BrokerBinary) {
		t.Errorf("finding title must name the real binary %q, got %q", BrokerBinary, f.Title)
	}
	for _, want := range []string{BrokerLabel, BrokerBinary, "NOT the legacy python"} {
		if !strings.Contains(f.Body, want) {
			t.Errorf("finding body must contain %q so the label cannot be misread; body=%q", want, f.Body)
		}
	}
}
