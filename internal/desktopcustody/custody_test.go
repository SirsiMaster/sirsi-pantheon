package desktopcustody

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeActuator struct {
	mu    sync.Mutex
	calls int
	err   error
}

type blockingFakeActuator struct {
	started chan struct{}
	release chan struct{}
	calls   int
}

func (a *blockingFakeActuator) DisruptDisplay(context.Context) error {
	a.calls++
	close(a.started)
	<-a.release
	return nil
}

func (a *fakeActuator) DisruptDisplay(context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	return a.err
}

func (a *fakeActuator) Calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

func testService(t *testing.T, now *time.Time) (*Service, *FileStore) {
	t.Helper()
	root := custodyRoot(t)
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(store).WithClock(func() time.Time { return *now }), store
}

func custodyRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func grantAt(now time.Time) Grant {
	return Grant{
		ID:              "grant-1",
		Host:            "m5",
		Owner:           "cylton",
		UnattendedStart: now.Add(-time.Minute),
		UnattendedEnd:   now.Add(5 * time.Minute),
		ExpiresAt:       now.Add(10 * time.Minute),
	}
}

func TestAuthorizeAndActRequiresExplicitActiveCustody(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	svc, _ := testService(t, &now)
	actuator := &fakeActuator{}
	if err := svc.AuthorizeAndAct(context.Background(), "m5", "cylton", "missing", actuator); !errors.Is(err, ErrAbsent) {
		t.Fatalf("absent custody error = %v, want ErrAbsent", err)
	}
	if actuator.Calls() != 0 {
		t.Fatal("actuator called without custody")
	}
	g, err := svc.Arm(grantAt(now))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AuthorizeAndAct(context.Background(), g.Host, g.Owner, g.ID, actuator); err != nil {
		t.Fatal(err)
	}
	if actuator.Calls() != 1 {
		t.Fatalf("calls = %d, want 1", actuator.Calls())
	}
}

func TestRevocationSurvivesRestartAndNeverCallsActuator(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	root := custodyRoot(t)
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	svc := New(store).WithClock(func() time.Time { return now })
	g, err := svc.Arm(grantAt(now))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Revoke(g.ID, "owner returned"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	actuator := &fakeActuator{}
	err = New(restarted).WithClock(func() time.Time { return now }).AuthorizeAndAct(context.Background(), g.Host, g.Owner, g.ID, actuator)
	if !errors.Is(err, ErrRevoked) {
		t.Fatalf("error = %v, want revoked", err)
	}
	if actuator.Calls() != 0 {
		t.Fatal("actuator called after durable revocation")
	}
}

func TestExpiryAndContestedOwnerDenyBeforeActuator(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	svc, _ := testService(t, &now)
	g, err := svc.Arm(grantAt(now))
	if err != nil {
		t.Fatal(err)
	}
	actuator := &fakeActuator{}
	if err := svc.AuthorizeAndAct(context.Background(), "m5", "another-owner", g.ID, actuator); !errors.Is(err, ErrContested) {
		t.Fatalf("error = %v, want contested", err)
	}
	now = g.ExpiresAt
	if err := svc.AuthorizeAndAct(context.Background(), g.Host, g.Owner, g.ID, actuator); !errors.Is(err, ErrExpired) {
		t.Fatalf("error = %v, want expired", err)
	}
	if actuator.Calls() != 0 {
		t.Fatal("actuator called for contested or expired custody")
	}
}

func TestRevocationSerializesAgainstActuation(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	svc, _ := testService(t, &now)
	g, err := svc.Arm(grantAt(now))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Revoke(g.ID, "active owner use"); err != nil {
		t.Fatal(err)
	}
	actuator := &fakeActuator{}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.AuthorizeAndAct(context.Background(), g.Host, g.Owner, g.ID, actuator); !errors.Is(err, ErrRevoked) {
				t.Errorf("error = %v, want revoked", err)
			}
		}()
	}
	wg.Wait()
	if actuator.Calls() != 0 {
		t.Fatalf("actuator calls = %d after serialized revocation", actuator.Calls())
	}
}

func TestTwoServicesShareTheFinalCrossProcessLock(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	root := custodyRoot(t)
	firstStore, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer firstStore.Close()
	secondStore, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer secondStore.Close()
	first := New(firstStore).WithClock(func() time.Time { return now })
	second := New(secondStore).WithClock(func() time.Time { return now })
	g, err := first.Arm(grantAt(now))
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Revoke(g.ID, "owner active"); err != nil {
		t.Fatal(err)
	}
	actuator := &fakeActuator{}
	if err := first.AuthorizeAndAct(context.Background(), g.Host, g.Owner, g.ID, actuator); !errors.Is(err, ErrRevoked) {
		t.Fatalf("error = %v, want durable cross-service revocation", err)
	}
	if actuator.Calls() != 0 {
		t.Fatal("second service revocation did not fence first service")
	}
}

func TestTwoServicesSerializeFinalActuationAndRevocation(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	root := custodyRoot(t)
	firstStore, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer firstStore.Close()
	secondStore, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer secondStore.Close()
	first := New(firstStore).WithClock(func() time.Time { return now })
	second := New(secondStore).WithClock(func() time.Time { return now })
	g, err := first.Arm(grantAt(now))
	if err != nil {
		t.Fatal(err)
	}
	actuator := &blockingFakeActuator{started: make(chan struct{}), release: make(chan struct{})}
	acted := make(chan error, 1)
	go func() { acted <- first.AuthorizeAndAct(context.Background(), g.Host, g.Owner, g.ID, actuator) }()
	<-actuator.started
	revoked := make(chan error, 1)
	go func() { revoked <- second.Revoke(g.ID, "owner returned") }()
	select {
	case err := <-revoked:
		t.Fatalf("revocation completed while the final actuator boundary held the lock: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(actuator.release)
	if err := <-acted; err != nil {
		t.Fatalf("authorized fake actuation failed: %v", err)
	}
	if err := <-revoked; err != nil {
		t.Fatalf("revocation failed after actuator completed: %v", err)
	}
	if err := first.AuthorizeAndAct(context.Background(), g.Host, g.Owner, g.ID, &fakeActuator{}); !errors.Is(err, ErrRevoked) {
		t.Fatalf("subsequent operation = %v, want revoked", err)
	}
}

func TestTwoServicesSharingOneStoreSerializeFinalActuationAndRevocation(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	root := custodyRoot(t)
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := New(store).WithClock(func() time.Time { return now })
	second := New(store).WithClock(func() time.Time { return now })
	g, err := first.Arm(grantAt(now))
	if err != nil {
		t.Fatal(err)
	}
	actuator := &blockingFakeActuator{started: make(chan struct{}), release: make(chan struct{})}
	acted := make(chan error, 1)
	go func() { acted <- first.AuthorizeAndAct(context.Background(), g.Host, g.Owner, g.ID, actuator) }()
	<-actuator.started
	revoked := make(chan error, 1)
	go func() { revoked <- second.Revoke(g.ID, "owner returned") }()
	select {
	case err := <-revoked:
		t.Fatalf("same-store revocation completed during final action: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(actuator.release)
	if err := <-acted; err != nil {
		t.Fatal(err)
	}
	if err := <-revoked; err != nil {
		t.Fatal(err)
	}
	if err := first.AuthorizeAndAct(context.Background(), g.Host, g.Owner, g.ID, &fakeActuator{}); !errors.Is(err, ErrRevoked) {
		t.Fatalf("post-revoke action = %v, want revoked", err)
	}
}

func TestCanceledContextNeverCallsActuator(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	svc, _ := testService(t, &now)
	g, err := svc.Arm(grantAt(now))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	actuator := &fakeActuator{}
	if err := svc.AuthorizeAndAct(ctx, g.Host, g.Owner, g.ID, actuator); err == nil {
		t.Fatal("canceled context unexpectedly acted")
	}
	if actuator.Calls() != 0 {
		t.Fatal("canceled context called actuator")
	}
}

func TestFileStoreRejectsNonPrivateRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(root); err == nil {
		t.Fatal("world-readable custody root unexpectedly accepted")
	}
}

func TestFileStoreRejectsSymlinkedRecord(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	root := custodyRoot(t)
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := New(store).WithClock(func() time.Time { return now })
	g, err := svc.Arm(grantAt(now))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "grant-"+g.ID+".json"), filepath.Join(root, "moved.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "moved.json"), filepath.Join(root, "grant-"+g.ID+".json")); err != nil {
		t.Fatal(err)
	}
	err = svc.AuthorizeAndAct(context.Background(), g.Host, g.Owner, g.ID, &fakeActuator{})
	if err == nil {
		t.Fatal("symlinked record unexpectedly accepted")
	}
}
