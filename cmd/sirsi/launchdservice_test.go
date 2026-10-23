package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

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
