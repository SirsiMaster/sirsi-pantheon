package router

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCivicQueueBridgeDoesNotLoadIsolatedCivicWorkspace(t *testing.T) {
	b, err := os.ReadFile("../../.agents/idea-router/agents.json")
	if err != nil {
		t.Fatal(err)
	}
	var reg Registry
	if err := json.Unmarshal(b, &reg); err != nil {
		t.Fatal(err)
	}
	cfg := reg.Agents["claude-kfca"]
	if cfg.Cwd == cfg.Repo || !strings.HasSuffix(cfg.Cwd, "/kfca-router-coordination") || !strings.HasSuffix(cfg.Repo, "/KingFarmCAB") {
		t.Fatal("queue bridge must not launch in civic content")
	}
	if cfg.Wake.Mechanism != WakeLaunchAgent {
		t.Fatal("existing queue bridge cannot wake")
	}
	for _, guard := range []string{"transport-only", "Do not open the civic repository", "Do not copy civic material", "sirsi router pull {{agent}}", "EVIDENCE", "NOT COMPLETION"} {
		if !strings.Contains(cfg.Consumer.Prompt, guard) {
			t.Fatalf("missing privacy/queue boundary %q", guard)
		}
	}
	joined := strings.Join(cfg.Consumer.Command, "\n")
	if !strings.Contains(joined, "SIRSI_AGENT_ID=claude-kfca") || !strings.Contains(joined, "spool:///private/var/sirsipantheon/relay") || !strings.Contains(joined, "--max-budget-usd\n1") {
		t.Fatal("missing own bounded transport identity")
	}
}
