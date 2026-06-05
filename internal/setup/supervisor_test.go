package setup

import (
	"runtime"
	"strings"
	"testing"
)

func TestSupervisorPlistContent(t *testing.T) {
	const (
		bin = "/usr/local/bin/sirsi"
		dir = "/Users/dev/Development/sirsi-pantheon"
	)
	plist := supervisorPlistContent(bin, dir)

	wants := []string{
		// Agreed contract: the LaunchAgent label.
		"ai.sirsi.horus.agent-router",
		// Explicit WorkingDirectory (never the launchd default cwd).
		"<key>WorkingDirectory</key>",
		// Runs the resident loop command.
		"horus supervise",
		// Both resolved paths are present.
		bin,
		dir,
		// KeepAlive + RunAtLoad so it survives crashes and starts at login.
		"<key>KeepAlive</key>",
		"<key>RunAtLoad</key>",
		// Background process type (headless resident loop, not interactive).
		"<string>Background</string>",
		// Login-shell exec wrapper.
		"/bin/zsh",
		// Explicit PATH so the wakeability probe (exec.LookPath) resolves agent
		// CLIs under launchd's minimal environment — a bare login shell skips
		// .zshrc where ~/.local/bin is added.
		"<key>EnvironmentVariables</key>",
		"<key>PATH</key>",
		// The sirsi binary's own directory leads the PATH (agent CLIs sit beside it).
		"/usr/local/bin",
	}
	for _, w := range wants {
		if !strings.Contains(plist, w) {
			t.Errorf("supervisor plist missing %q:\n%s", w, plist)
		}
	}
}

func TestSupervisorPathLeadsWithBinDir(t *testing.T) {
	path := supervisorPath("/Users/dev/.local/bin/sirsi")
	if !strings.HasPrefix(path, "/Users/dev/.local/bin:") {
		t.Errorf("supervisorPath should lead with the binary's dir, got %q", path)
	}
	// No duplicate entries (binary dir == ~/.local/bin must collapse to one).
	seen := map[string]bool{}
	for d := range strings.SplitSeq(path, ":") {
		if seen[d] {
			t.Errorf("supervisorPath has duplicate entry %q in %q", d, path)
		}
		seen[d] = true
	}
}

func TestSupervisorInstallNonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("non-darwin behavior only")
	}
	if SupervisorInstalled() {
		t.Error("SupervisorInstalled() must be false on non-darwin")
	}
	res := InstallSupervisor()
	if res.Status != StatusSkipped {
		t.Errorf("InstallSupervisor() on %s = %v, want StatusSkipped", runtime.GOOS, res.Status)
	}
	if res.Surface != SurfaceSupervisor {
		t.Errorf("InstallSupervisor() surface = %q, want %q", res.Surface, SurfaceSupervisor)
	}
}
