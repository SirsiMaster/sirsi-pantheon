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

// dashboardUnroutable resolves the registry for the dashboard's fleet board.
// Separate from unroutableAgents only because the dashboard command finds its
// repo root independently; both fail toward "routable" for the same reason.
func dashboardUnroutable() map[string]bool {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return map[string]bool{}
	}
	return unroutableAgents(repoRoot)
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
	board := dashboard.NewFleetTracker(unroutableAgents(repoRoot)).Observe(snap, now)

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
			Routable: l.Routable,
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
