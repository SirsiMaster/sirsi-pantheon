package main

import (
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"time"
)

// Compatibility callers use the same read-only canonical readiness projection.
func unroutableAgents(repoRoot string) (map[string]bool, error) {
	return router.ReadLaneRoutability(repoRoot)
}
func unroutableFromRuntime(reg *router.Registry, threads *router.ThreadRegistry, now time.Time) map[string]bool {
	return router.UnroutableFromRuntime(reg, threads, now)
}
