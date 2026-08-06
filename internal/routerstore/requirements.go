package routerstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RequirementEvidence is one reproducible completion reference. Kind is a
// fixed vocabulary so R6 cannot be satisfied by an unlabeled URL dump.
type RequirementEvidence struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Ref   string `json:"ref"`
}

// Requirement is one canonical obligation assigned to an execution lane.
// SourcePath+SourceAnchor points back to governing canon; task linkage makes
// an unmet requirement mechanically actionable under R1.
type Requirement struct {
	RequirementID string                `json:"requirement_id"`
	Agent         string                `json:"agent"`
	SourcePath    string                `json:"source_path"`
	SourceAnchor  string                `json:"source_anchor"`
	Statement     string                `json:"statement"`
	Status        string                `json:"status"`
	TaskID        string                `json:"task_id"`
	Evidence      []RequirementEvidence `json:"evidence"`
	Created       string                `json:"created"`
	Updated       string                `json:"updated"`
}

var ErrRequirementExists = errors.New("routerstore: requirement already exists")

func validRequirementStatus(status string) bool {
	switch status {
	case "unmet", "in-progress", "blocked", "verified", "waived":
		return true
	}
	return false
}

func validRequirementEvidenceKind(kind string) bool {
	switch kind {
	case "implementation", "test", "security", "design", "deployment", "production", "waiver":
		return true
	}
	return false
}

func validateRequirement(r Requirement) error {
	if strings.TrimSpace(r.RequirementID) == "" || strings.TrimSpace(r.Agent) == "" ||
		strings.TrimSpace(r.SourcePath) == "" || strings.TrimSpace(r.Statement) == "" {
		return fmt.Errorf("routerstore: requirement id, agent, source path, and statement are required")
	}
	if !validRequirementStatus(r.Status) {
		return fmt.Errorf("routerstore: invalid requirement status %q", r.Status)
	}
	kinds := map[string]bool{}
	for i, evidence := range r.Evidence {
		if !validRequirementEvidenceKind(evidence.Kind) || strings.TrimSpace(evidence.Label) == "" || strings.TrimSpace(evidence.Ref) == "" {
			return fmt.Errorf("routerstore: invalid requirement evidence %d", i)
		}
		kinds[evidence.Kind] = true
	}
	if r.Status == "verified" {
		for _, required := range []string{"implementation", "test", "deployment", "production"} {
			if !kinds[required] {
				return fmt.Errorf("routerstore: verified requirement requires %s evidence", required)
			}
		}
	}
	if r.Status == "waived" && !kinds["waiver"] {
		return fmt.Errorf("routerstore: waived requirement requires waiver evidence")
	}
	return nil
}

// AddRequirement persists a canon obligation exactly once.
func (s *Store) AddRequirement(r Requirement) error {
	if r.Status == "" {
		r.Status = "unmet"
	}
	if r.Evidence == nil {
		r.Evidence = []RequirementEvidence{}
	}
	now := s.clock().UTC().Format(time.RFC3339)
	if r.Created == "" {
		r.Created = now
	}
	if r.Updated == "" {
		r.Updated = now
	}
	if err := validateRequirement(r); err != nil {
		return err
	}
	evidence, err := json.Marshal(r.Evidence)
	if err != nil {
		return fmt.Errorf("routerstore: encode requirement evidence: %w", err)
	}
	_, err = s.db.Exec(`INSERT INTO requirements(requirement_id,agent,source_path,source_anchor,statement,status,task_id,evidence,created,updated) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		r.RequirementID, r.Agent, r.SourcePath, r.SourceAnchor, strings.TrimSpace(r.Statement), r.Status, r.TaskID, string(evidence), r.Created, r.Updated)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrRequirementExists
		}
		return fmt.Errorf("routerstore: add requirement %s: %w", r.RequirementID, err)
	}
	return nil
}

// ListRequirements returns deterministic canon truth, optionally lane-scoped.
func (s *Store) ListRequirements(agent string) ([]Requirement, error) {
	query := `SELECT requirement_id,agent,source_path,source_anchor,statement,status,task_id,evidence,created,updated FROM requirements`
	var args []any
	if agent != "" {
		query += ` WHERE agent = ?`
		args = append(args, agent)
	}
	query += ` ORDER BY agent, requirement_id`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("routerstore: list requirements: %w", err)
	}
	defer rows.Close()
	var out []Requirement
	for rows.Next() {
		var r Requirement
		var evidence string
		if err := rows.Scan(&r.RequirementID, &r.Agent, &r.SourcePath, &r.SourceAnchor, &r.Statement, &r.Status, &r.TaskID, &evidence, &r.Created, &r.Updated); err != nil {
			return nil, fmt.Errorf("routerstore: scan requirement: %w", err)
		}
		if err := json.Unmarshal([]byte(evidence), &r.Evidence); err != nil {
			return nil, fmt.Errorf("routerstore: decode requirement %s evidence: %w", r.RequirementID, err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("routerstore: iterate requirements: %w", err)
	}
	if out == nil {
		out = []Requirement{}
	}
	return out, nil
}

// GetRequirement reads one canonical obligation.
func (s *Store) GetRequirement(id string) (Requirement, error) {
	var r Requirement
	var evidence string
	err := s.db.QueryRow(`SELECT requirement_id,agent,source_path,source_anchor,statement,status,task_id,evidence,created,updated FROM requirements WHERE requirement_id=?`, id).
		Scan(&r.RequirementID, &r.Agent, &r.SourcePath, &r.SourceAnchor, &r.Statement, &r.Status, &r.TaskID, &evidence, &r.Created, &r.Updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Requirement{}, ErrNotFound
	}
	if err != nil {
		return Requirement{}, fmt.Errorf("routerstore: get requirement %s: %w", id, err)
	}
	if err := json.Unmarshal([]byte(evidence), &r.Evidence); err != nil {
		return Requirement{}, fmt.Errorf("routerstore: decode requirement %s evidence: %w", id, err)
	}
	return r, nil
}

// UpdateRequirement replaces lifecycle/evidence fields after validating the
// resulting record. It cannot detach identity or canon source coordinates.
func (s *Store) UpdateRequirement(id, status, taskID string, evidence []RequirementEvidence) (Requirement, error) {
	r, err := s.GetRequirement(id)
	if err != nil {
		return Requirement{}, err
	}
	if status != "" {
		r.Status = status
	}
	r.TaskID = strings.TrimSpace(taskID)
	if evidence != nil {
		r.Evidence = evidence
	}
	r.Updated = s.clock().UTC().Format(time.RFC3339)
	if validationErr := validateRequirement(r); validationErr != nil {
		return Requirement{}, validationErr
	}
	encoded, err := json.Marshal(r.Evidence)
	if err != nil {
		return Requirement{}, fmt.Errorf("routerstore: encode requirement evidence: %w", err)
	}
	if _, updateErr := s.db.Exec(`UPDATE requirements SET status=?,task_id=?,evidence=?,updated=? WHERE requirement_id=?`, r.Status, r.TaskID, string(encoded), r.Updated, id); updateErr != nil {
		return Requirement{}, fmt.Errorf("routerstore: update requirement %s: %w", id, updateErr)
	}
	return r, nil
}
