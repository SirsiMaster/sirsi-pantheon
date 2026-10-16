package router

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCompassPointBackstopsDeclareWorkingOwnQueueConsumers(t *testing.T) {
	b, err := os.ReadFile("../../.agents/idea-router/agents.json")
	if err != nil {
		t.Fatal(err)
	}
	var reg Registry
	if err := json.Unmarshal(b, &reg); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"claude-m5-compasspoint", "codex-compasspoint-contract"} {
		t.Run(id, func(t *testing.T) {
			cfg := reg.Agents[id]
			if cfg.ID != id || cfg.Wake.Mechanism != WakeLaunchAgent {
				t.Fatalf("missing durable wake contract: %+v", cfg.Wake)
			}
			workspace := "/compasspoint-build-worker"
			if cfg.Type == "codex" {
				workspace = "/compasspoint-contract-worker"
			}
			if !strings.HasSuffix(cfg.Repo, "/Development/compasspoint") || !strings.HasSuffix(cfg.Cwd, workspace) || cfg.Cwd == cfg.Repo {
				t.Fatalf("backstop must use the current verified project, not an ancestor/retired checkout: %s %s", cfg.Cwd, cfg.Repo)
			}
			joined := strings.Join(cfg.Consumer.Command, "\n")
			if !strings.Contains(joined, "SIRSI_AGENT_ID="+id) || !strings.Contains(joined, "spool:///private/var/sirsipantheon/relay") {
				t.Fatal("consumer must bind its own identity and canonical physical relay")
			}
			if strings.Contains(joined, "--dangerously") || strings.Contains(joined, "SIRSI_ROUTER_TOKEN=") {
				t.Fatal("no permission bypass or bearer credential in consumer argv")
			}
			if !strings.Contains(cfg.Consumer.Prompt, "{{thread}}") || !strings.Contains(cfg.Consumer.Prompt, "sirsi router pull {{agent}}") {
				t.Fatal("missing correlated own-thread/queue protocol")
			}
			if cfg.Type == "codex" {
				if !strings.Contains(joined, "SIRSI_ROUTER_SESSION_DIR=") || !strings.Contains(joined, "/worker-state/codex-compasspoint-contract/sessions") || strings.Contains(joined, "/.sirsi/sessions") {
					t.Fatal("sandbox must retain only its own writable session cache")
				}
				for _, arg := range cfg.Consumer.Command {
					if strings.Contains(arg, "permissions.") && strings.IndexByte(arg, 92) >= 0 {
						t.Fatal("literal shell escaping breaks TOML argv parsing")
					}
				}
				if !strings.Contains(joined, "permissions.sirsi-router.network.enabled=false") {
					t.Fatal("Codex backstop must remain no-network")
				}
			} else if !strings.Contains(joined, "--max-budget-usd\n1") {
				t.Fatal("Claude backstop must retain its bounded invocation budget")
			}
			// Resolve without starting a model, using a real isolated cwd.
			cfg.Cwd = t.TempDir()
			if c, reason := ResolveConsumer(cfg, t.TempDir()); c == nil {
				t.Fatalf("backstop cannot resolve: %s", reason)
			}
			cfg.Consumer = ConsumerConfig{}
			if c, _ := ResolveConsumer(cfg, t.TempDir()); c != nil {
				t.Fatal("old attended-only declaration must not pass as a consumer")
			}
		})
	}
}
