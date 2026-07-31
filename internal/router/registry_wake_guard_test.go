package router

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestRegistryWakeCoverage asserts the checked-in agent registry actually
// carries wake metadata for every agent that needs it.
//
// This guard exists because the fleet ran UNWAKEABLE for weeks and nothing
// caught it. The repo root sat on a squat branch whose HEAD (287dc7ea) had
// bulk-adopted a 2026-06-08-era registry from before the wake schema existed:
// 17 agents, 16 with an empty wake block. Every conduit pass reported ~11
// stranded agents, and the only signal was that stranded count. main itself was
// never broken — 287dc7ea was never an ancestor of it — so no test, lint or CI
// gate ever looked at the thing that was actually wrong.
//
// Reading the file from disk rather than a fixture is the entire point: the
// value being protected is the committed registry, not a copy of it.
func TestRegistryWakeCoverage(t *testing.T) {
	const path = "../../.agents/idea-router/agents.json"

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read agent registry %s: %v", path, err)
	}

	var reg Registry
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatalf("parse agent registry %s: %v", path, err)
	}
	if len(reg.Agents) == 0 {
		t.Fatalf("agent registry %s has no agents — a truncated or pre-schema copy", path)
	}

	// NO PREFIX EXEMPTION. This guard used to skip every id starting with
	// "codex-", on the assumption that codex lanes are wake-dead. That
	// assumption is false per-id and it hid a real defect for weeks:
	// codex-pantheon had a LIVE ai.sirsi.router.wake.codex-pantheon LaunchAgent
	// (pid 99109, status 0) while its registry block was {}, so doctor stamped
	// its items wake-unavailable and the guard — exempting it by name — could
	// never notice. Seven sibling codex ids genuinely are wake-dead. The prefix
	// could not tell them apart, because wake capability is a property of an
	// AGENT, not of its name (router items 20260729-045819, 20260729-141541).
	//
	// So the guard now tests a property of the DATA: every agent must state its
	// wake posture explicitly. A real mechanism, or "none" — which is a claim
	// the registry makes and can be checked — but never an empty block, which
	// is merely an absence and is indistinguishable from an oversight.
	//
	// Note there is deliberately no launch_agent_label assertion: that JSON key
	// appears in some entries but WakeConfig does not parse it, and the real
	// label is DERIVED from the agent id by WakeLaunchAgentLabel(). Asserting on
	// an inert field would be a check that reads strict and verifies nothing.
	var missing []string
	for id, cfg := range reg.Agents {
		if cfg.Wake.Mechanism == "" {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("%d of %d agents do not state a wake mechanism: %v\n"+
			"Horus cannot wake these, so work routed to them strands silently.\n"+
			"An agent that genuinely cannot be woken must say so with mechanism:none — "+
			"an empty wake block is an absence, not a decision.\n"+
			"If the registry looks reverted, check whether this worktree is on a branch "+
			"whose HEAD carries a pre-wake copy (see 287dc7ea) rather than origin/main's.",
			len(missing), len(reg.Agents), missing)
	}
}

func TestRegistryConsumerCoverage(t *testing.T) {
	const path = "../../.agents/idea-router/agents.json"

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read agent registry %s: %v", path, err)
	}

	var reg Registry
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatalf("parse agent registry %s: %v", path, err)
	}

	var missing []string
	var invalid []string
	for id, cfg := range reg.Agents {
		switch cfg.Consumer.Mode {
		case "", ConsumerModeCommand:
			if len(cfg.Consumer.Command) == 0 {
				missing = append(missing, id)
				continue
			}
			joined := ""
			for _, arg := range cfg.Consumer.Command {
				joined += arg + " "
			}
			joined += cfg.Consumer.Prompt
			if !containsEither(joined, id, consumerAgentPlaceholder) {
				invalid = append(invalid, id+": command consumer does not carry an inbox identity")
			}
		case ConsumerModeResident, "external/resident":
			if len(cfg.Consumer.Command) > 0 {
				invalid = append(invalid, id+": resident consumer declares a spawn command")
			}
			if len(cfg.Consumer.HealthCheck) == 0 {
				invalid = append(invalid, id+": resident consumer lacks health_check")
			}
		default:
			invalid = append(invalid, id+": unsupported consumer.mode "+cfg.Consumer.Mode)
		}
	}
	sort.Strings(missing)
	sort.Strings(invalid)

	if len(missing) > 0 {
		t.Errorf("%d of %d agents do not declare an inbox consumer: %v",
			len(missing), len(reg.Agents), missing)
	}
	if len(invalid) > 0 {
		t.Errorf("invalid consumer declarations: %v", invalid)
	}
}

func containsEither(s, a, b string) bool {
	return strings.Contains(s, a) || strings.Contains(s, b)
}
