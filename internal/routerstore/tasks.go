package routerstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// TaskStalledAfter is the single liveness threshold consumed by every ledger
// surface. Liveness is always derived and is never persisted.
const TaskStalledAfter = 4 * time.Hour

type TimelineEntry struct {
	Day   string  `json:"day"`
	Owner string  `json:"owner"`
	Hours float64 `json:"hours"`
	Label string  `json:"label"`
}

type TaskLink struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Task is an agent's durable execution commitment. Router items are messages;
// tasks are the normalized registry that says who owns what and its state.
type Task struct {
	Agent            string          `json:"agent"`
	TaskID           string          `json:"task_id"`
	Subject          string          `json:"subject"`
	Status           string          `json:"status"`
	Phase            string          `json:"phase"`
	ResponsibleParty string          `json:"responsible_party"`
	BlockedBy        string          `json:"blocked_by"`
	Created          string          `json:"created"`
	Updated          string          `json:"updated"`
	Charter          *string         `json:"charter"`
	CommissionedAt   string          `json:"commissioned_at"`
	CommissionedBy   string          `json:"commissioned_by"`
	Outline          *string         `json:"outline"`
	Timeline         []TimelineEntry `json:"timeline"`
	Links            []TaskLink      `json:"links"`
	Liveness         string          `json:"liveness"`
	TestState        string          `json:"test_state"`
	Stage            string          `json:"stage"`
	TokensConsumed   int64           `json:"tokens_consumed"`
	DurationSeconds  int64           `json:"duration_seconds"`
}

var ErrTaskExists = errors.New("routerstore: task already exists")

func validTaskStatus(status string) bool {
	switch status {
	case "pending", "in-progress", "blocked", "done":
		return true
	}
	return false
}

func validTestState(state string) bool {
	switch state {
	case "untested", "tested", "passed", "failed":
		return true
	}
	return false
}

var stageRank = map[string]int{"spec": 0, "build": 1, "review": 2, "verify": 3, "shipped": 4}

func validLinkKind(kind string) bool {
	switch kind {
	case "canon", "repo", "pr", "owner-instruction", "evidence":
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
	if !validTestState(t.TestState) {
		return fmt.Errorf("routerstore: invalid test-state %q", t.TestState)
	}
	if _, ok := stageRank[t.Stage]; !ok {
		return fmt.Errorf("routerstore: invalid stage %q", t.Stage)
	}
	if t.TokensConsumed < 0 || t.DurationSeconds < 0 {
		return fmt.Errorf("routerstore: task accounting values cannot be negative")
	}
	if _, err := time.Parse(time.RFC3339, t.CommissionedAt); err != nil {
		return fmt.Errorf("routerstore: commissioned-at must be RFC3339 UTC: %w", err)
	}
	for i, entry := range t.Timeline {
		if strings.TrimSpace(entry.Day) == "" || strings.TrimSpace(entry.Owner) == "" || strings.TrimSpace(entry.Label) == "" || entry.Hours < 0 || math.IsNaN(entry.Hours) || math.IsInf(entry.Hours, 0) {
			return fmt.Errorf("routerstore: invalid timeline entry %d", i)
		}
	}
	hasEvidence := false
	for i, link := range t.Links {
		if !validLinkKind(link.Kind) || strings.TrimSpace(link.Label) == "" || strings.TrimSpace(link.URL) == "" {
			return fmt.Errorf("routerstore: invalid task link %d", i)
		}
		hasEvidence = hasEvidence || link.Kind == "evidence"
	}
	if t.TestState == "passed" && !hasEvidence {
		return fmt.Errorf("routerstore: test-state passed requires an evidence link")
	}
	return nil
}

func deriveTaskLiveness(t Task, now time.Time) string {
	if strings.TrimSpace(t.BlockedBy) != "" {
		return "blocked"
	}
	updated, err := time.Parse(time.RFC3339, t.Updated)
	if err != nil {
		return "unknown"
	}
	age := now.Sub(updated)
	if t.Status == "in-progress" && age < TaskStalledAfter {
		return "active"
	}
	if (t.Status == "pending" || t.Status == "in-progress") && age >= TaskStalledAfter {
		return "stalled"
	}
	return "unknown"
}

func (s *Store) AddTask(t Task) error {
	if t.Status == "" {
		t.Status = "pending"
	}
	if t.ResponsibleParty == "" {
		t.ResponsibleParty = "self"
	}
	if t.TestState == "" {
		t.TestState = "untested"
	}
	if t.Stage == "" {
		t.Stage = "spec"
	}
	if t.Timeline == nil {
		t.Timeline = []TimelineEntry{}
	}
	if t.Links == nil {
		t.Links = []TaskLink{}
	}
	now := s.clock().UTC().Format(time.RFC3339)
	if t.Created == "" {
		t.Created = now
	}
	if t.Updated == "" {
		t.Updated = now
	}
	if t.CommissionedAt == "" {
		t.CommissionedAt = t.Created
	}
	if t.CommissionedBy == "" {
		t.CommissionedBy = t.Agent
	}
	if err := validateTask(t); err != nil {
		return err
	}
	timeline, err := json.Marshal(t.Timeline)
	if err != nil {
		return fmt.Errorf("routerstore: encode timeline: %w", err)
	}
	links, err := json.Marshal(t.Links)
	if err != nil {
		return fmt.Errorf("routerstore: encode links: %w", err)
	}
	_, err = s.db.Exec(`INSERT INTO tasks(agent,task_id,subject,status,phase,responsible_party,blocked_by,created,updated,charter,commissioned_at,commissioned_by,outline,timeline,links,test_state,stage,tokens_consumed,duration_seconds) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?);`,
		t.Agent, t.TaskID, strings.TrimSpace(t.Subject), t.Status, strings.TrimSpace(t.Phase), t.ResponsibleParty, strings.TrimSpace(t.BlockedBy), t.Created, t.Updated, t.Charter, t.CommissionedAt, t.CommissionedBy, t.Outline, string(timeline), string(links), t.TestState, t.Stage, t.TokensConsumed, t.DurationSeconds)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrTaskExists
		}
		return fmt.Errorf("routerstore: AddTask %s/%s: %w", t.Agent, t.TaskID, err)
	}
	return nil
}

type TaskUpdate struct {
	Subject, Status, Phase, ResponsibleParty, BlockedBy string
	BlockedBySet                                        bool
	Charter, Outline                                    string
	CharterSet, OutlineSet                              bool
	Timeline                                            []TimelineEntry
	TimelineSet                                         bool
	Links                                               []TaskLink
	TestState, Stage                                    string
	AddTokens, AddSeconds                               int64
}

func mergeLinks(existing, added []TaskLink) []TaskLink {
	out := append([]TaskLink(nil), existing...)
	seen := make(map[string]bool, len(out))
	for _, link := range out {
		seen[link.URL] = true
	}
	for _, link := range added {
		if !seen[link.URL] {
			out = append(out, link)
			seen[link.URL] = true
		}
	}
	return out
}

func hasOwnerInstruction(links []TaskLink) bool {
	for _, link := range links {
		if link.Kind == "owner-instruction" {
			return true
		}
	}
	return false
}

func (s *Store) UpdateTask(agent, taskID string, u TaskUpdate) (Task, error) {
	if u.AddTokens < 0 || u.AddSeconds < 0 {
		return Task{}, fmt.Errorf("routerstore: accounting increments cannot be negative")
	}
	t, err := s.GetTask(agent, taskID)
	if err != nil {
		return Task{}, err
	}
	oldStage, oldSubject, oldCharter := t.Stage, t.Subject, t.Charter
	if u.Subject != "" {
		t.Subject = u.Subject
	}
	if u.Status != "" {
		t.Status = u.Status
	}
	if u.Phase != "" {
		t.Phase = u.Phase
	}
	if u.ResponsibleParty != "" {
		t.ResponsibleParty = u.ResponsibleParty
	}
	if u.BlockedBySet {
		t.BlockedBy = strings.TrimSpace(u.BlockedBy)
	}
	if u.CharterSet {
		value := u.Charter
		t.Charter = &value
	}
	if u.OutlineSet {
		value := u.Outline
		t.Outline = &value
	}
	if u.TimelineSet {
		t.Timeline = u.Timeline
	}
	for i, link := range u.Links {
		if !validLinkKind(link.Kind) || strings.TrimSpace(link.Label) == "" || strings.TrimSpace(link.URL) == "" {
			return Task{}, fmt.Errorf("routerstore: invalid task link %d", i)
		}
	}
	t.Links = mergeLinks(t.Links, u.Links)
	if u.TestState != "" {
		t.TestState = u.TestState
	}
	if u.Stage != "" {
		t.Stage = u.Stage
	}
	if stageRank[t.Stage] < stageRank[oldStage] && (strings.TrimSpace(u.Subject) == "" || strings.TrimSpace(u.Subject) == strings.TrimSpace(oldSubject)) {
		return Task{}, fmt.Errorf("routerstore: stage regression requires a replacement subject stating why")
	}
	if u.CharterSet && oldCharter != nil && *oldCharter != u.Charter && !hasOwnerInstruction(u.Links) {
		return Task{}, fmt.Errorf("routerstore: charter amendment requires an owner-instruction link")
	}
	t.TokensConsumed += u.AddTokens
	t.DurationSeconds += u.AddSeconds
	if validationErr := validateTask(t); validationErr != nil {
		return Task{}, validationErr
	}
	t.Updated = s.clock().UTC().Format(time.RFC3339)
	timeline, err := json.Marshal(t.Timeline)
	if err != nil {
		return Task{}, fmt.Errorf("routerstore: encode timeline: %w", err)
	}
	links, err := json.Marshal(t.Links)
	if err != nil {
		return Task{}, fmt.Errorf("routerstore: encode links: %w", err)
	}
	_, err = s.db.Exec(`UPDATE tasks SET subject=?,status=?,phase=?,responsible_party=?,blocked_by=?,updated=?,charter=?,outline=?,timeline=?,links=?,test_state=?,stage=?,tokens_consumed=tokens_consumed+?,duration_seconds=duration_seconds+? WHERE agent=? AND task_id=?;`,
		t.Subject, t.Status, t.Phase, t.ResponsibleParty, t.BlockedBy, t.Updated, t.Charter, t.Outline, string(timeline), string(links), t.TestState, t.Stage, u.AddTokens, u.AddSeconds, agent, taskID)
	if err != nil {
		return Task{}, fmt.Errorf("routerstore: UpdateTask %s/%s: %w", agent, taskID, err)
	}
	return s.GetTask(agent, taskID)
}

const taskSelect = `SELECT agent,task_id,subject,status,phase,responsible_party,blocked_by,created,updated,charter,commissioned_at,commissioned_by,outline,timeline,links,test_state,stage,tokens_consumed,duration_seconds FROM tasks`

func scanTask(scanner interface{ Scan(...any) error }) (Task, error) {
	var t Task
	var charter, outline sql.NullString
	var timeline, links string
	err := scanner.Scan(&t.Agent, &t.TaskID, &t.Subject, &t.Status, &t.Phase, &t.ResponsibleParty, &t.BlockedBy, &t.Created, &t.Updated, &charter, &t.CommissionedAt, &t.CommissionedBy, &outline, &timeline, &links, &t.TestState, &t.Stage, &t.TokensConsumed, &t.DurationSeconds)
	if err != nil {
		return Task{}, err
	}
	if charter.Valid {
		t.Charter = &charter.String
	}
	if outline.Valid {
		t.Outline = &outline.String
	}
	if err := json.Unmarshal([]byte(timeline), &t.Timeline); err != nil {
		return Task{}, fmt.Errorf("decode timeline: %w", err)
	}
	if err := json.Unmarshal([]byte(links), &t.Links); err != nil {
		return Task{}, fmt.Errorf("decode links: %w", err)
	}
	if t.Timeline == nil {
		t.Timeline = []TimelineEntry{}
	}
	if t.Links == nil {
		t.Links = []TaskLink{}
	}
	return t, nil
}

func (s *Store) GetTask(agent, taskID string) (Task, error) {
	t, err := scanTask(s.db.QueryRow(taskSelect+` WHERE agent=? AND task_id=?;`, agent, taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("routerstore: GetTask %s/%s: %w", agent, taskID, err)
	}
	t.Liveness = deriveTaskLiveness(t, s.clock().UTC())
	return t, nil
}

func (s *Store) ListTasks(agent string) ([]Task, error) {
	query := taskSelect
	var rows *sql.Rows
	var err error
	if strings.TrimSpace(agent) == "" {
		rows, err = s.db.Query(query + ` ORDER BY agent,created,task_id;`)
	} else {
		rows, err = s.db.Query(query+` WHERE agent=? ORDER BY created,task_id;`, agent)
	}
	if err != nil {
		return nil, fmt.Errorf("routerstore: ListTasks: %w", err)
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		t, scanErr := scanTask(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("routerstore: ListTasks scan: %w", scanErr)
		}
		t.Liveness = deriveTaskLiveness(t, s.clock().UTC())
		out = append(out, t)
	}
	return out, rows.Err()
}
