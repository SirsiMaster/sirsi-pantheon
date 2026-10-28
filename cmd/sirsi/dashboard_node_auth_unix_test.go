//go:build unix

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/machineid"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestCollectDashboardNodeStatusDoesNotExecuteAgentAuthCommands(t *testing.T) {
	home, repo, bin := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(repo)
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_STORE_WAKE", "0")
	db := filepath.Join(home, "owned", "fixture.db")
	if err := os.Mkdir(filepath.Dir(db), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_ROUTER_DB", db)
	// Exercise the actual web collector, including repo-root discovery.
	// PATH has no host CLI fallback. Even the old production nil-probe call
	// can execute only these inert helpers, never an installed Claude/Codex.
	t.Setenv("PATH", bin)
	marker := filepath.Join(home, "agent-auth-invocations")
	t.Setenv("FABRIC_AUTH_SENTINEL", marker)
	// Refuse Git discovery so FindRepoRoot uses this fixture cwd only.
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"claude", "codex"} {
		script := "#!/bin/sh\nprintf '%s\\n' \"$0 $*\" >> \"$FABRIC_AUTH_SENTINEL\"\nprintf 'OK\\n'\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bin, "launchctl"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	previousMachineProbe := machineid.GetProbe()
	machineid.SetProbe(func() string { return "fixture-machine" })
	t.Cleanup(func() { machineid.SetProbe(previousMachineProbe) })

	root := filepath.Join(repo, ".agents", "idea-router")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	reg := router.Registry{Agents: map[string]router.AgentConfig{
		"claude-fixture": {ID: "claude-fixture", Type: "claude", Command: []string{"claude"}, Cwd: repo},
		"codex-fixture":  {ID: "codex-fixture", Type: "codex", Command: []string{"codex"}, Cwd: repo},
	}}
	now := time.Now().UTC()
	threads := router.ThreadRegistry{Threads: map[string]*router.Thread{
		"closed-fixture": {ThreadID: "closed-fixture", AgentID: "claude-fixture", Status: router.ThreadStatusClosed,
			MachineID: "fixture-machine", StartedAt: now, LastSeenAt: now},
	}}
	var registryBytes, threadBytes []byte
	for file, value := range map[string]any{
		"agents.json": reg, "threads.json": threads,
		"state.json": router.State{Version: 1, Pending: map[string][]string{"claude-fixture": {"pending-fixture"}}},
	} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, file), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if file == "agents.json" {
			registryBytes = data
		}
		if file == "threads.json" {
			threadBytes = data
		}
	}
	node, err := collectDashboardNodeStatus()
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(marker); !os.IsNotExist(err) {
		t.Fatalf("passive web node-status executed agent auth/model commands: %q (read error %v)", data, err)
	}
	if node.RepoRoot != repo || node.RouterHome != root {
		t.Fatalf("web collector escaped fixture root: %+v", node)
	}
	if len(node.AgentHealth) != 2 {
		t.Fatalf("agent health = %+v", node.AgentHealth)
	}
	for _, check := range node.AgentHealth {
		if !check.CLIFound || check.CLIPath != filepath.Join(bin, check.AgentType) {
			t.Fatalf("fixture must discover only inert %s helper: %+v", check.AgentType, check)
		}
		if check.AuthOK || check.NeedsLogin || !check.Degraded || check.BlockedItems != 0 || !strings.Contains(check.AuthError, "not probed") {
			t.Errorf("passive auth must remain explicit inconclusive, not healthy/logged out/blocked: %+v", check)
		}
	}
	if len(node.RegisteredAgents) != 2 || node.TotalPending != 1 || len(node.StrandedInbox) != 1 {
		t.Errorf("passive auth must not hide registry/backlog/stranding: %+v", node)
	}
	for file, before := range map[string][]byte{"agents.json": registryBytes, "threads.json": threadBytes} {
		after, err := os.ReadFile(filepath.Join(root, file))
		if err != nil || !bytes.Equal(before, after) {
			t.Errorf("web node-status changed %s: %v", file, err)
		}
	}

	// Existing explicit diagnostics retain the real default probe. Here its
	// only possible executables are the inert sentinels installed above.
	diagnostic, err := router.CollectNodeStatus(repo, func(...string) error { return os.ErrNotExist })
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"claude --print respond with OK", "codex --version"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("explicit default probe did not reach fake %q: %s", want, data)
		}
	}
	for _, check := range diagnostic.AgentHealth {
		if !check.AuthOK {
			t.Errorf("explicit diagnostic probe changed: %+v", check)
		}
	}
}
