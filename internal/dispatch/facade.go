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
	"time"

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

// Inbox lists open items addressed to agent from the canonical files.
func (f *Facade) Inbox(agent string) ([]work.Item, error) {
	return work.ListInbox(f.root, agent)
}

// Show returns one item's full markdown from the canonical file.
func (f *Facade) Show(id string) (string, error) {
	data, err := os.ReadFile(filepath.Join(f.root, "items", id+".md"))
	if err != nil {
		return "", fmt.Errorf("dispatch: read item: %w", err)
	}
	return string(data), nil
}

// CloseItem closes the canonical file item and mirrors the close into the
// store. A store row may legitimately not exist (items sent before the
// facade) or already be closed (idempotent re-close) — both are fine; any
// other mirror failure is reported without undoing the file close.
func (f *Facade) CloseItem(id, result string) error {
	if err := work.Close(f.root, id, result); err != nil {
		return err
	}
	if err := f.store.CloseItem(id, result); err != nil &&
		!errors.Is(err, routerstore.ErrNotFound) &&
		!errors.Is(err, routerstore.ErrAlreadyClosed) {
		return fmt.Errorf("dispatch: item %s closed but store mirror failed: %w", id, err)
	}
	return nil
}

// Wait blocks until an open item is addressed to agent or the timeout
// passes, then returns the canonical file inbox. Items sent through the
// facade wake the waiter event-driven in well under 250ms (PRD /goal #1 —
// the store signals its in-process waiters and the notify FIFO); items
// written by a legacy file-only writer are caught by a bounded 5s re-check,
// which still beats the retired 1s poll loop it replaces at a fifth of the
// wakeups. Returns (nil, nil) on a clean timeout.
func (f *Facade) Wait(ctx context.Context, agent string, timeout time.Duration) ([]work.Item, error) {
	deadline := time.Now().Add(timeout)
	for {
		items, err := work.ListInbox(f.root, agent)
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
