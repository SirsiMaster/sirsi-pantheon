-- Router ledger — Postgres schema (ADR-062 §1/§3, Migration step rs-05).
--
-- This is the SQLite schema at user_version 17 (internal/routerstore/store.go
-- migrations 1..17) translated once, plus the ADR-062 §3 identity tables
-- (sessions, lease_sessions) and identity columns on threads. A Postgres ledger is
-- always created fresh by `sirsi router migrate` from a quiesced SQLite dump
-- (rs-12), so there is no incremental migration chain here: one baseline,
-- versioned by router.schema_version so the migration tool can compare it
-- with SQLite's PRAGMA user_version.
--
-- Translation rules (keep in sync with the dialect layer, rs-06):
--   TEXT timestamps stay TEXT (RFC3339, UTC) — the store code compares them as
--     strings, and the migration diff (rs-12) must be byte-for-byte.
--   INSERT OR IGNORE                  → INSERT ... ON CONFLICT DO NOTHING
--   lower(hex(randomblob(16)))        → router.rand_hex32()
--   strftime('%Y-%m-%dT%H:%M:%SZ','now') → router.now_rfc3339()
--   BLOB                              → BYTEA
--   trigger WHEN with EXISTS(...)     → EXISTS moved into the function body
--     (Postgres trigger WHEN clauses cannot contain subqueries)
--
-- Roles (see roles.sql): DDL runs as router_migrator (owner); the service
-- runs as router_service with DML only. Nothing here is executed by a Mac.

CREATE SCHEMA IF NOT EXISTS router;
SET search_path = router;

-- ── helpers ────────────────────────────────────────────────────────────────

CREATE OR REPLACE FUNCTION router.now_rfc3339() RETURNS TEXT
LANGUAGE sql STABLE AS $$
  SELECT to_char((now() AT TIME ZONE 'UTC'), 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
$$;

-- 32 lowercase hex chars, same shape as SQLite's lower(hex(randomblob(16))).
CREATE OR REPLACE FUNCTION router.rand_hex32() RETURNS TEXT
LANGUAGE sql VOLATILE AS $$
  SELECT md5(random()::text || clock_timestamp()::text || txid_current()::text)
$$;

-- ── schema version (pairs with SQLite PRAGMA user_version) ─────────────────

CREATE TABLE IF NOT EXISTS schema_version (
    singleton  BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    version    INTEGER NOT NULL,
    applied_at TEXT    NOT NULL
);
INSERT INTO schema_version(version, applied_at) VALUES (17, router.now_rfc3339())
  ON CONFLICT (singleton) DO NOTHING;

-- ── v1 ─────────────────────────────────────────────────────────────────────

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
    wake_error        TEXT NOT NULL DEFAULT '',
    -- v2
    lease_token       TEXT    NOT NULL DEFAULT '',
    lease_expires     TEXT    NOT NULL DEFAULT '',
    claimed_by        TEXT    NOT NULL DEFAULT '',
    attempts          INTEGER NOT NULL DEFAULT 0,
    idem_key          TEXT    NOT NULL DEFAULT '',
    source_item       TEXT    NOT NULL DEFAULT '',
    failure_class     TEXT    NOT NULL DEFAULT '',
    occurrences       INTEGER NOT NULL DEFAULT 1,
    first_seen        TEXT    NOT NULL DEFAULT '',
    last_seen         TEXT    NOT NULL DEFAULT '',
    -- v3
    blocked_by        TEXT    NOT NULL DEFAULT '',
    -- v13
    lease_updated     TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_items_to_status      ON items(to_agent, status);
CREATE UNIQUE INDEX idx_items_idem    ON items(idem_key) WHERE idem_key <> '';
CREATE UNIQUE INDEX idx_items_singleton ON items(source_item, failure_class)
    WHERE source_item <> '' AND failure_class <> '';
CREATE INDEX idx_items_lease          ON items(status, lease_expires);
CREATE INDEX idx_items_blocked_by     ON items(blocked_by) WHERE blocked_by <> '';
CREATE INDEX idx_items_lease_updated  ON items(to_agent, status, lease_updated);

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

-- ── v2 ─────────────────────────────────────────────────────────────────────

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

-- ── v3 / v4 / v7 / v9 ──────────────────────────────────────────────────────

CREATE TABLE tasks (
    agent             TEXT NOT NULL,
    task_id           TEXT NOT NULL,
    subject           TEXT NOT NULL,
    status            TEXT NOT NULL,
    responsible_party TEXT NOT NULL,
    blocked_by        TEXT NOT NULL DEFAULT '',
    created           TEXT NOT NULL,
    updated           TEXT NOT NULL,
    -- v4
    phase             TEXT NOT NULL DEFAULT '',
    -- v7
    charter           TEXT,
    commissioned_at   TEXT NOT NULL DEFAULT '',
    commissioned_by   TEXT NOT NULL DEFAULT '',
    outline           TEXT,
    timeline          TEXT NOT NULL DEFAULT '[]',
    links             TEXT NOT NULL DEFAULT '[]',
    test_state        TEXT NOT NULL DEFAULT 'untested',
    stage             TEXT NOT NULL DEFAULT 'spec',
    tokens_consumed   INTEGER NOT NULL DEFAULT 0,
    duration_seconds  INTEGER NOT NULL DEFAULT 0,
    -- v9
    lease_token       TEXT NOT NULL DEFAULT '',
    lease_expires     TEXT NOT NULL DEFAULT '',
    claimed_by        TEXT NOT NULL DEFAULT '',
    thread_id         TEXT NOT NULL DEFAULT '',
    attempts          INTEGER NOT NULL DEFAULT 0,
    idempotency_key   TEXT NOT NULL DEFAULT '',
    result_ref        TEXT NOT NULL DEFAULT '',
    failure_reason    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (agent, task_id)
);
CREATE INDEX idx_tasks_agent_status ON tasks(agent, status);
CREATE UNIQUE INDEX idx_tasks_idempotency ON tasks(idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX idx_tasks_lease ON tasks(status, lease_expires);

-- ── v8 ─────────────────────────────────────────────────────────────────────

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
    updated        TEXT NOT NULL,
    -- v14
    waiver_ref     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_requirements_status ON requirements(status);
CREATE INDEX idx_requirements_owner  ON requirements(owner, status);

-- ── v10 ────────────────────────────────────────────────────────────────────

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
CREATE INDEX idx_wake_events_pending ON wake_events(status, next_attempt, created);
CREATE INDEX idx_wake_events_agent   ON wake_events(agent, status);
INSERT INTO state(key, value) VALUES ('operational_enforcement_since', router.now_rfc3339())
  ON CONFLICT (key) DO NOTHING;

-- ── v16 ────────────────────────────────────────────────────────────────────

CREATE TABLE threads (
    thread_id    TEXT PRIMARY KEY,
    agent        TEXT NOT NULL,
    status       TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    payload      BYTEA NOT NULL,
    -- ADR-062 §3: the service mints session at register time and binds it to
    -- (host, runtime, agent). runtime_hash is the binary's code-signature hash.
    host         TEXT NOT NULL DEFAULT '',
    user_id      TEXT NOT NULL DEFAULT '',
    session      TEXT NOT NULL DEFAULT '',
    runtime_hash TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_threads_agent_status ON threads(agent, status);
CREATE INDEX idx_threads_last_seen    ON threads(last_seen_at);
CREATE UNIQUE INDEX idx_threads_session ON threads(session) WHERE session <> '';

-- ── v17 — ADR-062 §3 sessions ──────────────────────────────────────────────

CREATE TABLE sessions (
    session_id   TEXT PRIMARY KEY,
    secret       TEXT NOT NULL,
    host         TEXT NOT NULL,
    agent        TEXT NOT NULL,
    runtime_hash TEXT NOT NULL,
    created      TEXT NOT NULL,
    last_seen    TEXT NOT NULL,
    revoked      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_sessions_host_agent ON sessions(host, agent);

-- Which session holds each lease. A side table, not columns on items/tasks:
-- items mirror work.Item field-for-field and identity never round-trips
-- through markdown.
CREATE TABLE lease_sessions (
    kind    TEXT NOT NULL,
    key     TEXT NOT NULL,
    session TEXT NOT NULL,
    PRIMARY KEY (kind, key)
);

-- ── wake-event triggers (final state after migrations 10..15) ─────────────
-- Each is one function + one trigger. A wake event is emitted in the SAME
-- transaction as the source mutation, exactly as in SQLite, so a writer
-- cannot forget to emit and strand work.

CREATE OR REPLACE FUNCTION router.trg_wake_item_created() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status = 'open' THEN
    INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
    VALUES (router.rand_hex32(),'item:create:'||NEW.id,NEW.to_agent,'router_item',NEW.id,'inbox item created',router.now_rfc3339(),router.now_rfc3339())
    ON CONFLICT DO NOTHING;
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER wake_item_created AFTER INSERT ON items
  FOR EACH ROW EXECUTE FUNCTION router.trg_wake_item_created();

CREATE OR REPLACE FUNCTION router.trg_wake_item_unblocked() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status = 'open' AND OLD.status <> 'open' THEN
    INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
    VALUES (router.rand_hex32(),'item:open:'||NEW.id||':'||NEW.attempts,NEW.to_agent,'router_item',NEW.id,'inbox item unblocked or lease expired',router.now_rfc3339(),router.now_rfc3339())
    ON CONFLICT DO NOTHING;
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER wake_item_unblocked AFTER UPDATE OF status ON items
  FOR EACH ROW EXECUTE FUNCTION router.trg_wake_item_unblocked();

CREATE OR REPLACE FUNCTION router.trg_wake_item_blocker_cleared() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status = 'open' AND NEW.blocked_by = '' AND OLD.blocked_by <> '' THEN
    INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
    VALUES (router.rand_hex32(),'item:blocker-cleared:'||NEW.id,NEW.to_agent,'router_item',NEW.id,'inbox item unblocked',router.now_rfc3339(),router.now_rfc3339())
    ON CONFLICT DO NOTHING;
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER wake_item_blocker_cleared AFTER UPDATE OF blocked_by ON items
  FOR EACH ROW EXECUTE FUNCTION router.trg_wake_item_blocker_cleared();

-- v12 wording
CREATE OR REPLACE FUNCTION router.trg_wake_item_dependency_terminal() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status IN ('closed','completed') THEN
    INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
    SELECT router.rand_hex32(),'item:dependency-terminal:'||d.id||':'||NEW.id,d.to_agent,'router_item',d.id,'inbox dependency completed successfully',router.now_rfc3339(),router.now_rfc3339()
    FROM items d WHERE d.status = 'open' AND d.blocked_by = NEW.id
    ON CONFLICT DO NOTHING;
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER wake_item_dependency_terminal AFTER UPDATE OF status ON items
  FOR EACH ROW EXECUTE FUNCTION router.trg_wake_item_dependency_terminal();

CREATE OR REPLACE FUNCTION router.trg_wake_task_created() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status IN ('pending','in-progress') AND NEW.blocked_by = '' THEN
    INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
    VALUES (router.rand_hex32(),'task:actionable:'||NEW.agent||':'||NEW.task_id||':'||NEW.updated,NEW.agent,'ledger_task',NEW.task_id,'ledger task assigned or unblocked',router.now_rfc3339(),router.now_rfc3339())
    ON CONFLICT DO NOTHING;
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER wake_task_created AFTER INSERT ON tasks
  FOR EACH ROW EXECUTE FUNCTION router.trg_wake_task_created();

CREATE OR REPLACE FUNCTION router.trg_wake_task_unblocked() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status IN ('pending','in-progress') AND NEW.blocked_by = ''
     AND (OLD.status NOT IN ('pending','in-progress') OR OLD.blocked_by <> '') THEN
    INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
    VALUES (router.rand_hex32(),'task:actionable:'||NEW.agent||':'||NEW.task_id||':'||NEW.updated,NEW.agent,'ledger_task',NEW.task_id,'ledger task assigned or unblocked',router.now_rfc3339(),router.now_rfc3339())
    ON CONFLICT DO NOTHING;
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER wake_task_unblocked AFTER UPDATE OF status, blocked_by ON tasks
  FOR EACH ROW EXECUTE FUNCTION router.trg_wake_task_unblocked();

CREATE OR REPLACE FUNCTION router.trg_wake_requirement_created() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status NOT IN ('satisfied','waived') THEN
    INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
    VALUES (router.rand_hex32(),'requirement:create:'||NEW.req_id,NEW.owner,'requirement',NEW.req_id,'requirement audit created an implementation gap',router.now_rfc3339(),router.now_rfc3339())
    ON CONFLICT DO NOTHING;
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER wake_requirement_created AFTER INSERT ON requirements
  FOR EACH ROW EXECUTE FUNCTION router.trg_wake_requirement_created();

-- v14 wording
CREATE OR REPLACE FUNCTION router.trg_wake_continue_after_item() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status IN ('closed','completed','dead_letter')
     AND EXISTS (SELECT 1 FROM items i WHERE i.to_agent = NEW.to_agent AND i.status = 'open') THEN
    INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
    VALUES (router.rand_hex32(),'continue:item:'||NEW.id,NEW.to_agent,'lane',NEW.to_agent,'worker completed item while more work exists',router.now_rfc3339(),router.now_rfc3339())
    ON CONFLICT DO NOTHING;
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER wake_continue_after_item AFTER UPDATE OF status ON items
  FOR EACH ROW EXECUTE FUNCTION router.trg_wake_continue_after_item();

-- v15
CREATE OR REPLACE FUNCTION router.trg_wake_task_dependency_done() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status = 'done' THEN
    INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
    SELECT router.rand_hex32(),'task:dependency-done:'||d.agent||':'||d.task_id||':'||NEW.task_id,d.agent,'ledger_task',d.task_id,'ledger task dependency completed successfully',router.now_rfc3339(),router.now_rfc3339()
    FROM tasks d WHERE d.agent = NEW.agent AND d.status IN ('pending','in-progress') AND d.blocked_by = NEW.task_id
    ON CONFLICT DO NOTHING;
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER wake_task_dependency_done AFTER UPDATE OF status ON tasks
  FOR EACH ROW EXECUTE FUNCTION router.trg_wake_task_dependency_done();

CREATE OR REPLACE FUNCTION router.trg_wake_continue_after_task() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status = 'done' AND EXISTS (
       SELECT 1 FROM tasks t WHERE t.agent = NEW.agent AND t.status IN ('pending','in-progress')
       AND (t.blocked_by = '' OR EXISTS (SELECT 1 FROM tasks d WHERE d.agent = t.agent AND d.task_id = t.blocked_by AND d.status = 'done'))
     ) THEN
    INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
    VALUES (router.rand_hex32(),'continue:task:'||NEW.agent||':'||NEW.task_id,NEW.agent,'lane',NEW.agent,'worker completed task while more work exists',router.now_rfc3339(),router.now_rfc3339())
    ON CONFLICT DO NOTHING;
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER wake_continue_after_task AFTER UPDATE OF status ON tasks
  FOR EACH ROW EXECUTE FUNCTION router.trg_wake_continue_after_task();

-- v14 wording
CREATE OR REPLACE FUNCTION router.trg_ack_wake_on_item_claim() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.lease_token <> '' AND OLD.lease_token = '' THEN
    UPDATE wake_events SET status='acked', ack_ref='router-lease:'||NEW.id||':'||NEW.lease_token,
           lease_token='', lease_expires='', updated=router.now_rfc3339()
    WHERE agent = NEW.to_agent AND status = 'leased'
      AND ((source_kind = 'router_item' AND source_id = NEW.id) OR source_kind = 'lane');
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER ack_wake_on_item_claim AFTER UPDATE OF lease_token ON items
  FOR EACH ROW EXECUTE FUNCTION router.trg_ack_wake_on_item_claim();

CREATE OR REPLACE FUNCTION router.trg_ack_wake_on_task_claim() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.lease_token <> '' AND OLD.lease_token = '' THEN
    UPDATE wake_events SET status='acked', ack_ref='task-lease:'||NEW.agent||':'||NEW.task_id||':'||NEW.lease_token,
           lease_token='', lease_expires='', updated=router.now_rfc3339()
    WHERE agent = NEW.agent AND status = 'leased'
      AND ((source_kind = 'ledger_task' AND source_id = NEW.task_id)
        OR (source_kind = 'requirement' AND NEW.task_id = 'requirement/'||source_id)
        OR source_kind = 'lane');
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER ack_wake_on_task_claim AFTER UPDATE OF lease_token ON tasks
  FOR EACH ROW EXECUTE FUNCTION router.trg_ack_wake_on_task_claim();

-- ── grants: the service is DML-only; DDL belongs to router_migrator ────────
GRANT USAGE ON SCHEMA router TO router_service;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA router TO router_service;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA router TO router_service;
ALTER DEFAULT PRIVILEGES IN SCHEMA router GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO router_service;
