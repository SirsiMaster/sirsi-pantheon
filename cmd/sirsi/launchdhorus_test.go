package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

func TestHorusPlanRequiresExistingActorBeforeInspection(t *testing.T) {
	for _, denied := range []bool{false, true} {
		b := &lifecycleCommandBackend{instance: setup.ServiceInstance{Label: setup.HorusServiceLabel, PlanSHA256: strings.Repeat("a", 64)}}
		refusal := errors.New("fixture actor denied")
		cmd := newLaunchdServiceCmdWith(b, func(context.Context, string) error {
			b.calls = append(b.calls, "authorize")
			if denied {
				return refusal
			}
			return nil
		})
		cmd.SilenceUsage = true // Cobra usage text is not published service state.
		var out, stderr bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{setup.HorusServiceLabel})
		err := cmd.Execute()
		if denied {
			if !errors.Is(err, refusal) || strings.Join(b.calls, ",") != "authorize" || out.Len() != 0 {
				t.Fatalf("denied plan: err=%v calls=%v output=%q", err, b.calls, out.String())
			}
		} else if err != nil || strings.Join(b.calls, ",") != "authorize,inspect:"+setup.HorusServiceLabel {
			t.Fatal("read-only plan changed lifecycle sequence")
		}
	}
}

func TestHorusApplyPinsBeforeMutation(t *testing.T) {
	for _, name := range []string{"missing-drain", "missing-plan", "changed-plan", "denied"} {
		t.Run(name, func(t *testing.T) {
			b := &lifecycleCommandBackend{instance: setup.ServiceInstance{Label: setup.HorusServiceLabel, PlanSHA256: strings.Repeat("a", 64)}}
			cmd := newLaunchdServiceCmdWith(b, func(context.Context, string) error {
				b.calls = append(b.calls, "authorize")
				if name == "denied" {
					return errors.New("denied")
				}
				return nil
			})
			args := []string{setup.HorusServiceLabel, "--apply", "--expected-pid=123", "--expected-start=start", "--expected-plist-sha256=" + strings.Repeat("b", 64), "--expected-program-sha256=" + strings.Repeat("c", 64)}
			if name != "missing-drain" {
				args = append(args, "--drained")
			}
			if name != "missing-plan" {
				args = append(args, "--expected-plan-sha256="+strings.Repeat("d", 64))
			}
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil {
				t.Fatal("unsafe apply accepted")
			}
			if strings.Contains(strings.Join(b.calls, ","), "refresh") {
				t.Fatal("unsafe apply reached mutation")
			}
			if (name == "missing-drain" || name == "missing-plan") && len(b.calls) != 0 {
				t.Fatal("incomplete pins reached native inspection")
			}
			if name == "denied" && strings.Join(b.calls, ",") != "authorize" {
				t.Fatal("denied Horus apply reached inspection")
			}
		})
	}
}

func TestHorusRegistryGrantIsExactSSAAndSHA(t *testing.T) {
	// Source fixture only: never consult an installed registry/cache or router.
	raw, err := os.ReadFile("../../.agents/idea-router/agents.json")
	if err != nil {
		t.Fatal(err)
	}
	var reg router.Registry
	if err = json.Unmarshal(raw, &reg); err != nil {
		t.Fatal(err)
	}
	for actor := range reg.Agents {
		want := actor == "sirsi-software-admin" || actor == "sirsi-hardware-admin"
		if got := lifecycleGrant(&reg, actor, setup.HorusServiceLabel) == nil; got != want {
			t.Errorf("Horus grant %s=%v want %v", actor, got, want)
		}
	}
	for _, actor := range []string{"sirsi-software-admin", "sirsi-hardware-admin"} {
		for _, label := range []string{"ai.sirsi.horus.*", "ai.sirsi.horus", "gui/501/" + setup.HorusServiceLabel} {
			if lifecycleGrant(&reg, actor, label) == nil {
				t.Fatal("broad or domain-substituted Horus grant")
			}
		}
	}
}
