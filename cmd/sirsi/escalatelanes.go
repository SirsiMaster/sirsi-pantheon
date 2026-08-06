package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/supervision"
)

// activeThreadCountByAgent returns a map of agent-id → count of threads that
// are non-terminal, non-suspended, AND have a fresh heartbeat. A positive count
// means the lane has at least one registered session or worker that is both
// confirmed not dead AND recently heard from — the lane may be running even when
// it has no automatic wake mechanism.
//
// Staleness check is intentional: status:"active" is stored state, populated at
// registration and only updated when the conduit sweep runs (~15 min cadence).
// A session that died without reaping keeps status:"active" until the next
// conduit pass — trusting status alone would suppress escalation for a lane that
// is genuinely stopped. LastSeenAt is the real liveness signal.
//
// Errors return an empty map (fail-open: escalate rather than suppress when
// thread state is unreadable — the safe direction for an alerting function).
func activeThreadCountByAgent(repoRoot string, now time.Time) map[string]int {
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	treg, err := router.LoadThreadRegistry(routerRoot)
	if err != nil {
		return map[string]int{}
	}
	counts := make(map[string]int)
	for _, thr := range treg.SortedThreads() {
		if thr.Status.IsTerminal() || thr.Status == router.ThreadStatusSuspended {
			continue
		}
		// ponytail: reuses IsStale() from threads.go — single staleness predicate
		if thr.IsStale(now, router.DefaultThreadStaleAfter) {
			continue
		}
		counts[thr.AgentID]++
	}
	return counts
}

// dashboardUnroutable resolves the registry for the dashboard's fleet board.
// Separate from unroutableAgents only because the dashboard command finds its
// repo root independently; both fail toward "routable" for the same reason.
func dashboardUnroutable() map[string]bool {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "dashboard: routability unknown (repo root: %v) — lanes will not be classified UNROUTABLE\n", err)
		return nil
	}
	un, err := unroutableAgents(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dashboard: routability unknown (%v) — lanes will not be classified UNROUTABLE\n", err)
		return nil
	}
	return un
}

// escalateStuckLanes routes lanes that no wake can reach to the owner, and
// returns what it actually sent.
//
// It reuses the fleet board's own producer rather than recomputing lane state.
// A second read model would drift from the board, and the owner would be paged
// about a lane whose row reads healthy — the same divergence-by-construction
// that made three surfaces disagree before the fleet board unified them.
func escalateStuckLanes(repoRoot string) ([]supervision.Escalation, error) {
	if repoRoot == "" {
		return nil, nil
	}
	now := time.Now()
	snap, err := ledger.Build(repoRoot, "", now.UTC(), ledger.DefaultStaleAfter)
	if err != nil {
		return nil, fmt.Errorf("escalate lanes: build ledger: %w", err)
	}
	// Escalation REQUIRES known routability. Paging the owner about a lane whose
	// reachability we could not establish is a claim we have not earned, and a
	// wrong escalation trains the channel to be ignored.
	unroutable, err := unroutableAgents(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("escalate lanes: %w — refusing to escalate on unknown routability", err)
	}
	board := dashboard.NewFleetTracker(unroutable).Observe(snap, now)
	threadCounts := activeThreadCountByAgent(repoRoot, now)

	lanes := make([]supervision.LaneInput, 0, len(board.Lanes))
	states := make(map[string]supervision.LaneState, len(board.Lanes))
	for _, l := range board.Lanes {
		lanes = append(lanes, supervision.LaneInput{
			Agent: l.Agent,
			Sources: supervision.Sources{
				OpenItems:       l.Inbox,
				ActionableTasks: l.Active + l.Stalled,
				BlockedTasks:    l.Blocked,
			},
			Routable:      l.Routable,
			ActiveThreads: threadCounts[l.Agent],
		})
		states[l.Agent] = supervision.LaneState(l.State)
	}
	return router.RouteLaneEscalations(
		filepath.Join(repoRoot, ".agents", "idea-router"),
		supervision.Escalations(lanes, states),
	)
}

// reportLaneEscalations runs one escalation pass and prints what it delivered.
//
// Escalation failures are printed, never fatal: a supervisor that exits because
// it could not page the owner stops supervising entirely, trading a missed
// notification for a dead sweep.
func reportLaneEscalations(repoRoot string) {
	sent, err := escalateStuckLanes(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "supervise: lane escalation: %v\n", err)
	}
	for _, e := range sent {
		fmt.Printf("  escalated to owner: %s (%s, %d open)\n", e.Agent, e.State, e.OpenItems)
	}
}
