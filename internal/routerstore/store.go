// Package routerstore is the durable, queryable index for the multi-agent
// work queue — Phase 1 of Router v2 (docs/prd/ROUTER_V2_DURABLE_DISPATCH.md).
//
// # Relationship to internal/work (the source of truth)
//
// The filesystem markdown items under <router>/items/*.md remain the CANONICAL
// source of truth (PANTHEON_RULES.md A26; Rule A12 additive-only). This package
// does NOT replace them. It is an ADDITIVE SQLite mirror/index: a queryable
// projection of the same items, so surfaces (dashboard, node-status, future
// event-driven dispatch) can ask "what's open for agent X?" with an indexed
// query instead of walking and parsing every file on every read.
//
// Nothing in the live router is wired to this store yet. It ships behind tests,
// in isolation, at zero risk to the working file router (PRD Phase 1 acceptance).
//
// # Design
//
//   - CGO-free SQLite via modernc.org/sqlite (Rule A3 static-binary mandate).
//   - The DB lives OUTSIDE any git repo (default ~/.sirsi/router.db) so runtime
//     state never pollutes the tree (PRD /goal #2). Callers pass the path.
//   - WAL mode + a single *sql.DB with a serialized writer (busy_timeout +
//     one connection for writes) keeps concurrent writes safe (Rule A21 intent
//     applied at the DB layer).
//   - The schema mirrors internal/work.Item field-for-field so import/export is
//     lossless (PRD /goal #4).
package routerstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // CGO-free SQLite driver (Rule A3)
)

// ErrNotFound is returned by Get/Close when no item has the given id.
var ErrNotFound = errors.New("routerstore: item not found")

// ErrAlreadyClosed is returned by Close when the item is already closed,
// mirroring internal/work.Close semantics.
var ErrAlreadyClosed = errors.New("routerstore: item already closed")

// Item is the durable projection of one work item. Fields mirror
// internal/work.Item exactly — field-for-field, same names, same semantics —
// so a markdown item round-trips with zero loss (PRD /goal #4). The parity is
// enforced by TestFieldFidelityWithWorkItem: adding a field to work.Item fails
// that test until this struct AND the schema catch up.
type Item struct {
	ID           string
	From         string
	To           string
	Title        string
	Type         string // ADR-024 §5: "proposal" | "review" | "decision" | ""
	Status       string // "open" | "closed"
	Opened       string // RFC3339
	Closed       string // RFC3339, empty if open
	Instructions string
	Result       string
	BlockedBy    string // optional item id dependency; terminal dependencies do not block

	// Wake-delivery truth, mirroring internal/work.Item's wake_* frontmatter
	// (PR#2 wake-or-declare-unavailable). Dropping these in a backfill would
	// silently lose whether a stranded item was ever woken — exactly the data
	// loss PRD /goal #4 forbids.
	WakeStatus      string // "" when the wake pass has never touched this item
	WakeAttemptedAt string // RFC3339, set when an adapter was invoked
	WakeAdapter     string // the adapter that fired (cli-spawn/api-call/launchagent/...)
	WakeError       string // why the item is wake-unavailable, when it is
}

// Store is a durable index over the work queue backed by SQLite.
// A Store is safe for concurrent use by multiple goroutines.
type Store struct {
	db              *sql.DB
	path            string
	escalationAgent string
	now             func() time.Time // injectable clock (Rule A16); nil means time.Now().UTC()

	// Event-driven dispatch (Phase 2): in-process waiters per agent, woken by
	// SendGuarded/NotifyAgent. Guarded by waitMu per Rule A21.
	waitMu  sync.Mutex
	waiters map[string][]chan struct{}
	// notifyDir overrides the cross-process FIFO directory (tests); empty
	// means ~/.sirsi/notify.
	notifyDir string
}

// Open opens (creating if absent) the SQLite store at path and applies the
// schema. path is a filesystem path such as ~/.sirsi/router.db; it MUST live
// outside any git repo. Use ":memory:" for tests.
//
// The caller owns the returned Store and must Close it.
func Open(path string) (*Store, error) {
	// WAL for reader/writer concurrency; busy_timeout so a momentarily locked
	// DB retries instead of erroring; foreign_keys off (single-table core).
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("routerstore: open %q: %w", path, err)
	}
	// SQLite is single-writer; serializing to one connection avoids
	// "database is locked" under concurrent writes (Rule A21 intent).
	db.SetMaxOpenConns(1)
	escalationAgent := strings.TrimSpace(os.Getenv("SIRSI_ESCALATION_AGENT"))
	if escalationAgent == "" {
		escalationAgent = "owner"
	}
	s := &Store{db: db, path: path, escalationAgent: escalationAgent}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// clock returns the store's time source (UTC), honoring an injected clock.
func (s *Store) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now().UTC()
}

// migrations is the ordered, append-only list of schema migrations.
// Each entry advances to its explicit version. Gaps are reserved schema
// versions owned by companion lineages and are never filled with fake no-ops.
// NEVER edit an entry once it has shipped — append a new migration instead
// (e.g. `ALTER TABLE items ADD COLUMN ...`), so future column adds work
// against existing databases without wiping them.
//
// Migration 1 uses plain CREATE TABLE (no IF NOT EXISTS) deliberately: a
// database that has tables but user_version 0 predates schema versioning and
// has an unknown shape — failing loudly beats silently stamping it v1. No such
// database can exist outside tests (the store has never been wired to
// anything), so nothing shipped is stranded by this choice.
type schemaMigration struct {
	version int
	sql     string
}

var migrations = []schemaMigration{
	// v1 — initial schema. items mirrors internal/work.Item field-for-field
	// (incl. the wake_* delivery-truth columns; see Item).
	{1, `
CREATE TABLE items (
    id                TEXT PRIMARY KEY,
    from_agent        TEXT NOT NULL,
    to_agent          TEXT NOT NULL,
    title             TEXT NOT NULL,
    type              TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'open',
    opened            TEXT NOT NULL DEFAULT '',
    closed            TEXT NOT NULL DEFAULT '',
    instructions      TEXT NOT NULL DEFAULT '',
    result            TEXT NOT NULL DEFAULT '',
    wake_status       TEXT NOT NULL DEFAULT '',
    wake_attempted_at TEXT NOT NULL DEFAULT '',
    wake_adapter      TEXT NOT NULL DEFAULT '',
    wake_error        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_items_to_status ON items(to_agent, status);

CREATE TABLE agents (
    id            TEXT PRIMARY KEY,
    registered_at TEXT NOT NULL DEFAULT '',
    last_seen     TEXT NOT NULL DEFAULT '',
    pid           INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);
`},

	// v2 — Phase 2 Dispatch Contract (PRD §2b, codex-SME APPROVED 2026-07-04;
	// ADR-035). Fenced-lease lifecycle columns, idempotency + singleton keys
	// (enforced as DATABASE invariants via partial unique indexes — the
	// property whose absence produced the 11,564-item flood), send quotas,
	// circuit breakers, and dispatch counters.
	{2, `
ALTER TABLE items ADD COLUMN lease_token   TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN lease_expires TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN claimed_by    TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN attempts      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE items ADD COLUMN idem_key      TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN source_item   TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN failure_class TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN occurrences   INTEGER NOT NULL DEFAULT 1;
ALTER TABLE items ADD COLUMN first_seen    TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN last_seen     TEXT    NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_items_idem ON items(idem_key) WHERE idem_key <> '';
CREATE UNIQUE INDEX idx_items_singleton ON items(source_item, failure_class)
    WHERE source_item <> '' AND failure_class <> '';
CREATE INDEX idx_items_lease ON items(status, lease_expires);

CREATE TABLE breakers (
    domain        TEXT PRIMARY KEY,
    failures      INTEGER NOT NULL DEFAULT 0,
    tripped_at    TEXT    NOT NULL DEFAULT '',
    operator_item TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE send_quota (
    sender TEXT NOT NULL,
    bucket TEXT NOT NULL,
    count  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (sender, bucket)
);

CREATE TABLE counters (
    name  TEXT PRIMARY KEY,
    value INTEGER NOT NULL DEFAULT 0
);
	`},

	// v3 — Universal Task Ledger. Items gain an optional dependency edge and
	// agents gain a durable task registry distinct from routed messages.
	{3, `
ALTER TABLE items ADD COLUMN blocked_by TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_items_blocked_by ON items(blocked_by) WHERE blocked_by <> '';

CREATE TABLE tasks (
    agent             TEXT NOT NULL,
    task_id           TEXT NOT NULL,
    subject           TEXT NOT NULL,
    status            TEXT NOT NULL,
    responsible_party TEXT NOT NULL,
    blocked_by        TEXT NOT NULL DEFAULT '',
    created           TEXT NOT NULL,
    updated           TEXT NOT NULL,
    PRIMARY KEY (agent, task_id)
);
CREATE INDEX idx_tasks_agent_status ON tasks(agent, status);
`},

	// v4 — Phase grouping for the Ledger Board (ADR-050 cross-surface spec).
	// Phase clusters tasks into plain-English groups for owner-facing rendering.
	{4, `ALTER TABLE tasks ADD COLUMN phase TEXT NOT NULL DEFAULT '';`},

	// v7 — Ledger Board drill-down contract (ADR-054 Part B). Versions 5 and
	// 6 belong to the ADR-051 conduit lineage; this migration intentionally
	// advances directly from any v4-v6 database to v7 in one additive step.
	{7, `
ALTER TABLE tasks ADD COLUMN charter TEXT;
ALTER TABLE tasks ADD COLUMN commissioned_at TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN commissioned_by TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN outline TEXT;
ALTER TABLE tasks ADD COLUMN timeline TEXT NOT NULL DEFAULT '[]';
ALTER TABLE tasks ADD COLUMN links TEXT NOT NULL DEFAULT '[]';
ALTER TABLE tasks ADD COLUMN test_state TEXT NOT NULL DEFAULT 'untested';
ALTER TABLE tasks ADD COLUMN stage TEXT NOT NULL DEFAULT 'spec';
ALTER TABLE tasks ADD COLUMN tokens_consumed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN duration_seconds INTEGER NOT NULL DEFAULT 0;
UPDATE tasks SET commissioned_at = created WHERE commissioned_at = '';
UPDATE tasks SET commissioned_by = agent WHERE commissioned_by = '';
`},

	// v8 — ADR-057 step 1: canonical requirement registry, plus the durable
	// identifier allocator that makes cross-claimed document numbers structurally
	// impossible.
	//
	// Why the allocator exists: ADR numbers were picked by agents reading the
	// filesystem and counting. That races by construction — two agents branching
	// from the same main both see the same highest number and both claim it.
	// origin/main carries TWO distinct ADR-054 documents today, and four open PRs
	// claim 051/054/055/056 between them. A CI uniqueness gate detects the
	// collision after the fact; PRIMARY KEY (namespace, number) prevents it,
	// because allocation goes through the same serialized store that already
	// prevents duplicate router items.
	//
	// Withdrawn allocations are RETAINED, never deleted: MAX(number) must keep
	// advancing or a withdrawn number gets handed out again and every existing
	// citation of it silently retargets.
	{8, `
CREATE TABLE identifiers (
    namespace  TEXT NOT NULL,
    number     INTEGER NOT NULL,
    slug       TEXT NOT NULL DEFAULT '',
    title      TEXT NOT NULL,
    owner      TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'claimed',
    claimed_at TEXT NOT NULL,
    PRIMARY KEY (namespace, number)
);
CREATE INDEX idx_identifiers_owner ON identifiers(namespace, owner);

CREATE TABLE requirements (
    req_id         TEXT PRIMARY KEY,
    title          TEXT NOT NULL,
    source         TEXT NOT NULL,
    source_ref     TEXT NOT NULL DEFAULT '',
    owner          TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'open',
    commit_ref     TEXT NOT NULL DEFAULT '',
    tests_ref      TEXT NOT NULL DEFAULT '',
    security_ref   TEXT NOT NULL DEFAULT '',
    design_ref     TEXT NOT NULL DEFAULT '',
    deployment_ref TEXT NOT NULL DEFAULT '',
    production_ref TEXT NOT NULL DEFAULT '',
    waiver_reason  TEXT NOT NULL DEFAULT '',
    created        TEXT NOT NULL,
    updated        TEXT NOT NULL
);
CREATE INDEX idx_requirements_status ON requirements(status);
CREATE INDEX idx_requirements_owner ON requirements(owner, status);
`},

	// v9 — ADR-057 step 2: task-ledger claims are durable, thread-bound, and
	// fenced exactly like router-item claims. A task cannot look "in progress"
	// merely because a worker process exists; executable ownership is represented
	// by a live lease row mutation.
	{9, `
ALTER TABLE tasks ADD COLUMN lease_token TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN lease_expires TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN claimed_by TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN thread_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN idempotency_key TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN result_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN failure_reason TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX idx_tasks_idempotency ON tasks(idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX idx_tasks_lease ON tasks(status, lease_expires);
`},

	// v10 — ADR-057 step 3: source-store transitions create durable wake events
	// in the SAME SQLite transaction. Triggers prevent a new writer from
	// forgetting to call an emitter and silently stranding work.
	{10, `
CREATE TABLE wake_events (
    event_id       TEXT PRIMARY KEY,
    event_key      TEXT NOT NULL UNIQUE,
    agent          TEXT NOT NULL,
    source_kind    TEXT NOT NULL,
    source_id      TEXT NOT NULL,
    reason         TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending',
    attempts       INTEGER NOT NULL DEFAULT 0,
    next_attempt   TEXT NOT NULL DEFAULT '',
    lease_token    TEXT NOT NULL DEFAULT '',
    lease_expires  TEXT NOT NULL DEFAULT '',
    ack_ref        TEXT NOT NULL DEFAULT '',
    last_error     TEXT NOT NULL DEFAULT '',
    created        TEXT NOT NULL,
    updated        TEXT NOT NULL
);
CREATE INDEX idx_wake_events_pending ON wake_events(status,next_attempt,created);
CREATE INDEX idx_wake_events_agent ON wake_events(agent,status);
INSERT OR IGNORE INTO state(key,value) VALUES('operational_enforcement_since',strftime('%Y-%m-%dT%H:%M:%SZ','now'));

CREATE TRIGGER wake_item_created AFTER INSERT ON items
WHEN NEW.status='open'
BEGIN
  INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  VALUES(lower(hex(randomblob(16))),'item:create:'||NEW.id,NEW.to_agent,'router_item',NEW.id,'inbox item created',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now'));
END;
CREATE TRIGGER wake_item_unblocked AFTER UPDATE OF status ON items
WHEN NEW.status='open' AND OLD.status<>'open'
BEGIN
  INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  VALUES(lower(hex(randomblob(16))),'item:open:'||NEW.id||':'||NEW.attempts,NEW.to_agent,'router_item',NEW.id,'inbox item unblocked or lease expired',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now'));
END;
CREATE TRIGGER wake_item_blocker_cleared AFTER UPDATE OF blocked_by ON items
WHEN NEW.status='open' AND NEW.blocked_by='' AND OLD.blocked_by<>''
BEGIN
  INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  VALUES(lower(hex(randomblob(16))),'item:blocker-cleared:'||NEW.id,NEW.to_agent,'router_item',NEW.id,'inbox item unblocked',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now'));
END;
CREATE TRIGGER wake_item_dependency_terminal AFTER UPDATE OF status ON items
WHEN NEW.status IN ('closed','completed')
BEGIN
  INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  SELECT lower(hex(randomblob(16))),'item:dependency-terminal:'||d.id||':'||NEW.id,d.to_agent,'router_item',d.id,'inbox dependency reached terminal disposition',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now')
  FROM items d WHERE d.status='open' AND d.blocked_by=NEW.id;
END;
CREATE TRIGGER wake_task_created AFTER INSERT ON tasks
WHEN NEW.status IN ('pending','in-progress') AND NEW.blocked_by=''
BEGIN
  INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  VALUES(lower(hex(randomblob(16))),'task:create:'||NEW.agent||':'||NEW.task_id,NEW.agent,'ledger_task',NEW.task_id,'ledger task created or assigned',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now'));
END;
CREATE TRIGGER wake_task_unblocked AFTER UPDATE OF status,blocked_by ON tasks
WHEN NEW.status IN ('pending','in-progress') AND NEW.blocked_by='' AND (OLD.status NOT IN ('pending','in-progress') OR OLD.blocked_by<>'')
BEGIN
  INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  VALUES(lower(hex(randomblob(16))),'task:actionable:'||NEW.agent||':'||NEW.task_id||':'||NEW.updated,NEW.agent,'ledger_task',NEW.task_id,'ledger task assigned or unblocked',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now'));
END;
CREATE TRIGGER wake_requirement_created AFTER INSERT ON requirements
WHEN NEW.status NOT IN ('satisfied','waived')
BEGIN
  INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  VALUES(lower(hex(randomblob(16))),'requirement:create:'||NEW.req_id,NEW.owner,'requirement',NEW.req_id,'requirement audit created an implementation gap',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now'));
END;
CREATE TRIGGER wake_continue_after_item AFTER UPDATE OF status ON items
WHEN NEW.status IN ('closed','completed','dead_letter') AND EXISTS (
  SELECT 1 FROM items i WHERE i.to_agent=NEW.to_agent AND i.status='open'
)
BEGIN
  INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  VALUES(lower(hex(randomblob(16))),'continue:item:'||NEW.id,NEW.to_agent,'lane',NEW.to_agent,'worker completed item while more work exists',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now'));
END;
CREATE TRIGGER wake_continue_after_task AFTER UPDATE OF status ON tasks
WHEN NEW.status='done' AND EXISTS (
  SELECT 1 FROM tasks t WHERE t.agent=NEW.agent AND t.status IN ('pending','in-progress') AND t.blocked_by=''
)
BEGIN
  INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  VALUES(lower(hex(randomblob(16))),'continue:task:'||NEW.agent||':'||NEW.task_id,NEW.agent,'lane',NEW.agent,'worker completed task while more work exists',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now'));
END;
CREATE TRIGGER ack_wake_on_item_claim AFTER UPDATE OF lease_token ON items
WHEN NEW.lease_token<>'' AND OLD.lease_token=''
BEGIN
  UPDATE wake_events SET status='acked',ack_ref='router-lease:'||NEW.id||':'||NEW.lease_token,lease_token='',lease_expires='',updated=strftime('%Y-%m-%dT%H:%M:%SZ','now')
  WHERE agent=NEW.to_agent AND status='leased' AND ((source_kind='router_item' AND source_id=NEW.id) OR source_kind='lane');
END;
CREATE TRIGGER ack_wake_on_task_claim AFTER UPDATE OF lease_token ON tasks
WHEN NEW.lease_token<>'' AND OLD.lease_token=''
BEGIN
  UPDATE wake_events SET status='acked',ack_ref='task-lease:'||NEW.agent||':'||NEW.task_id||':'||NEW.lease_token,lease_token='',lease_expires='',updated=strftime('%Y-%m-%dT%H:%M:%SZ','now')
  WHERE agent=NEW.agent AND status='leased' AND (
    (source_kind='ledger_task' AND source_id=NEW.task_id) OR
    (source_kind='requirement' AND NEW.task_id='requirement/'||source_id) OR source_kind='lane'
  );
END;
`},

	// v11 — wake acknowledgments are source-correlated. Databases that already
	// applied v10 carried agent-wide ACK triggers, which allowed claiming work B
	// to acknowledge a leased wake for work A. Replace them in-place; fresh
	// databases receive the same definitions in v10 and this idempotent cutover.
	{11, `
DROP TRIGGER IF EXISTS ack_wake_on_item_claim;
DROP TRIGGER IF EXISTS ack_wake_on_task_claim;
CREATE TRIGGER ack_wake_on_item_claim AFTER UPDATE OF lease_token ON items
WHEN NEW.lease_token<>'' AND OLD.lease_token=''
BEGIN
  UPDATE wake_events SET status='acked',ack_ref='router-lease:'||NEW.id||':'||NEW.lease_token,lease_token='',lease_expires='',updated=strftime('%Y-%m-%dT%H:%M:%SZ','now')
  WHERE agent=NEW.to_agent AND status='leased' AND ((source_kind='router_item' AND source_id=NEW.id) OR source_kind='lane');
END;
CREATE TRIGGER ack_wake_on_task_claim AFTER UPDATE OF lease_token ON tasks
WHEN NEW.lease_token<>'' AND OLD.lease_token=''
BEGIN
  UPDATE wake_events SET status='acked',ack_ref='task-lease:'||NEW.agent||':'||NEW.task_id||':'||NEW.lease_token,lease_token='',lease_expires='',updated=strftime('%Y-%m-%dT%H:%M:%SZ','now')
  WHERE agent=NEW.agent AND status='leased' AND (
    (source_kind='ledger_task' AND source_id=NEW.task_id) OR
    (source_kind='requirement' AND NEW.task_id='requirement/'||source_id) OR source_kind='lane'
  );
END;
`},

	// v12 — a failed prerequisite cannot release dependent execution. Only a
	// successful close/completion wakes the dependent; dead letters remain a
	// visible blocker until an explicit failure policy resolves them.
	{12, `
DROP TRIGGER IF EXISTS wake_item_dependency_terminal;
CREATE TRIGGER wake_item_dependency_terminal AFTER UPDATE OF status ON items
WHEN NEW.status IN ('closed','completed')
BEGIN
  INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  SELECT lower(hex(randomblob(16))),'item:dependency-terminal:'||d.id||':'||NEW.id,d.to_agent,'router_item',d.id,'inbox dependency completed successfully',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now')
  FROM items d WHERE d.status='open' AND d.blocked_by=NEW.id;
END;
`},

	// v13 — item execution mutation time. A live/long lease proves assignment,
	// not continuing work; Horus needs an independent timestamp to distinguish
	// WORKING from ASSIGNED without consulting session heartbeats.
	{13, `
ALTER TABLE items ADD COLUMN lease_updated TEXT NOT NULL DEFAULT '';
UPDATE items SET lease_updated=opened WHERE lease_token<>'' AND lease_updated='';
CREATE INDEX idx_items_lease_updated ON items(to_agent,status,lease_updated);
`},
	{14, `
ALTER TABLE requirements ADD COLUMN waiver_ref TEXT NOT NULL DEFAULT '';
DROP TRIGGER IF EXISTS wake_continue_after_item;
DROP TRIGGER IF EXISTS wake_continue_after_task;
DROP TRIGGER IF EXISTS ack_wake_on_item_claim;
DROP TRIGGER IF EXISTS ack_wake_on_task_claim;
CREATE TRIGGER wake_continue_after_item AFTER UPDATE OF status ON items
WHEN NEW.status IN ('closed','completed','dead_letter') AND EXISTS (SELECT 1 FROM items i WHERE i.to_agent=NEW.to_agent AND i.status='open')
BEGIN
 INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
 VALUES(lower(hex(randomblob(16))),'continue:item:'||NEW.id,NEW.to_agent,'lane',NEW.to_agent,'worker completed item while more work exists',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now'));
END;
CREATE TRIGGER wake_continue_after_task AFTER UPDATE OF status ON tasks
WHEN NEW.status='done' AND EXISTS (SELECT 1 FROM tasks t WHERE t.agent=NEW.agent AND t.status IN ('pending','in-progress') AND t.blocked_by='')
BEGIN
 INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
 VALUES(lower(hex(randomblob(16))),'continue:task:'||NEW.agent||':'||NEW.task_id,NEW.agent,'lane',NEW.agent,'worker completed task while more work exists',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now'));
END;
CREATE TRIGGER ack_wake_on_item_claim AFTER UPDATE OF lease_token ON items
WHEN NEW.lease_token<>'' AND OLD.lease_token=''
BEGIN
 UPDATE wake_events SET status='acked',ack_ref='router-lease:'||NEW.id||':'||NEW.lease_token,lease_token='',lease_expires='',updated=strftime('%Y-%m-%dT%H:%M:%SZ','now')
 WHERE agent=NEW.to_agent AND status='leased' AND ((source_kind='router_item' AND source_id=NEW.id) OR source_kind='lane');
END;
CREATE TRIGGER ack_wake_on_task_claim AFTER UPDATE OF lease_token ON tasks
WHEN NEW.lease_token<>'' AND OLD.lease_token=''
BEGIN
 UPDATE wake_events SET status='acked',ack_ref='task-lease:'||NEW.agent||':'||NEW.task_id||':'||NEW.lease_token,lease_token='',lease_expires='',updated=strftime('%Y-%m-%dT%H:%M:%SZ','now')
 WHERE agent=NEW.agent AND status='leased' AND ((source_kind='ledger_task' AND source_id=NEW.task_id) OR (source_kind='requirement' AND NEW.task_id='requirement/'||source_id) OR source_kind='lane');
END;
`},
	// v15 — a ledger task completing UNBLOCKS its dependents, and that must emit
	// a wake in the SAME transaction as the status change. A dependent whose
	// blocker just closed is runnable RIGHT THEN; leaving the wake to a later
	// sweep is how a lane sits idle holding work that is no longer blocked.
	//
	// Recovered from the live store: this trigger was applied to production by a
	// build that never pushed its source, so the schema existed while no commit
	// defined it and every peer binary fail-closed at v15. Extracted verbatim
	// from sqlite_master and committed here so the schema has a definition again.
	// ADR-057 also restores the continuation trigger from the same canonical
	// runnable predicate so newly released work is dispatched transactionally.
	{15, `
DROP TRIGGER IF EXISTS wake_task_dependency_done;
DROP TRIGGER IF EXISTS wake_continue_after_task;
CREATE TRIGGER wake_task_dependency_done AFTER UPDATE OF status ON tasks
WHEN NEW.status='done'
BEGIN
  INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  SELECT lower(hex(randomblob(16))),'task:dependency-done:'||d.agent||':'||d.task_id||':'||NEW.task_id,d.agent,'ledger_task',d.task_id,'ledger task dependency completed successfully',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now')
  FROM tasks d WHERE d.agent=NEW.agent AND d.status IN ('pending','in-progress') AND d.blocked_by=NEW.task_id;
END;
CREATE TRIGGER wake_continue_after_task AFTER UPDATE OF status ON tasks
WHEN NEW.status='done' AND EXISTS (
  SELECT 1 FROM tasks t WHERE t.agent=NEW.agent AND t.status IN ('pending','in-progress')
  AND (t.blocked_by='' OR EXISTS (SELECT 1 FROM tasks d WHERE d.agent=t.agent AND d.task_id=t.blocked_by AND d.status='done'))
)
BEGIN
  INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
  VALUES(lower(hex(randomblob(16))),'continue:task:'||NEW.agent||':'||NEW.task_id,NEW.agent,'lane',NEW.agent,'worker completed task while more work exists',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now'));
END;
`},
}

// migrate applies any pending numbered migrations, tracked via the SQLite
// user_version pragma. Idempotent — safe to call on every Open: a database
// already at the current version applies nothing.
// migrate is safe under CROSS-PROCESS concurrency on a fresh database. Two
// `sirsi` processes opening the same new store both start at user_version 0;
// because WAL lets a reader see a snapshot, a naive read-then-migrate lets the
// slower process re-run an already-applied step (e.g. `ALTER … ADD lease_token`
// → "duplicate column name"). The cutover makes this common — every agent opens
// the store. So each step runs under an explicit `BEGIN IMMEDIATE` (which grabs
// the write lock up front, contending via busy_timeout) and RE-READS
// user_version inside that lock: if a peer already advanced it, this process
// rolls back and re-loops instead of double-applying. All statements run on one
// pinned connection so the manual BEGIN/COMMIT is not spread across the pool.
func (s *Store) migrate() error {
	ctx := context.Background()
	// Pinning the connection establishes it — which on a FRESH database runs the
	// DSN pragmas, and setting journal_mode=WAL needs a brief exclusive lock. Two
	// processes opening the same new store race there and one sees SQLITE_BUSY at
	// connection-establishment, before any migration statement. Retry the pin on
	// busy so a concurrent first-open never fails the whole Open.
	conn, err := pinConnWithRetry(ctx, s.db)
	if err != nil {
		return fmt.Errorf("routerstore: migrate: pin connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Re-assert busy_timeout on this pinned connection for the BEGIN IMMEDIATE loop.
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout = 5000;`); err != nil {
		return fmt.Errorf("routerstore: migrate: set busy_timeout: %w", err)
	}

	initialVersion := -1
	for {
		var version int
		if err := conn.QueryRowContext(ctx, `PRAGMA user_version;`).Scan(&version); err != nil {
			return fmt.Errorf("routerstore: migrate: read user_version: %w", err)
		}
		if initialVersion < 0 {
			initialVersion = version
		}
		maxVersion := migrations[len(migrations)-1].version
		if version > maxVersion {
			return tooNewError(version, maxVersion, readMigrationProvenance(ctx, conn))
		}
		if version == maxVersion {
			return nil // up to date
		}
		// Version zero is first creation, not advancement of a deployed schema.
		// A clean host must be installable without a hidden migration override.
		if initialVersion > 0 && isSharedProductionStore(s.path) && os.Getenv("SIRSI_ALLOW_SCHEMA_MIGRATE") != "1" {
			return fmt.Errorf("routerstore: live schema advancement v%d→v%d is a deployment event; set SIRSI_ALLOW_SCHEMA_MIGRATE=1 only during an atomic reviewed binary rollout", version, maxVersion)
		}
		var next schemaMigration
		for _, candidate := range migrations {
			if candidate.version > version {
				next = candidate
				break
			}
		}

		// BEGIN IMMEDIATE acquires the write lock now (waiting up to busy_timeout
		// for a peer mid-migration) so the version re-check below is authoritative.
		// A fresh-init race can still surface SQLITE_BUSY before the lock is
		// grantable; retry with a bounded backoff rather than failing the Open.
		if err := beginImmediateWithRetry(ctx, conn); err != nil {
			return fmt.Errorf("routerstore: migrate to v%d: begin immediate: %w", next.version, err)
		}
		var locked int
		if err := conn.QueryRowContext(ctx, `PRAGMA user_version;`).Scan(&locked); err != nil {
			_, _ = conn.ExecContext(ctx, `ROLLBACK;`)
			return fmt.Errorf("routerstore: migrate to v%d: re-read user_version: %w", next.version, err)
		}
		if locked != version {
			// A concurrent opener already advanced past i under the lock. Bail out
			// of this attempt and re-evaluate from the top — no double-apply.
			if _, err := conn.ExecContext(ctx, `ROLLBACK;`); err != nil {
				return fmt.Errorf("routerstore: migrate to v%d: rollback stale: %w", next.version, err)
			}
			continue
		}
		// The gate: refuse a one-way write to shared state from source nobody
		// else can rebuild. Checked under the write lock so the version pair
		// reported is the one actually about to be applied.
		if gateErr := checkMigrationAllowed(version, next.version); gateErr != nil {
			_, _ = conn.ExecContext(ctx, `ROLLBACK;`)
			return gateErr
		}
		if _, err := conn.ExecContext(ctx, next.sql); err != nil {
			_, _ = conn.ExecContext(ctx, `ROLLBACK;`)
			return fmt.Errorf("routerstore: migrate to v%d: %w", next.version, err)
		}
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d;", next.version)); err != nil {
			_, _ = conn.ExecContext(ctx, `ROLLBACK;`)
			return fmt.Errorf("routerstore: migrate to v%d: set user_version: %w", next.version, err)
		}
		recordMigrationProvenance(ctx, conn, next.version)
		if _, err := conn.ExecContext(ctx, `COMMIT;`); err != nil {
			return fmt.Errorf("routerstore: migrate to v%d: commit: %w", next.version, err)
		}
	}
}

func checkSchemaCompatibility(current, maxSupported int) error {
	if current > maxSupported {
		return fmt.Errorf("routerstore: migrate: database is at schema version %d, newer than this binary understands (max %d) — refusing to touch it", current, maxSupported)
	}
	return nil
}

func isSharedProductionStore(path string) bool {
	if path == "" || path == ":memory:" {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return true // unknown identity fails closed
	}
	live := filepath.Join(home, ".sirsi", "router.db")
	if liveInfo, liveErr := os.Stat(live); liveErr == nil {
		if pathInfo, pathErr := os.Stat(path); pathErr == nil {
			return os.SameFile(liveInfo, pathInfo)
		}
	}
	// Pre-creation fallback: resolve cwd, symlinked parent directories, and
	// case-folding without requiring the database file itself to exist.
	canonical := func(p string) (string, error) {
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", err
		}
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			return resolved, nil
		}
		parent, base := filepath.Dir(abs), filepath.Base(abs)
		resolvedParent, err := filepath.EvalSymlinks(parent)
		if err != nil {
			return filepath.Clean(abs), nil
		}
		return filepath.Join(resolvedParent, base), nil
	}
	p, pErr := canonical(path)
	l, lErr := canonical(live)
	if pErr != nil || lErr != nil {
		return true
	}
	return strings.EqualFold(filepath.Clean(p), filepath.Clean(l))
}

// pinConnWithRetry acquires a pinned connection, retrying on a transient
// SQLITE_BUSY at connection-establishment — the fresh-DB WAL-init window where
// two processes contend to set journal_mode. Bounded ~5s (25 × 200ms).
func pinConnWithRetry(ctx context.Context, db *sql.DB) (*sql.Conn, error) {
	const attempts = 25
	var err error
	for a := 0; a < attempts; a++ {
		var conn *sql.Conn
		conn, err = db.Conn(ctx)
		if err == nil {
			// Force the connection to actually establish (run DSN pragmas) now, so
			// a WAL-init busy surfaces here where we can retry it, not later.
			if _, perr := conn.ExecContext(ctx, `PRAGMA user_version;`); perr == nil {
				return conn, nil
			} else {
				_ = conn.Close()
				err = perr
			}
		}
		if !isBusy(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil, err
}

// beginImmediateWithRetry runs BEGIN IMMEDIATE, retrying on a transient
// SQLITE_BUSY / "database is locked" — the fresh-init WAL window where two
// processes race to create -wal/-shm and grab the first write lock. busy_timeout
// covers most contention; this bounded backoff covers the residual init race so
// a concurrent first-open never fails the whole Open. ~5s ceiling (25 × 200ms),
// well beyond any real fresh-DB migration (a few ALTERs).
func beginImmediateWithRetry(ctx context.Context, conn *sql.Conn) error {
	const attempts = 25
	var err error
	for a := 0; a < attempts; a++ {
		if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE;`); err == nil {
			return nil
		}
		if !isBusy(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return err
}

// isBusy reports whether err is a SQLite busy/locked condition (driver-agnostic
// string match — modernc surfaces these as text, e.g. "database is locked (5)").
func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "SQLITE_BUSY") ||
		strings.Contains(msg, "(5)")
}
