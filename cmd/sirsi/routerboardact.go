package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// boardLedger is the slice of routerstore.Store the hosted board edits through. The router service enforces
// the same set server-side (routerstore ledgerEdits, the @ledger-editor capability): this list is the
// board's intent, the service's list is the authority.
type boardLedger interface {
	Get(id string) (routerstore.Item, error)
	CloseItem(id, result string) error
	ReassignItem(id, from, to, note string) error
	AddTask(t routerstore.Task) error
	GetTask(agent, taskID string) (routerstore.Task, error)
	UpdateTask(agent, taskID string, u routerstore.TaskUpdate) (routerstore.Task, error)
}

// boardAct is the request body every verb reads; each verb uses the fields it needs. There is no claim,
// renew, release or complete: a task lease belongs to the lane's own authenticated thread, so the board
// cannot hold one for another lane (routerstore ledgerEdits).
type boardAct struct {
	ID          string  `json:"id"`          // router item id
	To          string  `json:"to"`          // reassign-item target agent
	Note        string  `json:"note"`        // close result / reassign note
	Agent       string  `json:"agent"`       // task agent
	TaskID      string  `json:"task_id"`     // task id
	Subject     string  `json:"subject"`     // add-task / convert / update-task
	Status      string  `json:"status"`      // update-task: pending | blocked
	Responsible string  `json:"responsible"` // add-task / update-task / reassign-task
	BlockedBy   *string `json:"blocked_by"`  // update-task; "" clears
}

func runBoardAct(st boardLedger, actor, verb string, body []byte) (any, error) {
	var a boardAct
	if err := json.Unmarshal(body, &a); err != nil {
		return nil, fmt.Errorf("board: body: %w", err)
	}
	need := func(fields ...string) error {
		for i := 0; i < len(fields); i += 2 {
			if strings.TrimSpace(fields[i+1]) == "" {
				return fmt.Errorf("board: %s: %s is required", verb, fields[i])
			}
		}
		return nil
	}
	note := func(s string) string { return strings.TrimSpace(actor + ": " + s) }
	switch verb {
	case "close":
		if err := need("id", a.ID); err != nil {
			return nil, err
		}
		return ok(st.CloseItem(a.ID, note(a.Note)))
	case "reassign-item":
		if err := need("id", a.ID, "to", a.To); err != nil {
			return nil, err
		}
		it, err := st.Get(a.ID)
		if err != nil {
			return nil, err
		}
		return ok(st.ReassignItem(a.ID, it.To, a.To, note(a.Note)))
	case "convert":
		// ponytail: two calls (add task, then close item), not one transaction; a failed close leaves the
		// task added and the item open, which a retry of "close" settles. Swap to ConvertItemToTask once served.
		if err := need("id", a.ID, "task_id", a.TaskID); err != nil {
			return nil, err
		}
		it, err := st.Get(a.ID)
		if err != nil {
			return nil, err
		}
		agent := a.Agent
		if agent == "" {
			agent = it.To
		}
		subject := a.Subject
		if subject == "" {
			subject = it.Title
		}
		if err := st.AddTask(routerstore.Task{Agent: agent, TaskID: a.TaskID, Subject: subject,
			ResponsibleParty: a.Responsible, CommissionedBy: actor}); err != nil {
			return nil, err
		}
		return ok(st.CloseItem(a.ID, note("converted to ledger task "+agent+"/"+a.TaskID)))
	case "add-task":
		if err := need("agent", a.Agent, "task_id", a.TaskID, "subject", a.Subject); err != nil {
			return nil, err
		}
		return ok(st.AddTask(routerstore.Task{Agent: a.Agent, TaskID: a.TaskID, Subject: a.Subject,
			ResponsibleParty: a.Responsible, CommissionedBy: actor}))
	case "update-task", "reassign-task":
		if err := need("agent", a.Agent, "task_id", a.TaskID); err != nil {
			return nil, err
		}
		if verb == "reassign-task" {
			if err := need("responsible", a.Responsible); err != nil {
				return nil, err
			}
			a = boardAct{Agent: a.Agent, TaskID: a.TaskID, Responsible: a.Responsible}
		}
		t, err := st.GetTask(a.Agent, a.TaskID)
		if err != nil {
			return nil, err
		}
		u := routerstore.TaskUpdate{Subject: a.Subject, Status: a.Status, ResponsibleParty: a.Responsible}
		if a.BlockedBy != nil {
			u.BlockedBy, u.BlockedBySet = *a.BlockedBy, true
		}
		// ponytail: the actor rides a timeline entry appended to the task as read; a concurrent timeline
		// write between the read and this update is overwritten. Add an actor column if that ever matters.
		u.Timeline = append(t.Timeline, routerstore.TimelineEntry{Day: time.Now().UTC().Format("2006-01-02"),
			Owner: actor, Label: verb + " " + describeUpdate(a)})
		u.TimelineSet = true
		return st.UpdateTask(a.Agent, a.TaskID, u)
	}
	return nil, dashboard.ErrBoardVerb
}

func ok(err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

func describeUpdate(a boardAct) string {
	var parts []string
	for _, kv := range [][2]string{{"status", a.Status}, {"responsible", a.Responsible}, {"subject", a.Subject}} {
		if kv[1] != "" {
			parts = append(parts, kv[0]+"="+kv[1])
		}
	}
	if a.BlockedBy != nil {
		parts = append(parts, "blocked_by="+*a.BlockedBy)
	}
	if len(parts) == 0 {
		return "(no change)"
	}
	return strings.Join(parts, " ")
}

// boardActFn resolves the router store once per call. The board's token carries @ledger-editor; the
// service refuses anything outside it regardless of what this process asks for.
func boardActFn(_ context.Context, actor, verb string, body []byte) (any, error) {
	st, err := routerstore.Resolve()
	if err != nil {
		return nil, fmt.Errorf("board: router store: %w", err)
	}
	return runBoardAct(st, actor, verb, body)
}
