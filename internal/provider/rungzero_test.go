package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The local broker is rung zero of the certainties ladder and stays rung zero no
// matter what remote is configured. codex-pantheon blocked PR #339 on exactly
// this: Local() used to build an endpoint ONLY when conf.Provider was
// ""/gemma/local, so configuring any remote provider silently deleted the local
// rung — no error, no log, Sirsi just became cloud-only on the machine whose
// owner most expects the offline path to answer.

// localHome builds a fake $HOME whose gemma port file advertises a broker.
func localHome(t *testing.T, port string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatalf("mkdir .sirsi: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-server.port"), []byte(port), 0o600); err != nil {
		t.Fatalf("write port file: %v", err)
	}
	return home
}

func TestLocalSurvivesEveryRemoteProvider(t *testing.T) {
	home := localHome(t, "8765")

	// Every remote provider the conf can name. None may delete rung zero.
	for _, prov := range []string{"claude", "codex", "gemini", "openai", "anthropic"} {
		t.Run(prov, func(t *testing.T) {
			got := Local(home, Conf{Provider: prov, Model: "some-remote-model"})
			if got == nil {
				t.Fatalf("provider=%q deleted the local rung — the local broker must remain rung zero regardless of which remote is configured", prov)
			}
			if got.Endpoint != "http://127.0.0.1:8765/v1" {
				t.Fatalf("provider=%q: local endpoint = %q, want the broker's own port file", prov, got.Endpoint)
			}
			if got.TierValue != TierLocal {
				t.Fatalf("provider=%q: tier = %v, want TierLocal", prov, got.TierValue)
			}
		})
	}
}

// A remote conf carries a REMOTE endpoint. That endpoint must never be adopted
// as the local rung's address — doing so would send "local" traffic to the cloud
// while still labeling it local, which is worse than having no local rung.
func TestRemoteEndpointNeverBecomesTheLocalRung(t *testing.T) {
	home := localHome(t, "8765")
	got := Local(home, Conf{Provider: "openai", Endpoint: "https://api.openai.com/v1"})
	if got == nil {
		t.Fatal("local rung deleted by a remote conf")
	}
	if got.Endpoint != "http://127.0.0.1:8765/v1" {
		t.Fatalf("local rung adopted a non-loopback endpoint %q — local must always be loopback", got.Endpoint)
	}
}

// Ladder ordering is the contract the whole design rests on: local first, remote
// only as an enhancement. Assert the ORDER, not merely that both are present.
func TestLadderKeepsLocalFirstWithRemoteConfigured(t *testing.T) {
	home := localHome(t, "8765")
	t.Setenv("SIRSI_REMOTE_API_KEY", "sk-test")
	t.Setenv("SIRSI_REMOTE_ENDPOINT", "https://api.example.com/v1")

	lad := Ladder(context.Background(), home)
	if len(lad) < 2 {
		t.Fatalf("want both rungs with a remote configured, got %d: %v", len(lad), names(lad))
	}
	if lad[0].Name() != "local" {
		t.Fatalf("ladder order = %v, want local first — remote is an enhancement, never a replacement", names(lad))
	}
	if lad[1].Name() != "remote" {
		t.Fatalf("ladder order = %v, want remote second", names(lad))
	}
}

// The one legitimate reason to drop local: there is no broker to talk to.
func TestLocalAbsentOnlyWhenNoBrokerExists(t *testing.T) {
	if got := Local(t.TempDir(), Conf{}); got != nil {
		t.Fatalf("no port file must mean no local rung, got %+v", got)
	}
}

func names(ps []Provider) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Name())
	}
	return out
}
