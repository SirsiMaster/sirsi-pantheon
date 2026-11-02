package router

import (
	"os"
	"strings"
	"testing"
)

// The allowlist codex lanes declared by hand before the registry rendered it.
const legacyLaneDomains = `permissions.sirsi-router.network.domains={"100.88.242.95"="allow","100.92.193.26"="allow","100.87.95.88"="allow","192.168.1.185"="allow","10.96.127.2"="allow"}`

// The checked-in registry must render to exactly what every lane allowed before,
// so moving to the placeholder neither widens nor narrows a live sandbox.
func TestCheckedInRegistryRendersTheLegacyLaneAllowlist(t *testing.T) {
	b, err := os.ReadFile("../../.agents/idea-router/machines.json")
	if err != nil {
		t.Fatal(err)
	}
	ms, err := parseMachines(b)
	if err != nil {
		t.Fatal(err)
	}
	got, err := machineTailnetDomains(ms)
	if err != nil {
		t.Fatal(err)
	}
	declared := `permissions.sirsi-router.network.domains={` + consumerTailnetPlaceholder + `,"192.168.1.185"="allow","10.96.127.2"="allow"}`
	if r := strings.ReplaceAll(declared, consumerTailnetPlaceholder, got); r != legacyLaneDomains {
		t.Fatalf("rendered allowlist drifted:\n got %s\nwant %s", r, legacyLaneDomains)
	}
}

func TestMachineTailnetDomainsFailsClosed(t *testing.T) {
	m := func(id, ts string) Machine { return Machine{ID: id, Tailscale: ts} }
	for _, tc := range []struct {
		name string
		ms   []Machine
	}{
		{"none", []Machine{m("M1", "")}},
		{"wildcard", []Machine{m("M1", "*")}},
		{"table injection", []Machine{m("M1", `100.88.242.95"="allow","*`)}},
		{"outside tailnet", []Machine{m("M1", "8.8.8.8")}},
		{"lan address", []Machine{m("M1", "192.168.1.179")}},
		{"ipv6", []Machine{m("M1", "fd7a:115c:a1e0::1")}},
		{"claimed twice", []Machine{m("M1", "100.88.242.95"), m("M5", "100.88.242.95")}},
	} {
		if got, err := machineTailnetDomains(tc.ms); err == nil {
			t.Errorf("%s: rendered %q, want refusal", tc.name, got)
		}
	}
	got, err := machineTailnetDomains([]Machine{m("M1", "100.88.242.95"), m("Off", ""), m("M5", "100.92.193.26")})
	if err != nil || got != `"100.88.242.95"="allow","100.92.193.26"="allow"` {
		t.Fatalf("got %q, %v", got, err)
	}
}

// A lane that declares the placeholder on a host whose registry cannot render it
// must stay unspawned with a reason, never run with an empty allowlist.
func TestConsumerWithTailnetPlaceholderRefusesWithoutRegistry(t *testing.T) {
	root := t.TempDir()
	cfg := AgentConfig{ID: "codex-x", Consumer: ConsumerConfig{Command: []string{"/bin/echo", "{{agent}}", "domains={" + consumerTailnetPlaceholder + "}"}}}
	rc, why := ResolveConsumer(cfg, root)
	if rc != nil || !strings.Contains(why, "network allowlist") {
		t.Fatalf("rc=%v why=%q", rc, why)
	}
	if err := os.WriteFile(root+"/machines.json", []byte(`{"machines":[{"id":"M1","model":"m","tailscale":"100.88.242.95","registered_at":"t","admitted_by":"ra"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rc, why = ResolveConsumer(cfg, root)
	if rc == nil {
		t.Fatalf("refused with registry present: %s", why)
	}
	if want := `domains={"100.88.242.95"="allow"}`; rc.Argv[2] != want {
		t.Fatalf("argv %q, want %q", rc.Argv[2], want)
	}
}
