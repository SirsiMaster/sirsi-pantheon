package routerstore

// Canonical requirement registry — ADR-061 step 1.
//
// This is the referent for the third term of the runnable predicate ("unmet
// traced canon requirement") and the thing the completion gate traces to.
// Without it, "all canon implemented" is an opinion: nothing enumerates what
// canon requires, so nothing can report a gap, and "done" means whatever the
// claiming agent believed at the time.
//
// The registry deliberately stores EVIDENCE REFERENCES rather than booleans.
// A boolean `tests_pass` records that someone once believed the tests passed;
// a `tests_ref` records which run, so a reviewer can go look. ADR-061 §6
// requires seven references before a requirement may be called satisfied, and
// Satisfy() refuses without them — the gate is code, not a checklist an agent
// is trusted to have followed.

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Requirement statuses.
const (
	ReqOpen       = "open"
	ReqInProgress = "in_progress"
	ReqSatisfied  = "satisfied"
	ReqWaived     = "waived"
)

// Evidence carries the seven references ADR-061 §6 demands before a
// requirement may reach a terminal verified disposition.
type Evidence struct {
	Commit     string `json:"commit_ref,omitempty"`
	Tests      string `json:"tests_ref,omitempty"`
	Security   string `json:"security_ref,omitempty"`
	Design     string `json:"design_ref,omitempty"`
	Deployment string `json:"deployment_ref,omitempty"`
	Production string `json:"production_ref,omitempty"`
}

// Missing lists the evidence fields that are still empty, in the order
// ADR-061 §6 states them. The requirement ID itself is the seventh reference
// and is structural — a Requirement cannot exist without one.
func (e Evidence) Missing() []string {
	var missing []string
	for _, f := range []struct {
		name string
		val  string
	}{
		{"commit/PR", e.Commit},
		{"tests", e.Tests},
		{"security control", e.Security},
		{"design acceptance", e.Design},
		{"deployment version", e.Deployment},
		{"production verification", e.Production},
	} {
		if strings.TrimSpace(f.val) == "" {
			missing = append(missing, f.name)
		}
	}
	return missing
}

// Requirement is one canonical, traceable obligation.
type Requirement struct {
	ID           string   `json:"req_id"`
	Title        string   `json:"title"`
	Source       string   `json:"source"`
	SourceRef    string   `json:"source_ref,omitempty"`
	Owner        string   `json:"owner"`
	Status       string   `json:"status"`
	Evidence     Evidence `json:"evidence"`
	WaiverReason string   `json:"waiver_reason,omitempty"`
	WaiverRef    string   `json:"waiver_ref,omitempty"`
	Created      string   `json:"created"`
	Updated      string   `json:"updated"`
}

// Unmet reports whether this requirement still counts toward the runnable
// predicate. Waived requirements are met by explicit decision; satisfied ones
// carry full evidence. Everything else is outstanding work.
func (r Requirement) Unmet() bool { return r.Status != ReqSatisfied && r.Status != ReqWaived }

var reqIDPattern = regexp.MustCompile(`^REQ-[0-9]{3,}$`)

// AddRequirement registers a requirement, allocating its REQ-NNN through the
// same durable allocator that hands out ADR numbers — so two agents cannot
// register colliding requirement IDs.
func (s *SQLiteStore) AddRequirement(title, source, sourceRef, owner string) (Requirement, error) {
	title = strings.TrimSpace(title)
	source = strings.TrimSpace(source)
	owner = strings.TrimSpace(owner)
	if title == "" {
		return Requirement{}, errors.New("routerstore: requirement title is required")
	}
	if source == "" {
		return Requirement{}, errors.New("routerstore: requirement source is required — an untraced requirement cannot be audited against canon, which is the only reason the registry exists")
	}
	if owner == "" {
		return Requirement{}, errors.New("routerstore: requirement owner is required")
	}

	id, err := s.AllocateIdentifier("REQ", title, owner)
	if err != nil {
		return Requirement{}, err
	}

	now := s.clock().UTC().Format(time.RFC3339)
	req := Requirement{
		ID: id.Name(), Title: title, Source: source, SourceRef: sourceRef,
		Owner: owner, Status: ReqOpen, Created: now, Updated: now,
	}
	if err := s.withTx(func(tx *txHandle) error {
		_, err := tx.Exec(
			`INSERT INTO requirements (req_id, title, source, source_ref, owner, status, created, updated)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?);`,
			req.ID, req.Title, req.Source, req.SourceRef, req.Owner, req.Status, req.Created, req.Updated)
		return err
	}); err != nil {
		return Requirement{}, fmt.Errorf("routerstore: add %s: %w", req.ID, err)
	}
	return req, nil
}

// RecordEvidence attaches or updates evidence references without asserting
// satisfaction. Only non-empty fields overwrite; this is additive so evidence
// can accumulate across several passes.
func (s *SQLiteStore) RecordEvidence(reqID string, ev Evidence) error {
	reqID = strings.ToUpper(strings.TrimSpace(reqID))
	if !reqIDPattern.MatchString(reqID) {
		return fmt.Errorf("routerstore: invalid requirement id %q (want REQ-NNN)", reqID)
	}
	return s.withTx(func(tx *txHandle) error {
		var cur Evidence
		row := tx.QueryRow(
			`SELECT commit_ref, tests_ref, security_ref, design_ref, deployment_ref, production_ref
			   FROM requirements WHERE req_id = ?;`, reqID)
		if err := row.Scan(&cur.Commit, &cur.Tests, &cur.Security, &cur.Design, &cur.Deployment, &cur.Production); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("routerstore: %s not registered", reqID)
			}
			return err
		}
		merged := Evidence{
			Commit:     firstNonEmpty(ev.Commit, cur.Commit),
			Tests:      firstNonEmpty(ev.Tests, cur.Tests),
			Security:   firstNonEmpty(ev.Security, cur.Security),
			Design:     firstNonEmpty(ev.Design, cur.Design),
			Deployment: firstNonEmpty(ev.Deployment, cur.Deployment),
			Production: firstNonEmpty(ev.Production, cur.Production),
		}
		_, err := tx.Exec(
			`UPDATE requirements SET commit_ref=?, tests_ref=?, security_ref=?, design_ref=?,
			        deployment_ref=?, production_ref=?, status=CASE WHEN status=? THEN ? ELSE status END, updated=?
			   WHERE req_id=?;`,
			merged.Commit, merged.Tests, merged.Security, merged.Design, merged.Deployment, merged.Production,
			ReqOpen, ReqInProgress, s.clock().UTC().Format(time.RFC3339), reqID)
		return err
	})
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// ErrIncompleteEvidence is returned when Satisfy is called on a requirement
// that has not accumulated all six evidence references.
var ErrIncompleteEvidence = errors.New("routerstore: incomplete evidence")

// Satisfy moves a requirement to its terminal verified disposition. It REFUSES
// unless every ADR-061 §6 evidence reference is present.
//
// This is the completion gate. It is deliberately impossible to satisfy a
// requirement by asserting that it is done: the store checks the references
// itself, so a green build or a merged PR — each of which is only one of the
// six — cannot stand in for the whole.
func (s *SQLiteStore) Satisfy(reqID string) error {
	reqID = strings.ToUpper(strings.TrimSpace(reqID))
	if !reqIDPattern.MatchString(reqID) {
		return fmt.Errorf("routerstore: invalid requirement id %q (want REQ-NNN)", reqID)
	}
	return s.withTx(func(tx *txHandle) error {
		var ev Evidence
		row := tx.QueryRow(
			`SELECT commit_ref, tests_ref, security_ref, design_ref, deployment_ref, production_ref
			   FROM requirements WHERE req_id = ?;`, reqID)
		if err := row.Scan(&ev.Commit, &ev.Tests, &ev.Security, &ev.Design, &ev.Deployment, &ev.Production); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("routerstore: %s not registered", reqID)
			}
			return err
		}
		if missing := ev.Missing(); len(missing) > 0 {
			return fmt.Errorf("%w: %s cannot be satisfied — missing %s", ErrIncompleteEvidence, reqID, strings.Join(missing, ", "))
		}
		_, err := tx.Exec(`UPDATE requirements SET status=?, updated=? WHERE req_id=?;`,
			ReqSatisfied, s.clock().UTC().Format(time.RFC3339), reqID)
		return err
	})
}

// Waive records an explicit decision that a requirement will not be met. A
// reason is mandatory: a waiver without one is indistinguishable from a
// requirement that was quietly dropped.
func (s *SQLiteStore) Waive(reqID, reason, ownerDecisionRef string) error {
	reqID = strings.ToUpper(strings.TrimSpace(reqID))
	if !reqIDPattern.MatchString(reqID) {
		return fmt.Errorf("routerstore: invalid requirement id %q (want REQ-NNN)", reqID)
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("routerstore: waiver reason is required — an unexplained waiver is a dropped requirement wearing a terminal status")
	}
	ownerDecisionRef = strings.TrimSpace(ownerDecisionRef)
	if ownerDecisionRef == "" {
		return errors.New("routerstore: waiver requires a terminal owner decision router item reference")
	}
	return s.withTx(func(tx *txHandle) error {
		var decisions int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM items WHERE id=? AND from_agent='owner' AND type='decision' AND status IN ('closed','completed') AND result<>'';`, ownerDecisionRef).Scan(&decisions); err != nil {
			return err
		}
		if decisions != 1 {
			return fmt.Errorf("routerstore: waiver authority %q is not a terminal owner decision item with evidence", ownerDecisionRef)
		}
		res, err := tx.Exec(`UPDATE requirements SET status=?, waiver_reason=?, waiver_ref=?, updated=? WHERE req_id=?;`,
			ReqWaived, reason, ownerDecisionRef, s.clock().UTC().Format(time.RFC3339), reqID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("routerstore: %s not registered", reqID)
		}
		return nil
	})
}

// ListRequirements returns requirements, optionally filtered to one owner.
func (s *SQLiteStore) ListRequirements(owner string) ([]Requirement, error) {
	q := `SELECT req_id, title, source, source_ref, owner, status,
	             commit_ref, tests_ref, security_ref, design_ref, deployment_ref, production_ref,
	             waiver_reason, waiver_ref, created, updated FROM requirements`
	var args []any
	if o := strings.TrimSpace(owner); o != "" {
		q += ` WHERE owner = ?`
		args = append(args, o)
	}
	rows, err := s.db.Query(q+`;`, args...)
	if err != nil {
		return nil, fmt.Errorf("routerstore: list requirements: %w", err)
	}
	defer rows.Close()

	var out []Requirement
	for rows.Next() {
		var r Requirement
		if err := rows.Scan(&r.ID, &r.Title, &r.Source, &r.SourceRef, &r.Owner, &r.Status,
			&r.Evidence.Commit, &r.Evidence.Tests, &r.Evidence.Security, &r.Evidence.Design,
			&r.Evidence.Deployment, &r.Evidence.Production, &r.WaiverReason, &r.WaiverRef, &r.Created, &r.Updated); err != nil {
			return nil, fmt.Errorf("routerstore: list requirements: scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// UnmetRequirements returns the outstanding requirements for owner — the third
// term of the ADR-061 runnable predicate. An empty result is what lets a lane
// legitimately park; it is NOT the same as an absent registry, which is why
// callers must distinguish "zero unmet" from "no registry".
func (s *SQLiteStore) UnmetRequirements(owner string) ([]Requirement, error) {
	all, err := s.ListRequirements(owner)
	if err != nil {
		return nil, err
	}
	var unmet []Requirement
	for _, r := range all {
		if r.Unmet() {
			unmet = append(unmet, r)
		}
	}
	return unmet, nil
}
