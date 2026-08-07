package platform

import (
	"reflect"
	"testing"
)

func TestParseDisabledLabels(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{
			"only disabled entries counted",
			"\"ai.sirsi.triage\" => disabled\n\"ai.sirsi.pantheon\" => enabled\n",
			[]string{"ai.sirsi.triage"},
		},
		{
			// The 2026-08-06 corrupt key: five labels written as one by an
			// unquoted "$@". Verbatim it matches nothing and the quarantine
			// goes invisible.
			"space-joined key splits into every label",
			"\"ai.sirsi.horus.agent-router ai.sirsi.triage ai.sirsi.pantheon ai.sirsi.gemma-worker ai.sirsi.gemma-broker\" => disabled\n",
			[]string{
				"ai.sirsi.horus.agent-router", "ai.sirsi.triage", "ai.sirsi.pantheon",
				"ai.sirsi.gemma-worker", "ai.sirsi.gemma-broker",
			},
		},
		{
			"joined key marked enabled stays out",
			"\"ai.sirsi.triage ai.sirsi.pantheon\" => enabled\n",
			nil,
		},
		{"unquoted line skipped", "ai.sirsi.triage => disabled\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ParseDisabledLabels(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestManagedLabel(t *testing.T) {
	for _, l := range []string{"ai.sirsi.triage", "actions.runner.SirsiMaster-x.m5"} {
		if !ManagedLabel(l) {
			t.Errorf("%s should be managed", l)
		}
	}
	if ManagedLabel("com.apple.something") {
		t.Error("com.apple.* is not managed")
	}
}
