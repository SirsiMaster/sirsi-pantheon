package routerstore

import (
	"database/sql"
	"errors"
	"fmt"
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
func (s *Store) RegisterAgent(id string, pid int) error {
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
	if _, err := s.db.Exec(q, id, now, now, pid); err != nil {
		return fmt.Errorf("routerstore: RegisterAgent %q: %w", id, err)
	}
	return nil
}

// Heartbeat updates an agent's last_seen to now. It returns ErrNotFound if the
// agent was never registered — a heartbeat is proof of an existing
// registration, not a way to create one.
func (s *Store) Heartbeat(id string) error {
	now := s.clock().Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE agents SET last_seen=? WHERE id=?;`, now, id)
	if err != nil {
		return fmt.Errorf("routerstore: Heartbeat %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAgent returns one agent by id, or ErrNotFound.
func (s *Store) GetAgent(id string) (Agent, error) {
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
func (s *Store) ListAgents() ([]Agent, error) {
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
func (s *Store) SetState(key, value string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("routerstore: SetState: key is required")
	}
	const q = `INSERT INTO state (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value;`
	if _, err := s.db.Exec(q, key, value); err != nil {
		return fmt.Errorf("routerstore: SetState %q: %w", key, err)
	}
	return nil
}

// GetState returns the value for key. The second return is false if the key is
// absent (distinguishing an unset key from an empty-string value).
func (s *Store) GetState(key string) (string, bool, error) {
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
