package main

import (
	"fmt"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// unroutableAgents returns the set of agent ids that no automated wake path
// reaches, read from the registry.
//
// This lives in cmd rather than internal/dashboard on purpose: the dashboard
// renders lanes and must not grow a dependency on registry loading, and the
// command layer already owns registry access. The dashboard receives the answer,
// not the ability to compute it.
//
// A registry read failure returns an ERROR, never an empty set.
//
// The first version of this returned empty on failure and called it "the honest
// failure direction". codex-pantheon's review caught that it is the opposite:
// an empty set makes every lane read routable, which is precisely the
// false-green this function exists to remove — a surface reporting health it
// never established. Unknown routability must be visible as unknown and must
// block escalation claims, because "we could not read the registry" and "every
// lane is reachable" are different facts and only one of them is true.
func unroutableAgents(repoRoot string) (map[string]bool, error) {
	if repoRoot == "" {
		return nil, fmt.Errorf("unroutable: no repo root — routability is unknown, not healthy")
	}
	reg, err := router.LoadRegistry(filepath.Join(repoRoot, ".agents", "idea-router"))
	if err != nil {
		return nil, fmt.Errorf("unroutable: load registry: %w", err)
	}
	if reg == nil {
		return nil, fmt.Errorf("unroutable: registry loaded as nil — routability is unknown")
	}
	out := map[string]bool{}
	for id, cfg := range reg.Agents {
		if cfg.WakeMechanism() == router.WakeNone {
			out[id] = true
		}
	}
	return out, nil
}
