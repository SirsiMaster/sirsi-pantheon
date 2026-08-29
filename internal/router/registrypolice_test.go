package router

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

func TestRegistryPoliceStrandedUsesNativeReadModels(t *testing.T) {
	t.Setenv(routercfg.StoreWakeEnv, "0") // keep this fixture fully local to items/.
	root := t.TempDir()
	registry := `{"agents":{"claude-pantheon":{"id":"claude-pantheon","type":"claude","cwd":"/tmp/claude","workstream":"test","wake":{"mechanism":"launchagent"}},"manual":{"id":"manual","type":"codex","cwd":"/tmp/manual","workstream":"test","wake":{"mechanism":"none"}}}}`
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(registry), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Send(root, "author", "claude-pantheon", "needs consumer", "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Send(root, "author", "manual", "intentionally manual", "fixture"); err != nil {
		t.Fatal(err)
	}

	got, err := RegistryPoliceStranded(root, func(args ...string) error {
		return errNotLoaded // no loaded launchd consumer in this fixture
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].AgentID != "claude-pantheon" || got[0].OpenItems != 1 {
		t.Fatalf("stranded = %+v, want only claude-pantheon/1", got)
	}
}
