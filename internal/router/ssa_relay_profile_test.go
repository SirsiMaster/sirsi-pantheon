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
		`permissions.sirsi-router.filesystem={"/private/var/sirsipantheon/relay"="write"}`,
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
