//go:build !windows

package maat

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func lockTestDecision(i int) Decision {
	return Decision{
		Kind: "reservation grant", Requester: fmt.Sprintf("writer-%d", i), Resource: "m5",
		Assessed: "free", Determination: "grant", Why: "no overlap", Evidence: fmt.Sprintf("reservation:r-%d", i),
	}
}

// Many concurrent CLI-like writers (separate journal values, one path) must
// all succeed and none may lose a row: the lock waits instead of failing.
func TestJournalConcurrentAppendersNeverFailOrLoseRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	const n = 24
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := (&FileDecisionJournal{Path: path}).Append(lockTestDecision(i)); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent append failed: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(raw, []byte("\n")); got != n {
		t.Fatalf("journal has %d rows, want %d", got, n)
	}
	rows, err := (&FileDecisionJournal{Path: path}).Recent(n)
	if err != nil || len(rows) != n {
		t.Fatalf("strict read: %d rows, err %v", len(rows), err)
	}
}

// A lock file left behind by a dead process (the 0-byte .decisions.jsonl.lock
// from Oct 5 on the M5) is not a lock: flock dies with its holder.
func TestJournalLeftoverLockFileDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.jsonl")
	if err := os.WriteFile(filepath.Join(dir, ".decisions.jsonl.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := journalLockWait
	journalLockWait = 0 // zero wait: a leftover file must not need any waiting
	t.Cleanup(func() { journalLockWait = old })
	if err := (&FileDecisionJournal{Path: path}).Append(lockTestDecision(1)); err != nil {
		t.Fatalf("leftover lock file blocked append: %v", err)
	}
}

// Negative control + the fix: a live holder that releases within the wait
// window is waited out; the same holder with no wait is refused (old behavior).
func TestJournalLockWaitsForShortHolderAndRefusesWithoutWait(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	j := &FileDecisionJournal{Path: path}
	old := journalLockWait
	t.Cleanup(func() { journalLockWait = old })

	hold := func(d time.Duration) (release func()) {
		held, done := make(chan struct{}), make(chan struct{})
		go func() {
			_ = withJournalMutationLock(path, func() error {
				close(held)
				time.Sleep(d)
				return nil
			})
			close(done)
		}()
		<-held
		return func() { <-done }
	}

	journalLockWait = 0
	wait := hold(300 * time.Millisecond)
	err := j.Append(lockTestDecision(1))
	wait()
	if err == nil || !strings.Contains(err.Error(), "another local write or repair") {
		t.Fatalf("zero-wait append under a live holder = %v, want refusal", err)
	}

	journalLockWait = 3 * time.Second
	wait = hold(300 * time.Millisecond)
	if err := j.Append(lockTestDecision(2)); err != nil {
		t.Fatalf("append should have waited out a 300ms holder: %v", err)
	}
	wait()
}
