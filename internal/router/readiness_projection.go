package router

import (
	"fmt"
	"path/filepath"
	"time"
)

// ReadLaneRoutability is the existing registry + PingLane read predicate,
// shared by Fabric and the legacy fleet command. It performs no wake/repair.
func ReadLaneRoutability(repoRoot string) (map[string]bool, error) {
	if repoRoot == "" {
		return nil, fmt.Errorf("unroutable: no repo root — routability is unknown, not healthy")
	}
	reg, err := LoadRegistry(filepath.Join(repoRoot, ".agents", "idea-router"))
	if err != nil {
		return nil, fmt.Errorf("unroutable: load registry: %w", err)
	}
	if reg == nil {
		return nil, fmt.Errorf("unroutable: registry loaded as nil — routability is unknown")
	}
	threads, err := LoadThreadRegistry(filepath.Join(repoRoot, ".agents", "idea-router"))
	if err != nil {
		return nil, fmt.Errorf("unroutable: load live threads: %w", err)
	}
	return UnroutableFromRuntime(reg, threads, time.Now().UTC()), nil
}

// Use the same readiness predicate as router ping: wake=none may have a real
// attended consumer, and a configured launch agent may be unable to authenticate.
// This is reachability, not evidence that a queue item has been completed.
func UnroutableFromRuntime(reg *Registry, threads *ThreadRegistry, now time.Time) map[string]bool {
	return UnroutableFromReach(reg, ReachFromRuntime(reg, threads, now))
}

// ReachFromRuntime runs PingLane once per declared lane (humans and services are
// not lanes and get no verdict). It is the single readiness predicate: both the
// unroutable flag and the board's plain-English reason read this same result.
func ReachFromRuntime(reg *Registry, threads *ThreadRegistry, now time.Time) map[string]PingResult {
	out := make(map[string]PingResult, len(reg.Agents))
	for id, cfg := range reg.Agents {
		if cfg.Type == "human" || cfg.Type == "service" {
			continue
		}
		out[id] = PingLane(threads, cfg, id, now)
	}
	return out
}

// UnroutableFromReach derives the unroutable flag from ReachFromRuntime output.
func UnroutableFromReach(reg *Registry, reach map[string]PingResult) map[string]bool {
	out := make(map[string]bool, len(reg.Agents))
	for id := range reg.Agents {
		r, isLane := reach[id]
		out[id] = isLane && !r.Workable
	}
	return out
}
