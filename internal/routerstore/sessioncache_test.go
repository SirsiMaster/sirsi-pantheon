package routerstore

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Two different executables acting as the SAME agent (e.g. 19 wake loops that all fall back to the hostname, some running
// an older binary) must not invalidate each other's cached session: a session is valid only for the exact runtime that
// minted it, so the cache is keyed by runtime too. Before this, alternating runtimes minted a session on EVERY call
// (measured on the M5: 17 sessions in 30 s, ~86k in 48 h).
func TestSessionCacheDoesNotFlapAcrossRuntimes(t *testing.T) {
	backend := openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = backend.Close() })
	backend.notifyDir = t.TempDir()
	h, err := Handler(backend, ServerOptions{Token: "t0k", MaxWait: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	dir := t.TempDir()

	call := func(runtime string) {
		rs := NewRemoteStore(srv.URL, "t0k") // a fresh process each time: only the on-disk cache can save the mint
		rs.host, rs.agent, rs.runtime, rs.sessionDir, rs.threadID = "h", "loop", runtime, dir, ""
		if _, err := rs.Inbox("loop"); err != nil {
			t.Fatalf("inbox as runtime %s: %v", runtime, err)
		}
	}
	for i := 0; i < 10; i++ {
		call(strings.Repeat("a", 64))
		call(strings.Repeat("b", 64))
	}
	var n int
	if err := backend.db.QueryRow(`SELECT count(*) FROM sessions WHERE agent='loop'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("20 calls from 2 runtimes minted %d sessions, want 2 (one per runtime); the cache is flapping", n)
	}
}

// Stale cache files (a binary that no longer runs) are pruned so the cache directory cannot grow without bound.
func TestSessionCachePrunesFilesUntouchedForAWeek(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "gone-agent.deadbeef0000.json")
	fresh := filepath.Join(dir, "live-agent.cafe00000000.json")
	other := filepath.Join(dir, "notes.txt")
	for _, p := range []string{old, fresh, other} {
		if err := os.WriteFile(p, []byte(`{"id":"x","runtime_hash":"r"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	week := time.Now().Add(-8 * 24 * time.Hour)
	for _, p := range []string{old, other} {
		if err := os.Chtimes(p, week, week); err != nil {
			t.Fatal(err)
		}
	}
	pruneSessionCache(dir, 7*24*time.Hour)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("a week-old session cache file was not pruned")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("a fresh session cache file was pruned")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("a non-session file was pruned")
	}
}

// ---- server-side GC ----

func ageSession(t *testing.T, s *SQLiteStore, id, lastSeen, revoked string) {
	t.Helper()
	if _, err := s.db.Exec(`UPDATE sessions SET last_seen=?, revoked=? WHERE session_id=?`, lastSeen, revoked, id); err != nil {
		t.Fatal(err)
	}
}

func hasSession(t *testing.T, s *SQLiteStore, id string) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM sessions WHERE session_id=?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

// GC removes dead sessions and only those: idle ones and long-revoked ones go; fresh ones, recently revoked ones, and
// any session that still owns a lease on an OPEN item or unfinished task stay. A closed item's lease binding does not
// protect its session (those rows are never deleted, so protecting them would shield every session forever).
func TestGCSessionsRemovesOnlyDeadSessions(t *testing.T) {
	s := newTestStore(t)
	mint := func(agent string) string {
		sess, err := s.MintSession("h", agent, "rt")
		if err != nil {
			t.Fatal(err)
		}
		return sess.ID
	}
	const longAgo = "2020-01-01T00:00:00Z"
	fresh := mint("fresh")
	idle := mint("idle")
	revOld := mint("rev-old")
	revNew := mint("rev-new")
	leasedOpen := mint("leased-open")
	leasedClosed := mint("leased-closed")
	leasedTask := mint("leased-task")
	now := s.clock().Format(time.RFC3339)

	ageSession(t, s, idle, longAgo, "")
	ageSession(t, s, revOld, longAgo, longAgo)
	ageSession(t, s, revNew, now, now)
	ageSession(t, s, leasedOpen, longAgo, "")
	ageSession(t, s, leasedClosed, longAgo, "")
	ageSession(t, s, leasedTask, longAgo, "")

	openItem, err := s.Send("a", "b", "open work", "proposal", "x")
	if err != nil {
		t.Fatal(err)
	}
	closedItem, err := s.Send("a", "c", "closed work", "proposal", "x")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CloseItem(closedItem, "done"); err != nil {
		t.Fatal(err)
	}
	if err := s.BindItemSession(openItem, leasedOpen); err != nil {
		t.Fatal(err)
	}
	if err := s.BindItemSession(closedItem, leasedClosed); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTask(Task{Agent: "ra", TaskID: "t1", Subject: "s", Status: "in-progress"}); err != nil {
		t.Fatal(err)
	}
	if err := s.BindTaskSession("ra", "t1", leasedTask); err != nil {
		t.Fatal(err)
	}

	n, err := s.GCSessions(24*time.Hour, 100)
	if err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]string{"idle": idle, "revoked long ago": revOld, "leased closed item": leasedClosed} {
		if hasSession(t, s, id) {
			t.Fatalf("%s session survived GC", name)
		}
	}
	for name, id := range map[string]string{"fresh": fresh, "recently revoked": revNew, "leased OPEN item": leasedOpen, "leased unfinished task": leasedTask} {
		if !hasSession(t, s, id) {
			t.Fatalf("%s session was deleted by GC", name)
		}
	}
	if n != 3 {
		t.Fatalf("GC reported %d deletions, want 3", n)
	}
	// Idempotent, and bounded by the limit.
	if again, _ := s.GCSessions(24*time.Hour, 100); again != 0 {
		t.Fatalf("second GC deleted %d more", again)
	}
}

func TestGCSessionsHonorsTheBatchLimit(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		sess, err := s.MintSession("h", "a", "rt")
		if err != nil {
			t.Fatal(err)
		}
		ageSession(t, s, sess.ID, "2020-01-01T00:00:00Z", "")
	}
	if n, err := s.GCSessions(time.Hour, 2); err != nil || n != 2 {
		t.Fatalf("GC with limit 2 deleted %d, %v", n, err)
	}
	var left int
	_ = s.db.QueryRow(`SELECT count(*) FROM sessions`).Scan(&left)
	if left != 3 {
		t.Fatalf("%d sessions left, want 3", left)
	}
}

// Minting runs the GC opportunistically (Cloud Run has no scheduler), at most once per interval.
func TestMintSessionTriggersGCAtMostOncePerInterval(t *testing.T) {
	s := newTestStore(t)
	old, err := s.MintSession("h", "old", "rt")
	if err != nil {
		t.Fatal(err)
	}
	ageSession(t, s, old.ID, "2020-01-01T00:00:00Z", "")
	s.sessionGCNext = time.Time{} // the mint above already consumed this interval's slot; make a GC due again
	if _, err := s.MintSession("h", "trigger", "rt"); err != nil {
		t.Fatal(err)
	}
	if hasSession(t, s, old.ID) {
		t.Fatal("an idle session survived a mint that should have triggered GC")
	}
	old2, _ := s.MintSession("h", "old2", "rt") // within the interval (frozen test clock): GC must NOT run again
	ageSession(t, s, old2.ID, "2020-01-01T00:00:00Z", "")
	if _, err := s.MintSession("h", "again", "rt"); err != nil {
		t.Fatal(err)
	}
	if !hasSession(t, s, old2.ID) {
		t.Fatal("GC ran twice inside one interval")
	}
}

// The operator's dry run and the real GC must agree: SessionStats.Collectable is exactly what GCSessions would delete.
func TestSessionStatsCollectableMatchesGC(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 4; i++ {
		sess, err := s.MintSession("host-a", "x", "rt")
		if err != nil {
			t.Fatal(err)
		}
		if i < 3 {
			ageSession(t, s, sess.ID, "2020-01-01T00:00:00Z", "")
		}
	}
	st, err := s.SessionStats(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 4 || st.Idle != 3 || st.Collectable != 3 || st.Live != 1 || len(st.ByHost) != 1 || st.ByHost[0].Host != "host-a" {
		t.Fatalf("stats %+v", st)
	}
	n, err := s.GCSessions(24*time.Hour, 100)
	if err != nil || n != st.Collectable {
		t.Fatalf("GC deleted %d (err %v), dry-run said %d", n, err, st.Collectable)
	}
}
