package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubSupervisorInstall replaces installSupervisorFn for one test.
func stubSupervisorInstall(t *testing.T, res InstallResult) *int {
	t.Helper()
	var calls int
	orig := installSupervisorFn
	installSupervisorFn = func() InstallResult {
		calls++
		return res
	}
	t.Cleanup(func() { installSupervisorFn = orig })
	return &calls
}

// stubLaunchctl replaces launchctlExecFn for one test, recording invocations.
func stubLaunchctl(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	orig := launchctlExecFn
	launchctlExecFn = func(args ...string) error {
		calls = append(calls, args)
		return nil
	}
	t.Cleanup(func() { launchctlExecFn = orig })
	return &calls
}

// writeLegacyPlists drops the three legacy per-duty plists into a temp HOME
// and returns their paths keyed by label.
func writeLegacyPlists(t *testing.T, home string) map[string]string {
	t.Helper()
	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, label := range legacyRouterDaemonLabels {
		p := filepath.Join(agentDir, label+".plist")
		if err := os.WriteFile(p, []byte("<plist/>\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths[label] = p
	}
	return paths
}

// TestInstallRouterDaemons_NonDarwinSkips guards the platform gate.
func TestInstallRouterDaemons_NonDarwinSkips(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin path exercised elsewhere")
	}
	res := InstallRouterDaemons()
	if res.Status != StatusSkipped {
		t.Errorf("InstallRouterDaemons on %s = %v, want StatusSkipped", runtime.GOOS, res.Status)
	}
	if res.Surface != SurfaceRouterDaemons {
		t.Errorf("Surface = %q, want %q", res.Surface, SurfaceRouterDaemons)
	}
}

// TestInstallRouterDaemons_MigratesLegacyAgentsAway is the single-backstop
// contract (backlog ruling 20260629-230327): install-daemons ensures the ONE
// supervisor and unloads+removes the three legacy per-duty plists, reporting
// the migration. A second run finds nothing to migrate (idempotent).
func TestInstallRouterDaemons_MigratesLegacyAgentsAway(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("router automation is macOS only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	plists := writeLegacyPlists(t, home)
	launchctl := stubLaunchctl(t)
	supCalls := stubSupervisorInstall(t, InstallResult{
		Surface: SurfaceSupervisor, Status: StatusOK, Message: "agent-router supervisor installed and started",
	})

	res := InstallRouterDaemons()
	if res.Status != StatusOK {
		t.Fatalf("status = %v (%s), want StatusOK", res.Status, res.Message)
	}
	if *supCalls != 1 {
		t.Errorf("supervisor installer called %d times, want 1 (the single backstop)", *supCalls)
	}
	// All three legacy plists gone, each unloaded first.
	unloads := map[string]bool{}
	for _, call := range *launchctl {
		if len(call) == 2 && call[0] == "unload" {
			unloads[call[1]] = true
		}
	}
	for label, p := range plists {
		if fileExists(p) {
			t.Errorf("legacy plist %s still present after migration", label)
		}
		if !unloads[p] {
			t.Errorf("legacy agent %s was removed without an unload", label)
		}
	}
	if !strings.Contains(res.Message, "migrated away 3 legacy agent(s)") {
		t.Errorf("message should report the migration, got %q", res.Message)
	}

	// Second run: nothing left to migrate; still OK.
	res2 := InstallRouterDaemons()
	if res2.Status != StatusOK {
		t.Fatalf("second run status = %v (%s), want StatusOK", res2.Status, res2.Message)
	}
	if !strings.Contains(res2.Message, "no legacy agents to migrate") {
		t.Errorf("second run should report no migration needed, got %q", res2.Message)
	}
}

// TestInstallRouterDaemons_SupervisorFailureIsFailed proves the report is
// honest when the one backstop cannot be installed.
func TestInstallRouterDaemons_SupervisorFailureIsFailed(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("router automation is macOS only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	stubLaunchctl(t)
	stubSupervisorInstall(t, InstallResult{
		Surface: SurfaceSupervisor, Status: StatusFailed, Message: "launchctl load failed",
	})

	res := InstallRouterDaemons()
	if res.Status != StatusFailed {
		t.Errorf("status = %v, want StatusFailed when the supervisor install fails", res.Status)
	}
}

// TestRouterAutomationHealthy_RequiresSupervisorAndNoLegacy covers the
// health predicate both ways on darwin.
func TestRouterAutomationHealthy_RequiresSupervisorAndNoLegacy(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("router automation is macOS only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if RouterAutomationHealthy() {
		t.Error("healthy without a supervisor plist — must be false")
	}
	// Install the supervisor plist → healthy.
	if err := os.WriteFile(filepath.Join(agentDir, supervisorPlistLabel+".plist"), []byte("<plist/>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !RouterAutomationHealthy() {
		t.Error("supervisor installed and no legacy plists — must be healthy")
	}
	// A lingering legacy plist breaks the single-backstop shape.
	writeLegacyPlists(t, home)
	if RouterAutomationHealthy() {
		t.Error("legacy plists present — must not report healthy")
	}
}
