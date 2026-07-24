package liveness

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	// Ensure the gemma probe reads "down": point HOME at an empty dir with no
	// gemma-server.port, so probeGemma returns wedged deterministically.
	t.Setenv("HOME", t.TempDir())

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
	if got := recipientFor("some-future-owner-only-condition"); got != "user" {
		t.Errorf("unclassified condition = %q, want user (fail-safe to owner)", got)
	}
}
