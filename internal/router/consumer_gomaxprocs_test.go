package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findEnv(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix), true
		}
	}
	return "", false
}

// TestResolveConsumerCapsGOMAXPROCS proves the R7/G6 fix at the env-building
// level (task requirement 4a) — no real spawn, just the resolved environment
// a spawned consumer would receive.
func TestResolveConsumerCapsGOMAXPROCS(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "consumer.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrue\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := AgentConfig{
		ID: "worker-agent",
		Consumer: ConsumerConfig{
			Command: []string{script, "--agent", consumerAgentPlaceholder},
		},
	}
	rc, why := ResolveConsumer(cfg, dir)
	if rc == nil {
		t.Fatalf("ResolveConsumer refused: %s", why)
	}
	got, ok := findEnv(rc.Env, EnvGOMAXPROCS)
	if !ok {
		t.Fatal("GOMAXPROCS not present in resolved consumer env")
	}
	if got != "4" {
		t.Fatalf("GOMAXPROCS = %q, want default 4", got)
	}
}

// TestResolveConsumerRespectsExplicitGOMAXPROCS is the other direction: an
// operator-declared consumer.env GOMAXPROCS must not be silently overridden.
func TestResolveConsumerRespectsExplicitGOMAXPROCS(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "consumer.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrue\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := AgentConfig{
		ID: "worker-agent",
		Consumer: ConsumerConfig{
			Command: []string{script, "--agent", consumerAgentPlaceholder},
		},
		Env: map[string]string{"GOMAXPROCS": "8"},
	}
	rc, why := ResolveConsumer(cfg, dir)
	if rc == nil {
		t.Fatalf("ResolveConsumer refused: %s", why)
	}
	got, ok := findEnv(rc.Env, EnvGOMAXPROCS)
	if !ok {
		t.Fatal("GOMAXPROCS not present in resolved consumer env")
	}
	if got != "8" {
		t.Fatalf("GOMAXPROCS = %q, want operator override 8", got)
	}
}
