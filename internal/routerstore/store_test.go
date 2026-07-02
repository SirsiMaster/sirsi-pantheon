package routerstore

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// newTestStore opens an isolated in-memory store with a fixed clock so opened/
// closed timestamps are deterministic. Each test gets its own Store, so there
// is no package-global state to swap and no need for t.Parallel guards
// (repo lessons #129/#131/#139).
func newTestStore(t *testing.T) *Store {
	t.Helper()
	// A distinct DSN per test keeps in-memory DBs isolated even if the driver
	// shares a cache; ":memory:" alone is per-connection and we cap conns to 1.
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	fixed := time.Date(2026, 7, 2, 15, 4, 5, 0, time.UTC)
	s.now = func() time.Time { return fixed }
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenAndMigrateIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "router.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if _, sendErr := s1.Send("a", "b", "hello", "task", "", "do it"); sendErr != nil {
		t.Fatalf("Send: %v", sendErr)
	}
	if closeErr := s1.Close(); closeErr != nil {
		t.Fatalf("Close store: %v", closeErr)
	}
	// Re-open the same file: migrate must be idempotent and data must persist.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	defer s2.Close()
	all, err := s2.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1 persisted item, got %d", len(all))
	}
}

func TestSendGetClose(t *testing.T) {
	s := newTestStore(t)

	id, err := s.Send("claude-pantheon", "codex-pantheon", "Router v2 review", "review", "sirsi-pantheon", "please bind")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.From != "claude-pantheon" || got.To != "codex-pantheon" {
		t.Errorf("routing mismatch: from=%q to=%q", got.From, got.To)
	}
	if got.Type != "review" || got.Repo != "sirsi-pantheon" {
		t.Errorf("type/repo mismatch: type=%q repo=%q", got.Type, got.Repo)
	}
	if got.Status != "open" {
		t.Errorf("new item status = %q, want open", got.Status)
	}
	if got.Instructions != "please bind" {
		t.Errorf("instructions = %q", got.Instructions)
	}
	if got.Opened == "" || got.Closed != "" {
		t.Errorf("timestamps: opened=%q closed=%q", got.Opened, got.Closed)
	}

	if closeErr := s.CloseItem(id, "bound and merged"); closeErr != nil {
		t.Fatalf("CloseItem: %v", closeErr)
	}
	got, err = s.Get(id)
	if err != nil {
		t.Fatalf("Get after close: %v", err)
	}
	if got.Status != "closed" {
		t.Errorf("status after close = %q, want closed", got.Status)
	}
	if got.Closed == "" {
		t.Error("closed timestamp not set")
	}
	if got.Result != "bound and merged" {
		t.Errorf("result = %q", got.Result)
	}
}

func TestCloseErrors(t *testing.T) {
	s := newTestStore(t)

	if err := s.CloseItem("nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("CloseItem(unknown) = %v, want ErrNotFound", err)
	}

	id, err := s.Send("a", "b", "t", "", "", "i")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := s.CloseItem(id, "first"); err != nil {
		t.Fatalf("first CloseItem: %v", err)
	}
	if err := s.CloseItem(id, "second"); !errors.Is(err, ErrAlreadyClosed) {
		t.Errorf("double CloseItem = %v, want ErrAlreadyClosed", err)
	}
}

func TestCloseEmptyResultPlaceholder(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.Send("a", "b", "t", "", "", "i")
	if err := s.CloseItem(id, "   "); err != nil {
		t.Fatalf("CloseItem: %v", err)
	}
	got, _ := s.Get(id)
	if got.Result != "(closed without result)" {
		t.Errorf("empty-result placeholder = %q", got.Result)
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(missing) = %v, want ErrNotFound", err)
	}
}

func TestSendValidation(t *testing.T) {
	s := newTestStore(t)
	tests := []struct {
		name           string
		from, to, mtyp string
		wantErr        bool
	}{
		{"ok", "a", "b", "task", false},
		{"ok empty type", "a", "b", "", false},
		{"missing from", "", "b", "", true},
		{"missing to", "a", "", "", true},
		{"bad type", "a", "b", "gossip", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Send(tc.from, tc.to, "title", tc.mtyp, "", "instr")
			if (err != nil) != tc.wantErr {
				t.Errorf("Send err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestInboxFiltering(t *testing.T) {
	s := newTestStore(t)

	// Two open for alice, one open for bob, one closed for alice.
	mustPut(t, s, Item{ID: "20260101-000001-x-alice-a", From: "x", To: "alice", Title: "a", Status: "open", Opened: "t"})
	mustPut(t, s, Item{ID: "20260101-000002-x-alice-b", From: "x", To: "alice", Title: "b", Status: "open", Opened: "t"})
	mustPut(t, s, Item{ID: "20260101-000003-x-bob-c", From: "x", To: "bob", Title: "c", Status: "open", Opened: "t"})
	mustPut(t, s, Item{ID: "20260101-000004-x-alice-d", From: "x", To: "alice", Title: "d", Status: "closed", Opened: "t", Closed: "t2"})

	alice, err := s.Inbox("alice")
	if err != nil {
		t.Fatalf("Inbox(alice): %v", err)
	}
	if len(alice) != 2 {
		t.Fatalf("alice inbox = %d items, want 2 (open only)", len(alice))
	}
	// Oldest id first.
	if alice[0].ID >= alice[1].ID {
		t.Errorf("inbox not sorted oldest-first: %q then %q", alice[0].ID, alice[1].ID)
	}

	bob, _ := s.Inbox("bob")
	if len(bob) != 1 {
		t.Errorf("bob inbox = %d, want 1", len(bob))
	}

	// Empty agent → all open items (3 open across both agents).
	allOpen, _ := s.Inbox("")
	if len(allOpen) != 3 {
		t.Errorf("all-open inbox = %d, want 3", len(allOpen))
	}

	all, _ := s.ListAll()
	if len(all) != 4 {
		t.Errorf("ListAll = %d, want 4", len(all))
	}
}

func TestPutUpsert(t *testing.T) {
	s := newTestStore(t)
	it := Item{ID: "dup", From: "a", To: "b", Title: "v1", Status: "open", Opened: "t"}
	mustPut(t, s, it)
	it.Title = "v2"
	mustPut(t, s, it)

	got, err := s.Get("dup")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "v2" {
		t.Errorf("upsert title = %q, want v2", got.Title)
	}
	all, _ := s.ListAll()
	if len(all) != 1 {
		t.Errorf("upsert produced %d rows, want 1", len(all))
	}
}

func TestPutValidation(t *testing.T) {
	s := newTestStore(t)
	if err := s.Put(Item{ID: "", From: "a", To: "b"}); err == nil {
		t.Error("Put with empty id should error")
	}
	if err := s.Put(Item{ID: "x", Status: "weird"}); err == nil {
		t.Error("Put with invalid status should error")
	}
	// Empty status defaults to open.
	if err := s.Put(Item{ID: "y", From: "a", To: "b", Title: "t"}); err != nil {
		t.Fatalf("Put default-status: %v", err)
	}
	got, _ := s.Get("y")
	if got.Status != "open" {
		t.Errorf("defaulted status = %q, want open", got.Status)
	}
}

func TestBackfillIdempotent(t *testing.T) {
	s := newTestStore(t)
	items := []Item{
		{ID: "i1", From: "a", To: "b", Title: "one", Status: "open", Opened: "t", Instructions: "body one"},
		{ID: "i2", From: "a", To: "c", Title: "two", Status: "closed", Opened: "t", Closed: "t2", Result: "done"},
		{ID: "", From: "a", To: "b", Title: "bad"}, // malformed → recorded, skipped
	}

	rep, err := s.Backfill(items)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if rep.Seen != 3 || rep.Inserted != 2 || rep.Updated != 0 {
		t.Errorf("first backfill report = %+v", rep)
	}
	if len(rep.Errors) != 1 {
		t.Errorf("want 1 recorded error for malformed item, got %v", rep.Errors)
	}

	// Spot-check a body round-tripped losslessly (PRD /goal #4).
	got, err := s.Get("i1")
	if err != nil {
		t.Fatalf("Get i1: %v", err)
	}
	if got.Instructions != "body one" {
		t.Errorf("body not preserved: %q", got.Instructions)
	}

	// Re-run: same input must update in place, not duplicate.
	rep2, err := s.Backfill(items)
	if err != nil {
		t.Fatalf("second Backfill: %v", err)
	}
	if rep2.Inserted != 0 || rep2.Updated != 2 {
		t.Errorf("second backfill report = %+v, want 0 inserted / 2 updated", rep2)
	}
	all, _ := s.ListAll()
	if len(all) != 2 {
		t.Errorf("after 2 backfills: %d rows, want 2 (no duplicates)", len(all))
	}
}

func TestAgentsLifecycle(t *testing.T) {
	s := newTestStore(t)

	if err := s.RegisterAgent("", 1); err == nil {
		t.Error("RegisterAgent with empty id should error")
	}
	if err := s.RegisterAgent("claude-pantheon", 4242); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	a, err := s.GetAgent("claude-pantheon")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if a.PID != 4242 || a.RegisteredAt == "" {
		t.Errorf("agent record = %+v", a)
	}
	firstReg := a.RegisteredAt

	// Re-register (surface restart, new pid): registered_at preserved, pid updated, no dup.
	if err := s.RegisterAgent("claude-pantheon", 9999); err != nil {
		t.Fatalf("re-RegisterAgent: %v", err)
	}
	a, _ = s.GetAgent("claude-pantheon")
	if a.PID != 9999 {
		t.Errorf("pid after re-register = %d, want 9999", a.PID)
	}
	if a.RegisteredAt != firstReg {
		t.Errorf("registered_at changed on re-register: %q → %q", firstReg, a.RegisteredAt)
	}
	agents, _ := s.ListAgents()
	if len(agents) != 1 {
		t.Errorf("ListAgents = %d, want 1 (idempotent register)", len(agents))
	}

	// Heartbeat unknown agent → ErrNotFound.
	if err := s.Heartbeat("ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Heartbeat(ghost) = %v, want ErrNotFound", err)
	}
	if err := s.Heartbeat("claude-pantheon"); err != nil {
		t.Errorf("Heartbeat: %v", err)
	}
}

func TestState(t *testing.T) {
	s := newTestStore(t)
	if _, ok, err := s.GetState("k"); err != nil || ok {
		t.Errorf("GetState(absent) = ok=%v err=%v, want ok=false", ok, err)
	}
	if err := s.SetState("last_claude_read", "20260702-000000"); err != nil {
		t.Fatalf("SetState: %v", err)
	}
	v, ok, err := s.GetState("last_claude_read")
	if err != nil || !ok || v != "20260702-000000" {
		t.Errorf("GetState = %q ok=%v err=%v", v, ok, err)
	}
	// Overwrite.
	_ = s.SetState("last_claude_read", "20260702-120000")
	v, _, _ = s.GetState("last_claude_read")
	if v != "20260702-120000" {
		t.Errorf("SetState overwrite = %q", v)
	}
}

// TestConcurrentWrites exercises the single-writer / WAL configuration under
// the race detector: many goroutines Send + Close concurrently against a
// file-backed store. Run with `go test -race`.
func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "router.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	const workers = 16
	const perWorker = 20
	var wg sync.WaitGroup
	errCh := make(chan error, workers*perWorker)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				id := fmt.Sprintf("w%02d-i%03d", w, i)
				if putErr := s.Put(Item{ID: id, From: "a", To: fmt.Sprintf("agent-%d", w%4), Title: "t", Status: "open", Opened: "t"}); putErr != nil {
					errCh <- putErr
					return
				}
				if closeErr := s.CloseItem(id, "ok"); closeErr != nil {
					errCh <- closeErr
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent write error: %v", err)
	}

	all, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != workers*perWorker {
		t.Errorf("got %d items, want %d", len(all), workers*perWorker)
	}
	// Concurrent reads while confirming every row closed cleanly.
	for _, it := range all {
		if it.Status != "closed" {
			t.Errorf("item %s status = %q, want closed", it.ID, it.Status)
		}
	}
}

func TestSlugifyParity(t *testing.T) {
	// These expectations mirror internal/work.slugify so a Send here produces
	// the same id the file router would.
	tests := []struct{ in, want string }{
		{"Hello World", "hello-world"},
		{"  trim  ", "trim"},
		{"", "untitled"},
		{"!!!", "untitled"},
		{"UPPER_case-123", "upper-case-123"},
	}
	for _, tc := range tests {
		if got := slugify(tc.in); got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Length is capped at 60.
	long := ""
	for i := 0; i < 100; i++ {
		long += "a"
	}
	if got := slugify(long); len(got) != 60 {
		t.Errorf("slugify(long) len = %d, want 60", len(got))
	}
}

func TestSendIDShape(t *testing.T) {
	s := newTestStore(t)
	id, err := s.Send("claude-pantheon", "codex-pantheon", "Router v2 review", "review", "", "x")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	// Fixed clock is 2026-07-02 15:04:05 UTC → prefix must match the
	// timestamp-from-to-slug convention (same as internal/work.SendScoped).
	want := "20260702-150405-claude-pantheon-codex-pantheon-router-v2-review"
	if id != want {
		t.Errorf("Send id = %q, want %q", id, want)
	}
}

func TestGetAgentNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetAgent("nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAgent(nobody) = %v, want ErrNotFound", err)
	}
	agents, err := s.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents empty: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("empty ListAgents = %d, want 0", len(agents))
	}
}

func mustPut(t *testing.T, s *Store, it Item) {
	t.Helper()
	if err := s.Put(it); err != nil {
		t.Fatalf("Put(%s): %v", it.ID, err)
	}
}
