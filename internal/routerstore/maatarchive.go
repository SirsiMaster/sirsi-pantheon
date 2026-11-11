package routerstore

// The canonical maat:reservations row is rewritten whole, under the shared lock, by
// every reserve, heartbeat and release, so its size is the Ma'at front door's
// latency (4 MB, ~0.65 s of every call, 2026-11-11). It keeps only what admission
// and live readers need: every non-terminal row, and terminal rows that ended
// within maatHotWindow. Older finished rows move, field for field, to a per-day
// archive row (keyed by start date); nothing is ever deleted. Lookups by id
// (receipts, `status --id`, replay, mutate-by-id) fall back to the archive.
import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maatArchivePrefix = "maat:reservations-archive:" // + start date, YYYY-MM-DD
	maatArchiveAtKey  = "maat:archive-at"
	// Terminal rows stay hot this long after they END. No reader needs a finished
	// row hot: admission, who-is-on, live-holders and the controller fence read
	// active rows; receipts and `status --id` read by id (archive fallback). The
	// window only keeps `sirsi maat status` showing the last couple of hours.
	maatHotWindow = 2 * time.Hour
	// Coverage counts invalidated rows whose est_end is within 24h (plan.go).
	maatInvalidatedHold = 25 * time.Hour
	// Legacy rows with no ended_at that started this long ago are archived
	// regardless (the previous rule, kept as the backstop).
	maatRetention    = 48 * time.Hour
	maatArchiveEvery = 10 * time.Minute // the move re-parses the row, so not on every call
	// FindArchived reads the id's own day +-1 and the newest archive days: a row
	// is archived under its START day, so every day within 72h is covered.
	maatArchiveLookupDays = 5
)

var maatTerminal = map[string]bool{"released": true, "expired": true, "invalidated": true, "done": true}

type maatRow struct {
	Status        string `json:"status"`
	Start         string `json:"start"`
	EstEnd        string `json:"est_end"`
	Created       string `json:"created"`
	HeartbeatAt   string `json:"heartbeat_at"`
	LeaseTTLSec   int    `json:"lease_ttl_sec"`
	EndedAt       string `json:"ended_at"`
	InvalidatedBy string `json:"invalidated_by"`
}

// maatArchivable reports whether a row leaves the hot document, and its start.
// Rows that are not terminal, or have an unreadable start, always stay hot.
func maatArchivable(r maatRow, now time.Time) (time.Time, bool) {
	start, err := time.Parse(time.RFC3339, r.Start)
	if err != nil || !maatTerminal[r.Status] {
		return start, false
	}
	hold := maatHotWindow
	end, err := time.Parse(time.RFC3339, r.EndedAt)
	if err != nil {
		// Legacy row: no end was stamped. The latest instant it could have been
		// live is bounded below by its own timestamps; never earlier than those.
		if now.Sub(start) >= maatRetention {
			return start, true
		}
		end = start
		if t, e := time.Parse(time.RFC3339, r.Created); e == nil && t.After(end) {
			end = t
		}
		if t, e := time.Parse(time.RFC3339, r.HeartbeatAt); e == nil {
			if r.LeaseTTLSec > 0 {
				t = t.Add(time.Duration(r.LeaseTTLSec) * time.Second)
			}
			if t.After(end) {
				end = t
			}
		}
	}
	if r.InvalidatedBy != "" {
		hold = maatInvalidatedHold
		if t, e := time.Parse(time.RFC3339, r.EstEnd); e == nil && t.After(end) {
			end = t
		}
	}
	return start, now.Sub(end) >= hold
}

// archiveMaatHistory runs inside withMaatTransaction, after the row lock.
func (s *SQLiteStore) archiveMaatHistory(tx *txHandle) error {
	now := s.clock().UTC()
	var at string
	err := tx.QueryRow(`SELECT value FROM state WHERE key=?`, maatArchiveAtKey).Scan(&at)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if last, perr := time.Parse(time.RFC3339, at); perr == nil && now.Sub(last) < maatArchiveEvery {
		return nil
	}
	var raw string
	if err := tx.QueryRow(`SELECT value FROM state WHERE key=?`, maatReservationsKey).Scan(&raw); err != nil {
		return err
	}
	doc := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return fmt.Errorf("maat archive: parse ledger: %w", err)
	}
	var rows []json.RawMessage
	if data, ok := doc["reservations"]; ok {
		if err := json.Unmarshal(data, &rows); err != nil {
			return fmt.Errorf("maat archive: parse reservations: %w", err)
		}
	}
	keep, moved := rows[:0:0], map[string][]json.RawMessage{}
	for _, row := range rows {
		var r maatRow
		if json.Unmarshal(row, &r) != nil {
			keep = append(keep, row)
			continue
		}
		start, move := maatArchivable(r, now)
		if !move {
			keep = append(keep, row)
			continue
		}
		day := start.UTC().Format("2006-01-02")
		moved[day] = append(moved[day], row)
	}
	for day, add := range moved {
		key := maatArchivePrefix + day
		var old string
		if err := tx.QueryRow(`SELECT value FROM state WHERE key=?`, key).Scan(&old); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var arch []json.RawMessage
		if old != "" {
			if err := json.Unmarshal([]byte(old), &arch); err != nil {
				return fmt.Errorf("maat archive: parse %s: %w", key, err)
			}
		}
		out, _ := json.Marshal(append(arch, add...))
		if _, err := tx.Exec(`INSERT INTO state(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, string(out)); err != nil {
			return err
		}
	}
	if len(moved) > 0 {
		data, _ := json.Marshal(keep)
		doc["reservations"] = data
		out, _ := json.Marshal(doc)
		if _, err := tx.Exec(`UPDATE state SET value=? WHERE key=?`, string(out), maatReservationsKey); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`INSERT INTO state(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, maatArchiveAtKey, now.Format(time.RFC3339))
	return err
}

// archiveKeys lists the day rows that can hold id: its own day +-1 (ids from
// newID start with their creation time) then the newest archive days.
func (m maatTransactionState) archiveKeys(id string) ([]string, error) {
	var keys []string
	if len(id) > 16 && id[16] == '-' {
		if t, err := time.Parse("20060102T150405Z", id[:16]); err == nil {
			for _, d := range []int{0, -1, 1} {
				keys = append(keys, maatArchivePrefix+t.AddDate(0, 0, d).Format("2006-01-02"))
			}
		}
	}
	res, err := m.tx.Query(`SELECT key FROM state WHERE key LIKE ? ORDER BY key DESC LIMIT `+fmt.Sprint(maatArchiveLookupDays), maatArchivePrefix+"%")
	if err != nil {
		return nil, err
	}
	defer res.Close()
	for res.Next() {
		var k string
		if err := res.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, res.Err()
}

// locateArchived finds the archive day row and index holding id.
func (m maatTransactionState) locateArchived(id string) (key string, rows []json.RawMessage, at int, err error) {
	keys, err := m.archiveKeys(id)
	if err != nil {
		return "", nil, -1, err
	}
	// Skip the parse when the id's JSON text is not even in the day row. Only
	// plain ids are prefiltered; an id JSON might escape is always parsed.
	plain := !strings.ContainsFunc(id, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || strings.ContainsRune("._@:-", r))
	})
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		var raw string
		if err := m.tx.QueryRow(`SELECT value FROM state WHERE key=?`, k).Scan(&raw); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return "", nil, -1, err
		}
		if plain && !strings.Contains(raw, `"`+id+`"`) {
			continue
		}
		var day []json.RawMessage
		if err := json.Unmarshal([]byte(raw), &day); err != nil {
			return "", nil, -1, fmt.Errorf("maat archive: parse %s: %w", k, err)
		}
		for i, row := range day {
			var h struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(row, &h) == nil && h.ID == id {
				return k, day, i, nil
			}
		}
	}
	return "", nil, -1, nil
}

// FindArchived implements schedule.ArchiveStore.
func (m maatTransactionState) FindArchived(id string) (json.RawMessage, bool, error) {
	_, rows, at, err := m.locateArchived(id)
	if err != nil || at < 0 {
		return nil, false, err
	}
	return rows[at], true, nil
}

// ReplaceArchived implements schedule.ArchiveStore.
func (m maatTransactionState) ReplaceArchived(id string, row json.RawMessage) error {
	key, rows, at, err := m.locateArchived(id)
	if err != nil {
		return err
	}
	if at < 0 {
		return fmt.Errorf("maat archive: reservation %s not archived", id)
	}
	if row == nil {
		rows = append(rows[:at:at], rows[at+1:]...)
	} else {
		rows[at] = row
	}
	out, _ := json.Marshal(rows)
	return m.SetState(key, string(out))
}
