package guard

import (
	"sync"
	"testing"
)

// killRecorder is a concurrency-safe injectable kill fn that records PIDs it was
// asked to terminate (A21: guarded, restored under lock).
type killRecorder struct {
	mu   sync.Mutex
	pids []int
}

func (k *killRecorder) fn(pid int) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.pids = append(k.pids, pid)
	return nil
}

func (k *killRecorder) killed(pid int) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, p := range k.pids {
		if p == pid {
			return true
		}
	}
	return false
}

func withKillRecorder(t *testing.T) *killRecorder {
	t.Helper()
	rec := &killRecorder{}
	old := orphanKillFn
	orphanKillFn = rec.fn
	t.Cleanup(func() { orphanKillFn = old })
	return rec
}

// TestKillTrueOrphans_SparesActiveParentedProcesses is the regression test codex
// required: an ACTIVE parented dev tool (stale-parent or normally-parented) must
// NEVER be killed by the memory fix — only PPID<=1 true orphans are.
func TestKillTrueOrphans_SparesActiveParentedProcesses(t *testing.T) {
	rec := withKillRecorder(t)

	report := &OrphanReport{
		Orphans: []OrphanProcess{
			// Active, normally-parented dev tools that matched a pattern as
			// "stale" — parent ALIVE (PPID != 1). MUST survive.
			{ProcessInfo: ProcessInfo{PID: 1001, Name: "node"}, ParentPID: 900, ParentName: "vite", IsOrphaned: false, IsStale: true},
			{ProcessInfo: ProcessInfo{PID: 1002, Name: "gopls"}, ParentPID: 901, ParentName: "Code Helper", IsOrphaned: false, IsStale: true},
			{ProcessInfo: ProcessInfo{PID: 1003, Name: "com.docker.backend"}, ParentPID: 902, ParentName: "Docker", IsOrphaned: false, IsStale: true},
			{ProcessInfo: ProcessInfo{PID: 1004, Name: "electron"}, ParentPID: 903, ParentName: "Cursor", IsOrphaned: false, IsStale: true},
			// A genuine orphan — parent dead (PPID 1). Should be terminated.
			{ProcessInfo: ProcessInfo{PID: 2000, Name: "node"}, ParentPID: 1, ParentName: "launchd", IsOrphaned: true},
		},
	}

	res := KillTrueOrphans(report, false)

	for _, pid := range []int{1001, 1002, 1003, 1004} {
		if rec.killed(pid) {
			t.Errorf("ACTIVE parented process PID %d was killed — must be spared", pid)
		}
	}
	if !rec.killed(2000) {
		t.Error("true orphan PID 2000 (PPID 1) should have been terminated")
	}
	if len(res.Killed) != 1 || res.Killed[0].PID != 2000 {
		t.Errorf("Killed = %+v, want exactly PID 2000", res.Killed)
	}
	if len(res.Skipped) != 4 {
		t.Errorf("Skipped = %d, want 4 (the active parented procs)", len(res.Skipped))
	}
}

// TestKillTrueOrphans_DryRunSignalsNothing: dry-run never calls the kill fn.
func TestKillTrueOrphans_DryRunSignalsNothing(t *testing.T) {
	rec := withKillRecorder(t)
	report := &OrphanReport{
		Orphans: []OrphanProcess{
			{ProcessInfo: ProcessInfo{PID: 2000, Name: "node"}, ParentPID: 1, IsOrphaned: true},
		},
	}
	res := KillTrueOrphans(report, true)
	if rec.killed(2000) {
		t.Error("dry-run must not signal any process")
	}
	if len(res.Killed) != 1 {
		t.Errorf("dry-run should still REPORT 1 would-kill, got %d", len(res.Killed))
	}
}

// TestKillTrueOrphans_SkipsProtected: a true orphan that is also protected
// (e.g. PID 1) is never killed.
func TestKillTrueOrphans_SkipsProtected(t *testing.T) {
	rec := withKillRecorder(t)
	report := &OrphanReport{
		Orphans: []OrphanProcess{
			{ProcessInfo: ProcessInfo{PID: 1, Name: "launchd"}, ParentPID: 0, IsOrphaned: true},
		},
	}
	res := KillTrueOrphans(report, false)
	if rec.killed(1) {
		t.Error("protected PID 1 must never be killed")
	}
	if len(res.Killed) != 0 {
		t.Errorf("Killed = %d, want 0", len(res.Killed))
	}
}

// TestKillTrueOrphans_NilReport: defensive — nil report is a no-op.
func TestKillTrueOrphans_NilReport(t *testing.T) {
	if res := KillTrueOrphans(nil, false); res == nil || len(res.Killed) != 0 {
		t.Error("nil report should return an empty result, not panic")
	}
}
