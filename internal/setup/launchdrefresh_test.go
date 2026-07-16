package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubLaunchctl records every launchctl invocation and fails those whose
// joined args match failOn.
func stubLaunchctlRecording(t *testing.T, failOn string) *[][]string {
	t.Helper()
	var calls [][]string
	old := launchctlExecFn
	launchctlExecFn = func(args ...string) error {
		calls = append(calls, args)
		if failOn != "" && strings.Contains(strings.Join(args, " "), failOn) {
			return os.ErrPermission
		}
		return nil
	}
	t.Cleanup(func() { launchctlExecFn = old })
	oldSleep := sleepFn
	sleepFn = func(time.Duration) {} // retries must not slow the suite
	t.Cleanup(func() { sleepFn = oldSleep })
	return &calls
}

// TestRefreshSirsiLaunchAgents_BootstrapRetriesAcrossTeardown reproduces the
// live failure: bootout of a RUNNING job tears down asynchronously and the
// first bootstrap attempts fail with EIO until the old instance exits.
func TestRefreshSirsiLaunchAgents_BootstrapRetriesAcrossTeardown(t *testing.T) {
	dir := t.TempDir()
	writePlist(t, dir, "ai.sirsi.router.wake.claude-home.plist")

	failures := 3
	calls := stubLaunchctlRecording(t, "")
	old := launchctlExecFn
	launchctlExecFn = func(args ...string) error {
		if args[0] == "bootstrap" && failures > 0 {
			failures--
			return os.ErrInvalid // launchd EIO stand-in
		}
		return old(args...)
	}
	t.Cleanup(func() { launchctlExecFn = old })

	refreshed, problems := refreshSirsiLaunchAgentsIn(dir, 501)
	if len(problems) != 0 || len(refreshed) != 1 {
		t.Fatalf("refreshed=%v problems=%v, want success after teardown retries", refreshed, problems)
	}
	_ = calls
}

func writePlist(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("plist"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshSirsiLaunchAgents_BootoutThenBootstrapPerJob(t *testing.T) {
	dir := t.TempDir()
	writePlist(t, dir, "ai.sirsi.conduit.tick.plist")
	writePlist(t, dir, "ai.sirsi.router.wake.claude-pantheon.plist")
	// Never touched: backups, quarantined copies, non-sirsi jobs.
	writePlist(t, dir, "ai.sirsi.pantheon.plist.bak-precutover")
	writePlist(t, dir, "ai.sirsi.claude-worker.claude-pantheon.plist.quarantined")
	writePlist(t, dir, "com.apple.something.plist")

	calls := stubLaunchctlRecording(t, "")
	refreshed, problems := refreshSirsiLaunchAgentsIn(dir, 501)

	if len(problems) != 0 {
		t.Fatalf("problems = %v, want none", problems)
	}
	want := []string{"ai.sirsi.conduit.tick", "ai.sirsi.router.wake.claude-pantheon"}
	if len(refreshed) != len(want) || refreshed[0] != want[0] || refreshed[1] != want[1] {
		t.Fatalf("refreshed = %v, want %v", refreshed, want)
	}
	// Per job: bootout gui/<uid>/<label>, then bootstrap gui/<uid> <plist>.
	if len(*calls) != 4 {
		t.Fatalf("launchctl calls = %d, want 4: %v", len(*calls), *calls)
	}
	if got := strings.Join((*calls)[0], " "); got != "bootout gui/501/ai.sirsi.conduit.tick" {
		t.Errorf("call[0] = %q", got)
	}
	if got := strings.Join((*calls)[1], " "); got != "bootstrap gui/501 "+filepath.Join(dir, "ai.sirsi.conduit.tick.plist") {
		t.Errorf("call[1] = %q", got)
	}
}

func TestRefreshSirsiLaunchAgents_BootoutFailureTolerated(t *testing.T) {
	dir := t.TempDir()
	writePlist(t, dir, "ai.sirsi.gemma.plist")

	stubLaunchctlRecording(t, "bootout") // job wasn't loaded — bootout errors, bootstrap succeeds
	refreshed, problems := refreshSirsiLaunchAgentsIn(dir, 501)

	if len(problems) != 0 || len(refreshed) != 1 || refreshed[0] != "ai.sirsi.gemma" {
		t.Fatalf("refreshed=%v problems=%v, want the job refreshed despite bootout error", refreshed, problems)
	}
}

func TestRefreshSirsiLaunchAgents_BootstrapFailureReported(t *testing.T) {
	dir := t.TempDir()
	writePlist(t, dir, "ai.sirsi.gemma.plist")
	writePlist(t, dir, "ai.sirsi.triage.plist")

	stubLaunchctlRecording(t, "bootstrap gui/501 "+filepath.Join(dir, "ai.sirsi.gemma.plist"))
	refreshed, problems := refreshSirsiLaunchAgentsIn(dir, 501)

	if len(refreshed) != 1 || refreshed[0] != "ai.sirsi.triage" {
		t.Fatalf("refreshed = %v, want only ai.sirsi.triage", refreshed)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "ai.sirsi.gemma: bootstrap failed") {
		t.Fatalf("problems = %v, want one gemma bootstrap failure", problems)
	}
}

func TestRefreshSirsiLaunchAgents_EmptyDir(t *testing.T) {
	calls := stubLaunchctlRecording(t, "")
	refreshed, problems := refreshSirsiLaunchAgentsIn(t.TempDir(), 501)
	if refreshed != nil || problems != nil || len(*calls) != 0 {
		t.Fatalf("empty dir: refreshed=%v problems=%v calls=%v, want all empty", refreshed, problems, *calls)
	}
}
