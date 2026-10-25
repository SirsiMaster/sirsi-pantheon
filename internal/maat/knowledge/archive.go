package knowledge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
	_ "modernc.org/sqlite"
)

const (
	archiveSchemaVersion = "maat-knowledge-v1"
	filterVersion        = "local-patterns-v1"
	defaultPageSize      = 20
	maxPageSize          = 100
	maxRecordBytes       = 2 << 20
)

type ImportState string

const (
	ImportComplete     ImportState = "complete"
	ImportPartial      ImportState = "partial"
	ImportFailed       ImportState = "failed"
	ImportUnavailable  ImportState = "unavailable"
	ImportNoNewItems   ImportState = "no_new_items"
	ImportUnverifiable ImportState = "unverifiable"
)

type ResolutionState string

const (
	ResolutionOpen     ResolutionState = "open"
	ResolutionReviewed ResolutionState = "reviewed"
	ResolutionAccepted ResolutionState = "accepted"
	ResolutionRejected ResolutionState = "rejected"
)

// ArchiveRecord is Ma'at's canonical, local knowledge record. Content is
// returned only after the archive's privacy admission and projection filter.
type ArchiveRecord struct {
	ID             string               `json:"id"`
	Source         string               `json:"source"`
	Classification string               `json:"classification"`
	Resolution     ResolutionState      `json:"resolution"`
	Title          string               `json:"title"`
	Summary        string               `json:"summary"`
	Content        string               `json:"content,omitempty"`
	Excerpt        string               `json:"excerpt,omitempty"`
	References     []seshat.KIReference `json:"references"`
	Origins        []Origin             `json:"origins"`
	EquivalentIDs  []string             `json:"equivalent_ids,omitempty"`
	Annotations    []OwnerAnnotation    `json:"owner_annotations"`
	Revision       int                  `json:"revision"`
	ContentDigest  string               `json:"content_digest,omitempty"`
	ExtractedAt    time.Time            `json:"extracted_at,omitempty"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
	WithheldReason string               `json:"withheld_reason,omitempty"`
}

type Origin struct {
	RecordID     string    `json:"record_id"`
	Source       string    `json:"source"`
	InstanceID   string    `json:"instance_id"`
	NativeItemID string    `json:"native_item_id"`
	Reference    string    `json:"reference,omitempty"`
	ObservedAt   time.Time `json:"observed_at"`
}

type OwnerAnnotation struct {
	ID               int64           `json:"id"`
	State            ResolutionState `json:"state"`
	Text             string          `json:"text"`
	ReviewedRevision int             `json:"reviewed_revision"`
	ReviewedDigest   string          `json:"reviewed_digest"`
	CreatedAt        time.Time       `json:"created_at"`
}

// ProjectionAnnotation is one safe owner decision waiting for the existing
// Ma'at decision-journal projection. Its stable ID is the idempotency key.
type ProjectionAnnotation struct {
	ID        int64           `json:"id"`
	RecordID  string          `json:"record_id"`
	State     ResolutionState `json:"state"`
	Text      string          `json:"text"`
	CreatedAt time.Time       `json:"created_at"`
}

type ImportRun struct {
	ID         int64       `json:"id"`
	Source     string      `json:"source"`
	State      ImportState `json:"state"`
	StartedAt  time.Time   `json:"started_at"`
	FinishedAt time.Time   `json:"finished_at"`
	Seen       int         `json:"seen"`
	Added      int         `json:"added"`
	Versioned  int         `json:"versioned"`
	Unchanged  int         `json:"unchanged"`
	Withheld   int         `json:"withheld"`
	Rejected   int         `json:"rejected"`
	ErrorCode  string      `json:"error_code,omitempty"`
}

type Coverage struct {
	Sources               []ImportRun `json:"sources"`
	PendingProjectionDebt int         `json:"pending_projection_debt"`
}

type SearchQuery struct {
	Text           string
	Classification string
	Resolution     ResolutionState
	Source         string
	IncludeHistory bool
	WithheldOnly   bool
	Limit          int
	Cursor         string
}

type SearchPage struct {
	Items      []ArchiveRecord `json:"items"`
	Total      int             `json:"total"`
	Withheld   int             `json:"withheld"`
	Coverage   Coverage        `json:"coverage"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

type HistoryPage struct {
	Items      []ArchiveRecord `json:"items"`
	Total      int             `json:"total"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

type Archive struct {
	db       *sql.DB
	path     string
	location *secureArchiveLocation
}

type archiveQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// OpenDefault opens Ma'at's fixed local archive. It never changes ~/.sirsi
// permissions. On Darwin, a per-archive non-default VFS maps logical archive
// names through a retained private-directory descriptor and delegates SQLite
// locking, data I/O, and file sync to the pinned unix-posix VFS. Other Unix
// systems retain their existing descriptor path. The archive uses SQLite's
// DELETE journal and FULL synchronous mode; macOS also enables fullfsync.
// These guarantees depend on the filesystem and storage honoring sync requests.
// Namespace identity is rechecked at operation boundaries; this does not claim
// atomicity against a hostile same-UID process racing those checks.
func OpenDefault() (*Archive, error) {
	home, err := userHomeDir()
	if err != nil {
		return nil, fmt.Errorf("Ma'at knowledge home unavailable")
	}
	return Open(home)
}

// Open opens or initializes the canonical archive beneath home. Legacy
// Seshat data is not read here; migration is an explicit local refresh step.
func Open(home string) (*Archive, error) {
	location, err := prepareSecureArchiveLocation(home)
	if err != nil {
		return nil, err
	}
	archive, err := openPreparedArchive(location)
	if err != nil {
		_ = location.Close()
		return nil, err
	}
	return archive, nil
}

func openPreparedArchive(location *secureArchiveLocation) (*Archive, error) {
	return openPreparedArchiveWithHook(location, nil)
}

func openPreparedArchiveWithHook(location *secureArchiveLocation, beforeFirstSQLiteQuery func() error) (*Archive, error) {
	if location == nil || location.path == "" || location.verifyForSQLiteUse() != nil {
		return nil, errors.New("Ma'at knowledge archive path changed before SQLite open")
	}
	db, err := sql.Open("sqlite", archiveSQLiteDSN(location.sqlitePath(), location.vfsName))
	if err != nil {
		return nil, errors.New("Ma'at knowledge archive could not be opened")
	}
	closeDB := func() { _ = db.Close() }
	db.SetMaxOpenConns(1)
	// Acquire the actual SQLite connection before checking its identity. On
	// Darwin, the selected archive VFS resolves its logical filename beneath the
	// retained root; connection pragmas therefore cannot follow a substituted
	// canonical parent. Schema writes wait for the checks below.
	conn, err := db.Conn(context.Background())
	if err != nil {
		closeDB()
		return nil, errors.New("Ma'at knowledge archive could not be initialized")
	}
	defer conn.Close()
	if beforeFirstSQLiteQuery != nil {
		if err := beforeFirstSQLiteQuery(); err != nil {
			closeDB()
			return nil, errors.New("Ma'at knowledge archive namespace changed before SQLite query")
		}
	}
	rows, err := conn.QueryContext(context.Background(), `PRAGMA database_list`)
	if err != nil {
		closeDB()
		return nil, errors.New("Ma'at knowledge archive path could not be verified")
	}
	var databasePath string
	for rows.Next() {
		var sequence int
		var name, path string
		if err := rows.Scan(&sequence, &name, &path); err != nil {
			_ = rows.Close()
			closeDB()
			return nil, errors.New("Ma'at knowledge archive path could not be verified")
		}
		if name == "main" {
			databasePath = path
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		closeDB()
		return nil, errors.New("Ma'at knowledge archive path could not be verified")
	}
	if err := rows.Close(); err != nil || databasePath == "" || filepath.Clean(databasePath) != filepath.Clean(location.sqlitePath()) || location.verifyForSQLiteUse() != nil {
		closeDB()
		return nil, errors.New("Ma'at knowledge archive path changed during SQLite open")
	}
	// Keep this connection private and verify the canonical namespace again
	// before the first operation that can mutate the database. The DSN applies
	// durable settings to every SQLite connection the pool may create.
	if err := location.verifyForSQLiteUse(); err != nil {
		closeDB()
		return nil, errors.New("Ma'at knowledge archive path changed before initialization")
	}
	if err := configureArchiveDurability(conn); err != nil {
		closeDB()
		return nil, errors.New("Ma'at knowledge archive durable journal settings are unavailable")
	}
	// No persistent schema mutation occurs until the post-open identity check.
	if err := location.verifyForSQLiteUse(); err != nil {
		closeDB()
		return nil, errors.New("Ma'at knowledge archive path changed before initialization")
	}
	if err := initArchiveSchema(conn); err != nil {
		closeDB()
		return nil, errors.New("Ma'at knowledge archive schema is invalid")
	}
	var schema string
	if err := conn.QueryRowContext(context.Background(), `SELECT value FROM archive_meta WHERE key='schema'`).Scan(&schema); err != nil || schema != archiveSchemaVersion {
		closeDB()
		return nil, errors.New("Ma'at knowledge archive schema is unsupported")
	}
	return &Archive{db: db, path: location.path, location: location}, nil
}

func archiveSQLiteDSN(path, vfsName string) string {
	uri := url.URL{Scheme: "file", Path: path}
	query := uri.Query()
	if vfsName != "" {
		query.Set("vfs", vfsName)
	}
	query.Set("mode", "rw")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "synchronous(FULL)")
	if runtime.GOOS == "darwin" {
		query.Add("_pragma", "fullfsync(1)")
	}
	uri.RawQuery = query.Encode()
	return uri.String()
}

func configureArchiveDurability(conn *sql.Conn) error {
	var journalMode string
	if err := conn.QueryRowContext(context.Background(), `PRAGMA journal_mode=DELETE`).Scan(&journalMode); err != nil || !strings.EqualFold(journalMode, "delete") {
		return errors.New("SQLite DELETE rollback journal is not active")
	}
	return verifyArchiveDurability(conn)
}

func verifyArchiveDurability(conn *sql.Conn) error {
	var journalMode string
	if err := conn.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&journalMode); err != nil || !strings.EqualFold(journalMode, "delete") {
		return errors.New("SQLite DELETE rollback journal is not active")
	}
	var synchronous int
	if err := conn.QueryRowContext(context.Background(), `PRAGMA synchronous`).Scan(&synchronous); err != nil || synchronous != 2 {
		return errors.New("SQLite FULL synchronous mode is not active")
	}
	if runtime.GOOS == "darwin" {
		var fullfsync int
		if err := conn.QueryRowContext(context.Background(), `PRAGMA fullfsync`).Scan(&fullfsync); err != nil || fullfsync != 1 {
			return errors.New("SQLite fullfsync is not active")
		}
	}
	return nil
}

func (a *Archive) Close() error {
	if a == nil || a.db == nil {
		return nil
	}
	dbErr := a.db.Close()
	if dbErr != nil {
		// Keep the retained namespace/VFS authority alive if database/sql could
		// not confirm that every SQLite handle has closed.
		return dbErr
	}
	pathErr := a.location.Close()
	return pathErr
}

func (a *Archive) verifyLocation() error {
	if a == nil || a.location == nil {
		return errors.New("Ma'at knowledge archive path authority is unavailable")
	}
	if err := a.location.verifyForSQLiteUse(); err != nil {
		return errors.New("Ma'at knowledge archive namespace changed")
	}
	return nil
}

func initArchiveSchema(db archiveQueryer) error {
	var existingTables int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&existingTables); err != nil {
		return err
	}
	if existingTables > 0 {
		var schema string
		if err := db.QueryRowContext(context.Background(), `SELECT value FROM archive_meta WHERE key='schema'`).Scan(&schema); err != nil || schema != archiveSchemaVersion {
			return errors.New("existing Ma'at knowledge archive is not a recognized schema")
		}
	}
	_, err := db.ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS archive_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS records (
			id TEXT PRIMARY KEY,
			source TEXT NOT NULL,
			instance_id TEXT NOT NULL,
			native_item_id TEXT NOT NULL,
			classification TEXT NOT NULL,
			resolution TEXT NOT NULL DEFAULT 'open',
			title TEXT NOT NULL,
			summary TEXT NOT NULL,
			current_revision INTEGER NOT NULL DEFAULT 0,
			withheld_reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(source, instance_id, native_item_id)
		);
		CREATE TABLE IF NOT EXISTS versions (
			record_id TEXT NOT NULL REFERENCES records(id),
			revision INTEGER NOT NULL,
			content_digest TEXT NOT NULL,
			predecessor_digest TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			summary TEXT NOT NULL,
			content TEXT NOT NULL,
			references_json TEXT NOT NULL,
			extracted_at TEXT NOT NULL,
			schema_version TEXT NOT NULL,
			filter_version TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY(record_id, revision)
		);
		CREATE INDEX IF NOT EXISTS versions_digest ON versions(content_digest);
		CREATE TABLE IF NOT EXISTS origins (
			record_id TEXT NOT NULL REFERENCES records(id),
			source TEXT NOT NULL,
			instance_id TEXT NOT NULL,
			native_item_id TEXT NOT NULL,
			reference_json TEXT NOT NULL,
			first_seen TEXT NOT NULL,
			last_seen TEXT NOT NULL,
			PRIMARY KEY(record_id, source, instance_id, native_item_id, reference_json)
		);
		CREATE TABLE IF NOT EXISTS version_occurrences (
			record_id TEXT NOT NULL REFERENCES records(id),
			revision INTEGER NOT NULL,
			source TEXT NOT NULL,
			instance_id TEXT NOT NULL,
			native_item_id TEXT NOT NULL,
			reference_json TEXT NOT NULL,
			observed_at TEXT NOT NULL,
			PRIMARY KEY(record_id,revision,source,instance_id,native_item_id,reference_json)
		);
		CREATE INDEX IF NOT EXISTS version_occurrences_digest ON version_occurrences(record_id,revision);
		CREATE TABLE IF NOT EXISTS withheld_observations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_digest TEXT NOT NULL,
			safe_source TEXT NOT NULL,
			reason TEXT NOT NULL,
			observed_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS import_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			state TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			seen INTEGER NOT NULL,
			added INTEGER NOT NULL,
			versioned INTEGER NOT NULL,
			unchanged INTEGER NOT NULL,
			withheld INTEGER NOT NULL,
			rejected INTEGER NOT NULL,
			error_code TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS source_cursors (
			source TEXT PRIMARY KEY,
			cursor TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS legacy_imports (
			path_key TEXT PRIMARY KEY,
			file_digest TEXT NOT NULL,
			imported_at TEXT NOT NULL,
			state TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS annotations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_id TEXT NOT NULL REFERENCES records(id),
			state TEXT NOT NULL,
			annotation TEXT NOT NULL,
			reviewed_revision INTEGER NOT NULL DEFAULT 0,
			reviewed_digest TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS projection_debts (
			annotation_id INTEGER PRIMARY KEY REFERENCES annotations(id),
			record_id TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'pending',
			updated_at TEXT NOT NULL
		);
		CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_fts USING fts5(
			record_id UNINDEXED,
			revision UNINDEXED,
			title,
			summary,
			content,
			references_text,
			tokenize='unicode61'
		);
		INSERT OR IGNORE INTO archive_meta(key, value) VALUES ('schema', 'maat-knowledge-v1');
		INSERT OR IGNORE INTO archive_meta(key, value) VALUES ('generation', '0');
	`)
	if err != nil {
		return err
	}
	if err := ensureAnnotationVersionColumns(db); err != nil {
		return err
	}
	if _, err := db.ExecContext(context.Background(), `UPDATE records SET resolution='open' WHERE id IN (SELECT record_id FROM annotations WHERE reviewed_revision=0)`); err != nil {
		return err
	}
	// Backfill only exact version/reference matches. Older origins remain
	// immutable and are never attributed to a version with different refs.
	_, err = db.ExecContext(context.Background(), `INSERT OR IGNORE INTO version_occurrences(record_id,revision,source,instance_id,native_item_id,reference_json,observed_at)
		SELECT v.record_id,v.revision,o.source,o.instance_id,o.native_item_id,o.reference_json,o.first_seen
		FROM versions v JOIN origins o ON o.record_id=v.record_id AND o.reference_json=v.references_json`)
	return err
}

func ensureAnnotationVersionColumns(db archiveQueryer) error {
	columns := map[string]bool{}
	rows, err := db.QueryContext(context.Background(), `PRAGMA table_info(annotations)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var ordinal int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&ordinal, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		columns[name] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for name, declaration := range map[string]string{
		"reviewed_revision": "INTEGER NOT NULL DEFAULT 0",
		"reviewed_digest":   "TEXT NOT NULL DEFAULT ''",
	} {
		if !columns[name] {
			if _, err := db.ExecContext(context.Background(), `ALTER TABLE annotations ADD COLUMN `+name+` `+declaration); err != nil {
				return err
			}
		}
	}
	return nil
}

// ImportSource admits a source batch atomically after scanning every field for
// sensitive data. Returned adapter items are retained even when sourceErr is
// non-nil; that run becomes partial and never advances its cursor.
func (a *Archive) ImportSource(ctx context.Context, source string, records []seshat.SourceRecord, sourceErr error) (ImportRun, error) {
	if a == nil || a.db == nil || strings.TrimSpace(source) == "" {
		return ImportRun{}, errors.New("Ma'at knowledge archive is unavailable")
	}
	if err := a.verifyLocation(); err != nil {
		return ImportRun{}, err
	}
	if err := ctx.Err(); err != nil {
		return ImportRun{}, err
	}
	started := time.Now().UTC()
	sourceLabel, sourceSafe := safeArchiveSource(source)
	run := ImportRun{Source: sourceLabel, StartedAt: started, Seen: len(records)}
	state, code := stateForSourceError(sourceErr)
	if !sourceSafe {
		state, code = ImportUnverifiable, "source_identity_withheld"
	}
	if len(records) > 10_000 {
		run.Rejected = len(records) - 10_000
		records = records[:10_000]
		state, code = ImportPartial, "source_record_limit_exceeded"
	}
	if len(records) == 0 && sourceErr == nil && sourceSafe {
		state = ImportNoNewItems
	}
	filter := seshat.DefaultFilter()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportRun{}, errors.New("Ma'at knowledge import could not start")
	}
	defer tx.Rollback()
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return ImportRun{}, err
		}
		if record.Source != source || strings.TrimSpace(record.InstanceID) == "" || strings.TrimSpace(record.NativeItemID) == "" {
			run.Rejected++
			state, code = ImportUnverifiable, "identity_unverifiable"
			continue
		}
		if len(record.Source) > 80 || len(record.InstanceID) > 256 || len(record.NativeItemID) > 256 || len(record.Schema) > 128 || strings.ContainsAny(record.Source+record.InstanceID+record.NativeItemID+record.Schema, "\x00\r\n") {
			run.Rejected++
			state, code = ImportUnverifiable, "identity_unverifiable"
			continue
		}
		if record.ExtractedAt.IsZero() {
			record.ExtractedAt = started
		}
		if recordTooLarge(record) {
			run.Rejected++
			state, code = ImportPartial, "record_limit_exceeded"
			continue
		}
		// An unvalidated source label is never allowed into ordinary identity,
		// provenance, cursor, or search columns even when it does not match a
		// known-secret pattern. Preserve only the withheld opaque identity.
		if !sourceSafe || sensitiveRecord(filter, record) {
			if err := a.importWithheld(ctx, tx, record, started); err != nil {
				return ImportRun{}, errors.New("Ma'at knowledge import could not commit")
			}
			run.Withheld++
			continue
		}
		added, versioned, unchanged, err := importRecord(ctx, tx, record, started)
		if err != nil {
			return ImportRun{}, errors.New("Ma'at knowledge import could not commit")
		}
		run.Added += added
		run.Versioned += versioned
		run.Unchanged += unchanged
	}
	if run.Rejected > 0 && state == ImportComplete {
		state = ImportPartial
		code = "records_rejected"
	}
	if sourceErr != nil && len(records) > 0 {
		state = ImportPartial
		if code == "" || code == "source_unavailable" || code == "source_failed" {
			code = "source_partially_readable"
		}
	}
	run.State, run.ErrorCode, run.FinishedAt = state, code, time.Now().UTC()
	if sourceSafe && (state == ImportComplete || state == ImportNoNewItems) {
		cursor := run.FinishedAt.Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO source_cursors(source,cursor,updated_at) VALUES(?,?,?) ON CONFLICT(source) DO UPDATE SET cursor=excluded.cursor,updated_at=excluded.updated_at`, sourceLabel, cursor, run.FinishedAt.Format(time.RFC3339Nano)); err != nil {
			return ImportRun{}, errors.New("Ma'at knowledge source cursor could not be committed")
		}
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO import_runs(source,state,started_at,finished_at,seen,added,versioned,unchanged,withheld,rejected,error_code) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, run.Source, run.State, run.StartedAt.Format(time.RFC3339Nano), run.FinishedAt.Format(time.RFC3339Nano), run.Seen, run.Added, run.Versioned, run.Unchanged, run.Withheld, run.Rejected, run.ErrorCode)
	if err != nil {
		return ImportRun{}, errors.New("Ma'at knowledge coverage could not be committed")
	}
	run.ID, _ = res.LastInsertId()
	// Coverage is part of each search projection, so a newly committed run
	// invalidates page cursors even when the source returned no records.
	if _, err := tx.ExecContext(ctx, `UPDATE archive_meta SET value=CAST(value AS INTEGER)+1 WHERE key='generation'`); err != nil {
		return ImportRun{}, errors.New("Ma'at knowledge archive generation could not be committed")
	}
	if err := a.verifyLocation(); err != nil {
		return ImportRun{}, err
	}
	if err := tx.Commit(); err != nil {
		return ImportRun{}, errors.New("Ma'at knowledge import could not commit")
	}
	return run, nil
}

func stateForSourceError(err error) (ImportState, string) {
	if err == nil {
		return ImportComplete, ""
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "not found"), strings.Contains(text, "unavailable"):
		return ImportUnavailable, "source_unavailable"
	default:
		return ImportFailed, "source_failed"
	}
}

func recordTooLarge(record seshat.SourceRecord) bool {
	refs, _ := json.Marshal(record.References)
	return len(record.Title)+len(record.Summary)+len(record.Content)+len(refs)+len(record.Source)+len(record.InstanceID)+len(record.NativeItemID) > maxRecordBytes
}

func sensitiveRecord(filter *seshat.SecretsFilter, record seshat.SourceRecord) bool {
	fields := []string{record.Source, record.InstanceID, record.NativeItemID, record.Schema, record.Title, record.Summary, record.Content}
	for _, ref := range record.References {
		fields = append(fields, ref.Type, ref.Value)
	}
	content := strings.Join(fields, "\n")
	return len(filter.Scan(content)) != 0 || hasSystemOneSensitiveContent(content)
}

func safeArchiveSource(source string) (string, bool) {
	known := map[string]bool{
		"apple-notes": true, "chrome-history": true, "claude": true,
		"gemini": true, "google-workspace": true, "legacy-seshat": true,
	}
	if !known[source] {
		return "withheld", false
	}
	if len(source) == 0 || len(source) > 80 || source[0] < 'a' || source[0] > 'z' {
		return "withheld", false
	}
	for _, value := range source {
		if (value < 'a' || value > 'z') && (value < '0' || value > '9') && value != '-' {
			return "withheld", false
		}
	}
	if len(seshat.DefaultFilter().Scan(source)) != 0 || hasSystemOneSensitiveContent(source) {
		return "withheld", false
	}
	return source, true
}

func opaqueIdentity(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "opaque:" + hex.EncodeToString(digest[:])
}

func recordID(record seshat.SourceRecord) string {
	identity, _ := json.Marshal([]string{"maat-knowledge-id-v1", record.Source, record.InstanceID, record.NativeItemID})
	digest := sha256.Sum256(identity)
	return hex.EncodeToString(digest[:])
}

func contentDigest(record seshat.SourceRecord) string {
	canonical, _ := json.Marshal([]string{record.Title, record.Summary, record.Content})
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}

func classifySource(source string) string {
	switch source {
	case "claude", "gemini":
		return "conversation"
	case "chrome-history":
		return "search"
	case "apple-notes":
		return "note"
	case "google-workspace":
		return "document"
	default:
		return "unclassified"
	}
}

func importRecord(ctx context.Context, tx *sql.Tx, record seshat.SourceRecord, now time.Time) (added, versioned, unchanged int, err error) {
	id, digest := recordID(record), contentDigest(record)
	refs, err := json.Marshal(record.References)
	if err != nil {
		return 0, 0, 0, err
	}
	stamp := now.Format(time.RFC3339Nano)
	classification := classifySource(record.Source)
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO records(id,source,instance_id,native_item_id,classification,resolution,title,summary,created_at,updated_at) VALUES(?,?,?,?,?,'open',?,?,?,?)`, id, record.Source, record.InstanceID, record.NativeItemID, classification, record.Title, record.Summary, stamp, stamp)
	if err != nil {
		return 0, 0, 0, err
	}
	rows, _ := res.RowsAffected()
	var revision int
	var previousDigest string
	var previousReferences string
	var previousSchema string
	if err := tx.QueryRowContext(ctx, `SELECT current_revision FROM records WHERE id=?`, id).Scan(&revision); err != nil {
		return 0, 0, 0, err
	}
	if revision > 0 {
		if err := tx.QueryRowContext(ctx, `SELECT content_digest,references_json,schema_version FROM versions WHERE record_id=? AND revision=?`, id, revision).Scan(&previousDigest, &previousReferences, &previousSchema); err != nil {
			return 0, 0, 0, err
		}
	}
	versionSchema := archiveSchemaVersion + ":" + record.Schema
	if revision > 0 && previousDigest == digest && previousReferences == string(refs) && previousSchema == versionSchema {
		unchanged = 1
		if _, err := tx.ExecContext(ctx, `UPDATE records SET source=?,instance_id=?,native_item_id=?,classification=?,title=?,summary=?,withheld_reason='',updated_at=? WHERE id=?`, record.Source, record.InstanceID, record.NativeItemID, classification, record.Title, record.Summary, stamp, id); err != nil {
			return 0, 0, 0, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_fts WHERE record_id=? AND revision=?`, id, revision); err != nil {
			return 0, 0, 0, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_fts(record_id,revision,title,summary,content,references_text) VALUES(?,?,?,?,?,?)`, id, revision, record.Title, record.Summary, record.Content, referencesText(record.References)); err != nil {
			return 0, 0, 0, err
		}
	} else {
		revision++
		if _, err := tx.ExecContext(ctx, `INSERT INTO versions(record_id,revision,content_digest,predecessor_digest,title,summary,content,references_json,extracted_at,schema_version,filter_version,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, revision, digest, previousDigest, record.Title, record.Summary, record.Content, string(refs), record.ExtractedAt.UTC().Format(time.RFC3339Nano), versionSchema, filterVersion, stamp); err != nil {
			return 0, 0, 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE records SET source=?,instance_id=?,native_item_id=?,classification=?,resolution='open',title=?,summary=?,current_revision=?,withheld_reason='',updated_at=? WHERE id=?`, record.Source, record.InstanceID, record.NativeItemID, classification, record.Title, record.Summary, revision, stamp, id); err != nil {
			return 0, 0, 0, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_fts(record_id,revision,title,summary,content,references_text) VALUES(?,?,?,?,?,?)`, id, revision, record.Title, record.Summary, record.Content, referencesText(record.References)); err != nil {
			return 0, 0, 0, err
		}
		if rows == 0 {
			versioned = 1
		}
	}
	originJSON, err := json.Marshal(record.References)
	if err != nil {
		return 0, 0, 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO origins(record_id,source,instance_id,native_item_id,reference_json,first_seen,last_seen) VALUES(?,?,?,?,?,?,?)`, id, record.Source, record.InstanceID, record.NativeItemID, string(originJSON), stamp, stamp); err != nil {
		return 0, 0, 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO version_occurrences(record_id,revision,source,instance_id,native_item_id,reference_json,observed_at) VALUES(?,?,?,?,?,?,?)`, id, revision, record.Source, record.InstanceID, record.NativeItemID, string(originJSON), stamp); err != nil {
		return 0, 0, 0, err
	}
	if rows == 1 {
		added = 1
	}
	return added, versioned, unchanged, nil
}

func (a *Archive) importWithheld(ctx context.Context, tx *sql.Tx, record seshat.SourceRecord, now time.Time) error {
	id := recordID(record)
	stamp := now.Format(time.RFC3339Nano)
	source, sourceSafe := safeArchiveSource(record.Source)
	if !sourceSafe {
		source = "withheld"
	}
	instanceID := opaqueIdentity("instance\x00" + record.Source + "\x00" + record.InstanceID)
	nativeID := opaqueIdentity("native-item\x00" + record.Source + "\x00" + record.NativeItemID)
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO records(id,source,instance_id,native_item_id,classification,resolution,title,summary,current_revision,withheld_reason,created_at,updated_at) VALUES(?,?,?,?,'withheld','open','Withheld sensitive source item','',0,'sensitivity_rule',?,?)`, id, source, instanceID, nativeID, stamp, stamp)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE records SET withheld_reason='sensitivity_rule',updated_at=? WHERE id=?`, stamp, id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO withheld_observations(record_digest,safe_source,reason,observed_at) VALUES(?,?,?,?)`, id, source, "sensitivity_rule", stamp)
	return err
}

func (a *Archive) Coverage(ctx context.Context, limit int) (Coverage, error) {
	if err := a.verifyLocation(); err != nil {
		return Coverage{}, err
	}
	return a.coverageWith(ctx, a.db, limit)
}

func (a *Archive) coverageWith(ctx context.Context, db archiveQueryer, limit int) (Coverage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `SELECT id,source,state,started_at,finished_at,seen,added,versioned,unchanged,withheld,rejected,error_code FROM import_runs WHERE id IN (SELECT MAX(id) FROM import_runs GROUP BY source) ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return Coverage{}, errors.New("Ma'at knowledge coverage is unavailable")
	}
	defer rows.Close()
	coverage := Coverage{Sources: []ImportRun{}}
	for rows.Next() {
		var run ImportRun
		var state, started, finished string
		if err := rows.Scan(&run.ID, &run.Source, &state, &started, &finished, &run.Seen, &run.Added, &run.Versioned, &run.Unchanged, &run.Withheld, &run.Rejected, &run.ErrorCode); err != nil {
			return Coverage{}, errors.New("Ma'at knowledge coverage is malformed")
		}
		run.State = ImportState(state)
		run.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
		run.FinishedAt, _ = time.Parse(time.RFC3339Nano, finished)
		coverage.Sources = append(coverage.Sources, run)
	}
	if err := rows.Err(); err != nil {
		return Coverage{}, errors.New("Ma'at knowledge coverage could not finish")
	}
	if err := rows.Close(); err != nil {
		return Coverage{}, errors.New("Ma'at knowledge coverage could not finish")
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projection_debts WHERE state='pending'`).Scan(&coverage.PendingProjectionDebt); err != nil {
		return Coverage{}, errors.New("Ma'at knowledge projection debt is unavailable")
	}
	return coverage, nil
}

func (a *Archive) Detail(ctx context.Context, id string, revision int) (ArchiveRecord, error) {
	if err := a.verifyLocation(); err != nil {
		return ArchiveRecord{}, err
	}
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ArchiveRecord{}, errors.New("Ma'at knowledge read snapshot is unavailable")
	}
	defer tx.Rollback()
	record, err := a.detailWith(ctx, tx, id, revision)
	if err != nil {
		return ArchiveRecord{}, err
	}
	if err := a.verifyLocation(); err != nil {
		return ArchiveRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return ArchiveRecord{}, errors.New("Ma'at knowledge read snapshot could not finish")
	}
	return record, nil
}

func (a *Archive) detailWith(ctx context.Context, db archiveQueryer, id string, revision int) (ArchiveRecord, error) {
	var record ArchiveRecord
	var created, updated string
	var currentRevision int
	err := db.QueryRowContext(ctx, `SELECT id,source,classification,resolution,title,summary,current_revision,withheld_reason,created_at,updated_at FROM records WHERE id=?`, id).Scan(&record.ID, &record.Source, &record.Classification, &record.Resolution, &record.Title, &record.Summary, &currentRevision, &record.WithheldReason, &created, &updated)
	if err != nil {
		return ArchiveRecord{}, errors.New("Ma'at knowledge record is unavailable")
	}
	if revision <= 0 {
		revision = currentRevision
	}
	var refs, extracted string
	if revision > 0 {
		if err := db.QueryRowContext(ctx, `SELECT content_digest,title,summary,content,references_json,extracted_at FROM versions WHERE record_id=? AND revision=?`, id, revision).Scan(&record.ContentDigest, &record.Title, &record.Summary, &record.Content, &refs, &extracted); err != nil {
			return ArchiveRecord{}, errors.New("Ma'at knowledge version is unavailable")
		}
		_ = json.Unmarshal([]byte(refs), &record.References)
		if parsed, parseErr := time.Parse(time.RFC3339Nano, extracted); parseErr == nil {
			record.ExtractedAt = parsed
		}
	}
	record.Revision = revision
	record.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	record.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	record.Origins, record.EquivalentIDs, err = a.originsForVersionWith(ctx, db, id, revision, record.ContentDigest)
	if err != nil {
		return ArchiveRecord{}, err
	}
	record.Annotations, err = a.annotationsWith(ctx, db, id)
	if err != nil {
		return ArchiveRecord{}, err
	}
	if revision != currentRevision {
		record.Resolution = ResolutionOpen
	}
	for _, annotation := range record.Annotations {
		if annotation.ReviewedRevision == revision && annotation.ReviewedDigest == record.ContentDigest {
			record.Resolution = annotation.State
			break
		}
	}
	if record.WithheldReason != "" {
		record.Content = ""
		record.ContentDigest = ""
		record.References = nil
		record.Origins = nil
		record.Annotations = nil
		record.Title = "Withheld sensitive source item"
		record.Summary = ""
		record.Classification = "withheld"
		return record, nil
	}
	if sensitiveRecord(seshat.DefaultFilter(), seshat.SourceRecord{
		Source: record.Source, InstanceID: record.ID, NativeItemID: record.ID,
		Title: record.Title, Summary: record.Summary, Content: record.Content,
		References: record.References,
	}) {
		record.Content = ""
		record.ContentDigest = ""
		record.References = nil
		record.Origins = nil
		record.Annotations = nil
		record.Title = "Withheld sensitive source item"
		record.Summary = ""
		record.Classification = "withheld"
		record.WithheldReason = "sensitivity_rule"
		return record, nil
	}
	filteredAnnotations := record.Annotations[:0]
	for _, annotation := range record.Annotations {
		if len(seshat.DefaultFilter().Scan(annotation.Text)) == 0 && !hasSystemOneSensitiveContent(annotation.Text) {
			filteredAnnotations = append(filteredAnnotations, annotation)
		}
	}
	record.Annotations = filteredAnnotations
	return record, nil
}

func (a *Archive) annotationsWith(ctx context.Context, db archiveQueryer, id string) ([]OwnerAnnotation, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,state,annotation,reviewed_revision,reviewed_digest,created_at FROM annotations WHERE record_id=? ORDER BY id DESC LIMIT 50`, id)
	if err != nil {
		return nil, errors.New("Ma'at knowledge annotations are unavailable")
	}
	defer rows.Close()
	items := []OwnerAnnotation{}
	for rows.Next() {
		var item OwnerAnnotation
		var state, created string
		if err := rows.Scan(&item.ID, &state, &item.Text, &item.ReviewedRevision, &item.ReviewedDigest, &created); err != nil {
			return nil, errors.New("Ma'at knowledge annotations are malformed")
		}
		item.State = ResolutionState(state)
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *Archive) History(ctx context.Context, id string) ([]ArchiveRecord, error) {
	page, err := a.HistoryPage(ctx, id, 100, "")
	if err != nil {
		return nil, err
	}
	if page.NextCursor != "" {
		return nil, errors.New("knowledge history exceeds the compatibility bound; use paginated history")
	}
	return page.Items, nil
}

func (a *Archive) HistoryPage(ctx context.Context, id string, limit int, cursorValue string) (HistoryPage, error) {
	if err := a.verifyLocation(); err != nil {
		return HistoryPage{}, err
	}
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return HistoryPage{}, errors.New("Ma'at knowledge read snapshot is unavailable")
	}
	defer tx.Rollback()
	page, err := a.historyPageSnapshot(ctx, tx, id, limit, cursorValue)
	if err != nil {
		return HistoryPage{}, err
	}
	if err := a.verifyLocation(); err != nil {
		return HistoryPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return HistoryPage{}, errors.New("Ma'at knowledge read snapshot could not finish")
	}
	return page, nil
}

func (a *Archive) historyPageSnapshot(ctx context.Context, db archiveQueryer, id string, limit int, cursorValue string) (HistoryPage, error) {
	if limit < 0 || limit > maxPageSize {
		return HistoryPage{}, errors.New("knowledge history limit is outside its bound")
	}
	if limit == 0 {
		limit = defaultPageSize
	}
	queryHash := historyQueryHash(id)
	var generation int64
	if err := db.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM archive_meta WHERE key='generation'`).Scan(&generation); err != nil {
		return HistoryPage{}, errors.New("Ma'at knowledge archive generation is unavailable")
	}
	offset := 0
	if cursorValue != "" {
		cursor, err := decodeSearchCursor(cursorValue)
		if err != nil || cursor.QueryHash != queryHash || cursor.Offset < 0 || cursor.Generation != generation {
			return HistoryPage{}, errors.New("Ma'at knowledge history cursor is invalid or stale")
		}
		offset = cursor.Offset
	}
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM versions WHERE record_id=?`, id).Scan(&total); err != nil {
		return HistoryPage{}, errors.New("Ma'at knowledge history is unavailable")
	}
	rows, err := db.QueryContext(ctx, `SELECT revision FROM versions WHERE record_id=? ORDER BY revision DESC LIMIT ? OFFSET ?`, id, limit+1, offset)
	if err != nil {
		return HistoryPage{}, errors.New("Ma'at knowledge history is unavailable")
	}
	var revisions []int
	for rows.Next() {
		var revision int
		if err := rows.Scan(&revision); err != nil {
			_ = rows.Close()
			return HistoryPage{}, errors.New("Ma'at knowledge history is malformed")
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return HistoryPage{}, errors.New("Ma'at knowledge history could not finish")
	}
	if err := rows.Close(); err != nil {
		return HistoryPage{}, errors.New("Ma'at knowledge history could not finish")
	}
	more := len(revisions) > limit
	if more {
		revisions = revisions[:limit]
	}
	page := HistoryPage{Items: make([]ArchiveRecord, 0, len(revisions)), Total: total}
	for _, revision := range revisions {
		item, err := a.detailWith(ctx, db, id, revision)
		if err != nil {
			return HistoryPage{}, err
		}
		page.Items = append(page.Items, item)
	}
	if more {
		page.NextCursor = encodeSearchCursor(searchCursor{Generation: generation, Offset: offset + limit, QueryHash: queryHash})
	}
	return page, nil
}

func historyQueryHash(id string) string {
	digest := sha256.Sum256([]byte("maat-history-v1\x00" + id))
	return hex.EncodeToString(digest[:])
}

func (a *Archive) originsForVersionWith(ctx context.Context, db archiveQueryer, id string, revision int, digest string) ([]Origin, []string, error) {
	query := `SELECT o.record_id,o.source,o.instance_id,o.native_item_id,o.reference_json,o.observed_at
		FROM version_occurrences o JOIN records r ON r.id=o.record_id
		JOIN versions v ON v.record_id=o.record_id AND v.revision=o.revision
		WHERE r.withheld_reason='' AND ((o.record_id=? AND o.revision=?) OR (o.record_id<>? AND o.revision=r.current_revision AND v.content_digest=?))
		ORDER BY o.source,o.instance_id,o.native_item_id,o.reference_json`
	if digest == "" {
		query = `SELECT o.record_id,o.source,o.instance_id,o.native_item_id,o.reference_json,o.observed_at
			FROM version_occurrences o JOIN records r ON r.id=o.record_id
			WHERE r.withheld_reason='' AND o.record_id=? AND o.revision=? ORDER BY o.source,o.instance_id,o.native_item_id,o.reference_json`
	}
	args := []any{id, revision, id, digest}
	if digest == "" {
		args = []any{id, revision}
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, errors.New("Ma'at knowledge provenance is unavailable")
	}
	defer rows.Close()
	origins := []Origin{}
	ids := []string{}
	seenIDs := map[string]bool{}
	for rows.Next() {
		var origin Origin
		var refs, observed string
		if err := rows.Scan(&origin.RecordID, &origin.Source, &origin.InstanceID, &origin.NativeItemID, &refs, &observed); err != nil {
			return nil, nil, errors.New("Ma'at knowledge provenance is malformed")
		}
		var parsedRefs []seshat.KIReference
		_ = json.Unmarshal([]byte(refs), &parsedRefs)
		origin.Reference = strings.Join(referenceStrings(parsedRefs), " ")
		origin.ObservedAt, _ = time.Parse(time.RFC3339Nano, observed)
		origins = append(origins, origin)
		if !seenIDs[origin.RecordID] {
			seenIDs[origin.RecordID] = true
			ids = append(ids, origin.RecordID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return origins, ids, nil
}

func referenceStrings(refs []seshat.KIReference) []string {
	values := make([]string, 0, len(refs))
	for _, ref := range refs {
		values = append(values, ref.Type+":"+ref.Value)
	}
	return values
}

func (a *Archive) RebuildIndex(ctx context.Context) error {
	if err := a.verifyLocation(); err != nil {
		return err
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("Ma'at knowledge index rebuild could not start")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_fts`); err != nil {
		return errors.New("Ma'at knowledge index rebuild failed")
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.id,r.source,r.instance_id,r.native_item_id,v.revision,v.title,v.summary,v.content,v.references_json FROM records r JOIN versions v ON v.record_id=r.id WHERE r.withheld_reason='' ORDER BY r.id,v.revision`)
	if err != nil {
		return errors.New("Ma'at knowledge index rebuild failed")
	}
	filter := seshat.DefaultFilter()
	for rows.Next() {
		var id, source, instance, nativeID, title, summary, content, refs string
		var revision int
		if err := rows.Scan(&id, &source, &instance, &nativeID, &revision, &title, &summary, &content, &refs); err != nil {
			_ = rows.Close()
			return errors.New("Ma'at knowledge index rebuild failed")
		}
		var parsedRefs []seshat.KIReference
		if err := json.Unmarshal([]byte(refs), &parsedRefs); err != nil {
			_ = rows.Close()
			return errors.New("Ma'at knowledge index rebuild failed")
		}
		record := seshat.SourceRecord{Source: source, InstanceID: instance, NativeItemID: nativeID, Title: title, Summary: summary, Content: content, References: parsedRefs}
		if !sensitiveRecord(filter, record) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_fts(record_id,revision,title,summary,content,references_text) VALUES(?,?,?,?,?,?)`, id, revision, title, summary, content, refs); err != nil {
				_ = rows.Close()
				return errors.New("Ma'at knowledge index rebuild failed")
			}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return errors.New("Ma'at knowledge index rebuild failed")
	}
	if err := rows.Close(); err != nil {
		return errors.New("Ma'at knowledge index rebuild failed")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE archive_meta SET value=CAST(value AS INTEGER)+1 WHERE key='generation'`); err != nil {
		return errors.New("Ma'at knowledge index generation failed")
	}
	if err := a.verifyLocation(); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordOwnerAnnotation stores an explicit owner decision separately from
// imported source versions and opens a Casebook-projection debt in the same
// transaction. Callers must project that annotation through Ma'at's existing
// decision journal and then acknowledge it with MarkProjected.
func (a *Archive) RecordOwnerAnnotation(ctx context.Context, recordID string, expectedRevision int, expectedDigest string, state ResolutionState, annotation string) (int64, error) {
	if state != ResolutionReviewed && state != ResolutionAccepted && state != ResolutionRejected {
		return 0, errors.New("knowledge resolution must be reviewed, accepted, or rejected")
	}
	if len(annotation) > 4000 || strings.TrimSpace(recordID) == "" || strings.TrimSpace(annotation) == "" || expectedRevision < 1 || len(expectedDigest) != 64 {
		return 0, errors.New("knowledge resolution exceeds its bound")
	}
	if err := a.verifyLocation(); err != nil {
		return 0, err
	}
	filter := seshat.DefaultFilter()
	if len(filter.Scan(annotation)) != 0 || hasSystemOneSensitiveContent(annotation) {
		return 0, errors.New("sensitive owner annotations cannot be retained")
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, errors.New("knowledge resolution could not start")
	}
	defer tx.Rollback()
	var currentRevision int
	var currentDigest string
	if err := tx.QueryRowContext(ctx, `SELECT r.current_revision,v.content_digest FROM records r JOIN versions v ON v.record_id=r.id AND v.revision=r.current_revision WHERE r.id=? AND r.withheld_reason=''`, recordID).Scan(&currentRevision, &currentDigest); err != nil {
		return 0, errors.New("knowledge record is unavailable for resolution")
	}
	if currentRevision != expectedRevision || currentDigest != expectedDigest {
		return 0, errors.New("knowledge item changed since it was reviewed; reopen its current detail before resolving")
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx, `INSERT INTO annotations(record_id,state,annotation,reviewed_revision,reviewed_digest,created_at) VALUES(?,?,?,?,?,?)`, recordID, state, strings.TrimSpace(annotation), expectedRevision, expectedDigest, stamp)
	if err != nil {
		return 0, errors.New("knowledge resolution could not be retained")
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, errors.New("knowledge resolution receipt is unavailable")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE records SET resolution=?,updated_at=? WHERE id=?`, state, stamp, recordID); err != nil {
		return 0, errors.New("knowledge resolution could not be retained")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO projection_debts(annotation_id,record_id,state,updated_at) VALUES(?,?,'pending',?)`, id, recordID, stamp); err != nil {
		return 0, errors.New("Casebook projection debt could not be recorded")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE archive_meta SET value=CAST(value AS INTEGER)+1 WHERE key='generation'`); err != nil {
		return 0, errors.New("knowledge resolution generation could not be updated")
	}
	if err := a.verifyLocation(); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, errors.New("knowledge resolution could not be committed")
	}
	return id, nil
}

func (a *Archive) MarkProjected(ctx context.Context, annotationID int64) error {
	if err := a.verifyLocation(); err != nil {
		return err
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("Casebook projection acknowledgement failed")
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE projection_debts SET state='projected',updated_at=? WHERE annotation_id=? AND state='pending'`, time.Now().UTC().Format(time.RFC3339Nano), annotationID)
	if err != nil {
		return errors.New("Casebook projection acknowledgement failed")
	}
	if changed, _ := res.RowsAffected(); changed == 0 {
		var state string
		if err := tx.QueryRowContext(ctx, `SELECT state FROM projection_debts WHERE annotation_id=?`, annotationID).Scan(&state); err != nil || state != "projected" {
			return errors.New("Casebook projection debt is unavailable")
		}
	}
	if err := a.verifyLocation(); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *Archive) PendingProjectionDebts(ctx context.Context, limit int) ([]int64, error) {
	if err := a.verifyLocation(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := a.db.QueryContext(ctx, `SELECT annotation_id FROM projection_debts WHERE state='pending' ORDER BY annotation_id LIMIT ?`, limit)
	if err != nil {
		return nil, errors.New("Casebook projection debt is unavailable")
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, errors.New("Casebook projection debt is malformed")
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (a *Archive) PendingProjectionAnnotations(ctx context.Context, limit int) ([]ProjectionAnnotation, error) {
	if err := a.verifyLocation(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := a.db.QueryContext(ctx, `SELECT d.annotation_id,d.record_id,a.state,a.annotation,a.created_at FROM projection_debts d JOIN annotations a ON a.id=d.annotation_id WHERE d.state='pending' ORDER BY d.annotation_id LIMIT ?`, limit)
	if err != nil {
		return nil, errors.New("Casebook projection debt is unavailable")
	}
	defer rows.Close()
	items := make([]ProjectionAnnotation, 0, limit)
	for rows.Next() {
		var item ProjectionAnnotation
		var state, created string
		if err := rows.Scan(&item.ID, &item.RecordID, &state, &item.Text, &created); err != nil {
			return nil, errors.New("Casebook projection debt is malformed")
		}
		item.State = ResolutionState(state)
		parsed, parseErr := time.Parse(time.RFC3339Nano, created)
		if parseErr != nil || item.ID <= 0 || item.RecordID == "" || (item.State != ResolutionReviewed && item.State != ResolutionAccepted && item.State != ResolutionRejected) {
			return nil, errors.New("Casebook projection debt is malformed")
		}
		if len(seshat.DefaultFilter().Scan(item.Text)) != 0 || hasSystemOneSensitiveContent(item.Text) {
			return nil, errors.New("sensitive owner decision remains withheld from Casebook projection")
		}
		item.CreatedAt = parsed
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New("Casebook projection debt could not finish")
	}
	return items, nil
}
