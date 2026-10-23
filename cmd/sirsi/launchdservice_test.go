package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

type lifecycleCommandBackend struct {
	instance setup.ServiceInstance
	calls    []string
}

func (b *lifecycleCommandBackend) Inspect(_ context.Context, label string) (setup.ServiceInstance, error) {
	b.calls = append(b.calls, "inspect:"+label)
	return b.instance, nil
}

func (b *lifecycleCommandBackend) Refresh(context.Context, setup.ServiceInstance) error {
	b.calls = append(b.calls, "refresh")
	return nil
}

func TestRootRelayReadOnlyPlanAuthorizesBeforeInspection(t *testing.T) {
	denied := errors.New("fixture authorization denied")
	for _, tc := range []struct {
		name     string
		authErr  error
		canceled bool
	}{
		{name: "allowed"},
		{name: "denied", authErr: denied},
		{name: "canceled", authErr: context.Canceled, canceled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const label = "ai.sirsi.router.relay"
			backend := &lifecycleCommandBackend{instance: setup.ServiceInstance{Label: label, Target: "system/" + label, PID: 123, Started: "fixture-start"}}
			authorize := func(ctx context.Context, got string) error {
				backend.calls = append(backend.calls, "authorize:"+got)
				if got != label {
					t.Errorf("authorization label=%q, want %q", got, label)
				}
				if tc.canceled {
					return ctx.Err()
				}
				return tc.authErr
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.canceled {
				cancel()
			}
			c := newLaunchdServiceCmdWith(backend, authorize)
			c.SilenceUsage = true
			var out, stderr bytes.Buffer
			c.SetOut(&out)
			c.SetErr(&stderr)
			c.SetArgs([]string{label})
			err := c.ExecuteContext(ctx)
			if !errors.Is(err, tc.authErr) {
				t.Fatalf("plan error=%v, want %v", err, tc.authErr)
			}
			wantCalls := "authorize:" + label
			if tc.authErr == nil {
				wantCalls += ",inspect:" + label
				var plan setup.ServiceInstance
				if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
					t.Fatalf("plan JSON: %v", err)
				}
				if plan != backend.instance {
					t.Errorf("plan=%+v, want %+v", plan, backend.instance)
				}
			} else if out.Len() != 0 {
				t.Errorf("denied plan published output: %s", out.String())
			}
			if got := strings.Join(backend.calls, ","); got != wantCalls {
				t.Errorf("calls=%q, want %q (no refresh)", got, wantCalls)
			}
		})
	}
}

func TestOrdinaryReadOnlyServicePlansDoNotAuthorize(t *testing.T) {
	for _, label := range []string{"ai.sirsi.apollo.observer", "ai.sirsi.apollo-chat", "ai.sirsi.gemma-broker", "ai.sirsi.mercury-monitor", "ai.sirsi.mercury-chip"} {
		t.Run(label, func(t *testing.T) {
			backend := &lifecycleCommandBackend{instance: setup.ServiceInstance{Label: label, Target: "gui/501/" + label, PID: 123, Started: "fixture-start"}}
			c := newLaunchdServiceCmdWith(backend, func(context.Context, string) error {
				backend.calls = append(backend.calls, "authorize")
				return errors.New("ordinary read-only plan must not authorize")
			})
			var out, stderr bytes.Buffer
			c.SetOut(&out)
			c.SetErr(&stderr)
			c.SetArgs([]string{label})
			if err := c.Execute(); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(backend.calls, ","); got != "inspect:"+label {
				t.Errorf("calls=%q, want inspection only", got)
			}
			var plan setup.ServiceInstance
			if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
				t.Fatalf("plan JSON: %v", err)
			}
			if plan != backend.instance {
				t.Errorf("plan=%+v, want %+v", plan, backend.instance)
			}
		})
	}
}

func TestRootRelayApplyRetainsControllerAuthorization(t *testing.T) {
	const label = "ai.sirsi.router.relay"
	backend := &lifecycleCommandBackend{instance: setup.ServiceInstance{
		Label: label, Target: "system/" + label, PID: 123, Started: "fixture-start",
		Plist: "/fixture/relay.plist", PlistSHA256: strings.Repeat("a", 64),
		Program: "/fixture/sirsi", ProgramSHA256: strings.Repeat("b", 64),
	}}
	denied := errors.New("fixture controller authorization denied")
	c := newLaunchdServiceCmdWith(backend, func(_ context.Context, got string) error {
		backend.calls = append(backend.calls, "authorize:"+got)
		return denied
	})
	c.SilenceUsage = true
	var out, stderr bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&stderr)
	c.SetArgs([]string{label, "--apply", "--drained", "--expected-pid=123", "--expected-start=fixture-start",
		"--expected-plist-sha256=" + backend.instance.PlistSHA256,
		"--expected-program-sha256=" + backend.instance.ProgramSHA256})
	if err := c.Execute(); !errors.Is(err, denied) {
		t.Fatalf("apply error=%v, want controller refusal", err)
	}
	if got, want := strings.Join(backend.calls, ","), "inspect:"+label+",authorize:"+label; got != want {
		t.Errorf("calls=%q, want %q (controller authorizes once, no refresh)", got, want)
	}
	if out.Len() != 0 {
		t.Errorf("denied apply published output: %s", out.String())
	}
}

func TestLifecycleGrantIsExplicit(t *testing.T) {
	r := &router.Registry{Agents: map[string]router.AgentConfig{
		"owner": {LifecycleServices: []string{"ai.sirsi.apollo.observer"}},
		"other": {},
	}}
	for _, tc := range []struct {
		actor, label string
		allowed      bool
	}{
		{"owner", "ai.sirsi.apollo.observer", true},
		{"owner", "ai.sirsi.gemma-broker", false},
		{"other", "ai.sirsi.apollo.observer", false},
		{"sirsi-software-admin", "ai.sirsi.apollo.observer", false},
		{"owner", "ai.sirsi.*", false},
	} {
		if got := lifecycleGrant(r, tc.actor, tc.label) == nil; got != tc.allowed {
			t.Fatalf("grant %s/%s = %v", tc.actor, tc.label, got)
		}
	}
}

// A native profile is not permission: only the explicitly nominated operational
// owners may refresh the root relay. This also catches accidental broad grants
// to workers, the source lane, or the actor-only Horus identity.
func TestRootRelayRegistryGrantIsNarrow(t *testing.T) {
	r, err := router.LoadRegistry("../../.agents/idea-router")
	if err != nil {
		t.Fatal(err)
	}
	for actor := range r.Agents {
		want := actor == "sirsi-software-admin" || actor == "sirsi-hardware-admin"
		if got := lifecycleGrant(r, actor, "ai.sirsi.router.relay") == nil; got != want {
			t.Errorf("root relay grant for %q = %v; want %v", actor, got, want)
		}
	}
	for _, actor := range []string{"sirsi-software-admin", "sirsi-hardware-admin"} {
		for _, label := range []string{"ai.sirsi.router", "ai.sirsi.router.wake.sirsi-software-admin", "ai.sirsi.router.*", "system/ai.sirsi.router.relay"} {
			if lifecycleGrant(r, actor, label) == nil {
				t.Errorf("expanded grant accepted for %s/%s", actor, label)
			}
		}
	}
}

func TestLifecycleApplyRequiresEveryPinBeforeNativeInspection(t *testing.T) {
	for _, missing := range []string{"drained", "expected-pid", "expected-start", "expected-plist-sha256", "expected-program-sha256"} {
		t.Run(missing, func(t *testing.T) {
			args := []string{"ai.sirsi.apollo.observer", "--apply"}
			for k, v := range map[string]string{"drained": "true", "expected-pid": "123", "expected-start": "start", "expected-plist-sha256": strings.Repeat("a", 64), "expected-program-sha256": strings.Repeat("b", 64)} {
				if k != missing {
					args = append(args, "--"+k+"="+v)
				}
			}
			c := newLaunchdServiceCmd()
			var out bytes.Buffer
			c.SetOut(&out)
			c.SetErr(&out)
			c.SetArgs(args)
			if err := c.Execute(); err == nil || !strings.Contains(err.Error(), "all four exact plan pins") {
				t.Fatalf("unexpected result %v", err)
			}
		})
	}
}
