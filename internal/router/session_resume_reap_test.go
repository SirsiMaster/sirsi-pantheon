//go:build !windows

package router

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func exitedPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	return cmd.Process.Pid
}

// A desktop conversation outlives its process. When the old PID dies, the
// attended session record must stay renewable so the resumed session (new PID,
// same session_id) keeps its thread instead of being reaped into a second one.
func TestReapDeadThreads_AttendedSessionSurvivesPIDDeathAndResumes(t *testing.T) {
	root := t.TempDir()
	reg, _ := LoadThreadRegistry(root)
	now := time.Now().UTC()
	reg.Threads["thr-resume"] = &Thread{
		ThreadID: "thr-resume", AgentID: "ra", Surface: surfaceClaude, SessionID: "conv-1",
		Status: ThreadStatusActive, PID: exitedPID(t), StartedAt: now, LastSeenAt: now.Add(-10 * time.Minute),
		MachineID: MachineID(), Host: "host",
	}
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}

	reaped, err := ReapDeadThreads(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) != 0 {
		t.Fatalf("attended session reaped on PID death within its lease: %+v", reaped)
	}

	got, err := RegisterThread(root, &Thread{
		AgentID: "ra", Surface: surfaceClaude, SessionID: "conv-1", PID: exitedPID(t) + 1,
		MachineID: MachineID(), Host: "host",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ThreadID != "thr-resume" {
		t.Fatalf("resumed session minted %q; want it to renew thr-resume", got.ThreadID)
	}
}

// An attended session keeps its registration however long it is quiet and
// whatever became of its process: neither sweep ends it, and its next heartbeat
// renews the same thread id. Only an explicit close ends it.
func TestAttendedRegistrationIsPerpetual(t *testing.T) {
	oldCmd := getPIDCommandFn()
	setPIDCommandFn(func(int) string { return "claude" })
	defer setPIDCommandFn(oldCmd)

	root := t.TempDir()
	reg, _ := LoadThreadRegistry(root)
	pid := os.Getpid()
	quiet := time.Now().UTC().Add(-30 * 24 * time.Hour)
	for id, tc := range map[string]struct {
		surface string
		pid     int
		start   string
	}{
		"thr-live":     {surfaceClaude, pid, PIDStartTimeOf(pid)},
		"thr-recycled": {surfaceClaude, pid, "Thu Jan  1 00:00:00 1970"},
		"thr-exited":   {surfaceClaude, exitedPID(t), ""},
		"thr-no-pid":   {surfaceClaude, 0, ""},
		"thr-codex":    {surfaceCodex, 0, ""},
	} {
		reg.Threads[id] = &Thread{
			ThreadID: id, AgentID: "ra", Surface: tc.surface, SessionID: "conv-" + id,
			Status: ThreadStatusActive, PID: tc.pid, StartTime: tc.start, StartedAt: quiet, LastSeenAt: quiet,
			MachineID: MachineID(),
		}
	}
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}

	reaped, err := ReapDeadThreads(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) != 0 {
		t.Fatalf("attended sessions reaped: %+v", reaped)
	}

	reg, err = LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	retro := func(*Thread) (*SuspendPayload, bool) { return &SuspendPayload{}, true }
	for _, o := range ReconcileExits(reg, "", "", time.Now().UTC(), DefaultThreadStaleAfter, retro) {
		if o.Action == ReconcileSuspendedStale {
			t.Fatalf("attended session suspended: %+v", o)
		}
	}

	for id := range reg.Threads {
		if _, err := Heartbeat(root, id, HeartbeatUpdate{}); err != nil {
			t.Errorf("%s did not renew: %v", id, err)
		}
	}
}

// The renewal rule is for attended sessions. A worker keeps the OS-truth rule and
// is reaped when its process dies, and is suspended by reconcile when it goes quiet.
func TestWorkerRegistrationIsNotPerpetual(t *testing.T) {
	root := t.TempDir()
	reg, _ := LoadThreadRegistry(root)
	now := time.Now().UTC()
	reg.Threads["thr-worker"] = &Thread{
		ThreadID: "thr-worker", AgentID: "gemma", Surface: surfaceWorker, SessionID: "w-1",
		Status: ThreadStatusActive, PID: exitedPID(t), StartedAt: now, LastSeenAt: now, MachineID: MachineID(),
	}
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}

	reaped, err := ReapDeadThreads(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) != 1 || reaped[0].ThreadID != "thr-worker" {
		t.Fatalf("want worker reaped, got %+v", reaped)
	}

	quiet := now.Add(-time.Hour)
	reg = &ThreadRegistry{Threads: map[string]*Thread{"thr-quiet-worker": {
		ThreadID: "thr-quiet-worker", AgentID: "gemma", Surface: surfaceWorker, SessionID: "w-2",
		Status: ThreadStatusActive, StartedAt: quiet, LastSeenAt: quiet, MachineID: MachineID(),
	}}}
	retro := func(*Thread) (*SuspendPayload, bool) { return &SuspendPayload{}, true }
	outcomes := ReconcileExits(reg, "", "", now, DefaultThreadStaleAfter, retro)
	if len(outcomes) != 1 || outcomes[0].Action != ReconcileSuspendedStale {
		t.Fatalf("want quiet worker suspended, got %+v", outcomes)
	}
}
