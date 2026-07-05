package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubLaunchctl records calls and serves a canned `launchctl list` output.
type stubLaunchctl struct {
	listOutput string
	calls      [][]string
}

func (s *stubLaunchctl) run(args ...string) (string, error) {
	s.calls = append(s.calls, args)
	if len(args) == 1 && args[0] == "list" {
		return s.listOutput, nil
	}
	return "", nil
}

func (s *stubLaunchctl) bootouts() []string {
	var out []string
	for _, c := range s.calls {
		if len(c) == 2 && c[0] == "bootout" {
			out = append(out, c[1])
		}
	}
	return out
}

func withTempLaunchAgents(t *testing.T) string {
	t.Helper()
	old := launchAgentsDirOverride
	dir := t.TempDir()
	launchAgentsDirOverride = dir
	t.Cleanup(func() { launchAgentsDirOverride = old })
	return dir
}

func TestQuarantineWorkersCleanHostIsNoOp(t *testing.T) {
	withTempLaunchAgents(t)
	stub := &stubLaunchctl{listOutput: "PID\tStatus\tLabel\n123\t0\tcom.apple.something\n"}

	res, err := QuarantineWorkers(false, stub.run)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.BootedOut) != 0 || len(res.Quarantined) != 0 {
		t.Fatalf("clean host must be a no-op, got %+v", res)
	}
}

func TestQuarantineWorkersStopsWorkerAndRenamesPlist(t *testing.T) {
	dir := withTempLaunchAgents(t)
	plist := filepath.Join(dir, WorkerLabelPrefix+"claude-pantheon.plist")
	if err := os.WriteFile(plist, []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	stub := &stubLaunchctl{listOutput: strings.Join([]string{
		"456\t0\t" + WorkerLabelPrefix + "claude-pantheon",
		"789\t0\tai.sirsi.router.wake.claude-pantheon", // a wake-loop: MUST survive
	}, "\n")}

	res, err := QuarantineWorkers(false, stub.run)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.BootedOut) != 1 || !strings.HasSuffix(res.BootedOut[0], "claude-pantheon") {
		t.Fatalf("expected exactly the worker booted out, got %v", res.BootedOut)
	}
	boots := stub.bootouts()
	if len(boots) != 1 || !strings.Contains(boots[0], WorkerLabelPrefix) {
		t.Fatalf("expected one bootout of the worker label, got %v", boots)
	}
	if strings.Contains(strings.Join(boots, " "), "ai.sirsi.router.wake.") {
		t.Fatal("a wake-loop label was booted out — quarantine must never touch watchers")
	}
	if _, err := os.Stat(plist); !os.IsNotExist(err) {
		t.Fatal("worker plist still loadable — expected it renamed away")
	}
	if _, err := os.Stat(plist + quarantineSuffix); err != nil {
		t.Fatalf("expected quarantined plist to exist: %v", err)
	}
}

func TestQuarantineWorkersDryRunTouchesNothing(t *testing.T) {
	dir := withTempLaunchAgents(t)
	plist := filepath.Join(dir, WorkerLabelPrefix+"claude-nexus.plist")
	if err := os.WriteFile(plist, []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	stub := &stubLaunchctl{listOutput: "111\t0\t" + WorkerLabelPrefix + "claude-nexus\n"}

	res, err := QuarantineWorkers(true, stub.run)
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || len(res.BootedOut) != 1 || len(res.Quarantined) != 1 {
		t.Fatalf("dry-run must REPORT the full plan, got %+v", res)
	}
	if got := stub.bootouts(); len(got) != 0 {
		t.Fatalf("dry-run must not bootout, got %v", got)
	}
	if _, err := os.Stat(plist); err != nil {
		t.Fatalf("dry-run must not rename the plist: %v", err)
	}
}
