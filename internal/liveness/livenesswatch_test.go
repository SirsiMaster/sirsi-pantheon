package liveness

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	// Ensure the gemma probe reads "down": point HOME at an empty dir with no
	// gemma-server.port, so probeGemma returns wedged deterministically.
	t.Setenv("HOME", t.TempDir())
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

// TestProbeGemmaState_RSSFloor is the weightless-broker regression guard.
// A broker with RSS below 1 GB cannot have model weights loaded — this is the
// class that /health passes but generation probes see as wedged, and that a
// restart cannot fix when the HF model cache is absent (2026-08-05 incident).
// The RSS floor must fire BEFORE the generation probe so the detail message
// clearly says "weights likely absent" instead of "restore — free memory."
func TestProbeGemmaState_RSSFloor(t *testing.T) {
	old := getBrokerRSSFn()
	t.Cleanup(func() { setBrokerRSSFn(old) })

	// Stub a tiny RSS (100 MB — well below the 1 GB floor).
	const tinyRSSKB = 100 * 1024 // 100 MB
	setBrokerRSSFn(func(_ string) int64 { return tinyRSSKB })

	// The test server should never be reached — the RSS floor fires first.
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"completion_tokens":1}}`))
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
	if called {
		t.Error("generation probe should not be called when RSS is below the floor")
	}
}

// TestProbeGemmaState_RSSFloor_ZeroSkips verifies that a zero RSS (PID file
// absent or ps unavailable) does not falsely classify a healthy broker as
// weightless — fail-open is required (never falsely reap a live broker).
func TestProbeGemmaState_RSSFloor_ZeroSkips(t *testing.T) {
	old := getBrokerRSSFn()
	t.Cleanup(func() { setBrokerRSSFn(old) })

	setBrokerRSSFn(func(_ string) int64 { return 0 }) // pid file absent

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
	home := t.TempDir() // no gemma-server.port → probeGemma reads wedged
	t.Setenv("HOME", home)
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
		// 4bit (explicit) — no multiplier
		{"mlx-community/gemma-2-27b-it-bf16-4bit", 14.0}, // "bf16-4bit": 4bit wins, ×1
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

func TestRightSizeAdvice_FitsAlready(t *testing.T) {
	home := t.TempDir()
	// 2b model (~1.5 GB): 2×1.5+4 = 7 ≤ 20 → already fits, no advice
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-model.conf"),
		[]byte("mlx-community/gemma-2-2b-it-4bit"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("GEMMA_MODEL", "") // clear env override
	if got := rightSizeAdvice(home, 20.0); got != "" {
		t.Errorf("expected empty advice when model fits, got %q", got)
	}
}

func TestRightSizeAdvice_27bReturnsSmaller(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-model.conf"),
		[]byte("mlx-community/gemma-2-27b-it-bf16-4bit"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 9.68 GB available (the real spiral measurement): 27b needs 2×14+4=32 GB
	got := rightSizeAdvice(home, 9.68)
	if got == "" {
		t.Fatal("expected non-empty advice for oversized 27b model")
	}
	if !strings.Contains(got, "gemma-2-2b-it-4bit") {
		t.Errorf("expected advice to suggest 2b tier (the only one that fits in 9.68 GB), got: %q", got)
	}
	if !strings.Contains(got, "sirsi gemma serve --stop") {
		t.Errorf("expected advice to contain stop command, got: %q", got)
	}
}

func TestRightSizeAdvice_NothingFits_StopOnly(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-model.conf"),
		[]byte("mlx-community/gemma-2-9b-it-4bit"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Only 0.5 GB available — nothing fits, even 2b needs 2×1.5+4=7 GB
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

// TestApproximateModelGB_Gemma4_8bit pins the machine's live conf to ~14 GB.
// gemma-4-12B-it-8bit: 12b bucket = 7 GB at 4bit; 8bit quantizer → ×2 = 14 GB.
// Before the quantizer fix this returned 7 GB, causing rightSizeAdvice to print
// "~7 GB" and, at 18–32 GB available, to emit empty advice (false safe).
func TestApproximateModelGB_Gemma4_8bit(t *testing.T) {
	got := approximateModelGB("mlx-community/gemma-4-12B-it-8bit")
	if got != 14.0 {
		t.Errorf("approximateModelGB(gemma-4-12B-it-8bit) = %v, want 14.0", got)
	}
}

// TestRightSizeAdvice_Gemma4_NoGenDowngrade verifies that a gemma-4 node does
// not receive an echo-to-conf command pointing at a gemma-2 tier. When no
// same-generation tier is available, advice must be stop-only.
func TestRightSizeAdvice_Gemma4_NoGenDowngrade(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-model.conf"),
		[]byte("mlx-community/gemma-4-12B-it-8bit"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GEMMA_MODEL", "")
	old := getBrokerRSSFn()
	t.Cleanup(func() { setBrokerRSSFn(old) })
	// Simulate broker not running so advice falls back to approximateModelGB (14 GB).
	setBrokerRSSFn(func(_ string) int64 { return 0 })

	// 18 GB available: 2×14+4=32 > 18 → advice must fire.
	// Before quant fix: 2×7+4=18 ≤ 18 → false-safe, empty advice.
	got := rightSizeAdvice(home, 18.0)
	if got == "" {
		t.Fatal("expected non-empty advice: gemma-4-12B-it-8bit (~14 GB) does not fit in 18 GB (2×14+4=32)")
	}
	// Must not suggest a gemma-2 tier — that would silently downgrade the node.
	if strings.Contains(got, "gemma-2") {
		t.Errorf("advice must not offer a gemma-2 tier to a gemma-4 node (generation downgrade): %q", got)
	}
	if !strings.Contains(got, "serve --stop") {
		t.Errorf("expected stop command, got: %q", got)
	}
}

// TestRightSizeAdvice_BrokerRSSOverridesNameEstimate verifies that the actual
// broker RSS takes priority over approximateModelGB. The 2026-08-06 incident:
// gemma-4-12B-it-8bit has "12b" in its name → 7 GB name estimate (pre-fix),
// but actual RSS measured 34.9 GB. Without this fix: 2×7+4=18 ≤ 20 GB
// available → the "fits already" guard returns empty — a false-safe verdict
// that leaves a dangerously oversized model running. With the fix: 2×34.9+4=73.8
// > 20 → advice fires and tells the operator to right-size.
func TestRightSizeAdvice_BrokerRSSOverridesNameEstimate(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	// gemma-4-12B-it-8bit: "12b" → approximateModelGB returns 7 GB, but actual is ~35 GB.
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-model.conf"),
		[]byte("mlx-community/gemma-4-12B-it-8bit"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := getBrokerRSSFn()
	t.Cleanup(func() { setBrokerRSSFn(old) })
	// ~35 GB in KB (approximating the 34.9 GB measured on 2026-08-06).
	setBrokerRSSFn(func(_ string) int64 { return 35 * 1024 * 1024 }) // ~35 GB in KB

	// 20 GB available: name estimate (7 GB) says "fits" — advice would be empty.
	// Actual RSS (34.9 GB): 2×34.9+4=73.8 > 20 — advice must fire.
	got := rightSizeAdvice(home, 20.0)
	if got == "" {
		t.Fatal("expected non-empty advice: actual 34.9 GB model cannot fit in 20 GB; name-only estimate gave a false safe verdict")
	}
	if !strings.Contains(got, "serve --stop") {
		t.Errorf("expected stop command in advice, got: %q", got)
	}
}
