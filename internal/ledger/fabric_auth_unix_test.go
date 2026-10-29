//go:build unix

package ledger

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
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestReadFabricDoesNotExecuteAgentAuthCommands(t *testing.T) {
	home, repo, bin := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_STORE_WAKE", "0")
	db := filepath.Join(home, "owned", "fixture.db")
	if err := os.Mkdir(filepath.Dir(db), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_ROUTER_DB", db)
	// PATH has no host CLI fallback. Even the old production nil-probe call
	// can execute only these inert helpers, never an installed Claude/Codex.
	t.Setenv("PATH", bin)
	marker := filepath.Join(home, "agent-auth-invocations")
	t.Setenv("FABRIC_AUTH_SENTINEL", marker)
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
	store, err := routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := store.AddTask(routerstore.Task{Agent: "claude-fixture", TaskID: "pending", Subject: "inert fixture", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	expected, err := store.ClassifyLane("claude-fixture", false, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	read, err := ReadFabric(repo, now)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(marker); !os.IsNotExist(err) {
		t.Fatalf("passive ReadFabric executed agent auth/model commands: %q (read error %v)", data, err)
	}
	if len(read.Node.AgentHealth) != 2 {
		t.Fatalf("agent health = %+v", read.Node.AgentHealth)
	}
	for _, check := range read.Node.AgentHealth {
		if !check.CLIFound || check.CLIPath != filepath.Join(bin, check.AgentType) {
			t.Fatalf("fixture must discover only inert %s helper: %+v", check.AgentType, check)
		}
		if check.AuthOK || check.NeedsLogin || !check.Degraded || check.BlockedItems != 0 || !strings.Contains(check.AuthError, "not probed") {
			t.Errorf("passive auth must remain explicit inconclusive, not healthy/logged out/blocked: %+v", check)
		}
	}
	if len(read.Node.RegisteredAgents) != 2 || read.Node.TotalPending != 1 || len(read.Node.StrandedInbox) != 1 {
		t.Errorf("passive auth must not hide registry/backlog/stranding: %+v", read.Node)
	}
	if got := read.Evidence.Operational["claude-fixture"]; got.Agent != expected.Agent || got.Classification != expected.Classification {
		t.Errorf("Fabric must retain canonical lane classification: got %+v, want %+v", got, expected)
	}
	for file, before := range map[string][]byte{"agents.json": registryBytes, "threads.json": threadBytes} {
		after, err := os.ReadFile(filepath.Join(root, file))
		if err != nil || !bytes.Equal(before, after) {
			t.Errorf("Fabric changed %s: %v", file, err)
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
