package routerstore

// Lane-to-Mac pins (schema v30, owner decision 2026-11-25): the router's OWN record of which Mac each lane
// belongs to. Set and retired only by the router operator (`sirsi router lane-pin ...` against the service's
// backend); none of these methods is served or relayed (serve.go notServed, spool.go spoolRefused). The pin is
// never derived from a thread record's host, which is self-asserted. REPORT-ONLY: the verdict is counted at a
// served Wait (lanepinobserve.go) and changes no outcome.

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// LanePin is one row of lane_machine_pins. Retired is "" while the pin is active.
type LanePin struct {
	Agent     string `json:"agent"`
	MachineID string `json:"machine_id"`
	SetBy     string `json:"set_by"`
	Reason    string `json:"reason,omitempty"`
	Created   string `json:"created"`
	Retired   string `json:"retired,omitempty"`
}

var (
	// ErrLanePinInvalid: an agent, machine id, operator or reason the pin table refuses.
	ErrLanePinInvalid = errors.New("routerstore: lane pin: invalid agent, machine id, operator or reason")
	// ErrLanePinUnknown: RetireLanePin found no ACTIVE pin for that (agent, machine id).
	ErrLanePinUnknown = errors.New("routerstore: lane pin: no active pin for that agent and machine id")
)

const (
	lanePinMaxIdent  = 128
	lanePinMaxReason = 512
)

// validPinIdent: non-empty, bounded, no whitespace or control characters (it is compared byte for byte).
func validPinIdent(s string) bool {
	if s == "" || len(s) > lanePinMaxIdent {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// SetLanePin records that agent runs on machineID. A lane may hold several active pins. Setting a pin that is
// already active is a no-op that returns the existing row (its original set_by/reason/created are kept); a
// retired pin does not block a new one.
func (s *SQLiteStore) SetLanePin(agent, machineID, setBy, reason string) (LanePin, error) {
	agent, machineID, setBy, reason = strings.TrimSpace(agent), strings.TrimSpace(machineID), strings.TrimSpace(setBy), strings.TrimSpace(reason)
	if !validPinIdent(agent) || !validPinIdent(machineID) || !validPinIdent(setBy) || len(reason) > lanePinMaxReason {
		return LanePin{}, ErrLanePinInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return LanePin{}, fmt.Errorf("routerstore: SetLanePin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var cur LanePin
	err = tx.QueryRow(`SELECT agent,machine_id,set_by,reason,created,retired FROM lane_machine_pins WHERE agent=? AND machine_id=? AND retired=''`,
		agent, machineID).Scan(&cur.Agent, &cur.MachineID, &cur.SetBy, &cur.Reason, &cur.Created, &cur.Retired)
	switch {
	case err == nil:
		return cur, nil
	case !errors.Is(err, sql.ErrNoRows):
		return LanePin{}, fmt.Errorf("routerstore: SetLanePin: %w", err)
	}
	// Nanosecond precision: the primary key includes created, so a pin set again right after a retire must not collide.
	p := LanePin{Agent: agent, MachineID: machineID, SetBy: setBy, Reason: reason, Created: s.clock().Format(time.RFC3339Nano)}
	if _, err := tx.Exec(`INSERT INTO lane_machine_pins(agent,machine_id,set_by,reason,created,retired) VALUES(?,?,?,?,?,'')`,
		p.Agent, p.MachineID, p.SetBy, p.Reason, p.Created); err != nil {
		return LanePin{}, fmt.Errorf("routerstore: SetLanePin: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return LanePin{}, fmt.Errorf("routerstore: SetLanePin: commit: %w", err)
	}
	return p, nil
}

// RetireLanePin marks the ACTIVE pin retired (the row stays as history). ErrLanePinUnknown when none is active.
func (s *SQLiteStore) RetireLanePin(agent, machineID string) error {
	agent, machineID = strings.TrimSpace(agent), strings.TrimSpace(machineID)
	if !validPinIdent(agent) || !validPinIdent(machineID) {
		return ErrLanePinInvalid
	}
	res, err := s.exec(`UPDATE lane_machine_pins SET retired=? WHERE agent=? AND machine_id=? AND retired=''`,
		s.clock().Format(time.RFC3339Nano), agent, machineID)
	if err != nil {
		return fmt.Errorf("routerstore: RetireLanePin: %w", err)
	}
	if n, nerr := res.RowsAffected(); nerr == nil && n == 0 {
		return ErrLanePinUnknown
	}
	return nil
}

// ListLanePins returns the pins ordered by agent then creation; retired rows only when asked for.
func (s *SQLiteStore) ListLanePins(includeRetired bool) ([]LanePin, error) {
	q := `SELECT agent,machine_id,set_by,reason,created,retired FROM lane_machine_pins`
	if !includeRetired {
		q += ` WHERE retired=''`
	}
	rows, err := s.db.Query(q + ` ORDER BY agent, created, machine_id`)
	if err != nil {
		return nil, fmt.Errorf("routerstore: ListLanePins: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []LanePin
	for rows.Next() {
		var p LanePin
		if err := rows.Scan(&p.Agent, &p.MachineID, &p.SetBy, &p.Reason, &p.Created, &p.Retired); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ActiveLanePinMachines returns the machine ids with an ACTIVE pin for agent (retired pins are ignored), sorted.
// One indexed read (the primary key's agent prefix); the served Wait path caches it (lanepinobserve.go).
func (s *SQLiteStore) ActiveLanePinMachines(agent string) ([]string, error) {
	rows, err := s.db.Query(`SELECT machine_id FROM lane_machine_pins WHERE agent=? AND retired='' ORDER BY machine_id`, agent)
	if err != nil {
		return nil, fmt.Errorf("routerstore: ActiveLanePinMachines: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// LanePinVerdict is the outcome of comparing a caller's token machine id with its lane's active pins.
type LanePinVerdict string

const (
	LanePinMatch       LanePinVerdict = "match"         // the token's machine id is one of the lane's active pins
	LanePinMismatch    LanePinVerdict = "mismatch"      // the lane is pinned and the token's machine id is not among the pins
	LanePinUnpinned    LanePinVerdict = "unpinned"      // the lane has no active pin: nothing to compare against
	LanePinNoMachineID LanePinVerdict = "no-machine-id" // the lane is pinned but the token has adopted no machine id (or is the bootstrap token)
	lanePinError       LanePinVerdict = "error"         // the evaluation itself failed; counted, never an outcome
	lanePinSkipped     LanePinVerdict = "skipped"       // the lookup cache was full; counted, never an outcome
)

// EvaluateLanePin is the pure verdict. callerMachineID is host_tokens.machine_id of the token the request
// authenticated with ("" when none was adopted); activePins is the lane's active pin set. Order matters: with
// no pin there is nothing to mismatch, whatever the caller holds.
func EvaluateLanePin(callerMachineID string, activePins []string) LanePinVerdict {
	switch {
	case len(activePins) == 0:
		return LanePinUnpinned
	case callerMachineID == "":
		return LanePinNoMachineID
	}
	for _, p := range activePins {
		if p == callerMachineID {
			return LanePinMatch
		}
	}
	return LanePinMismatch
}
