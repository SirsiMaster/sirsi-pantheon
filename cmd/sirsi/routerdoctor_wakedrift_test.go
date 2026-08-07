package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// TestPartitionWakeDrift verifies the three-way classification of wake.mechanism
// ChangedFields against plist presence on disk.
func TestPartitionWakeDrift(t *testing.T) {
	// Build a temp LaunchAgents dir with one real plist.
	dir := t.TempDir()
	realLabel := "ai.sirsi.router.wake.claude-pantheon"
	if err := os.WriteFile(filepath.Join(dir, realLabel+".plist"), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Redirect wakeAgentPlistExists to look in dir instead of ~/Library/LaunchAgents.
	orig := launchAgentsDir
	launchAgentsDir = dir
	t.Cleanup(func() { launchAgentsDir = orig })

	fields := []router.ChangedField{
		// upstream=launchagent, live=none, plist ABSENT → aspirational
		{AgentID: "claude-home", Path: "wake.mechanism", Upstream: "launchagent", Live: "none"},
		// upstream=launchagent, live=none, plist PRESENT → real drift
		{AgentID: "claude-pantheon", Path: "wake.mechanism", Upstream: "launchagent", Live: "none"},
		// live=launchagent, plist ABSENT → broken claim
		{AgentID: "codex-home", Path: "wake.mechanism", Upstream: "none", Live: "launchagent"},
		// unrelated path → other
		{AgentID: "claude-io", Path: "consumer.some_field", Upstream: "old", Live: "new"},
	}

	real, aspirational, broken, other := partitionWakeDrift(fields)

	if len(aspirational) != 1 || aspirational[0].AgentID != "claude-home" {
		t.Errorf("aspirational: want [claude-home], got %v", aspirational)
	}
	if len(real) != 1 || real[0].AgentID != "claude-pantheon" {
		t.Errorf("real: want [claude-pantheon], got %v", real)
	}
	if len(broken) != 1 || broken[0].AgentID != "codex-home" {
		t.Errorf("broken: want [codex-home], got %v", broken)
	}
	if len(other) != 1 || other[0].AgentID != "claude-io" {
		t.Errorf("other: want [claude-io], got %v", other)
	}
}

// TestPartitionLostWake verifies that a lost wake.launch_agent_label is
// classified as aspirational when the named plist is absent, and real when
// the plist exists.
func TestPartitionLostWake(t *testing.T) {
	dir := t.TempDir()
	presentLabel := "ai.sirsi.router.wake.claude-pantheon"
	if err := os.WriteFile(filepath.Join(dir, presentLabel+".plist"), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := launchAgentsDir
	launchAgentsDir = dir
	t.Cleanup(func() { launchAgentsDir = orig })

	fields := []router.LostField{
		// plist absent → aspirational (live is correct)
		{AgentID: "claude-home", Path: "wake.launch_agent_label", Upstream: "ai.sirsi.router.wake.claude-home"},
		// plist present → real lost field
		{AgentID: "claude-pantheon", Path: "wake.launch_agent_label", Upstream: presentLabel},
		// different path → always real
		{AgentID: "claude-io", Path: "wake.other_field", Upstream: "val"},
	}

	real, aspirational := partitionLostWake(fields)

	if len(aspirational) != 1 || aspirational[0].AgentID != "claude-home" {
		t.Errorf("aspirational: want [claude-home], got %v", aspirational)
	}
	if len(real) != 2 {
		t.Errorf("real: want 2 (claude-pantheon + claude-io), got %v", real)
	}
}
