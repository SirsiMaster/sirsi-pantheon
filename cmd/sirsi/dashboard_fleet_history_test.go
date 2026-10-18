package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Retiring a delivery address does not retire its durable obligations. The
// dashboard must retain the same historical task inventory as router fleet.
func TestDashboardFleetPreservesAliasTaskHistory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(root, "fixture.db"))
	t.Chdir(root)
	routerRoot := filepath.Join(root, ".agents", "idea-router")
	if err := os.MkdirAll(routerRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"agents.json": `{"agents":{"current":{"id":"current","type":"codex","repo":"/fixture","workstream":"fixture","wake":{"mechanism":"none"}}},"aliases":{"retired":"current"}}`,
		"state.json":  `{"pending":{}}`,
	} {
		if err := os.WriteFile(filepath.Join(routerRoot, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := routerstore.OpenPath(filepath.Join(root, "fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"retired", "current"} {
		if err := s.AddTask(routerstore.Task{Agent: agent, TaskID: "same-id", Subject: agent, Status: "pending"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := dispatch.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	aliases, aliasErr := f.Aliases()
	_ = f.Close()
	if aliasErr != nil || aliases["retired"] != "current" {
		t.Fatalf("fixture must exercise a valid alias: %v %v", aliases, aliasErr)
	}
	snap, err := collectDashboardFleet()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Agents) != 2 {
		t.Fatalf("delivery alias hid durable task history: %+v", snap.Agents)
	}
	for _, agent := range snap.Agents {
		if len(agent.Tasks) != 1 || agent.Tasks[0].Agent != agent.AgentID || agent.Tasks[0].TaskID != "same-id" {
			t.Fatalf("history identity changed or collided: %+v", agent)
		}
	}
}
