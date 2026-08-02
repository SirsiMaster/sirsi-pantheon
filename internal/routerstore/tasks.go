package routerstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Task is an agent's durable execution commitment. Router items are messages;
// tasks are the normalized registry that says who owns what and its state.
type Task struct {
	Agent            string `json:"agent"`
	TaskID           string `json:"task_id"`
	Subject          string `json:"subject"`
	Status           string `json:"status"`
	ResponsibleParty string `json:"responsible_party"`
	BlockedBy        string `json:"blocked_by,omitempty"`
	Created          string `json:"created"`
	Updated          string `json:"updated"`
}

var ErrTaskExists = errors.New("routerstore: task already exists")

func validTaskStatus(status string) bool {
	switch status {
	case "pending", "in-progress", "blocked", "done":
		return true
	}
	return false
}

func validateTask(t Task) error {
	if strings.TrimSpace(t.Agent) == "" || strings.TrimSpace(t.TaskID) == "" || strings.TrimSpace(t.Subject) == "" {
		return fmt.Errorf("routerstore: task agent, task-id, and subject are required")
	}
	if !validTaskStatus(t.Status) {
		return fmt.Errorf("routerstore: invalid task status %q (want pending, in-progress, blocked, or done)", t.Status)
	}
	if strings.TrimSpace(t.ResponsibleParty) == "" {
		return fmt.Errorf("routerstore: task responsible-party is required")
	}
	return nil
}

// AddTask inserts one task and refuses accidental replacement.
func (s *Store) AddTask(t Task) error {
	if t.Status == "" {
		t.Status = "pending"
	}
	if t.ResponsibleParty == "" {
		t.ResponsibleParty = "self"
	}
	if err := validateTask(t); err != nil {
		return err
	}
	now := s.clock().Format(time.RFC3339)
	if t.Created == "" {
		t.Created = now
	}
	if t.Updated == "" {
		t.Updated = now
	}
	_, err := s.db.Exec(`INSERT INTO tasks(agent, task_id, subject, status, responsible_party, blocked_by, created, updated) VALUES (?, ?, ?, ?, ?, ?, ?, ?);`,
		t.Agent, t.TaskID, strings.TrimSpace(t.Subject), t.Status, t.ResponsibleParty, strings.TrimSpace(t.BlockedBy), t.Created, t.Updated)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrTaskExists
		}
		return fmt.Errorf("routerstore: AddTask %s/%s: %w", t.Agent, t.TaskID, err)
	}
	return nil
}

// UpdateTask replaces mutable task fields. Empty values mean keep the current
// value, except BlockedBySet permits an explicit dependency clear.
type TaskUpdate struct {
	Subject, Status, ResponsibleParty, BlockedBy string
	BlockedBySet                                 bool
}

func (s *Store) UpdateTask(agent, taskID string, u TaskUpdate) (Task, error) {
	t, err := s.GetTask(agent, taskID)
	if err != nil {
		return Task{}, err
	}
	if u.Subject != "" {
		t.Subject = u.Subject
	}
	if u.Status != "" {
		t.Status = u.Status
	}
	if u.ResponsibleParty != "" {
		t.ResponsibleParty = u.ResponsibleParty
	}
	if u.BlockedBySet {
		t.BlockedBy = strings.TrimSpace(u.BlockedBy)
	}
	if err := validateTask(t); err != nil {
		return Task{}, err
	}
	t.Updated = s.clock().Format(time.RFC3339)
	_, err = s.db.Exec(`UPDATE tasks SET subject=?, status=?, responsible_party=?, blocked_by=?, updated=? WHERE agent=? AND task_id=?;`,
		t.Subject, t.Status, t.ResponsibleParty, t.BlockedBy, t.Updated, agent, taskID)
	if err != nil {
		return Task{}, fmt.Errorf("routerstore: UpdateTask %s/%s: %w", agent, taskID, err)
	}
	return t, nil
}

func (s *Store) GetTask(agent, taskID string) (Task, error) {
	var t Task
	err := s.db.QueryRow(`SELECT agent, task_id, subject, status, responsible_party, blocked_by, created, updated FROM tasks WHERE agent=? AND task_id=?;`, agent, taskID).
		Scan(&t.Agent, &t.TaskID, &t.Subject, &t.Status, &t.ResponsibleParty, &t.BlockedBy, &t.Created, &t.Updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("routerstore: GetTask %s/%s: %w", agent, taskID, err)
	}
	return t, nil
}

// ListTasks returns every task, optionally filtered by agent, ordered by agent
// then creation time and id for deterministic surfaces.
func (s *Store) ListTasks(agent string) ([]Task, error) {
	query := `SELECT agent, task_id, subject, status, responsible_party, blocked_by, created, updated FROM tasks`
	var rows *sql.Rows
	var err error
	if strings.TrimSpace(agent) == "" {
		rows, err = s.db.Query(query + ` ORDER BY agent, created, task_id;`)
	} else {
		rows, err = s.db.Query(query+` WHERE agent=? ORDER BY created, task_id;`, agent)
	}
	if err != nil {
		return nil, fmt.Errorf("routerstore: ListTasks: %w", err)
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.Agent, &t.TaskID, &t.Subject, &t.Status, &t.ResponsibleParty, &t.BlockedBy, &t.Created, &t.Updated); err != nil {
			return nil, fmt.Errorf("routerstore: ListTasks scan: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
