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

// The wake loop resolves its declared consumer with exec.LookPath. launchd hands
// a job only /usr/bin:/bin:/usr/sbin:/sbin, so without an explicit PATH every
// agent whose CLI lives elsewhere resolves to "no consumer" and the loop
// degrades to WATCH-ONLY — heartbeating forever, dispatching nothing, while its
// inbox strands. That was the live state of all 11 codex lanes on 2026-08-06
// (`consumer command "codex" not found in PATH`, codex sitting in ~/.local/bin).
func TestWakePlistExportsPATHForConsumerLookup(t *testing.T) {
	content := wakeLaunchAgentPlist("ai.sirsi.router.wake.codex-x",
		AgentConfig{ID: "codex-x"}, "/Users/x/.local/bin/sirsi")

	if !strings.Contains(content, "<key>EnvironmentVariables</key>") {
		t.Fatalf("plist must export EnvironmentVariables:\n%s", content)
	}
	// The sirsi binary's own directory must lead: agent CLIs are installed
	// beside it (codex is a symlink next to sirsi in ~/.local/bin).
	if !strings.Contains(content, "<string>/Users/x/.local/bin:") {
		t.Fatalf("PATH must lead with the sirsi binary's directory:\n%s", content)
	}
	for _, want := range []string{"/usr/bin", "/bin", "/opt/homebrew/bin"} {
		if !strings.Contains(content, want) {
			t.Fatalf("PATH missing %q:\n%s", want, content)
		}
	}
}

func TestLaunchAgentPATHLeadsWithBinDirAndDedupes(t *testing.T) {
	got := LaunchAgentPATH("/usr/bin/sirsi")
	if !strings.HasPrefix(got, "/usr/bin:") {
		t.Fatalf("PATH must lead with the binary's own dir, got %q", got)
	}
	// /usr/bin is both the binary's dir and a system dir — it must appear once,
	// or the exported PATH grows a duplicate on every install.
	if n := strings.Count(":"+got+":", ":/usr/bin:"); n != 1 {
		t.Fatalf("/usr/bin must appear exactly once, got %d in %q", n, got)
	}
}
