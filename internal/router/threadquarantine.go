// Package router — threadquarantine.go
//
// Reversible parking of stale thread records. The owner's rule is that threads
// and sessions are never reaped, pruned, closed or deleted by an operator sweep;
// quarantine is the non-destructive alternative: the record stays in the
// registry untouched except for status=quarantined plus a Quarantine note
// (who/when/why/prior status). Its last_seen_at is preserved, so the staleness
// evidence survives, and the owning session's next heartbeat or re-register
// releases it ("proved alive") — a live session is never cut off.
package router

import (
	"fmt"
	"os"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// DefaultQuarantineStaleAfter is how long a heartbeat must be quiet before a
// thread may be quarantined; deliberately more conservative than the 5 minute
// display window of DefaultThreadStaleAfter.
const DefaultQuarantineStaleAfter = time.Hour

// ThreadQuarantine is the audit note stored on a quarantined record.
type ThreadQuarantine struct {
	At          time.Time    `json:"at"`
	By          string       `json:"by"`
	Reason      string       `json:"reason"`
	PriorStatus ThreadStatus `json:"prior_status"`
}

// Quarantine actions reported per thread.
const (
	QuarantineActionQuarantined = "quarantined"
	QuarantineActionWould       = "would-quarantine"
	QuarantineActionAlready     = "already-quarantined"
	QuarantineActionForeign     = "skipped-foreign-host"
)

// QuarantineChange is one row of the quarantine report.
type QuarantineChange struct {
	ThreadID    string       `json:"thread_id"`
	AgentID     string       `json:"agent_id"`
	Host        string       `json:"host"`
	Surface     string       `json:"surface"`
	IdleSeconds float64      `json:"idle_seconds"`
	PriorStatus ThreadStatus `json:"prior_status"`
	Action      string       `json:"action"`
}

// releaseQuarantine restores a quarantined record to its prior (live) status
// and drops the note. It reports whether anything changed. The caller owns the
// save; last_seen_at is the caller's to refresh when the release is a proof of life.
func releaseQuarantine(t *Thread, why string) bool {
	if t == nil || t.Status != ThreadStatusQuarantined {
		return false
	}
	prior := ThreadStatusActive
	if t.Quarantine != nil && t.Quarantine.PriorStatus != "" && !t.Quarantine.PriorStatus.IsTerminal() && !t.Quarantine.PriorStatus.IsParked() {
		prior = t.Quarantine.PriorStatus
	}
	t.Status = prior
	t.Quarantine = nil
	fmt.Fprintf(os.Stderr, "thread %s (%s): released from quarantine: %s\n", t.ThreadID, t.AgentID, why)
	return true
}

// QuarantineThreads parks stale threads. With threadID set it targets exactly
// that thread and REFUSES (error) unless it is stale; with threadID empty it
// parks every currently-stale thread this host owns. A thread is stale by the
// same loop-evidence-aware predicate `thread list` shows, so a record whose
// process or watcher loop is demonstrably alive is never parked. Records owned
// by another host are reported and left alone (the service refuses cross-host
// writes). dryRun reports without writing. The pass re-derives from a fresh
// load on a lost fence, so a heartbeat landing mid-pass drops that thread out.
func QuarantineThreads(routerRoot, threadID string, staleAfter time.Duration, by, reason string, dryRun bool) ([]QuarantineChange, error) {
	if staleAfter <= 0 {
		staleAfter = DefaultQuarantineStaleAfter
	}
	selfHost, err := routerstore.ThreadOwnerHost()
	if err != nil {
		return nil, err
	}
	var changes []QuarantineChange
	err = retryOnLostFenceErr(func() error {
		changes = nil
		reg, err := LoadThreadRegistry(routerRoot)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		sorted := reg.SortedThreads()
		newest := NewestActiveByAgent(sorted)
		candidates := sorted
		if threadID != "" {
			t, ok := reg.Threads[threadID]
			if !ok || t == nil {
				return fmt.Errorf("thread %q not registered", threadID)
			}
			candidates = []*Thread{t}
		}
		dirty := false
		for _, t := range candidates {
			single := threadID != ""
			switch {
			case t.Status.IsTerminal() || t.Status == ThreadStatusSuspended:
				if single {
					return fmt.Errorf("thread %q is %s; only live-but-silent threads can be quarantined", t.ThreadID, t.Status)
				}
				continue
			case t.Status == ThreadStatusQuarantined:
				if single {
					changes = append(changes, changeRow(t, now, QuarantineActionAlready, t.Quarantine.priorOr(t.Status)))
				}
				continue
			case !EffectiveStaleForNewest(t, now, staleAfter, newest[t.AgentID] == t.ThreadID):
				if single {
					return fmt.Errorf("refusing to quarantine %q: heartbeat is fresh or its process is alive (idle %s, window %s)", t.ThreadID, now.Sub(t.LastSeenAt).Round(time.Second), staleAfter)
				}
				continue
			case !ownsRecordHost(t.Host, selfHost):
				if single {
					return fmt.Errorf("thread %q is owned by host %q; quarantine it from that host", t.ThreadID, t.Host)
				}
				changes = append(changes, changeRow(t, now, QuarantineActionForeign, t.Status))
				continue
			}
			row := changeRow(t, now, QuarantineActionWould, t.Status)
			if !dryRun {
				row.Action = QuarantineActionQuarantined
				t.Quarantine = &ThreadQuarantine{At: now, By: by, Reason: reason, PriorStatus: t.Status}
				t.Status = ThreadStatusQuarantined
				dirty = true
			}
			changes = append(changes, row)
		}
		if !dirty {
			return nil
		}
		return SaveThreadRegistry(routerRoot, reg)
	})
	if err != nil {
		return nil, err
	}
	return changes, nil
}

func (q *ThreadQuarantine) priorOr(s ThreadStatus) ThreadStatus {
	if q != nil && q.PriorStatus != "" {
		return q.PriorStatus
	}
	return s
}

func changeRow(t *Thread, now time.Time, action string, prior ThreadStatus) QuarantineChange {
	return QuarantineChange{ThreadID: t.ThreadID, AgentID: t.AgentID, Host: t.Host, Surface: t.Surface,
		IdleSeconds: now.Sub(t.LastSeenAt).Seconds(), PriorStatus: prior, Action: action}
}

// UnquarantineThread restores a quarantined thread to its prior status without
// touching last_seen_at (a manual release is not proof of life; the record
// simply reads stale again until its session heartbeats).
func UnquarantineThread(routerRoot, threadID string) (*Thread, error) {
	if threadID == "" {
		return nil, fmt.Errorf("thread_id is required")
	}
	var out *Thread
	err := retryOnLostFenceErr(func() error {
		reg, err := loadThreadRegistryOne(routerRoot, threadID)
		if err != nil {
			return err
		}
		t, ok := reg.Threads[threadID]
		if !ok {
			return fmt.Errorf("thread %q not registered", threadID)
		}
		if t.Status != ThreadStatusQuarantined {
			return fmt.Errorf("thread %q is %s, not quarantined", threadID, t.Status)
		}
		releaseQuarantine(t, "released by operator")
		out = t
		return SaveThreadRegistry(routerRoot, reg)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
