//go:build darwin

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaatPodServiceRefusesSandboxBeforeResolution(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "must-not-be-created")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(parent, "router.db"))
	_, err := resolveMaatPodServicePlan(maatPodServiceOptions{
		pod: "fixture", journal: "/unused/journal", listen: "100.64.1.2:9443",
		cert: "/unused/cert", key: "/unused/key", clientCA: "/unused/ca",
		recipeDir: "/unused/recipe", clients: []string{"fixture=agent"},
	})
	if err == nil || !strings.Contains(err.Error(), "sandbox selectors") {
		t.Fatalf("sandbox selector not rejected before native probes/resolution: %v", err)
	}
	if _, err := os.Stat(parent); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry refusal created or inspected the sandbox incorrectly: %v", err)
	}
}

func TestMaatPodServiceCommandSurface(t *testing.T) {
	for _, verb := range []string{"plan", "install"} {
		t.Run(verb+" help", func(t *testing.T) {
			cmd := newMaatPodServiceCmd()
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs([]string{verb, "--help"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			for _, flag := range []string{"--pod", "--journal", "--listen", "--cert", "--key", "--client-ca", "--client", "--recipe-dir"} {
				if !strings.Contains(output.String(), flag) {
					t.Fatalf("help missing %s", flag)
				}
			}
		})
		t.Run(verb+" rejects positional arguments", func(t *testing.T) {
			cmd := newMaatPodServiceCmd()
			cmd.SetArgs([]string{verb, "unexpected"})
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg") {
				t.Fatalf("positional input was not rejected before resolution: %v", err)
			}
		})
		t.Run(verb+" requires explicit selectors", func(t *testing.T) {
			cmd := newMaatPodServiceCmd()
			cmd.SetArgs([]string{verb})
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "are required") {
				t.Fatalf("missing selectors were not refused before resolution: %v", err)
			}
		})
	}
}
