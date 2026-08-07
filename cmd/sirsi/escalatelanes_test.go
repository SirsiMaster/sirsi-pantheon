package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

// TestActiveThreadCountExcludesStaleHeartbeat is the test the review requested:
// a thread with status:"active" but an expired heartbeat must NOT count as a
// live thread and must not suppress lane escalation.
//
// The defect it catches: activeThreadCountByAgent previously trusted the stored
// status field, which stays "active" until the conduit sweep heals it (~15 min
// cadence). A dead session would suppress escalation for the whole inter-sweep
// window, turning a false-positive alarm into a false-negative (silent) one.
func TestActiveThreadCountExcludesStaleHeartbeat(t *testing.T) {
	t.Setenv(routercfg.StoreWakeEnv, "0")
	dir := t.TempDir()
	routerRoot := filepath.Join(dir, ".agents", "idea-router")
	if err := os.MkdirAll(routerRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	staleWindow := router.DefaultThreadStaleAfter // 5 min

	// Write a threads.json with one "active" record whose heartbeat is expired.
	reg := map[string]any{
		"threads": map[string]any{
			"thr-stale": map[string]any{
				"thread_id":    "thr-stale",
				"agent_id":     "claude-home",
				"surface":      "automation",
				"status":       "active",
				"started_at":   now.Add(-2 * time.Hour).Format(time.RFC3339),
				"last_seen_at": now.Add(-(staleWindow + time.Minute)).Format(time.RFC3339),
			},
		},
	}
	data, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(routerRoot, "threads.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// A stale "active" record must yield count 0, not 1.
	counts := activeThreadCountByAgent(dir, now)
	if counts["claude-home"] != 0 {
		t.Errorf("activeThreadCountByAgent: stale active record counted as live (got %d, want 0)", counts["claude-home"])
	}
}

// TestActiveThreadCountIncludesFreshThread verifies the happy path: a fresh
// heartbeat does count, so we do not suppress escalation for a lane that is
// actually alive.
func TestActiveThreadCountIncludesFreshThread(t *testing.T) {
	t.Setenv(routercfg.StoreWakeEnv, "0")
	dir := t.TempDir()
	routerRoot := filepath.Join(dir, ".agents", "idea-router")
	if err := os.MkdirAll(routerRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	reg := map[string]any{
		"threads": map[string]any{
			"thr-fresh": map[string]any{
				"thread_id":    "thr-fresh",
				"agent_id":     "claude-home",
				"surface":      "claude",
				"status":       "active",
				"started_at":   now.Add(-time.Minute).Format(time.RFC3339),
				"last_seen_at": now.Add(-30 * time.Second).Format(time.RFC3339),
			},
		},
	}
	data, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(routerRoot, "threads.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	counts := activeThreadCountByAgent(dir, now)
	if counts["claude-home"] != 1 {
		t.Errorf("activeThreadCountByAgent: fresh active record not counted (got %d, want 1)", counts["claude-home"])
	}
}
