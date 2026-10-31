//go:build darwin

package setup

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestMaintenanceRecoveryUsesOnlySelectedProtectedAuthority(t *testing.T) {
	for _, kind := range []string{"direct", "unprivileged", "spool", "missing_token", "wrong_profile", "drift_before", "drift_after", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			uid := 0
			if kind == "unprivileged" {
				uid = 501
			}
			url := "https://canonical.fixture.invalid"
			if kind == "spool" {
				url = "spool:///owned/held"
			}
			token := "private-owned-fixture-token"
			if kind == "missing_token" {
				token = ""
			}
			config := rootRelayConfiguration{Label: rootRelayLabel, UserName: "_sirsipantheon", ProgramArguments: []string{rootRelayProgram, "router", "relay", "serve", "--spool", rootRelaySpool}}
			if kind == "wrong_profile" {
				config.UserName = "root"
			}
			raw, err := json.Marshal(struct {
				rootRelayConfiguration
				EnvironmentVariables map[string]string
			}{config, map[string]string{"SIRSI_ROUTER_URL": url, "SIRSI_ROUTER_TOKEN": token}})
			if err != nil {
				t.Fatal(err)
			}
			reads, calls := 0, 0
			b := nativeServiceBackend{relay: &rootRelayDependencies{uid: func() int { return uid }, euid: func() int { return uid }, artifact: func(path string, capture bool) ([]byte, string, error) {
				if path != rootRelayPlist || !capture {
					t.Fatal("unexpected authority artifact")
				}
				reads++
				digest := strings.Repeat("1", 64)
				if kind == "drift_before" && reads > 1 || kind == "drift_after" && reads > 2 {
					digest = strings.Repeat("2", 64)
				}
				return raw, digest, nil
			}, read: func(ctx context.Context, input []byte, exe string, args ...string) ([]byte, error) {
				if exe != "/usr/bin/plutil" || string(input) != string(raw) {
					t.Fatal("unexpected private decode")
				}
				return raw, nil
			}}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancel" {
				cancel()
			}
			err = b.withMaintenanceAuthority(ctx, func(endpoint, secret, pin string) error {
				calls++
				if endpoint != url || secret != token || pin != "" {
					t.Fatal("selected authority changed")
				}
				return nil
			})
			wantCalls := 0
			if kind == "direct" || kind == "drift_after" {
				wantCalls = 1
			}
			if calls != wantCalls || (err == nil) != (kind == "direct") {
				t.Fatalf("%s calls%d err%v", kind, calls, err)
			}
			if err != nil && strings.Contains(err.Error(), token) && token != "" {
				t.Fatal("private token leaked")
			}
			if kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}
