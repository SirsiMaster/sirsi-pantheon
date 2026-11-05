package main

import (
	"strings"
	"testing"
)

func TestRouterRelayPinWarning(t *testing.T) {
	const token = "SIRSI_ROUTER_TOKEN => secret-value"
	for _, tc := range []struct {
		name   string
		system string // launchctl output for the system daemon; "" means not loaded
		gui    string // same, for the per-user agent
		want   string // substring of the warning; "" means no warning
	}{
		{name: "no relay loaded"},
		{name: "system relay pinned", system: token + "\nSIRSI_ROUTER_SPKI_PIN => abc"},
		{name: "user relay pinned", gui: token + "\nSIRSI_ROUTER_SPKI_PIN => abc"},
		{name: "system relay unpinned", system: token, want: "system/ai.sirsi.router.relay"},
		{name: "user relay unpinned", gui: token, want: "/ai.sirsi.router.relay"},
		{name: "second relay unpinned", system: token + "\nSIRSI_ROUTER_SPKI_PIN => abc", gui: token, want: "gui/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := relayJobEnvFn
			defer func() { relayJobEnvFn = old }()
			relayJobEnvFn = func(target string) (string, bool) {
				out := tc.gui
				if strings.HasPrefix(target, "system/") {
					out = tc.system
				}
				return out, out != ""
			}
			got := routerRelayPinWarning()
			if tc.want == "" {
				if got != "" {
					t.Fatalf("want no warning, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("warning %q does not name %q", got, tc.want)
			}
			if strings.Contains(got, "secret-value") {
				t.Fatalf("warning leaked launchctl output: %q", got)
			}
		})
	}
}
