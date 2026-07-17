package liveness

import (
	"bytes"
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
	first := openCount(t, root, "user")
	if first != 1 {
		t.Fatalf("first Run routed %d items to user, want exactly 1\n%s", first, buf.String())
	}

	buf.Reset()
	if err := Run(root, &buf); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if second := openCount(t, root, "user"); second != 1 {
		t.Errorf("second Run left %d open items, want 1 (dedup failed)\n%s", second, buf.String())
	}
	if !strings.Contains(buf.String(), "skip") {
		t.Errorf("second Run should report a dedup skip, got:\n%s", buf.String())
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
