package router

import (
	"testing"
	"time"
)

// seedWorker registers a worker thread directly in the registry with the given
// pid/status/lastSeen, returning its id.
func seedWorker(t *testing.T, root, agent string, pid int, lastSeen time.Time) string {
	t.Helper()
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	id := NewThreadID()
	reg.Threads[id] = &Thread{
		ThreadID:   id,
		AgentID:    agent,
		Surface:    surfaceWorker,
		PID:        pid,
		Status:     ThreadStatusActive,
		MachineID:  MachineID(),
		LastSeenAt: lastSeen,
	}
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestAdoptWorkerThread_StableAcrossRestart is the core P-heartbeat-owner
// contract: a wake-loop restart (new PID) reuses the existing worker record
// instead of minting a duplicate.
func TestAdoptWorkerThread_StableAcrossRestart(t *testing.T) {
	root := t.TempDir()
	old := getPIDStateFn()
	setPIDStateFn(func(int) PIDState { return PIDGone }) // prior loop's pid is dead
	defer setPIDStateFn(old)

	prior := seedWorker(t, root, "claude-pantheon", 40001, time.Now().UTC().Add(-time.Hour))

	thr, err := adoptWorkerThread(root, "claude-pantheon", 40002)
	if err != nil {
		t.Fatal(err)
	}
	if thr == nil || thr.ThreadID != prior {
		t.Fatalf("adopted = %+v, want the prior record %s", thr, prior)
	}
	if thr.PID != 40002 || thr.Status != ThreadStatusActive {
		t.Fatalf("adopted record not re-pointed: pid=%d status=%s", thr.PID, thr.Status)
	}
}

// TestAdoptWorkerThread_ReapsDeadDuplicates: the newest record is adopted, the
// OS-dead duplicate is retired, and a genuinely-alive duplicate is untouched.
func TestAdoptWorkerThread_ReapsDeadDuplicates(t *testing.T) {
	root := t.TempDir()
	const alivePID = 50001
	old := getPIDStateFn()
	setPIDStateFn(func(pid int) PIDState {
		if pid == alivePID {
			return PIDAlive
		}
		return PIDGone
	})
	defer setPIDStateFn(old)

	now := time.Now().UTC()
	deadDup := seedWorker(t, root, "claude-finalwishes", 50002, now.Add(-3*time.Hour))
	liveDup := seedWorker(t, root, "claude-finalwishes", alivePID, now.Add(-2*time.Hour))
	newest := seedWorker(t, root, "claude-finalwishes", 50003, now.Add(-time.Hour))
	otherAgent := seedWorker(t, root, "claude-nexus", 50004, now.Add(-3*time.Hour))

	thr, err := adoptWorkerThread(root, "claude-finalwishes", 50005)
	if err != nil {
		t.Fatal(err)
	}
	if thr.ThreadID != newest {
		t.Fatalf("adopted %s, want newest %s", thr.ThreadID, newest)
	}

	reg, _ := LoadThreadRegistry(root)
	if got := reg.Threads[deadDup].Status; got != ThreadStatusReaped {
		t.Errorf("dead duplicate status = %s, want reaped", got)
	}
	if got := reg.Threads[liveDup].Status; got != ThreadStatusActive {
		t.Errorf("live duplicate status = %s, want active (never touch a live thread)", got)
	}
	if got := reg.Threads[otherAgent].Status; got != ThreadStatusActive {
		t.Errorf("other agent's worker status = %s, want active (out of scope)", got)
	}
}

// TestAdoptWorkerThread_NoCandidate returns nil so the caller mints fresh.
func TestAdoptWorkerThread_NoCandidate(t *testing.T) {
	root := t.TempDir()
	thr, err := adoptWorkerThread(root, "claude-pantheon", 60001)
	if err != nil {
		t.Fatal(err)
	}
	if thr != nil {
		t.Fatalf("adopted %+v from an empty registry, want nil", thr)
	}
}

// TestAdoptWorkerThread_ForeignMachineUntouched: a worker record from another
// machine is neither adopted nor reaped.
func TestAdoptWorkerThread_ForeignMachineUntouched(t *testing.T) {
	root := t.TempDir()
	oldID := getMachineIDFn()
	setMachineIDFn(func() string { return "THIS-MACHINE" })
	defer setMachineIDFn(oldID)
	oldPID := getPIDStateFn()
	setPIDStateFn(func(int) PIDState { return PIDGone })
	defer setPIDStateFn(oldPID)

	reg, _ := LoadThreadRegistry(root)
	id := NewThreadID()
	reg.Threads[id] = &Thread{
		ThreadID: id, AgentID: "claude-pantheon", Surface: surfaceWorker,
		PID: 70001, Status: ThreadStatusActive, MachineID: "OTHER-MACHINE",
		LastSeenAt: time.Now().UTC(),
	}
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}

	thr, err := adoptWorkerThread(root, "claude-pantheon", 70002)
	if err != nil {
		t.Fatal(err)
	}
	if thr != nil {
		t.Fatalf("adopted a foreign machine's record: %+v", thr)
	}
	reg, _ = LoadThreadRegistry(root)
	if got := reg.Threads[id].Status; got != ThreadStatusActive {
		t.Errorf("foreign record status = %s, want active (untouched)", got)
	}
}
