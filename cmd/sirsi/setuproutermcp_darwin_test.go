//go:build darwin

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

type fixtureRouterMCPMigrator struct {
	selection      setup.RouterMCPSelection
	confirmation   string
	plans, applies int
}

func (f *fixtureRouterMCPMigrator) Plan(s setup.RouterMCPSelection) (setup.RouterMCPMigrationPlan, error) {
	f.selection = s
	f.plans++
	return setup.RouterMCPMigrationPlan{Selection: s, Status: "planned_client_reconnect_unobserved", Digest: "owned-digest"}, nil
}
func (f *fixtureRouterMCPMigrator) Apply(s setup.RouterMCPSelection, confirmed string) (setup.RouterMCPMigrationResult, error) {
	f.selection = s
	f.confirmation = confirmed
	f.applies++
	return setup.RouterMCPMigrationResult{Status: "configuration_migrated_client_reconnect_pending"}, nil
}

func TestSetupRouterMCPMigrationExactSelectionWiring(t *testing.T) {
	for _, action := range []string{"plan", "apply"} {
		t.Run(action, func(t *testing.T) {
			fake := &fixtureRouterMCPMigrator{}
			cmd := newSetupRouterMCPCmd(func() (routerMCPConfigMigrator, error) { return fake, nil })
			args := []string{action, "--client", "claude-code", "--config", "/owned/.claude.json", "--server", "router", "--project", "/owned/repo"}
			if action == "apply" {
				args = append(args, "--confirm", "exact-reviewed-digest")
			}
			cmd.SetArgs(args)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(io.Discard)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			want := setup.RouterMCPSelection{Client: "claude-code", Config: "/owned/.claude.json", Server: "router", Project: "/owned/repo"}
			if fake.selection != want {
				t.Fatal("actual flags lost exact selected scope")
			}
			if action == "plan" && (fake.plans != 1 || fake.applies != 0) {
				t.Fatal("preview invoked apply")
			}
			if action == "apply" && (fake.applies != 1 || fake.plans != 0 || fake.confirmation != "exact-reviewed-digest") {
				t.Fatal("apply lost confirmation")
			}
			var result map[string]interface{}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal("output not one JSON document")
			}
		})
	}
}

func TestSetupRouterMCPMigrationRefusesInvalidInvocationBeforeFactory(t *testing.T) {
	for _, args := range [][]string{
		{"plan", "unexpected", "--client", "cursor", "--config", "/owned/.cursor/mcp.json", "--server", "router"},
		{"plan", "--client", "cursor", "--config", "/owned/.cursor/mcp.json"},
		{"apply", "--client", "cursor", "--config", "/owned/.cursor/mcp.json", "--server", "router"},
	} {
		calls := 0
		cmd := newSetupRouterMCPCmd(func() (routerMCPConfigMigrator, error) { calls++; return &fixtureRouterMCPMigrator{}, nil })
		cmd.SetArgs(args)
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if err := cmd.Execute(); err == nil || calls != 0 {
			t.Fatal("ignored/required arguments reached migration factory")
		}
	}
}

func TestSetupRouterMCPDoesNotRetargetGenericMCP(t *testing.T) {
	cmd := newSetupRouterMCPCmd(func() (routerMCPConfigMigrator, error) { t.Fatal("help invoked config factory"); return nil, nil })
	cmd.SetArgs([]string{"--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "plan") || !strings.Contains(out.String(), "apply") {
		t.Fatal("separate explicit migration surface missing")
	}
}
