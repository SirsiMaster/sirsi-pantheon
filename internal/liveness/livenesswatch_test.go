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
