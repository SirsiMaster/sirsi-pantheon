package apprecovery

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryStore struct{ receipt Receipt }

func (s *memoryStore) Save(receipt Receipt) error   { s.receipt = receipt; return nil }
func (s *memoryStore) Load(string) (Receipt, error) { return s.receipt, nil }

type fakeDriver struct {
	pid      int
	started  bool
	stopped  bool
	startErr error
}

func (d *fakeDriver) Capture(context.Context, Target) (Snapshot, error) {
	return Snapshot{Files: map[string]string{"/state": "sha"}}, nil
}
func (d *fakeDriver) ClearTransientState(context.Context, Target) error { return nil }
func (d *fakeDriver) PID(context.Context, Target) (int, error) {
	if d.started {
		return d.pid + 1, nil
	}
	if d.stopped {
		return 0, nil
	}
	return d.pid, nil
}
func (d *fakeDriver) Stop(context.Context, Target, int) error { d.stopped = true; return nil }
func (d *fakeDriver) Start(context.Context, Target, Snapshot) error {
	if d.startErr != nil {
		return d.startErr
	}
	d.started = true
	d.stopped = false
	return nil
}
func (d *fakeDriver) Ready(context.Context, Target) error {
	if !d.started {
		return errors.New("not ready")
	}
	return nil
}

func TestRecoverPersistsVerifiedReplacement(t *testing.T) {
	driver := &fakeDriver{pid: 41}
	store := &memoryStore{}
	manager, err := NewManager([]Target{{ID: "browser", Kind: KindAppSavedState, BundleID: "com.example.browser", ExecutablePath: "/Applications/Browser.app/Contents/MacOS/Browser", StatePaths: []string{"/state"}, ReadyTimeout: time.Second}}, driver, store)
	if err != nil {
		t.Fatal(err)
	}
	manager.poll = time.Millisecond
	receipt, err := manager.Recover(context.Background(), "browser", ModeRestore)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Phase != PhaseReady || receipt.OldPID != 41 || receipt.NewPID != 42 {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
	if store.receipt.Phase != PhaseReady {
		t.Fatalf("receipt was not durable: %+v", store.receipt)
	}
}

func TestRecoverFailsClosedAndRecordsStartFailure(t *testing.T) {
	driver := &fakeDriver{pid: 41, startErr: errors.New("launch rejected")}
	store := &memoryStore{}
	manager, err := NewManager([]Target{{ID: "service", Kind: KindLaunchd, ExecutablePath: "/usr/local/bin/service", LaunchdTarget: "gui/501/com.example.service"}}, driver, store)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Recover(context.Background(), "service", ModeFresh)
	if err == nil {
		t.Fatal("expected recovery failure")
	}
	if store.receipt.Phase != PhaseFailed || store.receipt.FailureCode != "start_failed" {
		t.Fatalf("failure receipt missing: %+v", store.receipt)
	}
}

func TestResumeContinuesDurableStoppedRecovery(t *testing.T) {
	driver := &fakeDriver{pid: 9}
	store := &memoryStore{receipt: Receipt{Schema: "pantheon.app-recovery.v1", TargetID: "worker", Kind: KindCheckpointed, Mode: ModeRestore, Phase: PhaseStopped, OldPID: 9, Snapshot: Snapshot{Files: map[string]string{"/state": "sha"}}}}
	manager, err := NewManager([]Target{{ID: "worker", Kind: KindCheckpointed, ExecutablePath: "/usr/local/bin/worker", StatePaths: []string{"/state"}}}, driver, store)
	if err != nil {
		t.Fatal(err)
	}
	manager.poll = time.Millisecond
	receipt, err := manager.Resume(context.Background(), "worker")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Phase != PhaseReady || receipt.NewPID != 10 {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
}

func TestResumeRejectsReceiptFromDifferentAuthority(t *testing.T) {
	store := &memoryStore{receipt: Receipt{Schema: "pantheon.app-recovery.v1", TargetID: "worker", Kind: KindLaunchd, Mode: ModeFresh, Phase: PhaseStopped}}
	manager, err := NewManager([]Target{{ID: "worker", Kind: KindCheckpointed, ExecutablePath: "/usr/local/bin/worker"}}, &fakeDriver{}, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Resume(context.Background(), "worker"); err == nil {
		t.Fatal("expected authority mismatch rejection")
	}
}

func TestReconcilePendingResumesOnlyOptedInTarget(t *testing.T) {
	driver := &fakeDriver{pid: 9, stopped: true}
	store := &memoryStore{receipt: Receipt{Schema: "pantheon.app-recovery.v1", TargetID: "worker", Kind: KindCheckpointed, Mode: ModeRestore, Phase: PhaseStopped, OldPID: 9, Snapshot: Snapshot{Files: map[string]string{"/state": "sha"}}}}
	manager, err := NewManager([]Target{{ID: "worker", Kind: KindCheckpointed, ExecutablePath: "/usr/local/bin/worker", StatePaths: []string{"/state"}, AutoResume: true}}, driver, store)
	if err != nil {
		t.Fatal(err)
	}
	manager.poll = time.Millisecond
	results := manager.ReconcilePending(context.Background())
	if len(results) != 1 || results[0].Err != nil || results[0].Receipt.Phase != PhaseReady {
		t.Fatalf("unexpected reconciliation: %+v", results)
	}

	driver2 := &fakeDriver{pid: 9, stopped: true}
	manager2, err := NewManager([]Target{{ID: "worker", Kind: KindCheckpointed, ExecutablePath: "/usr/local/bin/worker", StatePaths: []string{"/state"}}}, driver2, store)
	if err != nil {
		t.Fatal(err)
	}
	if results := manager2.ReconcilePending(context.Background()); len(results) != 0 {
		t.Fatalf("non-opted-in target reconciled: %+v", results)
	}
}

func TestFreshRestartDoesNotRequireRestorableState(t *testing.T) {
	driver := &fakeDriver{pid: 21}
	store := &memoryStore{}
	manager, err := NewManager([]Target{{ID: "cache-worker", Kind: KindLaunchd, ExecutablePath: "/usr/local/bin/cache-worker", LaunchdTarget: "gui/501/com.example.cache-worker"}}, driver, store)
	if err != nil {
		t.Fatal(err)
	}
	manager.poll = time.Millisecond
	receipt, err := manager.Recover(context.Background(), "cache-worker", ModeFresh)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Mode != ModeFresh || len(receipt.Snapshot.Files) != 0 || receipt.Phase != PhaseReady {
		t.Fatalf("unexpected fresh restart receipt: %+v", receipt)
	}
}

func TestLaunchdPersistentCleanupFailsRegistryAdmission(t *testing.T) {
	_, err := NewManager([]Target{{ID: "service", Kind: KindLaunchd, ExecutablePath: "/usr/local/bin/service", LaunchdTarget: "gui/501/com.example.service", FreshStatePaths: []string{"/tmp/service-cache"}}}, &fakeDriver{}, &memoryStore{})
	if err == nil {
		t.Fatal("expected unsafe launchd persistent cleanup to be rejected")
	}
}
