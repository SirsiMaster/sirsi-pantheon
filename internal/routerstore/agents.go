package routerstore

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"strings"
	"time"
)

// Agent is the durable record of a registered router thread. It projects the
// liveness fields the file router tracks in state.json / thread records so
// node-status can be answered from an indexed query.
type Agent struct {
	ID           string
	RegisteredAt string // RFC3339
	LastSeen     string // RFC3339
	PID          int
}

// RegisterAgent records (or refreshes) an agent registration. It is idempotent
// on (id): re-registering an existing agent updates its pid and last_seen and
// preserves the original registered_at, so a surface restart never accumulates
// duplicate rows (Rule A27 idempotent-on-(agent_id,pid) intent).
func (s *SQLiteStore) RegisterAgent(id string, pid int) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("routerstore: RegisterAgent: id is required")
	}
	now := s.clock().Format(time.RFC3339)
	const q = `
INSERT INTO agents (id, registered_at, last_seen, pid)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    last_seen=excluded.last_seen,
    pid=excluded.pid;`
	if _, err := s.exec(q, id, now, now, pid); err != nil {
		return fmt.Errorf("routerstore: RegisterAgent %q: %w", id, err)
	}
	return nil
}

// Heartbeat updates an agent's last_seen to now. It returns ErrNotFound if the
// agent was never registered — a heartbeat is proof of an existing
// registration, not a way to create one.
func (s *SQLiteStore) Heartbeat(id string) error {
	now := s.clock().Format(time.RFC3339)
	res, err := s.exec(`UPDATE agents SET last_seen=? WHERE id=?;`, now, id)
	if err != nil {
		return fmt.Errorf("routerstore: Heartbeat %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAgent returns one agent by id, or ErrNotFound.
func (s *SQLiteStore) GetAgent(id string) (Agent, error) {
	var a Agent
	err := s.db.QueryRow(
		`SELECT id, registered_at, last_seen, pid FROM agents WHERE id=?;`, id,
	).Scan(&a.ID, &a.RegisteredAt, &a.LastSeen, &a.PID)
	if errors.Is(err, sql.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	if err != nil {
		return Agent{}, fmt.Errorf("routerstore: GetAgent %q: %w", id, err)
	}
	return a, nil
}

// ListAgents returns every registered agent, ordered by id.
func (s *SQLiteStore) ListAgents() ([]Agent, error) {
	rows, err := s.db.Query(`SELECT id, registered_at, last_seen, pid FROM agents ORDER BY id ASC;`)
	if err != nil {
		return nil, fmt.Errorf("routerstore: ListAgents: %w", err)
	}
	defer rows.Close()
	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.RegisteredAt, &a.LastSeen, &a.PID); err != nil {
			return nil, fmt.Errorf("routerstore: ListAgents scan: %w", err)
		}
		agents = append(agents, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("routerstore: ListAgents rows: %w", err)
	}
	return agents, nil
}

// SetState stores an arbitrary key/value pair, projecting the old state.json
// scalar keys (last_*_read etc.) into the durable index.
func (s *SQLiteStore) SetState(key, value string) error {
	if key == "maat:delegations" {
		return fmt.Errorf("maat: generic state write cannot create delegation authority; use compatible atomic RPC")
	}
	if key == desktopSleepKey {
		return errors.New("sleep custody: generic state writes cannot create or replace authority")
	}
	if key == maatReservationsKey {
		return fmt.Errorf("Ma'at reservation writes require the atomic scheduler API; upgrade this client")
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("routerstore: SetState: key is required")
	}
	const q = `INSERT INTO state (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value;`
	if _, err := s.exec(q, key, value); err != nil {
		return fmt.Errorf("routerstore: SetState %q: %w", key, err)
	}
	return nil
}

// GetState returns the value for key. The second return is false if the key is
// absent (distinguishing an unset key from an empty-string value).
func (s *SQLiteStore) GetState(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM state WHERE key=?;`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("routerstore: GetState %q: %w", key, err)
	}
	return value, true, nil
}

// CompareAndSwapState is the compatible exact-snapshot API. The scheduler's
// native operation API additionally keeps large ledger bytes at the service.
func (s *SQLiteStore) CompareAndSwapState(key, expected string, exists bool, value string) (bool, error) {
	if key == "maat:delegations" {
		return false, fmt.Errorf("maat: generic CAS cannot create delegation authority; use compatible atomic RPC")
	}
	if key == maatReservationsKey {
		var swapped bool
		err := s.withMaatTransaction(func(tx *txHandle, l *schedule.Ledger) error {
			m := maatTransactionState{tx}
			raw, present, err := m.GetState(key)
			if err != nil {
				return err
			}
			// withMaatTransaction initializes an absent reservations row; preserve the
			// legacy absent-state CAS's semantics against that exact empty document.
			if !exists && raw == `{"reservations":[]}` {
				expected = raw
				exists = true
			}
			if present != exists || raw != expected {
				return nil
			}
			if err := l.CheckLegacyReservationWrite(raw, value); err != nil {
				return err
			}
			swapped, err = m.CompareAndSwapState(key, expected, exists, value)
			return err
		})
		return swapped, err
	}
	if key == desktopSleepKey {
		return false, errors.New("sleep custody: generic CAS cannot create or replace authority")
	}
	if strings.TrimSpace(key) == "" {
		return false, fmt.Errorf("routerstore: CompareAndSwapState: key is required")
	}
	var result sql.Result
	var err error
	if exists {
		result, err = s.exec(`UPDATE state SET value=? WHERE key=? AND value=?`, value, key, expected)
	} else {
		result, err = s.exec(`INSERT INTO state (key, value) VALUES (?, ?) ON CONFLICT(key) DO NOTHING`, key, value)
	}
	if err != nil {
		return false, fmt.Errorf("routerstore: CompareAndSwapState: %w", err)
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
