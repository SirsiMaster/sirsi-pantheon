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
//   - Reads are dual-source ONLY until the cutover (routercfg.StoreWake): with
//     the cutover live the store is the sole authority and the file leg is
//     dropped entirely, because `close` writes the store alone and a frozen
//     items/<id>.md would otherwise resurrect closed work forever.
//
// Register/heartbeat stay with the mature thread registry (internal/router);
// folding them in is Phase-4 scope, deliberately not duplicated here (Rule 0).
package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// Store exposes the shared durable store to sibling read models. Facade.Close
// remains the owner of the handle; callers must not close it directly.
func (f *Facade) Store() *routerstore.Store { return f.store }

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

// OpenRoot is Open for callers that already hold the router root
// (<repo>/.agents/idea-router) rather than the repo root. Same store
// resolution; it just skips the join. Keeps callers from reconstructing the
// repo root with filepath.Dir(filepath.Dir(...)), which is wrong for any root
// that is not literally two levels below a repo (every t.TempDir(), for one).
func OpenRoot(routerRoot string) (*Facade, error) {
	f, err := Open(routerRoot) // resolves the store; root is overwritten below
	if err != nil {
		return nil, err
	}
	f.root = routerRoot
	return f, nil
}

// New builds a facade over an explicit root and store (test injection).
func New(root string, store *routerstore.Store) *Facade {
	return &Facade{store: store, root: root}
}

// Close releases the store handle.
func (f *Facade) Close() error { return f.store.Close() }

// AddTask validates both the owning identity and the normalized responsible
// party before the store can mutate. "self" is syntax, never an identity.
func (f *Facade) AddTask(t routerstore.Task) error {
	if err := f.ValidateAgent("task owner", t.Agent); err != nil {
		return err
	}
	if strings.TrimSpace(t.ResponsibleParty) == "" || t.ResponsibleParty == "self" {
		t.ResponsibleParty = t.Agent
	}
	if err := f.ValidateAgent("responsible party", t.ResponsibleParty); err != nil {
		return err
	}
	return f.store.AddTask(t)
}

// UpdateTask validates the final task state before mutation, including an
// unchanged legacy responsible party. This makes remediation explicit rather
// than silently carrying an undeclared identity into a new write.
func (f *Facade) UpdateTask(agent, taskID string, u routerstore.TaskUpdate) (routerstore.Task, error) {
	if err := f.ValidateAgent("task owner", agent); err != nil {
		return routerstore.Task{}, err
	}
	current, err := f.store.GetTask(agent, taskID)
	if err != nil {
		return routerstore.Task{}, err
	}
	responsible := current.ResponsibleParty
	if u.ResponsibleParty != "" {
		responsible = u.ResponsibleParty
	}
	if responsible == "self" {
		responsible = agent
		u.ResponsibleParty = agent
	}
	if err := f.ValidateAgent("responsible party", responsible); err != nil {
		return routerstore.Task{}, err
	}
	return f.store.UpdateTask(agent, taskID, u)
}

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
	if err := f.ValidateAgent("sender", from); err != nil {
		return SendResult{}, err
	}
	if err := f.ValidateAgent("recipient", to); err != nil {
		return SendResult{}, err
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

// ValidateAgent enforces ADR-054 declared identity at the shared facade
// boundary. Eligibility is registry declaration, never live-thread state.
func (f *Facade) ValidateAgent(party, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("dispatch: %s is required", party)
	}
	registryPath := filepath.Join(f.root, "agents.json")
	if id == "user" {
		return fmt.Errorf("dispatch: invalid %s %q: user is a legacy alias; use declared identity %q for new writes", party, id, "owner")
	}
	data, err := os.ReadFile(registryPath)
	if err != nil {
		return fmt.Errorf("dispatch: validate %s: read agents.json: %w", party, err)
	}
	var reg struct {
		Agents map[string]struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Repo       string `json:"repo"`
			Cwd        string `json:"cwd"`
			Workstream string `json:"workstream"`
			Wake       struct {
				Mechanism string `json:"mechanism"`
			} `json:"wake"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return fmt.Errorf("dispatch: validate %s: parse agents.json: %w", party, err)
	}
	cfg, ok := reg.Agents[id]
	if !ok {
		return undeclaredAgentError(party, id, registryPath)
	}
	// Existing registry rows call the repo root `cwd`; accept that established
	// spelling while new registrations migrate to the explicit `repo` field.
	if strings.TrimSpace(cfg.ID) == "" || strings.TrimSpace(cfg.Type) == "" ||
		(strings.TrimSpace(cfg.Repo) == "" && strings.TrimSpace(cfg.Cwd) == "") ||
		strings.TrimSpace(cfg.Workstream) == "" || strings.TrimSpace(cfg.Wake.Mechanism) == "" {
		return undeclaredAgentError(party, id, registryPath)
	}
	switch strings.TrimSpace(cfg.Wake.Mechanism) {
	case "launchagent", "session-message", "routine", "none", "owner-surface":
	default:
		return fmt.Errorf("dispatch: invalid %s %q in %s: wake.mechanism %q is outside the ADR-054 wake matrix (launchagent, session-message, routine, none, owner-surface)", party, id, registryPath, cfg.Wake.Mechanism)
	}
	return nil
}

func undeclaredAgentError(party, id, registryPath string) error {
	return fmt.Errorf("dispatch: invalid %s %q: identity is not fully declared in %s; required fields: id, type, repo (legacy cwd accepted), workstream, wake.mechanism", party, id, registryPath)
}

// Inbox lists open items addressed to agent. Post-cutover it is the store's
// open rows, full stop. Pre-cutover it is the Phase-4 dual-read window: the
// canonical files (which legacy writers still produce) merged with the store's
// open rows, union by id, so a store row whose audit file failed to write is
// never invisible (§2b axiom 8: a stale or missing file cannot change
// lifecycle).
func (f *Facade) Inbox(agent string) ([]work.Item, error) {
	if strings.TrimSpace(agent) != "" {
		if err := f.ValidateAgent("acting agent", agent); err != nil {
			return nil, err
		}
	}
	// Post-cutover the store is the SOLE authority and the file leg is a frozen
	// legacy copy: `close` writes the store only, so an items/<id>.md left at
	// `status: open` outlives its own closure and resurrects as a phantom on
	// every read — forever, fleet-wide, waking agents onto work finished weeks
	// ago. Reading store-only is what makes the cutover actually a cutover.
	if routercfg.StoreWake() {
		rows, err := f.store.Inbox(agent)
		if err != nil {
			return nil, fmt.Errorf("store inbox unavailable (store is the cutover authority): %w", err)
		}
		items := make([]work.Item, 0, len(rows))
		for _, r := range rows {
			items = append(items, itemFromRow(r))
		}
		sortItemsByID(items)
		return items, nil
	}
	items, err := work.ListInbox(f.root, agent)
	if err != nil {
		return nil, err
	}
	rows, err := f.store.Inbox(agent)
	if err != nil {
		// Pre-cutover only (the cutover path returned above): the store is merely
		// additive here, so a broken store must not strand the canonical file
		// inbox — degrade to files rather than fail the surface.
		return items, nil //nolint:nilerr // pre-cutover: files stay readable
	}
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		seen[it.ID] = true
	}
	for _, r := range rows {
		if seen[r.ID] {
			continue
		}
		// seen holds only file-OPEN items, so a row missing from it is either
		// store-only (include) or a stale open mirror of a file closed file-only
		// by a pre-facade binary (skip — the file is canon; a phantom-open row
		// must not resurface a closed item to pull/wait forever).
		if it, getErr := work.Get(f.root, r.ID); getErr == nil && it.Status != "open" {
			continue
		}
		items = append(items, itemFromRow(r))
	}
	sortItemsByID(items)
	return items, nil
}

// ListAll returns every item (open AND closed) as the dual-read union of the
// file router and the store, deduped by id. This is the read path for whole-
// fabric summaries (`router status`, the menubar router signal) so they report
// accurately after the cutover, when open items exist only as store rows.
func (f *Facade) ListAll() ([]work.Item, error) {
	// Same authority rule as Inbox: post-cutover the store row IS the record, so
	// unioning the frozen files inflates every whole-fabric count (open AND
	// closed) with items the store already resolved.
	if routercfg.StoreWake() {
		rows, err := f.store.ListAll()
		if err != nil {
			return nil, fmt.Errorf("store list unavailable (store is the cutover authority): %w", err)
		}
		items := make([]work.Item, 0, len(rows))
		for _, r := range rows {
			items = append(items, itemFromRow(r))
		}
		sortItemsByID(items)
		return items, nil
	}
	items, err := work.ListAll(f.root)
	if err != nil {
		return nil, err
	}
	rows, err := f.store.ListAll()
	if err != nil {
		// Pre-cutover only (the cutover path returned above): the store is merely
		// additive here, so a broken store must not blind the caller — degrade to
		// the canonical files rather than fail the surface.
		return items, nil //nolint:nilerr // pre-cutover: files stay readable
	}
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		seen[it.ID] = true
	}
	for _, r := range rows {
		if seen[r.ID] {
			continue
		}
		items = append(items, itemFromRow(r))
	}
	sortItemsByID(items)
	return items, nil
}

// SetWake records a wake-pass annotation on an item, routing to whichever
// store holds it: the canonical file when one exists (legacy items), else the
// store row (the post-cutover authority, where a store-only item has no file to
// annotate). This lets WakePass annotate — and therefore stay idempotent about —
// items regardless of the cutover state.
func (f *Facade) SetWake(id string, ann work.WakeAnnotation) error {
	// Post-cutover the annotation belongs on the store row. Writing it to the file
	// instead is worse than a stale read: it makes the wake pass an AMPLIFIER of
	// the phantom class, recording wake state onto items closed months ago
	// (claude-home item 20260724-232706 — two claude-ask-eliot rows closed since
	// June were being annotated on every conduit run).
	if routercfg.StoreWake() {
		return f.store.SetWake(id, ann.Status, ann.AttemptedAt, ann.Adapter, ann.Error)
	}
	if _, err := os.Stat(filepath.Join(f.root, "items", id+".md")); err == nil {
		return work.SetWake(f.root, id, ann)
	}
	return f.store.SetWake(id, ann.Status, ann.AttemptedAt, ann.Adapter, ann.Error)
}

// SetBlockedBy replaces one item's dependency edge on the authoritative store,
// with the legacy file kept in sync during the pre-cutover dual-write window.
func (f *Facade) SetBlockedBy(id, blockedBy string) error {
	if routercfg.StoreWake() {
		return f.store.SetBlockedBy(id, blockedBy)
	}
	if err := work.SetBlockedBy(f.root, id, blockedBy); err == nil {
		_ = f.store.SetBlockedBy(id, blockedBy)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("dispatch: set blocked_by on canonical item %s: %w", id, err)
	}
	return f.store.SetBlockedBy(id, blockedBy)
}

// itemFromRow adapts a store row into the file router's work.Item shape — the
// one conversion every read path shares, so a new column can never be wired
// into some reads and forgotten in others.
func itemFromRow(r routerstore.Item) work.Item {
	return work.Item{
		ID: r.ID, From: r.From, To: r.To, Title: r.Title, Type: r.Type,
		Status: r.Status, Opened: r.Opened, Closed: r.Closed,
		Instructions: r.Instructions, Result: r.Result,
		WakeStatus: r.WakeStatus, WakeAttemptedAt: r.WakeAttemptedAt,
		WakeAdapter: r.WakeAdapter, WakeError: r.WakeError, BlockedBy: r.BlockedBy,
	}
}

// sortItemsByID keeps the merged inbox in the file router's oldest-first
// order (ids sort chronologically by construction).
func sortItemsByID(items []work.Item) {
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
}

// Show returns one item's full markdown. Post-cutover it renders the store row —
// the file, if one survives from before the cutover, is frozen at whatever
// status it held when closes stopped touching it, so displaying it would show a
// closed item as open. Pre-cutover it prefers the canonical file (a
// hand-annotated audit view is authoritative for display) and falls back to the
// store row when no file exists (§2b axiom 8: a missing file never hides work).
func (f *Facade) Show(id string) (string, error) {
	if routercfg.StoreWake() {
		return f.renderFromStore(id)
	}
	data, err := os.ReadFile(filepath.Join(f.root, "items", id+".md"))
	if err == nil {
		return string(data), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("dispatch: read item: %w", err)
	}
	return f.renderFromStore(id)
}

// renderFromStore renders one item from the durable store — the post-cutover
// display path, and the pre-cutover fallback when no audit file exists.
func (f *Facade) renderFromStore(id string) (string, error) {
	rendered, err := f.store.Render(id)
	if err != nil {
		if errors.Is(err, routerstore.ErrNotFound) {
			return "", fmt.Errorf("dispatch: item %s not found (no file, no store row)", id)
		}
		return "", fmt.Errorf("dispatch: render item from store: %w", err)
	}
	return rendered, nil
}

// Get returns one item's parsed form, canonical file first (same authority
// order as Show), store row otherwise — so callers can read from:/title:
// metadata for store-only items, which post-cutover is every new item.
func (f *Facade) Get(id string) (work.Item, error) {
	if routercfg.StoreWake() {
		return f.getFromStore(id)
	}
	it, err := work.Get(f.root, id)
	if err == nil {
		return it, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return work.Item{}, fmt.Errorf("dispatch: read item: %w", err)
	}
	return f.getFromStore(id)
}

// getFromStore reads one item's parsed form from the durable store.
func (f *Facade) getFromStore(id string) (work.Item, error) {
	r, err := f.store.Get(id)
	if err != nil {
		if errors.Is(err, routerstore.ErrNotFound) {
			return work.Item{}, fmt.Errorf("dispatch: item %s not found (no file, no store row)", id)
		}
		return work.Item{}, fmt.Errorf("dispatch: get item from store: %w", err)
	}
	return itemFromRow(r), nil
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
			// An already-closed file may still have a stale open store row (a
			// file-only close by a pre-facade binary). Fall through so the store
			// mirror below heals the phantom instead of it being un-closable.
			if !errors.Is(err, work.ErrAlreadyClosed) {
				return err
			}
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
// waitRedeliverAfter bounds the edge-trigger's other direction: a consumer that
// crashed after delivery (cursor written, items never closed) gets the same
// inbox REDELIVERED once the cursor is this old, so edge semantics can never
// strand work forever. One hour keeps redelivery rare without hiding a wedge.
const waitRedeliverAfter = time.Hour

// Wait blocks until the agent's inbox CHANGES, and delivers each inbox state
// exactly once.
//
// It used to be LEVEL-triggered: any non-empty inbox returned instantly. The
// documented /loop arming instruction — injected into every session — calls
// wait in a shell loop, so ONE stuck-open item (owner-gated, tracked, or simply
// unclosed) turned the loop into a full-speed spin: wait returned the same
// unchanged inbox thousands of times. Reported independently twice (router
// items 20260729-225311 and 20260731-182937), and it is the recorded
// fork-storm class (a `router wait` in `while true` while items are open).
//
// Edge semantics via a durable per-agent cursor (a hash of the sorted open item
// ids, stored under the router root):
//   - inbox differs from the cursor → deliver it and advance the cursor;
//     a first-ever wait (no cursor) always delivers, so a consumer arriving to
//     existing work still sees it — the anti-stranding property that made
//     level-triggering tempting in the first place;
//   - inbox unchanged → park on the store FIFO until it changes or the timeout
//     lapses (returning empty), so a stuck item costs ONE wake per timeout
//     period instead of a spin;
//   - cursor older than waitRedeliverAfter → redeliver, so a consumer that
//     died after delivery cannot strand the inbox on its stale cursor.
func (f *Facade) Wait(ctx context.Context, agent string, timeout time.Duration) ([]work.Item, error) {
	deadline := time.Now().Add(timeout)
	for {
		items, err := f.Inbox(agent)
		if err != nil {
			return nil, err
		}
		if len(items) > 0 && f.waitCursorDiffers(agent, items) {
			f.writeWaitCursor(agent, items)
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

func (f *Facade) waitCursorPath(agent string) string {
	return filepath.Join(f.root, ".wait-cursor-"+sanitizeAgentFile(agent))
}

// sanitizeAgentFile keeps the cursor filename safe for any agent id.
func sanitizeAgentFile(agent string) string {
	var b strings.Builder
	for _, r := range agent {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return b.String()
}

func inboxHash(items []work.Item) string {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	return hex.EncodeToString(sum[:])
}

// waitCursorDiffers reports whether this inbox state is NEW relative to the last
// delivery. Missing/unreadable cursor and stale cursor both answer true — every
// failure mode falls toward delivery, never toward stranding.
func (f *Facade) waitCursorDiffers(agent string, items []work.Item) bool {
	st, err := os.Stat(f.waitCursorPath(agent))
	if err != nil {
		return true
	}
	if time.Since(st.ModTime()) > waitRedeliverAfter {
		return true
	}
	prev, err := os.ReadFile(f.waitCursorPath(agent))
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(prev)) != inboxHash(items)
}

func (f *Facade) writeWaitCursor(agent string, items []work.Item) {
	// Best-effort: a failed cursor write degrades to level-triggered delivery
	// (the old behavior), never to stranding.
	_ = os.WriteFile(f.waitCursorPath(agent), []byte(inboxHash(items)+"\n"), 0o644)
}
