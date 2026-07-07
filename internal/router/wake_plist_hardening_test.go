package router

import (
	"os"
	"strings"
	"testing"
)

// A bare binary name is unspawnable under launchd (its PATH has no
// ~/.local/bin) — the 2026-07-07 nine-loop crash-loop (15k respawns, no
// logs). Install must refuse rather than write an unspawnable plist.
func TestInstallWakeRefusesUnresolvableBinary(t *testing.T) {
	old := launchAgentsDirOverride
	launchAgentsDirOverride = t.TempDir()
	t.Cleanup(func() { launchAgentsDirOverride = old })
	t.Setenv("PATH", t.TempDir()) // no sirsi anywhere

	_, _, err := InstallWakeLaunchAgent(AgentConfig{ID: "claude-test"}, "")
	if err == nil || !strings.Contains(err.Error(), "bare name") {
		t.Fatalf("expected a refuse-bare-name error, got %v", err)
	}
}

// The plist must carry the observability + crash-loop bounds: log paths and
// a respawn throttle. Silent KeepAlive death-loops are how nine watchers
// died 15k times without a line of evidence.
func TestWakePlistCarriesLogsAndThrottle(t *testing.T) {
	content := wakeLaunchAgentPlist("ai.sirsi.router.wake.claude-x", AgentConfig{ID: "claude-x"}, "/abs/sirsi")
	for _, want := range []string{"StandardOutPath", "StandardErrorPath", "wake-claude-x.log", "ThrottleInterval"} {
		if !strings.Contains(content, want) {
			t.Fatalf("plist missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "<string>sirsi</string>") {
		t.Fatal("plist must never carry a bare binary name")
	}
	_ = os.Getenv // keep imports honest if assertions change
}
