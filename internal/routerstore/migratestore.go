package routerstore

// ADR-062 rs-12: move a ledger between backends with proof.
//
// The proof shape SSA required (ADR-062 rev 3, Migration step 3):
//   (a) quiesce           — caller holds the fabric-quarantine marker for the
//                            whole interval; MigrateStore re-dumps the source
//                            after import and refuses if it moved.
//   (b) canonical dump    — every table, every row, PK order, as JSON; a
//                            SHA-256 per table and overall. Includes leases,
//                            tombstones and ordering columns because it is
//                            SELECT *, not a projection.
//   (c) dry run           — reports what it would write, writes nothing.
//   (d) import            — INSERT … ON CONFLICT DO NOTHING per row.
//   (e) full diff         — destination dump must hash-equal the source dump.
//   (f) idempotence       — a second import writes zero rows, same hash.
//
// Works on any two *SQLiteStore handles (the type carries a dialect), so
// SQLite→SQLite, SQLite→Postgres and Postgres→Postgres all use one path.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// migratedTable lists a table and the columns that order its rows.
type migratedTable struct {
	name string
	pk   []string
}

// migratedTables is every table in schema v18, in dependency-neutral order
// (no foreign keys exist; order only affects the dump's byte layout).
// router.schema_version (Postgres only) is deliberately absent: it is the
// destination's own version stamp, verified by OpenPostgres, never copied.
var migratedTables = []migratedTable{
	{"items", []string{"id"}},
	{"agents", []string{"id"}},
	{"state", []string{"key"}},
	{"breakers", []string{"domain"}},
	{"send_quota", []string{"sender", "bucket"}},
	{"counters", []string{"name"}},
	{"tasks", []string{"agent", "task_id"}},
	{"identifiers", []string{"namespace", "number"}},
	{"requirements", []string{"req_id"}},
	{"wake_events", []string{"event_id"}},
	{"threads", []string{"thread_id"}},
	{"sessions", []string{"session_id"}},
	{"lease_sessions", []string{"kind", "key"}},
	{"host_tokens", []string{"token_id"}},
}

// MigrateOptions tune MigrateStore.
type MigrateOptions struct {
	DryRun bool
	// ScrubNUL strips 0x00 bytes from every text cell, in the source dump AND
	// on insert, so the hashes still compare. Postgres text cannot hold NUL
	// (SQLSTATE 22021); without this flag a source containing one is REFUSED
	// with the offending (table, key, column) listed — the owner decides.
	ScrubNUL bool
}

// TableDump is one table's canonical form.
type TableDump struct {
	Table   string   `json:"table"`
	Columns []string `json:"columns"`
	Rows    int      `json:"rows"`
	SHA256  string   `json:"sha256"`
	rows    [][]any
}

// Dump is a whole-ledger canonical dump.
type Dump struct {
	Tables []TableDump `json:"tables"`
	SHA256 string      `json:"sha256"` // over every table hash, in table order
}

// CanonicalDump reads every table in PK order and hashes it.
func CanonicalDump(s *SQLiteStore) (Dump, error) { return canonicalDump(s, false) }

func canonicalDump(s *SQLiteStore, scrubNUL bool) (Dump, error) {
	var d Dump
	overall := sha256.New()
	for _, t := range migratedTables {
		td, err := dumpTable(s, t, scrubNUL)
		if err != nil {
			return Dump{}, err
		}
		d.Tables = append(d.Tables, td)
		overall.Write([]byte(td.Table + ":" + td.SHA256 + "\n"))
	}
	d.SHA256 = hex.EncodeToString(overall.Sum(nil))
	return d, nil
}

func dumpTable(s *SQLiteStore, t migratedTable, scrubNUL bool) (TableDump, error) {
	rows, err := s.db.Query("SELECT * FROM " + t.name + " ORDER BY " + strings.Join(t.pk, ", "))
	if err != nil {
		return TableDump{}, fmt.Errorf("dump %s: %w", t.name, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return TableDump{}, err
	}
	// Column order is engine-defined; canonicalize by sorting names and
	// permuting values to match, so both engines hash identically.
	order := make([]int, len(cols))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return cols[order[a]] < cols[order[b]] })
	sortedCols := make([]string, len(cols))
	for i, idx := range order {
		sortedCols[i] = cols[idx]
	}
	h := sha256.New()
	td := TableDump{Table: t.name, Columns: sortedCols}
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return TableDump{}, fmt.Errorf("dump %s: scan: %w", t.name, err)
		}
		if scrubNUL {
			for i := range raw {
				if str, ok := raw[i].(string); ok && strings.ContainsRune(str, 0) {
					raw[i] = strings.ReplaceAll(str, "\x00", "")
				}
			}
		}
		vals := make([]any, len(cols))
		for i, idx := range order {
			vals[i] = normalizeCell(raw[idx])
		}
		line, err := json.Marshal(vals)
		if err != nil {
			return TableDump{}, err
		}
		h.Write(line)
		h.Write([]byte{'\n'})
		td.rows = append(td.rows, raw) // engine column order, for re-insert
		td.Rows++
	}
	if err := rows.Err(); err != nil {
		return TableDump{}, err
	}
	td.SHA256 = hex.EncodeToString(h.Sum(nil))
	td.rows = append([][]any{{cols}}, td.rows...) // row 0 carries the column names
	return td, nil
}

// normalizeCell makes the two engines' driver types hash the same way:
// every integer kind → int64, []byte → base64 via json, text → string.
func normalizeCell(v any) any {
	switch x := v.(type) {
	case int32:
		return int64(x)
	case int:
		return int64(x)
	case []byte:
		return x // json encodes as base64 on both engines
	case string:
		return x
	default:
		return v
	}
}

// MigrateReport is what MigrateStore proves.
type MigrateReport struct {
	DryRun      bool           `json:"dry_run"`
	Source      Dump           `json:"source"`
	SourceAfter string         `json:"source_after_sha256"` // must equal Source.SHA256
	Destination Dump           `json:"destination,omitempty"`
	Wrote       map[string]int `json:"wrote"` // rows inserted per table (0 on dry run)
	WouldWrite  map[string]int `json:"would_write"`
	Idempotent  bool           `json:"idempotent"` // set by the caller's second pass
	Notes       []string       `json:"notes,omitempty"`
	// NULCells lists (table key column) cells holding a 0x00 byte in the source.
	NULCells []string `json:"nul_cells,omitempty"`
	// TriggerExtrasRemoved counts destination wake_events minted by the
	// destination's own insert triggers for rows whose source had no event
	// (pre-v10 items); they are deleted after import so dst == src exactly.
	TriggerExtrasRemoved int `json:"trigger_extras_removed"`
}

// ErrSourceHasNUL is returned when a text cell contains 0x00 and ScrubNUL is off.
var ErrSourceHasNUL = errors.New("routerstore: source contains NUL bytes in text cells (Postgres cannot store them); re-run with ScrubNUL/--scrub-nul to strip them, or clean the rows listed")

var ErrSourceMoved = errors.New("routerstore: source ledger changed during migration; not quiesced — aborting, destination may be partial")

// MigrateStore copies src into dst. On a real run the destination dump must
// hash-equal the source dump or an error is returned (the rows are already
// written; the caller decides whether to keep them — they are a strict
// subset of the source, never divergent, because every insert is DO NOTHING).
func MigrateStore(src, dst *SQLiteStore, opts MigrateOptions) (MigrateReport, error) {
	dryRun := opts.DryRun
	rep := MigrateReport{DryRun: dryRun, Wrote: map[string]int{}, WouldWrite: map[string]int{}}
	// Pre-flight: NUL bytes. Listed always; fatal unless scrubbing.
	unscrubbed, err := canonicalDump(src, false)
	if err != nil {
		return rep, fmt.Errorf("source dump: %w", err)
	}
	for _, td := range unscrubbed.Tables {
		if len(td.rows) == 0 {
			continue
		}
		cols := td.rows[0][0].([]string)
		pkIdx := pkIndexes(td.Table, cols)
		for _, r := range td.rows[1:] {
			for ci, v := range r {
				if str, ok := v.(string); ok && strings.ContainsRune(str, 0) {
					key := make([]string, 0, len(pkIdx))
					for _, pi := range pkIdx {
						key = append(key, fmt.Sprint(r[pi]))
					}
					rep.NULCells = append(rep.NULCells, td.Table+" "+strings.Join(key, "/")+" "+cols[ci])
				}
			}
		}
	}
	if len(rep.NULCells) > 0 && !opts.ScrubNUL {
		return rep, fmt.Errorf("%w: %d cell(s), first: %s", ErrSourceHasNUL, len(rep.NULCells), rep.NULCells[0])
	}
	before := unscrubbed
	if opts.ScrubNUL {
		scrubbed, serr := canonicalDump(src, true)
		if serr != nil {
			return rep, fmt.Errorf("source dump (scrubbed): %w", serr)
		}
		before = scrubbed
	}
	rep.Source = before

	// Import order matters on ONE axis: the destination's insert triggers
	// (wake_item_created, wake_task_created, wake_requirement_created) mint
	// fresh wake_events when items/tasks/requirements land. Copying the
	// source's wake_events FIRST makes those trigger inserts hit ON CONFLICT
	// DO NOTHING on event_key, so the destination carries the source's rows
	// (same ids, same timestamps) and the dumps hash equal.
	ordered := make([]TableDump, 0, len(before.Tables))
	for _, td := range before.Tables {
		if td.Table == "wake_events" {
			ordered = append(ordered, td)
		}
	}
	for _, td := range before.Tables {
		if td.Table != "wake_events" {
			ordered = append(ordered, td)
		}
	}
	for _, td := range ordered {
		if len(td.rows) == 0 {
			continue
		}
		cols := td.rows[0][0].([]string)
		dataRows := td.rows[1:]
		rep.WouldWrite[td.Table] = len(dataRows)
		if dryRun {
			continue
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",")
		// INSERT OR IGNORE is rewritten to ON CONFLICT DO NOTHING on Postgres.
		q := "INSERT OR IGNORE INTO " + td.Table + "(" + strings.Join(cols, ",") + ") VALUES(" + placeholders + ")"
		if td.Table == "state" {
			// pg/schema.sql pre-seeds state (operational_enforcement_since) with
			// the DESTINATION's clock at apply time; DO NOTHING would keep it and
			// the dumps would never agree. The source is the truth: upsert, and
			// count a write only when the value actually changes so a re-import
			// still reports zero.
			ki, vi := -1, -1
			for i, c := range cols {
				switch c {
				case "key":
					ki = i
				case "value":
					vi = i
				}
			}
			for _, r := range dataRows {
				if opts.ScrubNUL {
					if str, ok := r[vi].(string); ok && strings.ContainsRune(str, 0) {
						r[vi] = strings.ReplaceAll(str, "\x00", "")
					}
				}
				var cur string
				qerr := dst.db.QueryRow(`SELECT value FROM state WHERE key=?`, r[ki]).Scan(&cur)
				switch {
				case qerr == nil && cur == fmt.Sprint(r[vi]):
					continue
				case qerr == nil:
					if _, uerr := dst.db.Exec(`UPDATE state SET value=? WHERE key=?`, r[vi], r[ki]); uerr != nil {
						return rep, fmt.Errorf("update state %v: %w", r[ki], uerr)
					}
				default:
					if _, ierr := dst.db.Exec(`INSERT INTO state(key,value) VALUES(?,?)`, r[ki], r[vi]); ierr != nil {
						return rep, fmt.Errorf("insert state %v: %w", r[ki], ierr)
					}
				}
				rep.Wrote[td.Table]++
			}
			continue
		}
		for _, r := range dataRows {
			if opts.ScrubNUL {
				for i := range r {
					if str, ok := r[i].(string); ok && strings.ContainsRune(str, 0) {
						r[i] = strings.ReplaceAll(str, "\x00", "")
					}
				}
			}
			res, xerr := dst.db.Exec(q, r...)
			if xerr != nil {
				return rep, fmt.Errorf("insert into %s: %w", td.Table, xerr)
			}
			if n, _ := res.RowsAffected(); n > 0 {
				rep.Wrote[td.Table] += int(n)
			}
		}
	}

	// The destination's insert triggers mint a wake event for any open item /
	// actionable task / open requirement whose SOURCE had none (rows that
	// predate the v10 triggers). Those extras make dst ≠ src; remove them so
	// the destination is exactly the source, nothing more.
	if !dryRun {
		removed, rerr := removeTriggerExtras(dst, before)
		if rerr != nil {
			return rep, rerr
		}
		rep.TriggerExtrasRemoved = removed
	}

	// (a) the source must not have moved: the caller's quiesce is a claim, this
	// is the check.
	after, err := canonicalDump(src, opts.ScrubNUL)
	if err != nil {
		return rep, fmt.Errorf("source re-dump: %w", err)
	}
	rep.SourceAfter = after.SHA256
	if after.SHA256 != before.SHA256 {
		return rep, ErrSourceMoved
	}
	if dryRun {
		return rep, nil
	}
	// (e) full diff by canonical hash.
	dd, err := canonicalDump(dst, opts.ScrubNUL)
	if err != nil {
		return rep, fmt.Errorf("destination dump: %w", err)
	}
	rep.Destination = dd
	if dd.SHA256 != before.SHA256 {
		for i := range before.Tables {
			if i < len(dd.Tables) && before.Tables[i].SHA256 != dd.Tables[i].SHA256 {
				rep.Notes = append(rep.Notes, fmt.Sprintf("table %s differs: src %d rows, dst %d rows", before.Tables[i].Table, before.Tables[i].Rows, dd.Tables[i].Rows))
			}
		}
		return rep, fmt.Errorf("routerstore: destination dump %s != source dump %s (%s)", dd.SHA256[:12], before.SHA256[:12], strings.Join(rep.Notes, "; "))
	}
	return rep, nil
}

// pkIndexes maps a table's PK column names to their positions in cols.
func pkIndexes(table string, cols []string) []int {
	var out []int
	for _, t := range migratedTables {
		if t.name != table {
			continue
		}
		for _, pk := range t.pk {
			for i, c := range cols {
				if c == pk {
					out = append(out, i)
				}
			}
		}
	}
	return out
}

// removeTriggerExtras deletes destination wake_events whose event_key the
// source never had. Returns the number removed.
func removeTriggerExtras(dst *SQLiteStore, src Dump) (int, error) {
	var srcKeys map[string]bool
	for _, td := range src.Tables {
		if td.Table != "wake_events" || len(td.rows) == 0 {
			continue
		}
		cols := td.rows[0][0].([]string)
		ki := -1
		for i, c := range cols {
			if c == "event_key" {
				ki = i
			}
		}
		if ki < 0 {
			return 0, errors.New("wake_events has no event_key column")
		}
		srcKeys = make(map[string]bool, len(td.rows))
		for _, r := range td.rows[1:] {
			srcKeys[fmt.Sprint(r[ki])] = true
		}
	}
	rows, err := dst.db.Query(`SELECT event_key FROM wake_events`)
	if err != nil {
		return 0, err
	}
	var extras []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if !srcKeys[k] {
			extras = append(extras, k)
		}
	}
	_ = rows.Close()
	for _, k := range extras {
		if _, err := dst.db.Exec(`DELETE FROM wake_events WHERE event_key=?`, k); err != nil {
			return 0, fmt.Errorf("remove trigger-minted wake event %s: %w", k, err)
		}
	}
	return len(extras), nil
}
