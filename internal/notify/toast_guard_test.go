package notify

import (
	"os"
	"strings"
	"testing"
)

// TestDefaultToastExecSilentInTests pins the 2026-07-02 fix: a test binary must
// never post a real user notification. os.Args[0] of every `go test` binary ends
// in ".test", so the real osascript sink is a no-op here by construction.
func TestDefaultToastExecSilentInTests(t *testing.T) {
	if !strings.HasSuffix(os.Args[0], ".test") {
		t.Skip("not running as a test binary (unexpected)")
	}
	// Must return nil WITHOUT spawning osascript. If it spawned, the user
	// would see a banner during every test run — the bug this pins.
	if err := defaultToastExec(`display notification "guard" with title "guard"`); err != nil {
		t.Fatalf("guarded exec should be a silent no-op in tests, got %v", err)
	}
}

// TestSirsiNoToastEnvMutes pins the automation mute switch.
func TestSirsiNoToastEnvMutes(t *testing.T) {
	t.Setenv("SIRSI_NO_TOAST", "1")
	if err := defaultToastExec(`display notification "x" with title "x"`); err != nil {
		t.Fatalf("SIRSI_NO_TOAST=1 should silence the sink, got %v", err)
	}
}
