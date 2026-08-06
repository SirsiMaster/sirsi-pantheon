//nolint:govet // Test setup deliberately scopes operation errors next to each assertion.
package routerstore

import (
	"path/filepath"
	"testing"
)

func TestThreadMigrationIsCeilingAndUpgradesV15(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := MaxSupportedSchemaVersion(); got != 16 {
		t.Fatalf("schema ceiling = %d, want 16", got)
	}
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 16 {
		t.Fatalf("fresh schema = %d, want 16", version)
	}
	if _, err := s.db.Exec(`DROP TABLE threads; PRAGMA user_version=15;`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatalf("v15 to v16: %v", err)
	}
	defer s.Close()
	if _, err := s.ListThreads(); err != nil {
		t.Fatalf("threads absent after v15 to v16: %v", err)
	}
}

func TestUpsertAndListThreads(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	want := ThreadRecord{ThreadID: "thr-1", Agent: "codex-pantheon", Status: "active", LastSeenAt: "2026-08-06T09:00:00Z", Payload: []byte(`{"thread_id":"thr-1"}`)}
	if err := s.UpsertThreads([]ThreadRecord{want}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListThreads()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ThreadID != want.ThreadID || string(got[0].Payload) != string(want.Payload) {
		t.Fatalf("threads = %#v", got)
	}
}

func TestConcurrentThreadUpsertsPreserveDistinctRowsAndTerminalState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.db")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	active := func(id string) ThreadRecord {
		return ThreadRecord{ThreadID: id, Agent: "codex-pantheon", Status: "active", LastSeenAt: "2026-08-06T09:00:00Z", Payload: []byte(`{"status":"active"}`)}
	}
	done := make(chan error, 2)
	go func() { done <- a.UpsertThreads([]ThreadRecord{active("thr-a")}) }()
	go func() { done <- b.UpsertThreads([]ThreadRecord{active("thr-b")}) }()
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	rows, err := a.ListThreads()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("concurrent registrations left %d rows, want 2", len(rows))
	}
	closed := active("thr-a")
	closed.Status = "closed"
	closed.LastSeenAt = "2026-08-06T09:00:30Z"
	closed.Payload = []byte(`{"status":"closed"}`)
	if err := a.UpsertThreads([]ThreadRecord{closed}); err != nil {
		t.Fatal(err)
	}
	staleHeartbeat := active("thr-a")
	staleHeartbeat.LastSeenAt = "2026-08-06T09:01:00Z"
	if err := b.UpsertThreads([]ThreadRecord{staleHeartbeat}); err != nil {
		t.Fatal(err)
	}
	rows, err = a.ListThreads()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ThreadID == "thr-a" && row.Status != "closed" {
			t.Fatalf("late heartbeat revived terminal row: %#v", row)
		}
	}
}

func TestHeartbeatCannotImplicitlyResumeConcurrentSuspend(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	record := ThreadRecord{ThreadID: "thr", Agent: "a", Status: "active", LastSeenAt: "2026-08-06T09:00:00Z", Payload: []byte(`{"status":"active"}`)}
	if err := s.UpsertThreads([]ThreadRecord{record}); err != nil {
		t.Fatal(err)
	}
	suspendedAt := "2026-08-06T09:01:00Z"
	suspended := record
	suspended.Status = "suspended"
	suspended.LastSeenAt = suspendedAt
	suspended.Payload = []byte(`{"status":"suspended"}`)
	if err := s.UpsertThreads([]ThreadRecord{suspended}); err != nil {
		t.Fatal(err)
	}
	lateHeartbeat := record
	lateHeartbeat.LastSeenAt = "2026-08-06T09:02:00Z"
	if err := s.UpsertThreads([]ThreadRecord{lateHeartbeat}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListThreads()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != "suspended" {
		t.Fatalf("late heartbeat implicitly resumed suspended thread: %#v", rows)
	}
	if err := s.ResumeThreadCAS(lateHeartbeat, suspendedAt); err != nil {
		t.Fatal(err)
	}
	rows, err = s.ListThreads()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != "active" {
		t.Fatalf("explicit resume did not persist: %#v", rows)
	}
}

func TestStaleCrossRowSnapshotCannotOverwriteNewerHeartbeat(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rec := func(id, seen, item string) ThreadRecord {
		return ThreadRecord{ThreadID: id, Agent: "a", Status: "active", LastSeenAt: seen, Payload: []byte(`{"current_item":"` + item + `"}`)}
	}
	if err := s.UpsertThreads([]ThreadRecord{rec("a", "2026-08-06T09:00:00Z", "old-a"), rec("b", "2026-08-06T09:00:00Z", "old-b")}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertThreads([]ThreadRecord{rec("b", "2026-08-06T09:02:00Z", "new-b")}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertThreads([]ThreadRecord{rec("a", "2026-08-06T09:01:00Z", "new-a"), rec("b", "2026-08-06T09:00:00Z", "old-b")}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListThreads()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ThreadID == "a" && string(row.Payload) != `{"current_item":"new-a"}` {
			t.Fatalf("valid target update rolled back: %s", row.Payload)
		}
		if row.ThreadID == "b" && string(row.Payload) != `{"current_item":"new-b"}` {
			t.Fatalf("stale cross-row overwrite: %s", row.Payload)
		}
	}
}
