package router

import (
	"os"
	"testing"
	"time"
)

func TestLegacyTimezoneMismatchIsNotProofOfRecycledPID(t *testing.T) {
	instant := time.Date(2026, 11, 18, 3, 19, 9, 0, time.UTC)
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	recorded := instant.In(chicago).Format(time.ANSIC)
	live := instant.UTC().Format(time.ANSIC)
	restore := probeStubs(t, PIDAlive, live)
	defer restore()
	if got := PIDStateOf(4242, recorded); got != PIDUnknown || DeadByOSTruth(got) {
		t.Fatalf("ambiguous legacy local times falsely classified a live PID as dead: %s", got)
	}
}

func TestAmbiguousConsumerIsPreservedAndOccupiesSlot(t *testing.T) {
	root := t.TempDir()
	writeConsumerPIDFile(root, "existing", 4242, "Tue Nov 17 21:19:09 2026")
	restore := probeStubs(t, PIDAlive, "Wed Nov 18 03:19:09 2026")
	defer restore()
	adopted := adoptRunningConsumer(root, "existing")
	if adopted == nil || !adopted.running() {
		t.Fatal("uncertain consumer must not be replaced")
	}
	if _, err := os.Stat(consumerPIDFilePath(root, "existing")); err != nil {
		t.Fatalf("uncertain consumer marker was removed: %v", err)
	}
	if n := runningConsumersExcept(root, "different"); n != 1 {
		t.Fatalf("uncertain consumer must reserve capacity: got %d", n)
	}
}

func TestUncertainConsumerClearsOnceExitIsConfirmed(t *testing.T) {
	root := t.TempDir()
	writeConsumerPIDFile(root, "existing", 4242, "Tue Nov 17 21:19:09 2026")
	restore := probeStubs(t, PIDAlive, "Wed Nov 18 03:19:09 2026")
	adopted := adoptRunningConsumer(root, "existing")
	if adopted == nil || !adopted.running() {
		restore()
		t.Fatal("uncertain consumer was not preserved")
	}
	restore()
	restore = probeStubs(t, PIDGone, "")
	defer restore()
	if adopted.running() {
		t.Fatal("confirmed exited consumer still counted as running")
	}
	if again := adoptRunningConsumer(root, "existing"); again != nil {
		t.Fatal("confirmed exited consumer was adopted")
	}
	if _, err := os.Stat(consumerPIDFilePath(root, "existing")); !os.IsNotExist(err) {
		t.Fatalf("confirmed exited consumer marker remains: %v", err)
	}
	if n := runningConsumersExcept(root, "different"); n != 0 {
		t.Fatalf("exited consumer still occupies %d slots", n)
	}
}

func TestAmbiguousLegacyRegistrationDoesNotMintOrAdopt(t *testing.T) {
	root := t.TempDir()
	first, err := RegisterThread(root, &Thread{AgentID: "same", Surface: "worker", PID: 4242,
		StartTime: "Tue Nov 17 21:19:09 2026"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = RegisterThread(root, &Thread{AgentID: "same", Surface: "worker", PID: 4242,
		StartTime: "Wed Nov 18 03:19:09 2026"})
	if err == nil {
		t.Fatal("ambiguous identity must fail without adoption or duplication")
	}
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Threads) != 1 || reg.Threads[first.ThreadID] == nil {
		t.Fatalf("registry changed: %+v", reg.Threads)
	}
}

func TestLegacyDifferentStartTimesRemainUnknown(t *testing.T) {
	restore := probeStubs(t, PIDAlive, "Wed Nov 18 04:19:09 2026")
	defer restore()
	got := PIDStateOf(4242, "Tue Nov 17 21:19:09 2026")
	if got != PIDUnknown || DeadByOSTruth(got) {
		t.Fatalf("timezone-free timestamps cannot prove recycling: %s", got)
	}
}

func TestMixedSignatureFormatsRemainUnknown(t *testing.T) {
	for _, pair := range [][2]string{
		{"Tue Nov 17 21:19:09 2026", "epoch:1794971949"},
		{"epoch:1794971949", "Wed Nov 18 03:19:09 2026"},
	} {
		restore := probeStubs(t, PIDAlive, pair[1])
		got := PIDStateOf(4242, pair[0])
		restore()
		if got != PIDUnknown || DeadByOSTruth(got) {
			t.Fatalf("mixed formats falsely classified a live PID as dead: %s", got)
		}
	}
}

func TestMatchingLegacyStartRemainsAlive(t *testing.T) {
	stamp := "Tue Nov 17 21:19:09 2026"
	restore := probeStubs(t, PIDAlive, stamp)
	defer restore()
	if got := PIDStateOf(4242, stamp); got != PIDAlive {
		t.Fatalf("matching signature must remain alive: %s", got)
	}
}

func TestReaperKeepsThreadWithAmbiguousLegacyTimezone(t *testing.T) {
	root := t.TempDir()
	thread, err := RegisterThread(root, &Thread{
		AgentID: "claude-test", Surface: "claude", PID: 4242, Host: "test-host",
		StartTime: "Tue Nov 17 21:19:09 2026",
	})
	if err != nil {
		t.Fatal(err)
	}
	restore := probeStubs(t, PIDAlive, "Wed Nov 18 03:19:09 2026")
	defer restore()
	reaped, err := ReapDeadThreads(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) != 0 {
		t.Fatalf("ambiguous timezone reaped thread: %+v", reaped)
	}
	registry, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.Threads[thread.ThreadID].Status; got != ThreadStatusActive {
		t.Fatalf("thread status = %q; want active", got)
	}
}

func TestUnknownWatcherDoesNotSupersedeSuspendedThread(t *testing.T) {
	const machine = "TEST-MACHINE"
	oldMachine := getMachineIDFn()
	setMachineIDFn(func() string { return machine })
	defer setMachineIDFn(oldMachine)
	restore := probeStubs(t, PIDAlive, "Wed Nov 18 03:19:09 2026")
	defer restore()
	root := t.TempDir()
	reg := &ThreadRegistry{Threads: map[string]*Thread{
		"uncertain": {ThreadID: "uncertain", AgentID: "same", Surface: "claude", PID: 4242,
			StartTime: "Tue Nov 17 21:19:09 2026", MachineID: machine, Status: ThreadStatusActive},
		"suspended": {ThreadID: "suspended", AgentID: "same", Surface: "claude",
			MachineID: machine, Status: ThreadStatusSuspended},
	}}
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}
	retired, err := ReapStrayThreads(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(retired) != 0 {
		t.Fatalf("unknown watcher retired a sibling: %+v", retired)
	}
	got, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Threads["suspended"].Status != ThreadStatusSuspended {
		t.Fatal("suspended thread changed")
	}
}
