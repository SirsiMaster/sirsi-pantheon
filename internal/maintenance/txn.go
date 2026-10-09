// Package maintenance implements ADR-079's save-before-maintenance contract.
//
// This is P1 only: Enumerate + a provisional Mint. SSA's ACCEPT_BOUNDED_DESIGN
// (PR #1055 head 6fe284ce) is explicit that P1 is strictly preservation-only —
// it may run and produce evidence, but it never grants readiness/QUIESCENT by
// itself. Nothing in this package can be mistaken for a readiness decision:
// Txn carries no Ready/Quiescent field, Mint always sets Provisional=true, and
// there is no Decide/Publish phase here. The reconciled (registry UNION
// independent-discovery) mint and the quiescence/fence machinery are P2/P3
// work and do not exist in this package.
package maintenance

import (
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// Scope bounds an enumeration/mint to one host. Host is required: a
// cross-host scope needs the reconciled union discovery P1 does not perform.
// AgentPrefix optionally narrows to agent ids sharing a prefix; empty means
// every agent on the host.
type Scope struct {
	Host        string `json:"host"`
	AgentPrefix string `json:"agent_prefix,omitempty"`
}

// Participant is one live, registered thread captured at enumeration time.
type Participant struct {
	ThreadID  string `json:"thread_id"`
	AgentID   string `json:"agent_id"`
	Host      string `json:"host"`
	MachineID string `json:"machine_id,omitempty"`
	PID       int    `json:"pid,omitempty"`
}

// EnumerateRegistered reads the thread registry for every live (non-terminal,
// non-suspended — ADR-025 suspended threads are a resting state, not a live
// participant) thread in scope. This is Decision §1's P1 slice only: the
// registered list alone, with no independent-discovery reconciliation
// against A33 census output. A readiness-capable phase (P2/P3) must still
// perform that reconciliation before anything here can gate a maintenance
// action. Fails closed on a nil registry or missing scope — never returns a
// partial list silently.
func EnumerateRegistered(reg *router.ThreadRegistry, scope Scope) ([]Participant, error) {
	if reg == nil {
		return nil, fmt.Errorf("maintenance: nil thread registry")
	}
	if scope.Host == "" {
		return nil, fmt.Errorf("maintenance: scope.Host is required")
	}
	var out []Participant
	for _, t := range reg.SortedThreads() {
		if t == nil || t.Host != scope.Host {
			continue
		}
		if t.Status.IsTerminal() || t.Status == router.ThreadStatusSuspended {
			continue
		}
		if scope.AgentPrefix != "" && !strings.HasPrefix(t.AgentID, scope.AgentPrefix) {
			continue
		}
		out = append(out, Participant{
			ThreadID:  t.ThreadID,
			AgentID:   t.AgentID,
			Host:      t.Host,
			MachineID: t.MachineID,
			PID:       t.PID,
		})
	}
	sortParticipants(out)
	return out, nil
}

// Txn is a P1 preservation-only snapshot: an expiring id plus a digest
// binding the exact scope and participant list at mint time. Provisional is
// always true for a P1 mint. Deliberately absent: any field a caller could
// read as a readiness grant. Callers MUST NOT treat a Txn as authorizing any
// maintenance action.
type Txn struct {
	ID           string        `json:"id"`
	Scope        Scope         `json:"scope"`
	Participants []Participant `json:"participants"`
	Digest       string        `json:"digest"`
	MintedAt     time.Time     `json:"minted_at"`
	ExpiresAt    time.Time     `json:"expires_at"`
	Provisional  bool          `json:"provisional"`
}

// Mint issues a provisional P1 txn over participants already produced by
// EnumerateRegistered. Per SSA's binding reading, a mint over provisional
// registry-only data is NOT a readiness transaction; the actual
// readiness-capable mint must be frozen over the reconciled
// (registry-union-independent-discovery) set, which this function does not
// compute. now is injected so callers get deterministic expiry in tests.
func Mint(scope Scope, participants []Participant, ttl time.Duration, now time.Time) (*Txn, error) {
	if scope.Host == "" {
		return nil, fmt.Errorf("maintenance: scope.Host is required")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("maintenance: ttl must be positive")
	}
	sorted := append([]Participant(nil), participants...)
	sortParticipants(sorted)

	id, err := randomID()
	if err != nil {
		return nil, err
	}
	digest, err := digestOf(id, scope, sorted)
	if err != nil {
		return nil, err
	}
	return &Txn{
		ID:           id,
		Scope:        scope,
		Participants: sorted,
		Digest:       digest,
		MintedAt:     now,
		ExpiresAt:    now.Add(ttl),
		Provisional:  true,
	}, nil
}

func sortParticipants(p []Participant) {
	sort.Slice(p, func(i, j int) bool { return p[i].ThreadID < p[j].ThreadID })
}

// digestOf hashes the exact bytes a participant is agreeing to: the txn id,
// scope, and sorted participant list. Go's encoding/json already sorts map
// keys and this payload has no maps, so plain Marshal over the
// already-sorted slice is deterministic — no RFC 8785 canonicalizer needed
// for a fixed-shape internal struct (unlike namespec's external schema
// documents).
func digestOf(id string, scope Scope, sorted []Participant) (string, error) {
	payload := struct {
		ID           string        `json:"id"`
		Scope        Scope         `json:"scope"`
		Participants []Participant `json:"participants"`
	}{id, scope, sorted}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("maintenance: marshal digest payload: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func randomID() (string, error) {
	var buf [16]byte
	if _, err := cryptorand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("maintenance: generate txn id: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
