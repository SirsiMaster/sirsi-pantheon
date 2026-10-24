package routerstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SpoolAgentOutbox is one agent's held (queued-for-retry) outbox depth on a
// spool relay host — a read-only count, never a mutation. An item lands here
// when the relay could not reach the service and is holding the request for
// drainOutbox's next scheduled retry (ADR-069).
//
// Unreadable (with Error set) means the outbox directory exists but could not
// be listed (permission denied, not-a-directory, ...) — this is NOT the same
// as "no held items" and must render as unknown/error, never as quiet
// (codex-pantheon review of PR #931, item 20261001-010622: filepath.Glob
// silently swallows directory read errors, reporting a chmod-0000 outbox as
// nil/nil — zero held items, indistinguishable from genuinely empty).
type SpoolAgentOutbox struct {
	Agent           string            `json:"agent"`
	QueuedForRetry  int               `json:"queued_for_retry,omitempty"`
	Unreadable      bool              `json:"unreadable,omitempty"`
	Error           string            `json:"error,omitempty"`
	ObserverAccess  string            `json:"observer_access,omitempty"`
	RetryState      string            `json:"retry_state,omitempty"`
	RetryCount      *int              `json:"retry_count,omitempty"`
	RetrySource     string            `json:"retry_source,omitempty"`
	RetryObservedAt *time.Time        `json:"retry_observed_at,omitempty"`
	RetryStartedAt  *time.Time        `json:"retry_started_at,omitempty"`
	RepairEligible  bool              `json:"repair_eligible,omitempty"`
	RepairSpool     string            `json:"repair_spool,omitempty"`
	Scope           *SpoolOutboxScope `json:"scope,omitempty"`
}

// Scope accompanies every row, including readable post-repair observations.
// Consent and verification bind the same agent AND this opened root identity.
type SpoolOutboxScope struct {
	CanonicalSpool string `json:"canonical_spool"`
	RootDevice     uint64 `json:"root_device"`
	RootInode      uint64 `json:"root_inode"`
}

// Coverage is independent of optional publisher availability. A completely
// readable direct inventory needs no publisher; a partial inventory is never
// complete merely because some individually eligible rows exist.
type RelayOutboxCoverage struct {
	Complete   bool       `json:"complete"`
	Source     string     `json:"source"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}

type RelayOutboxHealth struct {
	Outbox      []SpoolAgentOutbox
	Observation *RelayOutboxObservation
	ErrorCode   string
	Coverage    RelayOutboxCoverage
}

// CollectRelayOutboxHealth separates observer access from authenticated
// producer metadata. A fresh board cannot freshen an old producer snapshot.
// Incomplete coverage never acquires an aggregate; individually observed lanes
// remain useful. Merely sharing the root's gid never authenticates a producer.
func CollectRelayOutboxHealth(spoolRoot, trustGroup string) RelayOutboxHealth {
	r, err := openRelayObservationRoot(spoolRoot, trustGroup)
	if err != nil {
		return RelayOutboxHealth{ErrorCode: err.Error(), Coverage: RelayOutboxCoverage{Source: "unavailable"}}
	}
	defer r.close()
	direct := r.scan(time.Now())
	if direct.validate(time.Now(), uint64(r.stat.Dev), uint64(r.stat.Ino), r.uid) != nil || r.continuity() != nil {
		direct.Enumerated, direct.Complete, direct.Queued, direct.ErrorCode = false, false, nil, "limit"
		for i := range direct.Lanes {
			direct.Lanes[i].State, direct.Lanes[i].Queued, direct.Lanes[i].ErrorCode = "unavailable", nil, "limit"
		}
	}
	result := RelayOutboxHealth{Coverage: RelayOutboxCoverage{Complete: direct.Complete, Source: "direct-metadata", StartedAt: &direct.StartedAt, ObservedAt: &direct.ObservedAt}}
	if !direct.Enumerated {
		result.ErrorCode = "relay observer: " + direct.ErrorCode
	}
	snapshot, err := r.read(time.Now, nil)
	if err != nil {
		result.ErrorCode = err.Error()
	} else {
		result.Observation = &snapshot
		if !direct.Complete {
			result.Coverage = RelayOutboxCoverage{Complete: snapshot.Complete, Source: "relay-producer", StartedAt: &snapshot.StartedAt, ObservedAt: &snapshot.ObservedAt}
		}
	}
	scope := &SpoolOutboxScope{CanonicalSpool: r.path, RootDevice: uint64(r.stat.Dev), RootInode: uint64(r.stat.Ino)}
	rows := map[string]SpoolAgentOutbox{}
	for _, lane := range direct.Lanes {
		row := SpoolAgentOutbox{Agent: lane.Agent, ObserverAccess: lane.State, RetryState: lane.State, RetryCount: lane.Queued, RetrySource: "direct-metadata", RetryStartedAt: &direct.StartedAt, RetryObservedAt: &direct.ObservedAt, Scope: scope}
		if lane.Queued != nil {
			row.QueuedForRetry = *lane.Queued
		}
		if lane.State == "unavailable" {
			row.Unreadable = true
			row.Error = "Direct outbox observation unavailable"
			row.RetryObservedAt = nil
			row.RetryStartedAt = nil
			row.RetrySource = ""
			row.RetryCount = nil
			if int(r.uid) != os.Geteuid() {
				row.ObserverAccess = "service-private"
			}
			row.RepairEligible = InspectSpoolOutboxRepair(spoolRoot, lane.Agent) == nil
			if row.RepairEligible {
				row.RepairSpool = spoolRoot
			}
		}
		rows[lane.Agent] = row
	}
	if result.Observation != nil {
		for _, lane := range snapshot.Lanes {
			row, ok := rows[lane.Agent]
			if !ok {
				row = SpoolAgentOutbox{Agent: lane.Agent, ObserverAccess: "unavailable", Unreadable: true, Error: "Direct outbox observation unavailable", Scope: scope}
				if int(r.uid) != os.Geteuid() {
					row.ObserverAccess = "service-private"
				}
			}
			// Prefer direct readable metadata; private service-owned lanes use
			// only producer evidence and retain their separate access status.
			if row.Unreadable {
				row.RetryState = lane.State
				row.RetryCount = lane.Queued
				row.RetrySource = "relay-producer"
				row.RetryObservedAt = &snapshot.ObservedAt
				row.RetryStartedAt = &snapshot.StartedAt
			}
			rows[lane.Agent] = row
		}
	}
	for _, row := range rows {
		result.Outbox = append(result.Outbox, row)
	}
	sort.Slice(result.Outbox, func(i, j int) bool { return result.Outbox[i].Agent < result.Outbox[j].Agent })
	return result
}

// SpoolOutboxRepair is the bounded outcome of restoring a locally-owned relay
// outbox to its private, readable mode. It deliberately contains no queue
// contents: repairing visibility must never drain, replay, or discard work.
type SpoolOutboxRepair struct {
	Agent          string `json:"agent"`
	CanonicalSpool string `json:"canonical_spool"`
	PreviousMode   string `json:"previous_mode"`
	RepairedMode   string `json:"repaired_mode"`
}

// SpoolOutboxHealth lists every agent under spoolRoot that has a held outbox
// item OR an unreadable outbox directory, sorted by agent id. It only reads
// <spoolRoot>/<agent>/outbox/ — it never drains, retries, or deletes
// anything. An absent outbox directory is legitimately quiet (no entry); any
// other read failure (permission denied, not a directory, ...) is reported
// as Unreadable, never silently counted as zero. Call it only on a host that
// actually runs the relay (routerstore.SpoolDir(SIRSI_ROUTER_URL) != "");
// elsewhere spoolRoot will not exist and this returns that error honestly.
func SpoolOutboxHealth(spoolRoot string) ([]SpoolAgentOutbox, error) {
	entries, err := os.ReadDir(spoolRoot)
	if err != nil {
		return nil, fmt.Errorf("read spool root %s: %w", spoolRoot, err)
	}
	var out []SpoolAgentOutbox
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, rErr := os.ReadDir(filepath.Join(spoolRoot, e.Name(), "outbox"))
		if rErr != nil {
			if errors.Is(rErr, os.ErrNotExist) {
				continue // no outbox at all — legitimately quiet
			}
			out = append(out, SpoolAgentOutbox{Agent: e.Name(), Unreadable: true, Error: rErr.Error()})
			continue
		}
		count := 0
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
				count++
			}
		}
		if count == 0 {
			continue
		}
		out = append(out, SpoolAgentOutbox{Agent: e.Name(), QueuedForRetry: count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Agent < out[j].Agent })
	return out, nil
}
