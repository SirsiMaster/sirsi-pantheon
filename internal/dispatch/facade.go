// Package dispatch is the ONE send/read facade over the router — Router v2
// Phase 3 (PRD ROUTER_V2_DURABLE_DISPATCH; ADR-035 axiom 1: one executable
// dispatch authority).
//
// Before this package, the two item-emitting surfaces had drifted into two
// implementations: the CLI (`sirsi router send`) wrote items/*.md via
// internal/work, while the MCP handler (router_submit) wrote to the
// proposals//reviews//decisions/ directories + state.json inboxes — the
// pre-ADR-024 model that was retired ("ONE inbox: items/ only"). Both now
// call THIS facade:
//
//   - Writes commit to the routerstore FIRST — idempotency key, per-sender
//     quotas, and circuit breakers run BEFORE anything is dispatched (§2b
//     axioms 4–7). No store row, no dispatch. Over-quota updates a throttle
//     singleton and refuses; it never appends.
//   - The items/<id>.md audit view is then dual-written byte-identically to
//     the file router's own format (§2b axiom 8). A failed audit write
//     degrades loudly but does not undo the dispatch.
//   - Reads (Inbox/Show) stay on the canonical files until the Phase-4
//     migration/cutover, so no pre-store item is ever invisible.
//
// Register/heartbeat stay with the mature thread registry (internal/router);
// folding them in is Phase-4 scope, deliberately not duplicated here (Rule 0).
package dispatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// Facade is the single dispatch entry point shared by the CLI verbs and the
// MCP router_* handlers.
type Facade struct {
	store *routerstore.Store
	root  string // <repo>/.agents/idea-router
}

// Open resolves the repo's router root and the durable store
// (~/.sirsi/router.db — outside any git tree, PRD /goal #2).
// SIRSI_ROUTER_DB overrides the store path — REQUIRED for tests and sandboxes
// so a test send can never write a row into the live store (the "test
// binaries reaching the user" storm class, PR #151).
func Open(repoRoot string) (*Facade, error) {
	dbPath := os.Getenv("SIRSI_ROUTER_DB")
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("dispatch: resolve home: %w", err)
		}
		dbPath = filepath.Join(home, ".sirsi", "router.db")
	}
	// A fresh HOME has no ~/.sirsi yet — SQLite cannot create the db file in a
	// missing directory (SQLITE_CANTOPEN, surfaced as CI's "unable to open
	// database file"). Create the parent first.
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("dispatch: create store dir: %w", err)
		}
	}
	store, err := routerstore.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return New(filepath.Join(repoRoot, ".agents", "idea-router"), store), nil
}

// New builds a facade over an explicit root and store (test injection).
func New(root string, store *routerstore.Store) *Facade {
	return &Facade{store: store, root: root}
}

// Close releases the store handle.
func (f *Facade) Close() error { return f.store.Close() }

// SendResult reports one guarded dispatch.
type SendResult struct {
	ID        string
	Deduped   bool   // an idempotent duplicate — the existing item was returned
	AuditPath string // the items/<id>.md audit view (empty if its write failed)
}

// Send is THE write path (§2b axiom 4). Guards run before dispatch:
// idempotency (a retried/rephrased duplicate returns the existing id),
// per-sender quotas (ErrOverQuota — throttle singleton updated, nothing
// appended), and circuit breakers (ErrBreakerOpen). Success means the store
// row exists; the markdown audit view is then dual-written with the same id.
func (f *Facade) Send(from, to, title, msgType, instructions string) (SendResult, error) {
	if err := work.EnsureRoot(f.root); err != nil {
		return SendResult{}, fmt.Errorf("dispatch: ensure root: %w", err)
	}
	id, deduped, err := f.store.SendGuarded(routerstore.SendReq{
		From: from, To: to, Title: title, Type: msgType, Instructions: instructions,
	})
	if err != nil {
		return SendResult{}, err // refused: no store row, no dispatch (§2b axiom 8)
	}
	// After the cutover the store row IS the record — no items/<id>.md audit view
	// is written, consumers wake via `router wait`, and Show/Close read the store.
	if routercfg.StoreWake() {
		return SendResult{ID: id, Deduped: deduped}, nil
	}
	path, err := f.store.ExportItem(filepath.Join(f.root, "items"), id)
	if err != nil {
		// The dispatch HAPPENED (store row committed); the audit view failed.
		// Loud, not fatal — the sweep flags store-only items, and ExportMarkdown
		// can regenerate every audit file from the store.
		return SendResult{ID: id, Deduped: deduped},
			fmt.Errorf("dispatch: item %s dispatched but audit file write failed: %w", id, err)
	}
	return SendResult{ID: id, Deduped: deduped, AuditPath: path}, nil
}

// Inbox lists open items addressed to agent — the Phase-4 dual-read window:
// the canonical files (which legacy writers still produce) merged with the
// store's open rows (the dispatch authority), union by id. A store row whose
// audit file failed to write — or predates a file that was mutated by hand —
// is therefore never invisible (§2b axiom 8: a stale or missing file cannot
// change lifecycle). File items keep their exact work.Item shape; store-only
// rows are adapted into it.
func (f *Facade) Inbox(agent string) ([]work.Item, error) {
	items, err := work.ListInbox(f.root, agent)
	if err != nil {
		return nil, err
	}
	rows, err := f.store.Inbox(agent)
	if err != nil {
		// The store is additive infrastructure during the dual-read window —
		// a broken store must not strand the canonical file inbox.
		return items, nil //nolint:nilerr // deliberate: files stay readable
	}
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		seen[it.ID] = true
	}
	for _, r := range rows {
		if seen[r.ID] {
			continue
		}
		items = append(items, work.Item{
			ID: r.ID, From: r.From, To: r.To, Title: r.Title, Type: r.Type,
			Status: r.Status, Opened: r.Opened, Closed: r.Closed,
			Instructions: r.Instructions, Result: r.Result,
			WakeStatus: r.WakeStatus, WakeAttemptedAt: r.WakeAttemptedAt,
			WakeAdapter: r.WakeAdapter, WakeError: r.WakeError,
		})
	}
	sortItemsByID(items)
	return items, nil
}

// ListAll returns every item (open AND closed) as the dual-read union of the
// file router and the store, deduped by id. This is the read path for the
// operator summary (`router status`) so it reports accurately after the
// cutover, when open items exist only as store rows.
func (f *Facade) ListAll(agent string) ([]work.Item, error) {
	items, err := work.ListAll(f.root)
	if err != nil {
		return nil, err
	}
	rows, err := f.store.ListAll()
	if err != nil {
		return items, nil //nolint:nilerr // deliberate: a broken store must not blind the file view
	}
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		seen[it.ID] = true
	}
	for _, r := range rows {
		if seen[r.ID] {
			continue
		}
		items = append(items, work.Item{
			ID: r.ID, From: r.From, To: r.To, Title: r.Title, Type: r.Type,
			Status: r.Status, Opened: r.Opened, Closed: r.Closed,
			Instructions: r.Instructions, Result: r.Result,
			WakeStatus: r.WakeStatus, WakeAttemptedAt: r.WakeAttemptedAt,
			WakeAdapter: r.WakeAdapter, WakeError: r.WakeError,
		})
	}
	sortItemsByID(items)
	return items, nil
}

// sortItemsByID keeps the merged inbox in the file router's oldest-first
// order (ids sort chronologically by construction).
func sortItemsByID(items []work.Item) {
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
}

// Show returns one item's full markdown. It prefers the canonical file (a
// hand-annotated audit view is authoritative for display), and falls back to
// rendering the store row when no file exists — the post-cutover steady state
// where Send no longer writes items/<id>.md, and also any store-only item whose
// audit write once failed (§2b axiom 8: a missing file never hides work).
func (f *Facade) Show(id string) (string, error) {
	data, err := os.ReadFile(filepath.Join(f.root, "items", id+".md"))
	if err == nil {
		return string(data), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("dispatch: read item: %w", err)
	}
	rendered, rerr := f.store.Render(id)
	if rerr != nil {
		if errors.Is(rerr, routerstore.ErrNotFound) {
			return "", fmt.Errorf("dispatch: item %s not found (no file, no store row)", id)
		}
		return "", fmt.Errorf("dispatch: render item from store: %w", rerr)
	}
	return rendered, nil
}

// CloseItem closes the item in both worlds. It closes the canonical file when
// one exists, and closes the store row. Post-cutover there is no file, so the
// close lands in the store alone; pre-facade items may have a file but no store
// row. The only true error is an id that exists in NEITHER place (or a real
// store failure) — an already-closed row is idempotent success.
func (f *Facade) CloseItem(id, result string) error {
	fileExists := false
	if _, statErr := os.Stat(filepath.Join(f.root, "items", id+".md")); statErr == nil {
		fileExists = true
	}
	if fileExists {
		if err := work.Close(f.root, id, result); err != nil {
			return err
		}
	}
	switch storeErr := f.store.CloseItem(id, result); {
	case storeErr == nil, errors.Is(storeErr, routerstore.ErrAlreadyClosed):
		return nil
	case errors.Is(storeErr, routerstore.ErrNotFound):
		// No store row is fine only if we actually closed a file; otherwise the
		// id exists nowhere.
		if fileExists {
			return nil
		}
		return fmt.Errorf("dispatch: item %s not found (no file, no store row)", id)
	default:
		return fmt.Errorf("dispatch: item %s closed but store mirror failed: %w", id, storeErr)
	}
}

// Wait blocks until an open item is addressed to agent or the timeout
// passes, then returns the dual-read inbox (store rows ∪ file items). Items
// sent through the facade wake the waiter event-driven in well under 250ms
// (PRD /goal #1 — the store signals its in-process waiters and the notify
// FIFO); items written by a legacy file-only writer are caught by a bounded 5s
// re-check, which still beats the retired 1s poll loop it replaces at a fifth
// of the wakeups. Returns (nil, nil) on a clean timeout.
//
// The work-check is the union (f.Inbox), NOT the file inbox alone: a store-only
// send — the steady state after the ADR-036 file-write cutover — must wake the
// waiter even when no items/<id>.md exists. Using Inbox here is what lets a
// `/loop` watcher move off the items/ directory-watch onto the store event
// wake without stranding at cutover.
func (f *Facade) Wait(ctx context.Context, agent string, timeout time.Duration) ([]work.Item, error) {
	deadline := time.Now().Add(timeout)
	for {
		items, err := f.Inbox(agent)
		if err != nil {
			return nil, err
		}
		if len(items) > 0 {
			return items, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, nil
		}
		slice := 5 * time.Second
		if remaining < slice {
			slice = remaining
		}
		if _, err := f.store.Wait(ctx, agent, slice); err != nil {
			return nil, err
		}
	}
}
