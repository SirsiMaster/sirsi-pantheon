package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// A lane that DECLARES a wake mechanism must still read unroutable once its
// consecutive wake misses reach MaxWakeAttempts — the option (b) slice from
// lane-ping-and-incoming-notice remaining (4): the static WakeNone check alone
// cannot see a declared-but-never-actually-waking lane. The on-disk shape
// (routerRoot/wake-attempts.json, a plain map[string]int) is the same
// contract internal/router's TestWakeAttemptsRoundTrip exercises from the
// writer side; this test exercises it from the reader side only.
func TestUnroutableAgentsIncludesTerminalWakeFailures(t *testing.T) {
	repoRoot := t.TempDir()
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	if err := os.MkdirAll(routerRoot, 0o755); err != nil {
		t.Fatalf("mkdir router root: %v", err)
	}
	reg := &router.Registry{Agents: map[string]router.AgentConfig{
		"none-agent":    {ID: "none-agent", Wake: router.WakeConfig{Mechanism: router.WakeNone}},
		"healthy-agent": {ID: "healthy-agent", Wake: router.WakeConfig{Mechanism: router.WakeAPICall, Endpoint: "http://x"}},
		"flaky-agent":   {ID: "flaky-agent", Wake: router.WakeConfig{Mechanism: router.WakeAPICall, Endpoint: "http://x"}},
	}}
	if err := router.SaveRegistry(routerRoot, reg); err != nil {
		t.Fatalf("save registry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(routerRoot, "wake-attempts.json"),
		[]byte(`{"flaky-agent": `+strconv.Itoa(routerstore.MaxWakeAttempts)+`}`), 0o644); err != nil {
		t.Fatalf("write wake-attempts.json: %v", err)
	}

	got, err := unroutableAgents(repoRoot)
	if err != nil {
		t.Fatalf("unroutableAgents: %v", err)
	}
	if !got["none-agent"] {
		t.Error("none-agent (WakeNone) must be unroutable")
	}
	if got["healthy-agent"] {
		t.Error("healthy-agent must not be unroutable")
	}
	if !got["flaky-agent"] {
		t.Error("flaky-agent (MaxWakeAttempts consecutive misses) must be unroutable")
	}
}
