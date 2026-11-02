package routerstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// convertedPrefix opens the close result of a converted item; the rest is "<agent>/<task-id> by <actor>".
const convertedPrefix = "converted to task "

// convertAfterTaskHook runs between the task insert and the item close. Nil in production; tests use it to
// fail mid-transaction and prove neither write survives (same bare-var pattern as scanRowHook).
var convertAfterTaskHook func() error

// ConvertItemToTask turns an open router item into ledger work in ONE transaction: it adds a pending task
// on agent's ledger (subject = item title, outline = the item's sender and instructions) and closes the
// item with result "converted to task <agent>/<taskID> by <actor>". An empty taskID uses the item id,
// which is already unique. Re-running for an item it already converted returns that task (idempotent).
// An item closed for any other reason is refused with ErrAlreadyClosed.
func (s *SQLiteStore) ConvertItemToTask(itemID, agent, taskID, actor string) (Task, error) {
	agent, actor = strings.TrimSpace(agent), strings.TrimSpace(actor)
	if itemID == "" || agent == "" || actor == "" {
		return Task{}, fmt.Errorf("routerstore: ConvertItemToTask: item id, agent and actor are required")
	}
	if strings.TrimSpace(taskID) == "" {
		taskID = itemID
	}
	var doneAgent, doneTask string
	err := s.withTx(func(tx *txHandle) error {
		var title, from, status, instructions, result string
		err := tx.QueryRow(`SELECT title, from_agent, status, instructions, result FROM items WHERE id=?;`, itemID).
			Scan(&title, &from, &status, &instructions, &result)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("routerstore: ConvertItemToTask %q: %w", itemID, err)
		}
		if status != "open" {
			if a, t, ok := parseConverted(result); ok {
				doneAgent, doneTask = a, t
				return nil
			}
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
		res, err := tx.Exec(`UPDATE items SET status='closed', closed=?, result=? WHERE id=? AND status='open';`,
			s.clock().Format(time.RFC3339), fmt.Sprintf("%s%s/%s by %s", convertedPrefix, agent, taskID, actor), itemID)
		if err != nil {
			return fmt.Errorf("routerstore: ConvertItemToTask %q: close: %w", itemID, err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return fmt.Errorf("routerstore: ConvertItemToTask %q: item changed during convert", itemID)
		}
		doneAgent, doneTask = agent, taskID
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	return s.GetTask(doneAgent, doneTask)
}

// parseConverted reads "<agent>/<task-id>" back out of a converted item's close result.
func parseConverted(result string) (agent, taskID string, ok bool) {
	rest, found := strings.CutPrefix(result, convertedPrefix)
	if !found {
		return "", "", false
	}
	ref, _, _ := strings.Cut(rest, " by ")
	agent, taskID, ok = strings.Cut(ref, "/")
	return agent, taskID, ok && agent != "" && taskID != ""
}
