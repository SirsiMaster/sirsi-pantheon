package routerstore

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
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
	if _, sendErr := s1.Send("a", "b", "hello", "review", "do it"); sendErr != nil {
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

// TestConcurrentOpenMigratesOnce reproduces the cross-process migration race:
// several openers hit a FRESH database at once, each starting at user_version 0.
// Every one of them runs Open (its own *sql.DB, like a separate process). With
// the naive read-then-migrate, a slower opener re-ran an applied step and failed
// with "duplicate column name: lease_token"; with the BEGIN IMMEDIATE + in-lock
// re-check, all succeed and the schema ends up applied exactly once.
func TestConcurrentOpenMigratesOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.db")
	const openers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, openers)
	for i := 0; i < openers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all at once to maximize contention on the fresh DB
			s, err := Open(path)
			if err != nil {
				errs <- err
				return
			}
			_ = s.Close()
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Open on a fresh DB failed (migration race): %v", err)
	}
	// The migrated schema must be intact and usable exactly once.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("post-race Open: %v", err)
	}
	defer s.Close()
	if _, err := s.Send("a", "b", "post-race", "review", "works"); err != nil {
		t.Fatalf("Send on post-race store: %v", err)
	}
}

func TestSendGetClose(t *testing.T) {
	s := newTestStore(t)

	id, err := s.Send("claude-pantheon", "codex-pantheon", "Router v2 review", "review", "please bind")
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
	if got.Type != "review" {
		t.Errorf("type mismatch: type=%q", got.Type)
	}
	// A new item has never been wake-touched — mirrors work.SendTyped writing
	// no wake_* frontmatter.
	if got.WakeStatus != "" || got.WakeAttemptedAt != "" || got.WakeAdapter != "" || got.WakeError != "" {
		t.Errorf("new item carries wake state: %+v", got)
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

	id, err := s.Send("a", "b", "t", "", "i")
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
	id, _ := s.Send("a", "b", "t", "", "i")
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
		{"ok proposal", "a", "b", "proposal", false},
		{"ok review", "a", "b", "review", false},
		{"ok decision", "a", "b", "decision", false},
		{"ok empty type", "a", "b", "", false},
		{"missing from", "", "b", "", true},
		{"missing to", "a", "", "", true},
		{"bad type", "a", "b", "gossip", true},
		// "task" is NOT in the ADR-024 §5 vocabulary (proposal|review|decision)
		// — Send holds the documented line even though work.SendTyped is lax.
		{"task rejected", "a", "b", "task", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Send(tc.from, tc.to, "title", tc.mtyp, "instr")
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
	id, err := s.Send("claude-pantheon", "codex-pantheon", "Router v2 review", "review", "x")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	// Fixed clock is 2026-07-02 15:04:05 UTC → prefix must match the
	// timestamp-from-to-slug convention (same as internal/work.SendTyped).
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

// columnFor maps each routerstore.Item field to its items-table column. It is
// the single declared field↔column correspondence; TestFieldFidelityWithWorkItem
// checks it is a bijection against the live schema.
var columnFor = map[string]string{
	"ID":              "id",
	"From":            "from_agent",
	"To":              "to_agent",
	"Title":           "title",
	"Type":            "type",
	"Status":          "status",
	"Opened":          "opened",
	"Closed":          "closed",
	"Instructions":    "instructions",
	"Result":          "result",
	"WakeStatus":      "wake_status",
	"WakeAttemptedAt": "wake_attempted_at",
	"WakeAdapter":     "wake_adapter",
	"WakeError":       "wake_error",
}

func fieldNames(t reflect.Type) map[string]bool {
	names := make(map[string]bool, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		names[t.Field(i).Name] = true
	}
	return names
}

// TestFieldFidelityWithWorkItem is the durable schema-fidelity guarantee
// (PRD /goal #4, zero data loss). It enforces, structurally:
//
//  1. routerstore.Item carries EXACTLY the fields of internal/work.Item —
//     every work.Item field is persisted (frontmatter or body), so a field
//     added to work.Item FAILS here until the mirror catches up, and a field
//     invented on the store side (the old Repo column) fails the reverse check;
//  2. every Item field maps to a real column in the live schema, and the
//     schema has no unmapped columns;
//  3. every field actually round-trips through Put→Get (a column that exists
//     but isn't wired into the SQL would fail here).
func TestFieldFidelityWithWorkItem(t *testing.T) {
	workT := reflect.TypeOf(work.Item{})
	storeT := reflect.TypeOf(Item{})

	// 1. Field-set equality, both directions.
	workFields := fieldNames(workT)
	storeFields := fieldNames(storeT)
	for f := range workFields {
		if !storeFields[f] {
			t.Errorf("work.Item field %q missing from routerstore.Item — Phase-4 backfill would silently drop it (PRD /goal #4)", f)
		}
	}
	for f := range storeFields {
		if !workFields[f] {
			t.Errorf("routerstore.Item field %q does not exist in work.Item — invented columns break schema fidelity", f)
		}
	}

	// 2. Field↔column bijection against the live schema.
	s := newTestStore(t)
	rows, err := s.db.Query(`PRAGMA table_info(items);`)
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()
	schemaCols := map[string]bool{}
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             any
		)
		if scanErr := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); scanErr != nil {
			t.Fatalf("scan table_info: %v", scanErr)
		}
		schemaCols[name] = true
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		t.Fatalf("table_info rows: %v", rowsErr)
	}
	for f := range storeFields {
		col, ok := columnFor[f]
		if !ok {
			t.Errorf("Item field %q has no declared column mapping — add it to columnFor and the schema", f)
			continue
		}
		if !schemaCols[col] {
			t.Errorf("Item field %q maps to column %q which is missing from the items schema", f, col)
		}
	}
	mapped := map[string]bool{}
	for _, col := range columnFor {
		mapped[col] = true
	}
	// Phase-2 §2b dispatch-contract columns are STORE-ONLY lifecycle state
	// (fenced leases, idempotency keys, singleton keys, bounded counters) —
	// deliberately NOT mirrored into work.Item: the markdown file is the human
	// audit view, and a stale or mutated file must never be able to change
	// lifecycle (§2b axiom 8). Every column outside this documented set stays
	// under the strict no-invented-columns law.
	dispatchContractCols := map[string]bool{
		"lease_token": true, "lease_expires": true, "claimed_by": true,
		"attempts": true, "idem_key": true, "source_item": true,
		"failure_class": true, "occurrences": true, "first_seen": true, "last_seen": true,
	}
	for col := range schemaCols {
		if !mapped[col] && !dispatchContractCols[col] {
			t.Errorf("schema column %q maps to no work.Item field — invented columns are forbidden", col)
		}
	}

	// 3. Every field round-trips through Put→Get with a distinct value.
	in := Item{}
	v := reflect.ValueOf(&in).Elem()
	for i := 0; i < storeT.NumField(); i++ {
		v.Field(i).SetString("v-" + storeT.Field(i).Name)
	}
	in.ID = "fidelity-item"
	in.Status = "closed" // must be a valid status to pass Put validation
	if putErr := s.Put(in); putErr != nil {
		t.Fatalf("Put: %v", putErr)
	}
	got, err := s.Get(in.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(in, got) {
		t.Errorf("field round-trip mismatch:\n put: %+v\n got: %+v", in, got)
	}
}

// TestCloseItemConcurrentDoubleClose proves the close guard is atomic: two
// goroutines racing to close the same item must yield EXACTLY one success and
// one ErrAlreadyClosed, every trial. The pre-fix Get→check→UPDATE window let
// both closers win (TOCTOU). Run with -race.
func TestCloseItemConcurrentDoubleClose(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	const trials = 200
	for i := 0; i < trials; i++ {
		id := fmt.Sprintf("race-%03d", i)
		if err := s.Put(Item{ID: id, From: "a", To: "b", Title: "t", Status: "open", Opened: "t"}); err != nil {
			t.Fatalf("trial %d: Put: %v", i, err)
		}
		start := make(chan struct{})
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for g := 0; g < 2; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs <- s.CloseItem(id, "winner")
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		var nilCount, alreadyClosed int
		for err := range errs {
			switch {
			case err == nil:
				nilCount++
			case errors.Is(err, ErrAlreadyClosed):
				alreadyClosed++
			default:
				t.Fatalf("trial %d: unexpected error: %v", i, err)
			}
		}
		if nilCount != 1 || alreadyClosed != 1 {
			t.Fatalf("trial %d: nil=%d alreadyClosed=%d, want exactly one of each", i, nilCount, alreadyClosed)
		}
	}
}

// adaptWorkItem is the Phase-4 importer adaptation, spelled out field-for-field.
// TestFieldFidelityWithWorkItem guarantees this list is complete.
func adaptWorkItem(w work.Item) Item {
	return Item{
		ID:              w.ID,
		From:            w.From,
		To:              w.To,
		Title:           w.Title,
		Type:            w.Type,
		Status:          w.Status,
		Opened:          w.Opened,
		Closed:          w.Closed,
		Instructions:    w.Instructions,
		Result:          w.Result,
		WakeStatus:      w.WakeStatus,
		WakeAttemptedAt: w.WakeAttemptedAt,
		WakeAdapter:     w.WakeAdapter,
		WakeError:       w.WakeError,
	}
}

// TestExportMarkdownRoundTrip proves the PRD recovery path honestly:
// real file-router items (written by work.SendTyped/Close/SetWake) →
// work.ListAll → Backfill → ExportMarkdown must reproduce the ORIGINAL FILES
// BYTE-FOR-BYTE, and work.ListAll over the exported tree must parse back to
// equal items. If the SQLite index is ever corrupted or distrusted, this is
// the proof the dump is readable by the existing file router with zero loss.
func TestExportMarkdownRoundTrip(t *testing.T) {
	root := t.TempDir()

	// Items written by the REAL file-router writers, covering: typed + wake
	// annotation (open), untyped + closed with result, typed + wake-unavailable
	// + closed without result (placeholder).
	id1, err := work.SendTyped(root, "claude-pantheon", "codex-pantheon", "review the store", "review", "please bind\n\nwith care")
	if err != nil {
		t.Fatalf("SendTyped: %v", err)
	}
	if wakeErr := work.SetWake(root, id1, work.WakeAnnotation{
		Status: "wake-attempted", AttemptedAt: "2026-07-02T15:04:05Z", Adapter: "cli-spawn",
	}); wakeErr != nil {
		t.Fatalf("SetWake: %v", wakeErr)
	}
	id2, err := work.Send(root, "alice", "bob", "plain item", "just do it")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if closeErr := work.Close(root, id2, "done & merged"); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	id3, err := work.SendTyped(root, "x-agent", "y-agent", "third: a decision", "decision", "decide")
	if err != nil {
		t.Fatalf("SendTyped: %v", err)
	}
	if wakeErr := work.SetWake(root, id3, work.WakeAnnotation{
		Status: "wake-unavailable", AttemptedAt: "2026-07-02T15:04:06Z", Adapter: "launchagent", Error: "no adapter for y-agent",
	}); wakeErr != nil {
		t.Fatalf("SetWake: %v", wakeErr)
	}
	if closeErr := work.Close(root, id3, ""); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}

	// file → Backfill (the Phase-4 importer spine).
	fileItems, err := work.ListAll(root)
	if err != nil {
		t.Fatalf("work.ListAll: %v", err)
	}
	if len(fileItems) != 3 {
		t.Fatalf("fixture items = %d, want 3", len(fileItems))
	}
	items := make([]Item, 0, len(fileItems))
	for _, w := range fileItems {
		items = append(items, adaptWorkItem(w))
	}
	s := newTestStore(t)
	rep, err := s.Backfill(items)
	if err != nil || rep.Inserted != 3 || len(rep.Errors) != 0 {
		t.Fatalf("Backfill = %+v, err=%v", rep, err)
	}

	// Backfill → ExportMarkdown into a fresh root's items/ dir.
	exportRoot := t.TempDir()
	n, err := s.ExportMarkdown(filepath.Join(exportRoot, "items"))
	if err != nil {
		t.Fatalf("ExportMarkdown: %v", err)
	}
	if n != 3 {
		t.Fatalf("ExportMarkdown wrote %d files, want 3", n)
	}

	// Byte-comparable: exported file == original file, for every item.
	for _, w := range fileItems {
		orig, readErr := os.ReadFile(filepath.Join(root, "items", w.ID+".md"))
		if readErr != nil {
			t.Fatalf("read original %s: %v", w.ID, readErr)
		}
		exported, readErr := os.ReadFile(filepath.Join(exportRoot, "items", w.ID+".md"))
		if readErr != nil {
			t.Fatalf("read exported %s: %v", w.ID, readErr)
		}
		if !bytes.Equal(orig, exported) {
			t.Errorf("%s not byte-identical after round-trip:\n--- original ---\n%s\n--- exported ---\n%s", w.ID, orig, exported)
		}
	}

	// Parse-equal: the file router itself reads the export back losslessly.
	reparsed, err := work.ListAll(exportRoot)
	if err != nil {
		t.Fatalf("work.ListAll(export): %v", err)
	}
	if !reflect.DeepEqual(fileItems, reparsed) {
		t.Errorf("re-parsed export differs:\n orig: %+v\n export: %+v", fileItems, reparsed)
	}
}

// TestExportMarkdownErrors covers the empty-store and unwritable-dir paths.
func TestExportMarkdownErrors(t *testing.T) {
	s := newTestStore(t)
	n, err := s.ExportMarkdown(filepath.Join(t.TempDir(), "empty"))
	if err != nil || n != 0 {
		t.Errorf("empty export = (%d, %v), want (0, nil)", n, err)
	}
	// A regular FILE at the target path makes MkdirAll fail.
	blocker := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	if _, err := s.Send("a", "b", "t", "", "i"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, err := s.ExportMarkdown(blocker); err == nil {
		t.Error("export into a file path should error")
	}
}

// TestCloseNilStore covers the nil-receiver guard on Store.Close.
func TestCloseNilStore(t *testing.T) {
	var s *Store
	if err := s.Close(); err != nil {
		t.Errorf("nil Store.Close = %v, want nil", err)
	}
}

func TestSetStateEmptyKey(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetState("", "v"); err == nil {
		t.Error("SetState with empty key should error")
	}
}

// TestMigrateVersioning proves the user_version machinery: a fresh database
// lands on the current schema version, and a database stamped with a FUTURE
// version is refused loudly instead of being half-migrated.
func TestMigrateVersioning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version;`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != len(migrations) {
		t.Errorf("fresh db user_version = %d, want %d", version, len(migrations))
	}
	// Stamp a future version and re-open: must refuse.
	if _, err := s.db.Exec(`PRAGMA user_version = 99;`); err != nil {
		t.Fatalf("stamp future version: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("Open of a future-versioned db should refuse, got nil error")
	}
}
