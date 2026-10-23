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
