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
	t.Cleanup(func() { _ = f.Close() })
	return f
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
