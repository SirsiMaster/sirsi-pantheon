package routerstore

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
)

// The existing harness selects SQLite by default or a disposable PostgreSQL
// schema via SIRSI_TEST_PG_DSN. Never parallelize schema-resetting fixtures.
func pruneStores(t *testing.T) (*SQLiteStore, *SQLiteStore) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "router.db")
	s := openBackendStore(t, path)
	t.Cleanup(func() { _ = s.Close() })
	var peer *SQLiteStore
	var err error
	if pgTestDSN() != "" {
		peer, err = OpenPostgres(pgTestDSN()) // same schema, no second reset
	} else {
		peer, err = OpenPath(path)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	return s, peer
}

const pruneFixtureSecret = "fixture-private-resume-text"

func pruneRecord() ThreadRecord {
	return ThreadRecord{ThreadID: "prune-fixture", Agent: "lane-a", Host: "host-a", Status: "active",
		LastSeenAt: "2026-08-06T09:00:00.000000000Z", Payload: []byte(`{"status":"active","item":"original","resume_prompt":"` + pruneFixtureSecret + `"}`)}
}

func seedPruneRecord(t *testing.T, s *SQLiteStore, r ThreadRecord) {
	t.Helper()
	if applied, err := s.UpsertThreadCAS(r); err != nil || !applied {
		t.Fatalf("seed: applied=%v err=%v", applied, err)
	}
}

func retirePruneRecord(t *testing.T, s *SQLiteStore, r ThreadRecord, scoped bool) {
	t.Helper()
	var applied bool
	var err error
	if scoped {
		applied, err = s.DeleteThreadCASScoped(r.ThreadID, r.Status, r.LastSeenAt, r.Host, []string{r.Agent})
	} else {
		applied, err = s.DeleteThreadCAS(r.ThreadID, r.Status, r.LastSeenAt, r.Host)
	}
	if err != nil || !applied {
		t.Fatalf("production retirement: applied=%v err=%v", applied, err)
	}
}

func TestThreadPruneProductionRetirementPreservesFence(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		name := "unscoped"
		if scoped {
			name = "scoped"
		}
		t.Run(name, func(t *testing.T) {
			s, peer := pruneStores(t)
			remove := func(r ThreadRecord) (bool, error) {
				if scoped {
					return peer.DeleteThreadCASScoped(r.ThreadID, r.Status, r.LastSeenAt, r.Host, []string{r.Agent})
				}
				return peer.DeleteThreadCAS(r.ThreadID, r.Status, r.LastSeenAt, r.Host)
			}
			for _, status := range []string{"closed", "reaped"} {
				r := pruneRecord()
				r.ThreadID, r.Status = "ordinary-"+status, status
				seedPruneRecord(t, s, r)
				for _, change := range []func(*ThreadRecord){
					func(x *ThreadRecord) { x.Status = "active" },
					func(x *ThreadRecord) { x.LastSeenAt = "stale-time" },
					func(x *ThreadRecord) { x.Host = "foreign-host" },
				} {
					stale := r
					change(&stale)
					if applied, err := remove(stale); err != nil || applied {
						t.Fatalf("inexact deletion: applied=%v err=%v", applied, err)
					}
				}
				if applied, err := remove(r); err != nil || !applied {
					t.Fatalf("ordinary deletion: applied=%v err=%v", applied, err)
				}
				assertPrunedRecord(t, s, r)
				if applied, err := remove(r); err != nil || applied {
					t.Fatalf("absent deletion: applied=%v err=%v", applied, err)
				}
			}
			r := pruneRecord()
			seedPruneRecord(t, s, r)
			retirePruneRecord(t, s, r, scoped)
			explicit := r
			explicit.Status = threadStatusPruned
			if applied, err := remove(explicit); err != nil || applied {
				t.Fatalf("G1 removed seeded G2 fence: applied=%v err=%v", applied, err)
			}
			if applied, err := peer.DeleteThreadCASScoped(explicit.ThreadID, explicit.Status, explicit.LastSeenAt, explicit.Host, []string{"foreign-lane"}); !errors.Is(err, ErrLaneScope) || applied {
				t.Fatalf("G1 lost seeded fence lane refusal: applied=%v err=%v", applied, err)
			}
			assertPrunedRecord(t, s, r)
		})
	}
}

func assertPrunedRecord(t *testing.T, s *SQLiteStore, original ThreadRecord) {
	t.Helper()
	if _, err := s.GetThreadRecord(original.ThreadID); !errors.Is(err, ErrThreadUnknown) {
		t.Fatalf("pruned GetThreadRecord: %v", err)
	}
	rows, err := s.ListThreads()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ThreadID == original.ThreadID {
			t.Fatal("retirement fence leaked into ListThreads")
		}
	}
	binding, err := s.ThreadBinding(original.ThreadID)
	if err != nil || binding.Status != threadStatusPruned || binding.Agent != original.Agent || binding.Host != original.Host || binding.LastSeenAt != original.LastSeenAt {
		t.Fatalf("retained authority identity: %+v err=%v", binding, err)
	}
	var payload []byte
	if err := s.db.QueryRow(`SELECT payload FROM threads WHERE thread_id=?`, original.ThreadID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, []byte("{}")) || bytes.Contains(payload, []byte(pruneFixtureSecret)) {
		t.Fatal("raw retired row retained user payload instead of empty JSON object")
	}
}

func TestThreadPruneFenceRejectsStaleWriterAcrossHandles(t *testing.T) {
	s, peer := pruneStores(t)
	r := pruneRecord()
	seedPruneRecord(t, s, r)
	stale, err := peer.GetThreadRecord(r.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	retirePruneRecord(t, s, r, false)
	stale.LastSeenAt = "2026-08-06T10:00:00.000000000Z"
	stale.Payload = []byte(`{"item":"dirty-stale-heartbeat"}`)
	if applied, err := peer.UpsertThreadCAS(stale); err != nil || applied {
		t.Fatalf("stale writer resurrected pruned row: applied=%v err=%v", applied, err)
	}
	assertPrunedRecord(t, peer, r)
	if applied, err := peer.DeleteThreadCAS(r.ThreadID, threadStatusPruned, r.LastSeenAt, r.Host); err != nil || applied {
		t.Fatalf("repeated retirement was not a no-op: %v %v", applied, err)
	}
	next := r
	next.ThreadID = "unrelated-new-id"
	seedPruneRecord(t, peer, next)
	if _, err := peer.GetThreadRecord(next.ThreadID); err != nil {
		t.Fatalf("fence blocked unrelated registration: %v", err)
	}
	gate := &server{store: s}
	if err := gate.ruleOfRa(Session{ID: "fixture-session", ThreadID: r.ThreadID, Agent: r.Agent, Host: r.Host}, "SendGuarded"); !errors.Is(err, ErrUnregistered) {
		t.Fatalf("pruned binding gained mutation authority: %v", err)
	}
}

func TestThreadPruneFenceMutationMatrix(t *testing.T) {
	for _, scopedRetirement := range []bool{false, true} {
		name := "unscoped-retirement"
		if scopedRetirement {
			name = "scoped-retirement"
		}
		t.Run(name, func(t *testing.T) {
			s, _ := pruneStores(t)
			r := pruneRecord()
			seedPruneRecord(t, s, r)
			retirePruneRecord(t, s, r, scopedRetirement)
			for _, status := range []string{"active", "idle", "blocked", "closed", "reaped", "suspended"} {
				for _, seen := range []string{r.LastSeenAt, "2026-08-06T10:00:00.000000000Z"} {
					attempt := r
					attempt.Status, attempt.LastSeenAt = status, seen
					attempt.Payload = []byte(`{"changed":true}`)
					if applied, err := s.UpsertThreadCAS(attempt); err != nil || applied {
						t.Fatalf("single %s/%s: %v %v", status, seen, applied, err)
					}
					if err := s.UpsertThreads([]ThreadRecord{attempt}); err != nil {
						t.Fatal(err)
					}
					if applied, err := s.UpsertThreadCASScoped(attempt, []string{r.Agent}); err != nil || applied {
						t.Fatalf("scoped single %s/%s: %v %v", status, seen, applied, err)
					}
					attempt.CreateOnly = true
					if applied, err := s.UpsertThreadCAS(attempt); err != nil || applied {
						t.Fatalf("create-only %s/%s revived retained key: %v %v", status, seen, applied, err)
					}
					if applied, err := s.UpsertThreadCASScoped(attempt, []string{r.Agent}); err != nil || applied {
						t.Fatalf("scoped create-only %s/%s revived retained key: %v %v", status, seen, applied, err)
					}
					attempt.CreateOnly = false
					if err := s.UpsertThreadsScoped([]ThreadRecord{attempt}, []string{r.Agent}); err != nil {
						t.Fatal(err)
					}
					if err := s.ResumeThreadCAS(attempt, r.LastSeenAt); err == nil {
						t.Fatal("resume revived pruned row")
					}
					if err := s.ResumeThreadCASScoped(attempt, r.LastSeenAt, []string{r.Agent}); err == nil {
						t.Fatal("scoped resume revived pruned row")
					}
					assertPrunedRecord(t, s, r)
				}
			}
		})
	}
}

func TestThreadPruneFencePreservesHostAndLanePredicates(t *testing.T) {
	s, _ := pruneStores(t)
	r := pruneRecord()
	seedPruneRecord(t, s, r)
	newer := r
	newer.LastSeenAt = "2026-08-06T10:00:00.000000000Z"
	seedPruneRecord(t, s, newer) // heartbeat wins before the stale prune
	if applied, err := s.DeleteThreadCAS(r.ThreadID, r.Status, r.LastSeenAt, r.Host); err != nil || applied {
		t.Fatalf("stale unscoped prune won: %v %v", applied, err)
	}
	if applied, err := s.DeleteThreadCASScoped(r.ThreadID, r.Status, r.LastSeenAt, r.Host, []string{r.Agent}); err != nil || applied {
		t.Fatalf("stale scoped prune won: %v %v", applied, err)
	}
	if applied, err := s.DeleteThreadCAS(newer.ThreadID, newer.Status, newer.LastSeenAt, "foreign-host"); err != nil || applied {
		t.Fatalf("foreign-host prune won: %v %v", applied, err)
	}
	if applied, err := s.DeleteThreadCASScoped(newer.ThreadID, newer.Status, newer.LastSeenAt, "foreign-host", []string{r.Agent}); err != nil || applied {
		t.Fatalf("foreign-host scoped prune won: %v %v", applied, err)
	}
	// Model a completed lane reassignment after an old owner observed the row.
	if _, err := s.db.Exec(`UPDATE threads SET agent=? WHERE thread_id=?`, "lane-b", r.ThreadID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteThreadCASScoped(newer.ThreadID, newer.Status, newer.LastSeenAt, newer.Host, []string{r.Agent}); !errors.Is(err, ErrLaneScope) {
		t.Fatalf("old lane pruned reassigned row: %v", err)
	}
	newer.Agent = "lane-b"
	retirePruneRecord(t, s, newer, true)
	for _, mutation := range []func() error{
		func() error { _, err := s.UpsertThreadCASScoped(r, []string{r.Agent}); return err },
		func() error { return s.UpsertThreadsScoped([]ThreadRecord{r}, []string{r.Agent}) },
		func() error { return s.ResumeThreadCASScoped(r, r.LastSeenAt, []string{r.Agent}) },
	} {
		if err := mutation(); !errors.Is(err, ErrLaneScope) {
			t.Fatalf("pruned authority lost foreign lane refusal: %v", err)
		}
	}
	foreign := newer
	foreign.Host = "foreign-host"
	foreign.LastSeenAt = "2026-08-06T11:00:00.000000000Z"
	if applied, err := s.UpsertThreadCAS(foreign); err != nil || applied {
		t.Fatalf("foreign host revived retained fence: %v %v", applied, err)
	}
	assertPrunedRecord(t, s, newer)
}

func TestThreadPruneReservedStatusCannotBeManufactured(t *testing.T) {
	s, _ := pruneStores(t)
	r := pruneRecord()
	r.Status = threadStatusPruned
	for name, mutation := range map[string]func() error{
		"single": func() error { _, err := s.UpsertThreadCAS(r); return err },
		"create-only": func() error {
			candidate := r
			candidate.CreateOnly = true
			candidate.ThreadID = "new-create-only"
			_, err := s.UpsertThreadCAS(candidate)
			return err
		},
		"bulk":          func() error { return s.UpsertThreads([]ThreadRecord{pruneRecord(), r}) },
		"scoped-single": func() error { _, err := s.UpsertThreadCASScoped(r, []string{r.Agent}); return err },
		"scoped-create-only": func() error {
			candidate := r
			candidate.CreateOnly = true
			candidate.ThreadID = "new-scoped-create-only"
			_, err := s.UpsertThreadCASScoped(candidate, []string{r.Agent})
			return err
		},
		"scoped-bulk":   func() error { return s.UpsertThreadsScoped([]ThreadRecord{pruneRecord(), r}, []string{r.Agent}) },
		"resume":        func() error { return s.ResumeThreadCAS(r, r.LastSeenAt) },
		"scoped-resume": func() error { return s.ResumeThreadCASScoped(r, r.LastSeenAt, []string{r.Agent}) },
		"import":        func() error { return s.ImportThreadsIfEmpty([]ThreadRecord{pruneRecord(), r}) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := mutation(); err == nil {
				t.Fatal("ordinary input manufactured reserved status")
			}
			var count int
			if err := s.db.QueryRow(`SELECT count(*) FROM threads`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("rejected batch partially wrote rows: count=%d err=%v", count, err)
			}
		})
	}
	// Even a legitimate suspended row cannot be retired through Resume's input.
	original := pruneRecord()
	original.Status = "suspended"
	seedPruneRecord(t, s, original)
	if err := s.ResumeThreadCAS(r, original.LastSeenAt); err == nil {
		t.Fatal("resume manufactured pruned from suspended")
	}
	if err := s.ResumeThreadCASScoped(r, original.LastSeenAt, []string{r.Agent}); err == nil {
		t.Fatal("scoped resume manufactured pruned from suspended")
	}
	got, err := s.GetThreadRecord(r.ThreadID)
	if err != nil || got.Status != "suspended" {
		t.Fatalf("rejected resume changed suspended row: %+v %v", got, err)
	}
}

func TestThreadPruneAllPrunedImportAndReopen(t *testing.T) {
	s, peer := pruneStores(t)
	r := pruneRecord()
	seedPruneRecord(t, s, r)
	retirePruneRecord(t, s, r, false)
	legacy := r
	legacy.ThreadID = "legacy-unrelated"
	if err := peer.ImportThreadsIfEmpty([]ThreadRecord{r, legacy}); err != nil {
		t.Fatal(err)
	}
	if _, err := peer.GetThreadRecord(legacy.ThreadID); !errors.Is(err, ErrThreadUnknown) {
		t.Fatalf("all-pruned store was reseeded: %v", err)
	}
	path := s.path
	if err := peer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var reopened *SQLiteStore
	var err error
	if pgTestDSN() != "" {
		reopened, err = OpenPostgres(pgTestDSN())
	} else {
		reopened, err = OpenPath(path)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	assertPrunedRecord(t, reopened, r)
}

func TestThreadPruneRawMigrationRetainsFence(t *testing.T) {
	src, err := OpenPath(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	r := pruneRecord()
	seedPruneRecord(t, src, r)
	retirePruneRecord(t, src, r, false)
	dst := newDst(t) // SQLite -> SQLite or the existing opt-in PG harness
	report, err := MigrateStore(src, dst, MigrateOptions{})
	if err != nil || report.Source.SHA256 != report.Destination.SHA256 || report.Wrote["threads"] != 1 {
		t.Fatalf("raw migration lost fence: report=%+v err=%v", report, err)
	}
	assertPrunedRecord(t, dst, r)
	back, err := OpenPath(filepath.Join(t.TempDir(), "roundtrip.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer back.Close()
	if _, err := MigrateStore(dst, back, MigrateOptions{}); err != nil {
		t.Fatalf("raw reverse migration: %v", err)
	}
	assertPrunedRecord(t, back, r)
	if applied, err := back.UpsertThreadCAS(r); err != nil || applied {
		t.Fatalf("migrated fence allowed resurrection: %v %v", applied, err)
	}
}

func TestThreadPrunePhysicalDeleteNegativeControl(t *testing.T) {
	s, peer := pruneStores(t)
	r := pruneRecord()
	seedPruneRecord(t, s, r)
	// Explicit legacy baseline only: production now retains the key. Physically
	// deleting it demonstrates why old direct writers cannot join a G2 rollout.
	result, err := s.db.Exec(`DELETE FROM threads WHERE thread_id=? AND status=? AND last_seen_at=? AND (host='' OR ?='' OR host=?)`, r.ThreadID, r.Status, r.LastSeenAt, r.Host, r.Host)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("legacy physical deletion: rows=%d err=%v", n, err)
	}
	r.Payload = []byte(`{"dirty":"stale"}`)
	if applied, err := peer.UpsertThreadCAS(r); err != nil || !applied {
		t.Fatalf("negative control did not reproduce absent-row resurrection: %v %v", applied, err)
	}
	if _, err := peer.GetThreadRecord(r.ThreadID); err != nil {
		t.Fatal(err)
	}
}

func TestThreadPruneG1GuardOmissionNegativeControls(t *testing.T) {
	for _, mutation := range []string{"legacy-upsert", "legacy-delete"} {
		t.Run(mutation, func(t *testing.T) {
			s, _ := pruneStores(t)
			r := pruneRecord()
			seedPruneRecord(t, s, r)
			retirePruneRecord(t, s, r, false)
			if mutation == "legacy-upsert" {
				// Pre-guard INSERT/ON CONFLICT shape: omit only pruned from the
				// status guard; retain host, freshness and argument semantics.
				result, err := s.db.Exec(`INSERT INTO threads(thread_id,agent,status,last_seen_at,payload,host) VALUES(?,?,?,?,?,?)
ON CONFLICT(thread_id) DO UPDATE SET
 agent=excluded.agent,status=excluded.status,last_seen_at=excluded.last_seen_at,payload=excluded.payload,
 host=CASE WHEN excluded.host='' THEN threads.host ELSE excluded.host END
WHERE threads.status NOT IN ('closed','reaped','suspended')
  AND (threads.host='' OR excluded.host='' OR threads.host=excluded.host)
  AND (excluded.last_seen_at > threads.last_seen_at
       OR excluded.last_seen_at = threads.last_seen_at
          AND (excluded.status IN ('closed','reaped','suspended')
               OR excluded.status = threads.status AND excluded.payload > threads.payload))`,
					r.ThreadID, r.Agent, "active", "2026-08-06T10:00:00.000000000Z", r.Payload, r.Host)
				if err != nil {
					t.Fatal(err)
				}
				if n, err := result.RowsAffected(); err != nil || n != 1 {
					t.Fatalf("legacy upsert did not revive fence: rows=%d err=%v", n, err)
				}
				if got, err := s.GetThreadRecord(r.ThreadID); err != nil || got.Status != "active" {
					t.Fatalf("legacy resurrection not observed: %+v err=%v", got, err)
				}
			} else {
				result, err := s.db.Exec(`DELETE FROM threads WHERE thread_id=? AND status=? AND last_seen_at=? AND (host='' OR ?='' OR host=?)`,
					r.ThreadID, threadStatusPruned, r.LastSeenAt, r.Host, r.Host)
				if err != nil {
					t.Fatal(err)
				}
				if n, err := result.RowsAffected(); err != nil || n != 1 {
					t.Fatalf("legacy delete did not remove fence: rows=%d err=%v", n, err)
				}
				if _, err := s.ThreadBinding(r.ThreadID); !errors.Is(err, ErrThreadUnknown) {
					t.Fatalf("legacy fence removal not observed: %v", err)
				}
			}
		})
	}
}
