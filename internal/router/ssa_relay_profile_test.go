package router

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSSAConsumerDeclaresCanonicalRelayPermissions(t *testing.T) {
	b, err := os.ReadFile("../../.agents/idea-router/agents.json")
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Agents map[string]AgentConfig `json:"agents"`
	}
	if err := json.Unmarshal(b, &registry); err != nil {
		t.Fatal(err)
	}
	cmd := registry.Agents["sirsi-software-admin"].Consumer.Command
	joined := strings.Join(cmd, "\n")
	for _, required := range []string{
		`default_permissions="sirsi-router"`,
		`permissions.sirsi-router.extends=":workspace"`,
		`permissions.sirsi-router.filesystem={"/private/var/sirsipantheon/relay"="write","/Users/thekryptodragon/.sirsi/maat/decisions.jsonl"="write","/Users/thekryptodragon/.sirsi/maat/.decisions.jsonl.lock"="write","/Users/thekryptodragon/.sirsi/evidence/maat-runs"="write"}`,
		`permissions.sirsi-router.network.enabled=true`,
		`features.network_proxy.enabled=true`,
		`permissions.sirsi-router.network.allow_local_binding=false`,
	} {
		if !strings.Contains(joined, "-c\n"+required) {
			t.Errorf("missing configuration %q", required)
		}
	}
	if strings.Contains(joined, "sandbox_workspace_write") || strings.Contains(joined, "--sandbox") {
		t.Fatal("legacy sandbox selection must not override the named permissions")
	}
}

func TestRestrictedConsumersAllowOnlyExactMaatLedgerEvidenceAndRelay(t *testing.T) {
	b, err := os.ReadFile("../../.agents/idea-router/agents.json")
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Agents map[string]AgentConfig `json:"agents"`
	}
	if err := json.Unmarshal(b, &registry); err != nil {
		t.Fatal(err)
	}
	wantIDs := map[string]bool{
		"codex-assiduous": true, "codex-deck": true, "codex-finalwishes": true,
		"codex-finalwishes-web": true, "codex-homebrew": true, "codex-mail": true,
		"codex-pantheon": true, "codex-porch-and-alley": true,
		"sirsi-hardware-admin": true, "sirsi-software-admin": true,
		"codex-apollo": true, "codex-compasspoint-contract": true,
	}
	const want = `permissions.sirsi-router.filesystem={"/private/var/sirsipantheon/relay"="write","/Users/thekryptodragon/.sirsi/maat/decisions.jsonl"="write","/Users/thekryptodragon/.sirsi/maat/.decisions.jsonl.lock"="write","/Users/thekryptodragon/.sirsi/evidence/maat-runs"="write"}`
	for id, agent := range registry.Agents {
		named := false
		for _, arg := range agent.Consumer.Command {
			if arg == `default_permissions="sirsi-router"` {
				named = true
			}
		}
		if !named {
			continue
		}
		if !wantIDs[id] {
			t.Errorf("unexpected restricted consumer %q", id)
		}
		delete(wantIDs, id)
		found := 0
		for _, arg := range agent.Consumer.Command {
			// Include dotted overrides: a second filesystem declaration must not
			// silently grant the home, whole .sirsi tree or unrelated siblings.
			if strings.HasPrefix(arg, "permissions.sirsi-router.filesystem") {
				found++
				if arg != want {
					t.Errorf("%s: unexpected writable scope %q", id, arg)
				}
			}
		}
		if found != 1 {
			t.Errorf("%s: got %d filesystem declarations", id, found)
		}
	}
	for id := range wantIDs {
		t.Errorf("missing restricted consumer %q", id)
	}
	// Rebase the complete quoted policy, not just the journal path. Exact
	// equality retains the relay and file boundaries with no ancestor grant.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	foreign := strings.ReplaceAll(want, "/Users/thekryptodragon", "/Users/no-such-user-xyz")
	expected := strings.ReplaceAll(want, "/Users/thekryptodragon", home)
	if got := rebaseForeignHome(foreign); got != expected {
		t.Fatalf("rebased policy=%q, want %q", got, expected)
	}
	if got := rebaseForeignHome(expected); got != expected {
		t.Fatalf("own-home policy changed: %q", got)
	}
}
