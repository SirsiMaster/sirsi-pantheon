package routerstore

// The canonical maat:reservations row is rewritten whole, under the shared lock, by
// every reserve, heartbeat and release. Kept forever it grew past 2.4 MB and made
// the Ma'at front door time out (2026-11-03). Finished rows past the retention
// window move, field for field, to a per-day archive row; nothing is deleted.
import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	maatArchivePrefix = "maat:reservations-archive:" // + start date, YYYY-MM-DD
	maatArchiveAtKey  = "maat:archive-at"
	maatRetention     = 48 * time.Hour   // maat: receipts look a reservation up by id inside this window
	maatArchiveEvery  = 10 * time.Minute // the move re-parses the row, so not on every call
)

var maatTerminal = map[string]bool{"released": true, "expired": true, "invalidated": true, "done": true}

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
		var r struct{ Status, Start string }
		start, perr := time.Time{}, json.Unmarshal(row, &r)
		if perr == nil {
			start, perr = time.Parse(time.RFC3339, r.Start)
		}
		if perr != nil || !maatTerminal[r.Status] || now.Sub(start) < maatRetention {
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
