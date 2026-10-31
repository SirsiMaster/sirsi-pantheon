//go:build !windows

package router

import (
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

// Outside the lease it is still reaped, and a worker with a session id keeps
// the OS-truth rule: only an attended session resumes across processes.
func TestReapDeadThreads_SessionPIDDeathStillReapsWorkersAndExpiredLeases(t *testing.T) {
	root := t.TempDir()
	reg, _ := LoadThreadRegistry(root)
	now := time.Now().UTC()
	reg.Threads["thr-worker"] = &Thread{
		ThreadID: "thr-worker", AgentID: "gemma", Surface: surfaceWorker, SessionID: "w-1",
		Status: ThreadStatusActive, PID: exitedPID(t), StartedAt: now, LastSeenAt: now, MachineID: MachineID(),
	}
	reg.Threads["thr-expired"] = &Thread{
		ThreadID: "thr-expired", AgentID: "ra", Surface: surfaceClaude, SessionID: "conv-old",
		Status: ThreadStatusActive, PID: exitedPID(t), StartedAt: now, LastSeenAt: now.Add(-(LeaseSessionTTL + time.Minute)),
		MachineID: MachineID(),
	}
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}

	reaped, err := ReapDeadThreads(root)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range reaped {
		seen[r.ThreadID] = true
	}
	if !seen["thr-worker"] || !seen["thr-expired"] {
		t.Fatalf("want worker and expired-lease session reaped, got %+v", reaped)
	}
}
