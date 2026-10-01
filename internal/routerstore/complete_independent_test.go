package routerstore

// Independent-instance coverage for Complete (router item 20261001-140902,
// codex-pantheon review of PR944 20236ff8). The review asked for two
// independently authenticated instances exercising the actual completion
// path against both SQLite and PostgreSQL backends. Correction (20261001,
// router item 20261001-145610): an earlier version of this file claimed the
// codebase ships no PostgreSQL store — false; internal/routerstore/open_postgres.go
// is what `sirsi router serve` runs on in production, and pgtest_test.go
// already wires a dual-driver harness (rs-07) via SIRSI_TEST_PG_DSN. These
// tests use that harness: instance A opens through openBackendStore (which
// resets the schema when SIRSI_TEST_PG_DSN is set), and every subsequent
// instance opens through openSecondHandle, which targets the SAME backend
// without resetting it — the real multi-process shape both backends support,
// one on-disk SQLite file or one Postgres database shared across handles.
// Unset SIRSI_TEST_PG_DSN -> SQLite only (CI default). Set it against a
// reachable Postgres (see internal/routerstore/pg/README.md) to run the same
// three tests against the Postgres backend.

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// openSecondHandle opens another handle at the same target as a prior
// openBackendStore(t, path) call, WITHOUT resetting the schema — simulating
// a second process attaching to a ledger a first process already created.
func openSecondHandle(t *testing.T, path string) *SQLiteStore {
	t.Helper()
	if dsn := pgTestDSN(); dsn != "" {
		s, err := OpenPostgres(dsn)
		if err != nil {
			t.Fatalf("openSecondHandle: OpenPostgres: %v", err)
		}
		return s
	}
	s, err := OpenPath(path)
	if err != nil {
		t.Fatalf("openSecondHandle: OpenPath: %v", err)
	}
	return s
}

// Two independent store handles on the same file: the holder that loses its
// lease to expiry/reclaim (stale) is refused, and the reclaiming instance's
// result is the one preserved — not silently overwritten by the stale
// holder's late completion.
func TestCompleteTwoIndependentInstancesStaleClaimantRefusedNewerResultPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "router.db")

	a := openBackendStore(t, path)
	t.Cleanup(func() { _ = a.Close() })

	start := time.Date(2026, 7, 2, 15, 4, 5, 0, time.UTC)
	a.now = func() time.Time { return start }

	if _, sendErr := a.Send("owner", "codex-home", "item", "review", "x"); sendErr != nil {
		t.Fatalf("send: %v", sendErr)
	}

	// Instance A claims first — short TTL so it goes stale quickly.
	leaseA, err := a.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatalf("instance A claim: %v", err)
	}

	// Instance B: a second, independent handle on the SAME backend target.
	b := openSecondHandle(t, path)
	t.Cleanup(func() { _ = b.Close() })
	past := start.Add(2 * time.Hour) // past A's lease expiry
	b.now = func() time.Time { return past }

	// A's lease has now expired; B's ClaimNext reclaims it and mints a fresh
	// token — this is the crash-safe self-heal path (reclaimExpiredTx), not a
	// race within one process.
	leaseB, err := b.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatalf("instance B reclaim: %v", err)
	}
	if leaseB.Token == leaseA.Token {
		t.Fatal("instance B must mint a fresh token on reclaim, not reuse A's stale token")
	}

	// A, unaware its lease was reclaimed, tries to complete with its stale
	// token. It MUST be refused — this is the exact split-brain the fenced
	// Complete exists to prevent.
	staleErr := a.Complete(leaseA.ItemID, leaseA.Token, "stale result from instance A")
	if !errors.Is(staleErr, ErrLeaseInvalid) {
		t.Fatalf("stale instance A's Complete must fail with ErrLeaseInvalid, got %v", staleErr)
	}

	// B, the current lease holder, completes successfully.
	if completeErr := b.Complete(leaseB.ItemID, leaseB.Token, "result from instance B"); completeErr != nil {
		t.Fatalf("instance B Complete: %v", completeErr)
	}

	// The preserved result is B's, never A's stale write — read back through
	// a THIRD independent handle to prove the mutation is durable on disk,
	// not an artifact of one connection's local state.
	c := openSecondHandle(t, path)
	t.Cleanup(func() { _ = c.Close() })
	row, err := c.Get(leaseA.ItemID)
	if err != nil {
		t.Fatalf("instance C read: %v", err)
	}
	if row.Result != "result from instance B" {
		t.Fatalf("preserved result = %q, want instance B's result", row.Result)
	}
	if row.Status != StatusCompleted {
		t.Fatalf("status = %q, want %q", row.Status, StatusCompleted)
	}

	// A's subsequent retry with the same stale token still refuses — the
	// terminal state is now a second, independent reason to refuse.
	if err := a.Complete(leaseA.ItemID, leaseA.Token, "late retry"); !errors.Is(err, ErrTerminal) && !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("stale retry against a completed item must refuse (terminal or invalid lease), got %v", err)
	}
}

// Negative control: a store handle that has been closed (the "unavailable
// backend" shape — the connection a caller holds can no longer serve reads
// or writes) must fail the completion closed, never silently succeed or
// return a false positive.
func TestCompleteUnavailableStoreFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "router.db")

	s := openBackendStore(t, path)
	if _, sendErr := s.Send("owner", "codex-home", "item", "review", "x"); sendErr != nil {
		t.Fatalf("send: %v", sendErr)
	}
	lease, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if closeErr := s.Close(); closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}

	// The backend is now unavailable to this handle. Complete must return an
	// error — never a nil (false-success) result.
	if completeErr := s.Complete(lease.ItemID, lease.Token, "result after close"); completeErr == nil {
		t.Fatal("Complete against a closed store must fail, not succeed")
	}

	// Verify via a fresh handle that nothing was written: the item is still
	// claimed, not completed with the above result.
	r := openSecondHandle(t, path)
	t.Cleanup(func() { _ = r.Close() })
	row, err := r.Get(lease.ItemID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.Status == StatusCompleted {
		t.Fatal("a failed Complete against an unavailable store must not have completed the item")
	}
}

// Negative control: completing an item id that does not exist in the store
// must refuse, not silently succeed — the "read failure" shape at the Complete
// boundary.
func TestCompleteUnknownItemFailsClosed(t *testing.T) {
	s := newTestStore(t)
	if err := s.Complete("does-not-exist", "any-token", "result"); err == nil {
		t.Fatal("Complete against an unknown item id must fail, not succeed")
	}
}
