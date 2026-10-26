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
	out := make(map[string]bool, len(reg.Agents))
	for id, cfg := range reg.Agents {
		// A human, or a service (Horus), is not a lane: nothing is sent there to
		// be worked, so "no wake mechanism" is its design, not an incident. Only
		// lanes can be unroutable; judging a service by lane rules is how Horus
		// escalated itself ("Lane needs you: horus cannot be reached").
		if cfg.Type == "human" || cfg.Type == "service" {
			out[id] = false
		} else {
			out[id] = !PingLane(threads, cfg, id, now).Workable
		}
	}
	return out
}
