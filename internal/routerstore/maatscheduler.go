package routerstore

// Reservation read/modify/write belongs in one database transaction, not in
// separate GetState/SetState RPCs. All service instances lock the same row.
import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

const maatReservationsKey = "maat:reservations"

type maatTransactionState struct{ tx *txHandle }

func (m maatTransactionState) CompareAndSwapState(key, expected string, exists bool, value string) (bool, error) {
	var result sql.Result
	var err error
	if exists {
		result, err = m.tx.Exec(`UPDATE state SET value=? WHERE key=? AND value=?`, value, key, expected)
	} else {
		result, err = m.tx.Exec(`INSERT INTO state(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING`, key, value)
	}
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (m maatTransactionState) GetState(key string) (string, bool, error) {
	var value string
	err := m.tx.QueryRow(`SELECT value FROM state WHERE key=?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return value, err == nil, err
}

func (m maatTransactionState) SetState(key, value string) error {
	_, err := m.tx.Exec(`INSERT INTO state(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *SQLiteStore) withMaatScheduler(fn func(*schedule.Ledger) error) error {
	return s.withMaatTransaction(func(_ *txHandle, l *schedule.Ledger) error { return fn(l) })
}

// Custody consume and reservation admission serialize on the SAME canonical
// row, including across PostgreSQL replicas; no read-then-write race window.
func (s *SQLiteStore) withMaatTransaction(fn func(*txHandle, *schedule.Ledger) error) error {
	return s.withTx(func(tx *txHandle) error {
		if _, err := tx.Exec(`INSERT INTO state(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING`, maatReservationsKey, `{"reservations":[]}`); err != nil {
			return err
		}
		// Lock without rewriting the large PostgreSQL row on reads. SQLite's
		// BEGIN IMMEDIATE already serializes writers. Both fence service replicas.
		if s.dialect().name == "postgres" {
			var key string
			if err := tx.QueryRow(`SELECT key FROM state WHERE key=? FOR UPDATE`, maatReservationsKey).Scan(&key); err != nil {
				return err
			}
		}
		return fn(tx, schedule.NewLedger(maatTransactionState{tx}).WithClock(s.clock))
	})
}

func (s *SQLiteStore) MaatReserve(req schedule.Reservation, queue bool) (schedule.ReserveResult, error) {
	var out schedule.ReserveResult
	err := s.withMaatTransaction(func(tx *txHandle, l *schedule.Ledger) (err error) {
		if err = sleepReserveFence(tx, l, req.Resource); err != nil {
			return err
		}
		out, err = l.Reserve(req, queue)
		return err
	})
	return out, err
}

func (s *SQLiteStore) MaatMutate(action, id, value string) (*schedule.Reservation, error) {
	var out *schedule.Reservation
	err := s.withMaatScheduler(func(l *schedule.Ledger) (err error) {
		switch action {
		case "heartbeat":
			out, err = l.Heartbeat(id)
		case "release":
			out, err = l.Release(id)
		case "extend":
			out, err = l.Extend(id, value)
		case "invalidate":
			out, err = l.Invalidate(id, value)
		default:
			err = fmt.Errorf("unknown Ma'at mutation %q", action)
		}
		return err
	})
	return out, err
}

func (s *SQLiteStore) MaatRead(kind, resource string) ([]schedule.Reservation, error) {
	var out []schedule.Reservation
	err := s.withMaatScheduler(func(l *schedule.Ledger) (err error) {
		switch kind {
		case "who-is-on":
			var row *schedule.Reservation
			row, err = l.WhoIsOn(resource)
			if row != nil {
				out = []schedule.Reservation{*row}
			}
		case "live-holders":
			out, err = l.LiveHolders(resource)
		case "status":
			out, err = l.Status(resource)
		default:
			err = fmt.Errorf("unknown Ma'at read %q", kind)
		}
		return err
	})
	return out, err
}
