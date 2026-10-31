package routerstore

import (
	"context"
	"testing"
	"time"
)

func TestThreadPrunePostgresG1WaitingMutationsSeeCommittedG2Fence(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*SQLiteStore, ThreadRecord) (bool, error)
	}{
		{"single", func(s *SQLiteStore, r ThreadRecord) (bool, error) {
			r.LastSeenAt = "2026-08-06T10:00:00.000000000Z"
			return s.UpsertThreadCAS(r)
		}},
		{"scoped-single", func(s *SQLiteStore, r ThreadRecord) (bool, error) {
			r.LastSeenAt = "2026-08-06T10:00:00.000000000Z"
			return s.UpsertThreadCASScoped(r, []string{r.Agent})
		}},
		{"bulk", func(s *SQLiteStore, r ThreadRecord) (bool, error) {
			r.LastSeenAt = "2026-08-06T10:00:00.000000000Z"
			return false, s.UpsertThreads([]ThreadRecord{r})
		}},
		{"scoped-bulk", func(s *SQLiteStore, r ThreadRecord) (bool, error) {
			r.LastSeenAt = "2026-08-06T10:00:00.000000000Z"
			return false, s.UpsertThreadsScoped([]ThreadRecord{r}, []string{r.Agent})
		}},
		{"delete", func(s *SQLiteStore, r ThreadRecord) (bool, error) {
			return s.DeleteThreadCAS(r.ThreadID, r.Status, r.LastSeenAt, r.Host)
		}},
		{"scoped-delete", func(s *SQLiteStore, r ThreadRecord) (bool, error) {
			return s.DeleteThreadCASScoped(r.ThreadID, r.Status, r.LastSeenAt, r.Host, []string{r.Agent})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) { testG1PostgresWaitingMutation(t, tc.run) })
	}
}

func testG1PostgresWaitingMutation(t *testing.T, mutate func(*SQLiteStore, ThreadRecord) (bool, error)) {
	t.Helper()
	if pgTestDSN() == "" {
		t.Skip("requires disposable SIRSI_TEST_PG_DSN; PostgreSQL contention not credited when skipped")
	}
	s, peer := pruneStores(t)
	r := pruneRecord()
	seedPruneRecord(t, s, r)
	// Pin the peer pool to one backend so pid and statement_timeout apply to
	// the actual G1 production mutation below, not a surrogate SQL write.
	peer.db.SetMaxOpenConns(1)
	if _, err := peer.db.Exec(`SET statement_timeout = '5s'`); err != nil {
		t.Fatal(err)
	}
	var pid int
	if err := peer.db.QueryRow(`SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	blocker, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback() }()
	// Hold accepted b1 G2 fixture SQL uncommitted, not G1's physical delete.
	retired, err := blocker.Exec(retireThreadSQL, []byte("{}"), r.ThreadID, r.Status, r.LastSeenAt, r.Host, r.Host)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := retired.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("fixture retirement: rows=%d err=%v", n, err)
	}
	stale := r
	stale.Payload = []byte(`{"dirty":"before-retirement-commit"}`)
	var applied bool
	var writeErr error
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		applied, writeErr = mutate(peer, stale)
	}()
	// Every exit releases the row lock and joins the writer. Its own backend
	// timeout bounds even a failed fixture; no global hooks or leaked callers.
	defer func() {
		_ = blocker.Rollback()
		select {
		case <-exited:
		case <-time.After(6 * time.Second):
			t.Error("waiting mutation did not terminate after unblock/statement timeout")
		}
	}()
	waiting := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var locked bool
		if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=?`, pid).Scan(&locked); err != nil {
			t.Fatal(err)
		}
		if locked {
			waiting = true
			break
		}
		select {
		case <-exited:
			t.Fatal("mutation returned before retirement committed; expected actual lock contention")
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("production mutation was not observed waiting before retirement commit")
	}
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-time.After(6 * time.Second):
		t.Fatal("mutation did not finish after retirement commit")
	}
	if writeErr != nil || applied {
		t.Fatalf("waiting mutation ignored committed retirement: applied=%v err=%v", applied, writeErr)
	}
	assertPrunedRecord(t, s, r)
}
