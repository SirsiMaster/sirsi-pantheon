package main

import (
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
// On any registry read failure it returns an EMPTY set, so every lane is treated
// as routable. That is the honest failure direction: claiming a lane is
// unroutable because the registry could not be read would escalate a supervisor
// bug to the owner as a lane problem, which is exactly the misattribution the
// supervision contract exists to prevent. Under-reporting is recoverable on the
// next pass; a false escalation trains the owner to ignore the channel.
func unroutableAgents(repoRoot string) map[string]bool {
	out := map[string]bool{}
	if repoRoot == "" {
		return out
	}
	reg, err := router.LoadRegistry(filepath.Join(repoRoot, ".agents", "idea-router"))
	if err != nil || reg == nil {
		return out
	}
	for id, cfg := range reg.Agents {
		if cfg.WakeMechanism() == router.WakeNone {
			out[id] = true
		}
	}
	return out
}
