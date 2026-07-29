package main

import (
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
)

// commandDeckRows renders the first screen of the menubar: a compact operator
// cockpit for local intelligence, hardware pressure, router state, context, and
// risk. It stays pure so tests can hold the product language steady.
func commandDeckRows(s *StatsSnapshot, ops dashboard.OpsSummary) []string {
	if s == nil {
		s = &StatsSnapshot{}
	}
	return []string{
		localAIRow(ops),
		computeDeckRow(s),
		routerDeckRow(ops),
		contextDeckRow(ops),
		riskDeckRow(s),
	}
}

func localAIRow(ops dashboard.OpsSummary) string {
	gemma := agentSignal(ops, "gemma-pantheon")
	bare := agentSignal(ops, "gemma")
	switch {
	case gemma.live > 0:
		return fmt.Sprintf("  ◉ Local AI — Gemma online · %s", adminCapabilityPhrase(gemma.pending))
	case bare.live > 0 || bare.stale > 0:
		return "  ◌ Local AI — Gemma misregistered · admin held"
	case gemma.pending > 0:
		return fmt.Sprintf("  ◐ Local AI — Gemma has %d queued · wake needed", gemma.pending)
	default:
		return "  ◌ Local AI — Gemma offline · start broker"
	}
}

func adminCapabilityPhrase(pending int) string {
	if pending > 0 {
		return fmt.Sprintf("%d admin task(s) queued", pending)
	}
	return "admin tools pending"
}

func computeDeckRow(s *StatsSnapshot) string {
	accel := strings.TrimSpace(s.PrimaryAccelerator)
	if accel == "" {
		accel = "detecting hardware"
	}
	mem := "memory steady"
	switch s.RAMPressure {
	case "medium":
		mem = "memory watched"
	case "high":
		mem = "memory constrained"
	}
	return fmt.Sprintf("  ⚡ Compute — %s · %.0f%% RAM · %s", accel, s.RAMPercent, mem)
}

func routerDeckRow(ops dashboard.OpsSummary) string {
	state := "armed"
	switch {
	case ops.HasDriftOrAuthIssue:
		state = "drift/auth attention"
	case ops.StaleThreadCount > 0 || ops.RecentFailureCount > 0:
		state = "needs cleanup"
	}
	return fmt.Sprintf("  𓇶 Router — %d live · %d queued · %s", ops.LiveThreadCount, ops.QueueOpenItems, state)
}

func contextDeckRow(ops dashboard.OpsSummary) string {
	if ops.QueueOpenItems > 0 {
		return fmt.Sprintf("  𓁢 Context — full wake digest · %d item(s) in motion", ops.QueueOpenItems)
	}
	return "  𓁢 Context — full wake digest · quiet"
}

func riskDeckRow(s *StatsSnapshot) string {
	branch := strings.TrimSpace(s.GitBranch)
	if branch == "" {
		branch = "no branch"
	}
	risk := strings.TrimSpace(s.OsirisRisk)
	if risk == "" {
		risk = "unknown"
	}
	return fmt.Sprintf("  𓆄 Risk — %s · %d changed · %s", branch, s.UncommittedFiles, risk)
}

type agentDeckSignal struct {
	live    int
	stale   int
	pending int
}

func agentSignal(ops dashboard.OpsSummary, agentID string) agentDeckSignal {
	var sig agentDeckSignal
	for _, a := range ops.Agents {
		if a.AgentID == agentID {
			sig.live += a.LiveThreads
			sig.stale += a.StaleThreads
			sig.pending += a.PendingItems
		}
	}
	return sig
}
