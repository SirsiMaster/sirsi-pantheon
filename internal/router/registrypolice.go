package router

import "fmt"

// RegistryPoliceStranded returns the per-agent A27 accountability violations
// from the same cutover-aware inbox and watcher read models used by the router.
// It is read-only: it never starts a consumer, changes a thread, or performs a
// recovery. A read failure is returned so the resident supervisor can report a
// truthful failure rather than silently treating missing state as healthy.
func RegistryPoliceStranded(routerRoot string, launchctlCheck LaunchctlChecker) ([]StrandedAgent, error) {
	reg, err := LoadRegistry(routerRoot)
	if err != nil {
		return nil, fmt.Errorf("registry police: load registry: %w", err)
	}
	items, err := OpenItems(routerRoot, "")
	if err != nil {
		return nil, fmt.Errorf("registry police: read open items: %w", err)
	}
	pending := make(map[string][]string)
	for _, item := range items {
		pending[item.To] = append(pending[item.To], item.ID)
	}
	agents := make([]string, 0, len(pending))
	for agent := range pending {
		agents = append(agents, agent)
	}
	return computeStranded(routerRoot, pending, liveWakeAgents(agents, launchctlCheck), noWakeAgents(reg)), nil
}
