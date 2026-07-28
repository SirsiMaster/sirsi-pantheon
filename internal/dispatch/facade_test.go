package dispatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

func testFacade(t *testing.T) *Facade {
	t.Helper()
	store, err := routerstore.Open(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	f := New(filepath.Join(t.TempDir(), "idea-router"), store)
	writeTestRegistry(t, f.root)
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func writeTestRegistry(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := `{
		"agents": {
			"b": {"type": "test", "command": ["true"], "cwd": "/tmp"},
			"victim": {"type": "test", "command": ["true"], "cwd": "/tmp"},
			"codex-pantheon": {"type": "codex", "command": ["codex"], "cwd": "/tmp"},
			"claude-home": {"type": "claude", "command": ["claude"], "cwd": "/tmp"}
		}
	}`
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(registry), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSendCommitsStoreThenAuditFile: the store row is the authority and the
// items/<id>.md audit view carries the SAME id in the file router's format.
func TestSendCommitsStoreThenAuditFile(t *testing.T) {
	f := testFacade(t)
	res, err := f.Send("claude-pantheon", "codex-pantheon", "review the facade", "review", "please review")
	if err != nil {
		t.Fatal(err)
	}
	if res.Deduped || res.ID == "" || res.AuditPath == "" {
		t.Fatalf("unexpected result: %+v", res)
	}
	data, err := os.ReadFile(res.AuditPath)
	if err != nil {
		t.Fatalf("audit file missing: %v", err)
	}
	for _, want := range []string{`from: "claude-pantheon"`, `to: "codex-pantheon"`, `type: "review"`, "status: open", "## Instructions"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("audit file missing %q:\n%s", want, data)
		}
	}
	// The file router's own reader must see it — one id, both worlds.
	items, err := f.Inbox("codex-pantheon")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != res.ID {
		t.Fatalf("file router does not see the dispatched item: %+v", items)
	}
}

func TestSendRejectsUnregisteredRecipientBeforeDispatch(t *testing.T) {
	f := testFacade(t)

	_, err := f.Send("claude-pantheon", "claude-deck", "lost work", "review", "do not strand this")
	if err == nil {
		t.Fatal("Send to an unregistered recipient must fail")
	}
	if !strings.Contains(err.Error(), `agent "claude-deck" not registered`) {
		t.Fatalf("unexpected error: %v", err)
	}

	inbox, inboxErr := f.Inbox("claude-deck")
	if inboxErr != nil {
		t.Fatal(inboxErr)
	}
	if len(inbox) != 0 {
		t.Fatalf("unregistered recipient send wrote work: %+v", inbox)
	}
	all, allErr := f.ListAll()
	if allErr != nil {
		t.Fatal(allErr)
	}
	if len(all) != 0 {
		t.Fatalf("rejected send must not create any router item, got %+v", all)
	}
}

func TestInboxFailsClosedWhenStoreErrorsAndNoFileItems(t *testing.T) {
	f := testFacade(t)
	res, err := f.Send("claude-pantheon", "codex-pantheon", "store-only work", "review", "do not hide")
	if err != nil {
		t.Fatal(err)
	}
	if removeErr := os.Remove(res.AuditPath); removeErr != nil {
		t.Fatal(removeErr)
	}
	if closeErr := f.store.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	items, err := f.Inbox("codex-pantheon")
	if err == nil {
		t.Fatalf("Inbox returned success with no file corroboration and an unreadable store: %+v", items)
	}
	if !strings.Contains(err.Error(), "no file items corroborate an empty inbox") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestStoreWakeCutover exercises the full post-cutover steady state: with
// SIRSI_ROUTER_STORE_WAKE=1, Send writes NO items/<id>.md (the store row is the
// record), yet Show/Inbox/Close all work store-only. This is what makes it safe
// to stop writing files — the whole read/close path is store-capable.
func TestStoreWakeCutover(t *testing.T) {
	t.Setenv(routercfg.StoreWakeEnv, "1")
	f := testFacade(t)

	res, err := f.Send("a", "b", "cutover item", "review", "do it")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	// No audit file written, and none on disk.
	if res.AuditPath != "" {
		t.Fatalf("StoreWake Send should not write an audit file, got %q", res.AuditPath)
	}
	if _, statErr := os.Stat(filepath.Join(f.root, "items", res.ID+".md")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no items/%s.md on disk, stat err = %v", res.ID, statErr)
	}
	// Inbox surfaces it (from the store).
	inbox, err := f.Inbox("b")
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(inbox) != 1 || inbox[0].ID != res.ID {
		t.Fatalf("Inbox store-only = %+v, want the one item", inbox)
	}
	// Show renders from the store (no file).
	md, err := f.Show(res.ID)
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	for _, want := range []string{`from: "a"`, `to: "b"`, "## Instructions", "do it"} {
		if !strings.Contains(md, want) {
			t.Fatalf("store-rendered Show missing %q:\n%s", want, md)
		}
	}
	// Close lands in the store even with no file.
	if err := f.CloseItem(res.ID, "done"); err != nil {
		t.Fatalf("CloseItem store-only: %v", err)
	}
	if remaining, _ := f.Inbox("b"); len(remaining) != 0 {
		t.Fatalf("after close, inbox = %d, want 0", len(remaining))
	}
	// Closing a genuinely unknown id still errors (exists nowhere).
	if err := f.CloseItem("20260101-000000-x-y-nope", "x"); err == nil {
		t.Fatal("CloseItem(unknown) should error, got nil")
	}
}

// TestSetWakeRoutesToStoreForFilelessItem covers the wake-authority gap the
// review flagged: post-cutover WakePass must be able to annotate a store-only
// item (no file), or it loses idempotency and re-wakes every pass. SetWake must
// route to the store when there is no file, and that annotation must read back.
func TestSetWakeRoutesToStoreForFilelessItem(t *testing.T) {
	t.Setenv(routercfg.StoreWakeEnv, "1")
	f := testFacade(t)

	res, err := f.Send("a", "b", "fileless wake", "review", "do it")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	// No file was written (cutover mode) — SetWake must still land, on the store.
	if werr := f.SetWake(res.ID, work.WakeAnnotation{Status: "wake-attempted", AttemptedAt: "2026-07-10T00:00:00Z", Adapter: "launchagent"}); werr != nil {
		t.Fatalf("SetWake store-only: %v", werr)
	}
	inbox, err := f.Inbox("b")
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("inbox = %d, want 1", len(inbox))
	}
	if got := inbox[0].WakeStatus; got != "wake-attempted" {
		t.Fatalf("WakeStatus = %q, want wake-attempted (annotation did not persist to the store)", got)
	}
	if got := inbox[0].WakeAdapter; got != "launchagent" {
		t.Fatalf("WakeAdapter = %q, want launchagent", got)
	}
	// An unknown id has neither file nor row — the store reports not-found.
	if werr := f.SetWake("20260101-000000-x-y-nope", work.WakeAnnotation{Status: "armed"}); werr == nil {
		t.Fatal("SetWake(unknown) should error, got nil")
	}
}

// TestWaitDetectsStoreOnlyItem proves the wake path survives the ADR-036
// file-write cutover: with the store row present but NO items/<id>.md audit
// file (the steady state after file writes stop), Wait must still surface the
// work from the store union — not block until timeout waiting for a file that
// will never be written. This is what lets a `/loop` watcher move off the
// items/ directory-watch onto `sirsi router wait` (the store FIFO).
func TestWaitDetectsStoreOnlyItem(t *testing.T) {
	f := testFacade(t)
	res, err := f.Send("a", "b", "store-only wake", "", "body")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate post-cutover: the store holds the dispatch, the audit file is gone.
	if res.AuditPath == "" {
		t.Fatalf("expected an audit file to remove: %+v", res)
	}
	if rmErr := os.Remove(res.AuditPath); rmErr != nil {
		t.Fatalf("remove audit file: %v", rmErr)
	}
	// The file inbox is now empty — the pre-fix Wait (work.ListInbox) would hang.
	files, err := work.ListInbox(f.root, "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("expected empty file inbox after audit removal, got %d", len(files))
	}
	// Wait returns the item from the store union, well within the timeout.
	items, err := f.Wait(context.Background(), "b", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != res.ID {
		t.Fatalf("Wait did not surface the store-only item: %+v", items)
	}
}

// TestSendRefusalDispatchesNothing: a guard refusal (over quota) leaves NO
// new audit file — no store row, no dispatch (§2b axiom 8).
func TestSendRefusalDispatchesNothing(t *testing.T) {
	f := testFacade(t)
	oldQ := routerstore.MaxSendsPerSenderPerWindow
	routerstore.MaxSendsPerSenderPerWindow = 2
	t.Cleanup(func() { routerstore.MaxSendsPerSenderPerWindow = oldQ })

	for i := 0; i < 2; i++ {
		if _, err := f.Send("flooder", "victim", "spam "+strings.Repeat("x", i+1), "", "body"); err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.Send("flooder", "victim", "spam three", "", "body")
	if !errors.Is(err, routerstore.ErrOverQuota) {
		t.Fatalf("third send must be refused over quota, got %v", err)
	}
	items, err := f.Inbox("victim")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("refused send must not create an audit item: %d items", len(items))
	}
}

// TestSendDedupesRephrasedRetry: same subject key → one item, deduped result.
func TestSendDedupesRephrasedRetry(t *testing.T) {
	f := testFacade(t)
	r1, err := f.Send("a", "b", "deploy failed #1", "", "x")
	if err != nil {
		t.Fatal(err)
	}
	// Rephrased title, same logical send via explicit subject key path:
	// the facade passes titles through, so same title = same key here.
	r2, err := f.Send("a", "b", "deploy failed #1", "", "x")
	if err != nil {
		t.Fatal(err)
	}
	if !r2.Deduped || r2.ID != r1.ID {
		t.Fatalf("retry must dedupe to the same item: %+v vs %+v", r1, r2)
	}
	items, _ := f.Inbox("b")
	if len(items) != 1 {
		t.Fatalf("dedupe must not append: %d items", len(items))
	}
}

// TestCloseMirrorsToStore: closing the file item closes the store row too,
// and re-closing is a clean error from the file layer (unchanged semantics).
func TestCloseMirrorsToStore(t *testing.T) {
	f := testFacade(t)
	res, err := f.Send("a", "b", "close me", "", "x")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.CloseItem(res.ID, "done"); err != nil {
		t.Fatal(err)
	}
	items, _ := f.Inbox("b")
	if len(items) != 0 {
		t.Fatalf("closed item still in inbox: %+v", items)
	}
	// Audit file shows the close; store row is closed too.
	text, err := f.Show(res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "status: closed") || !strings.Contains(text, "## Result") {
		t.Fatalf("audit file not closed:\n%s", text)
	}
}

// TestClosePreFacadeItemStillWorks: an item written by the legacy file path
// (no store row) closes fine — the store mirror is best-effort by design.
func TestClosePreFacadeItemStillWorks(t *testing.T) {
	f := testFacade(t)
	if err := work.EnsureRoot(f.root); err != nil {
		t.Fatal(err)
	}
	id, err := work.SendTyped(f.root, "old", "b", "pre-facade item", "", "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.CloseItem(id, "handled"); err != nil {
		t.Fatalf("closing a pre-store item must succeed: %v", err)
	}
}

// TestOpenCreatesStoreDirOnFreshHome: a brand-new HOME (CI runners, first
// run on a new machine) has no ~/.sirsi — Open must create the store's
// parent directory instead of failing SQLITE_CANTOPEN (the CI-only
// TestRouterPullModelRoundtrip failure this reproduces).
func TestOpenCreatesStoreDirOnFreshHome(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "nested", "never-made", "router.db"))
	f, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open on a fresh home must create the store dir: %v", err)
	}
	defer func() { _ = f.Close() }()
	writeTestRegistry(t, f.root)
	if _, err := f.Send("a", "b", "first ever send", "", "x"); err != nil {
		t.Fatalf("send on fresh store: %v", err)
	}
}

// TestPhantomOpenStoreRowHiddenAndHealable: a file closed file-only by a
// pre-facade binary leaves a stale open store row. Inbox must not resurface
// it (phantom-open), and CloseItem must heal the store mirror instead of
// failing with "already closed" (the A28 divergence, 2026-07-22).
func TestPhantomOpenStoreRowHiddenAndHealable(t *testing.T) {
	f := testFacade(t)
	res, err := f.Send("a", "b", "diverge me", "", "x")
	if err != nil {
		t.Fatal(err)
	}
	// Close the FILE only (what a pre-facade binary did) — store row stays open.
	if err = work.Close(f.root, res.ID, "file-only close"); err != nil {
		t.Fatal(err)
	}
	items, err := f.Inbox("b")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("phantom-open store row resurfaced a closed item: %+v", items)
	}
	// Re-close through the facade: must heal the store, not error.
	if err = f.CloseItem(res.ID, "heal"); err != nil {
		t.Fatalf("CloseItem on already-closed file must heal the store: %v", err)
	}
	rows, err := f.store.Inbox("b")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("store row still open after heal: %+v", rows)
	}
}

// TestGetReadsStoreOnlyItem: Get must resolve items that exist only as store
// rows — the post-cutover steady state that broke the file-based respond
// wrapper ("item not found" for every new item).
func TestGetReadsStoreOnlyItem(t *testing.T) {
	t.Setenv(routercfg.StoreWakeEnv, "1") // store-only: Send writes no audit file
	f := testFacade(t)
	res, err := f.Send("claude-finalwishes", "claude-home", "bind PR #105", "review", "please bind")
	if err != nil {
		t.Fatal(err)
	}
	if res.AuditPath != "" {
		t.Fatalf("cutover-on Send should not write an audit file, got %q", res.AuditPath)
	}
	it, err := f.Get(res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if it.From != "claude-finalwishes" || it.To != "claude-home" || it.Title != "bind PR #105" || it.Status != "open" {
		t.Fatalf("unexpected item: %+v", it)
	}
	if _, err := f.Get("20260101-000000-nobody-nobody-missing"); err == nil {
		t.Fatal("Get of a nonexistent id must error")
	}
}

// TestStoreWakeIgnoresFrozenOpenFile is the phantom-resurrection regression
// (P0, 2026-07-24). Post-cutover `close` writes the STORE only, so any
// items/<id>.md left over from before the cutover stays frozen at
// `status: open` forever. While the reads unioned files with the store, that
// frozen copy outranked its own closed store row: 34 long-closed items came
// back as open on this host, `router status` reported 47 open against the
// store's 9, and `pull` handed agents work finished five weeks earlier. With
// the cutover live the store is the sole authority — the file is ignored.
func TestStoreWakeIgnoresFrozenOpenFile(t *testing.T) {
	t.Setenv(routercfg.StoreWakeEnv, "0") // send pre-cutover so the audit FILE exists
	f := testFacade(t)

	res, err := f.Send("a", "b", "phantom item", "review", "do it")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	filePath := filepath.Join(f.root, "items", res.ID+".md")
	if _, statErr := os.Stat(filePath); statErr != nil {
		t.Fatalf("pre-cutover Send should write the audit file: %v", statErr)
	}

	// Close it from a DIFFERENT router root over the SAME store — exactly how it
	// happens on the host, where a close run from a git worktree finds no file at
	// its own root and updates the store alone. The original file is never
	// touched and stays frozen at `status: open`.
	t.Setenv(routercfg.StoreWakeEnv, "1")
	other := New(t.TempDir(), f.store)
	if closeErr := other.CloseItem(res.ID, "done"); closeErr != nil {
		t.Fatalf("CloseItem from another root: %v", closeErr)
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}

	if inbox, inboxErr := f.Inbox("b"); inboxErr != nil || len(inbox) != 0 {
		t.Errorf("Inbox = %d items (err %v), want 0 — the frozen file must not resurrect a closed item", len(inbox), inboxErr)
	}
	all, err := f.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	for _, it := range all {
		if it.ID == res.ID && it.Status != "closed" {
			t.Errorf("ListAll reports %s as %q, want closed (store is the authority)", it.ID, it.Status)
		}
	}
	if got, err := f.Get(res.ID); err != nil || got.Status != "closed" {
		t.Errorf("Get status = %q (err %v), want closed", got.Status, err)
	}

	// Guard the premise: if the file ever stops being frozen-open, this test is
	// passing for the wrong reason and must be re-derived, not deleted.
	if !strings.Contains(string(raw), "status: open") {
		t.Fatal("premise broke: the audit file is no longer frozen at `status: open`")
	}
}

// TestCutoverReadsAreExactlyTheStore is codex-pantheon's requested verification
// of the losslessness premise (#315 review). The cutover marker asserts that the
// Phase-4 migration COMPLETED — every file has a store row. This fixture holds
// all three ways the two sources can diverge and proves post-cutover reads
// return exactly the store's ids and statuses, with no reconciliation of
// file-only rows back into the store during a read.
func TestCutoverReadsAreExactlyTheStore(t *testing.T) {
	t.Setenv(routercfg.StoreWakeEnv, "0") // seed pre-cutover so audit files exist
	f := testFacade(t)
	other := New(t.TempDir(), f.store) // closes store-side only (the worktree split)

	// (a) store-closed / file-open — the phantom that started this.
	phantom, err := f.Send("a", "b", "phantom", "review", "x")
	if err != nil {
		t.Fatal(err)
	}
	// (b) store-open / file-closed — the inverse: a file closed by a pre-facade
	//     binary while its store row stayed open.
	inverse, err := f.Send("a", "b", "inverse", "review", "x")
	if err != nil {
		t.Fatal(err)
	}
	// (c) file-only open — no store row at all (a pre-importer leftover).
	const orphan = "20260101-000000-a-b-file-only-open"
	orphanBody := "---\nfrom: \"a\"\nto: \"b\"\ntitle: \"file only\"\nstatus: open\nopened: 2026-01-01T00:00:00Z\n---\n\n## Instructions\n\nx\n"
	if werr := os.WriteFile(filepath.Join(f.root, "items", orphan+".md"), []byte(orphanBody), 0o644); werr != nil {
		t.Fatal(werr)
	}

	if cerr := other.CloseItem(phantom.ID, "done"); cerr != nil { // store only
		t.Fatal(cerr)
	}
	if cerr := work.Close(f.root, inverse.ID, "done"); cerr != nil { // file only
		t.Fatal(cerr)
	}

	// Pre-cutover: the union is unchanged — the file wins where it exists, and
	// the file-only orphan is visible.
	pre, err := f.Inbox("b")
	if err != nil {
		t.Fatal(err)
	}
	preIDs := map[string]bool{}
	for _, it := range pre {
		preIDs[it.ID] = true
	}
	if !preIDs[phantom.ID] || !preIDs[orphan] || preIDs[inverse.ID] {
		t.Errorf("pre-cutover inbox = %v; want the file-open phantom and the file-only orphan, not the file-closed inverse", preIDs)
	}

	// Post-cutover: exactly the store's open rows. The phantom is gone (store
	// says closed), the inverse IS present (store says open), and the file-only
	// orphan is invisible — and stays absent from the store: no read reconciles.
	t.Setenv(routercfg.StoreWakeEnv, "1")
	post, err := f.Inbox("b")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(post))
	for _, it := range post {
		got = append(got, it.ID)
	}
	if len(got) != 1 || got[0] != inverse.ID {
		t.Errorf("post-cutover inbox = %v, want exactly [%s] (the one row the store calls open)", got, inverse.ID)
	}
	if _, serr := f.store.Get(orphan); serr == nil {
		t.Error("a read reconciled the file-only row into the store — reads must never write")
	}
}
