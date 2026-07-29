package router

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

func wakeExempt(id string) bool {
	return strings.HasPrefix(id, "codex-")
}

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

	var missing []string
	for id, cfg := range reg.Agents {
		if wakeExempt(id) {
			continue
		}
		if cfg.Wake.Mechanism == "" {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("%d of %d agents have no wake mechanism: %v\n"+
			"Horus cannot wake these, so work routed to them strands silently.\n"+
			"If the registry looks reverted, check whether this worktree is on a branch "+
			"whose HEAD carries a pre-wake copy (see 287dc7ea) rather than origin/main's.",
			len(missing), len(reg.Agents), missing)
	}
}
