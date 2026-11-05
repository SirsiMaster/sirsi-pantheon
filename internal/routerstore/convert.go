package routerstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// convertLinkPrefix keys the item→task link in the state table: "item-convert:<item id>" →
// {"agent","task_id"}. Written only inside ConvertItemToTask's transaction, never derived from the
// item's free-text close result (which any recipient can set with CloseItem). Scoped tokens cannot
// write state (SetState is not in the lane API). A state row rather than a column: the Postgres
// ledger pins an exact schema_version, and this needs no migration.
const convertLinkPrefix = "item-convert:"

type convertLink struct {
	Agent  string `json:"agent"`
	TaskID string `json:"task_id"`
}

// convertAfterTaskHook runs between the task insert and the item close. Nil in production; tests use it to
// fail mid-transaction and prove neither write survives (same bare-var pattern as scanRowHook).
var convertAfterTaskHook func() error

// ConvertItemToTask turns an open router item into ledger work in ONE transaction: it adds a pending task
// on agent's ledger (subject = item title, outline = the item's sender and instructions), records the
// item→task link, and closes the item with result "converted to task <agent>/<taskID> by <actor>".
// An empty taskID uses the item id, which is already unique.
//
// Re-running returns the linked task only when it is on the same agent's ledger (and, if a taskID is
// given, has that id); anything else is refused. An item closed any other way — whatever its result
// text says — is refused with ErrAlreadyClosed.
func (s *SQLiteStore) ConvertItemToTask(itemID, agent, taskID, actor string) (Task, error) {
	agent, actor, taskID = strings.TrimSpace(agent), strings.TrimSpace(actor), strings.TrimSpace(taskID)
	if itemID == "" || agent == "" || actor == "" {
		return Task{}, fmt.Errorf("%w: routerstore: ConvertItemToTask: item id, agent and actor are required", ErrInvalidInput)
	}
	requested := taskID
	if taskID == "" {
		taskID = itemID
	}
	if strings.ContainsRune(taskID, '/') || strings.IndexFunc(taskID, unicode.IsSpace) >= 0 {
		return Task{}, fmt.Errorf("%w: routerstore: ConvertItemToTask: task id %q must not contain '/' or whitespace", ErrInvalidInput, taskID)
	}
	// Postgres runs concurrent transactions, so lock the item row: a second convert waits for the first and
	// then reads its link (the idempotent answer) instead of racing to ErrTaskExists. SQLite's withTx is
	// BEGIN IMMEDIATE, already serialized.
	lock := ""
	if s.dialect().name == "postgres" {
		lock = " FOR UPDATE"
	}
	err := s.withTx(func(tx *txHandle) error {
		var title, from, status, instructions string
		err := tx.QueryRow(`SELECT title, from_agent, status, instructions FROM items WHERE id=?`+lock+`;`, itemID).
			Scan(&title, &from, &status, &instructions)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("routerstore: ConvertItemToTask %q: %w", itemID, err)
		}
		var raw string
		switch err := tx.QueryRow(`SELECT value FROM state WHERE key=?;`, convertLinkPrefix+itemID).Scan(&raw); {
		case err == nil:
			var l convertLink
			if json.Unmarshal([]byte(raw), &l) != nil || l.Agent != agent || (requested != "" && l.TaskID != requested) {
				return fmt.Errorf("%w: %s was already converted to a different task", ErrAlreadyClosed, itemID)
			}
			taskID = l.TaskID
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("routerstore: ConvertItemToTask %q: read link: %w", itemID, err)
		}
		if status != "open" {
			return fmt.Errorf("%w: %s was closed, not converted", ErrAlreadyClosed, itemID)
		}
		outline := fmt.Sprintf("Converted from router item %s (from %s).\n\n%s", itemID, from, strings.TrimSpace(instructions))
		if err := s.insertTask(tx.Exec, Task{Agent: agent, TaskID: taskID, Subject: title, Outline: &outline, CommissionedBy: from}); err != nil {
			return err
		}
		if convertAfterTaskHook != nil {
			if err := convertAfterTaskHook(); err != nil {
				return err
			}
		}
		link, _ := json.Marshal(convertLink{Agent: agent, TaskID: taskID})
		if _, err := tx.Exec(`INSERT INTO state(key, value) VALUES (?, ?);`, convertLinkPrefix+itemID, string(link)); err != nil {
			return fmt.Errorf("routerstore: ConvertItemToTask %q: record link: %w", itemID, err)
		}
		res, err := tx.Exec(`UPDATE items SET status='closed', closed=?, result=? WHERE id=? AND status='open';`,
			s.clock().Format(time.RFC3339), fmt.Sprintf("converted to task %s/%s by %s", agent, taskID, actor), itemID)
		if err != nil {
			return fmt.Errorf("routerstore: ConvertItemToTask %q: close: %w", itemID, err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return fmt.Errorf("routerstore: ConvertItemToTask %q: item changed during convert", itemID)
		}
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	return s.GetTask(agent, taskID)
}
