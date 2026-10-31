//go:build darwin

package setup

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLegacyAppAuthorizationIsScopedRootAndExactInstalledEngine(t *testing.T) {
	for _, kind := range []string{"allowed", "user", "setuid_only", "bundle", "hash", "signature", "drift", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			uid, euid := 0, 0
			exe := LegacyAppEngine
			switch kind {
			case "user":
				uid, euid = 501, 501
			case "setuid_only":
				uid = 501
			case "bundle":
				exe = "/Applications/Pantheon.app/Contents/MacOS/sirsi"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancel" {
				cancel()
			}
			calls := 0
			err := authorizeLegacyAppAt(ctx, uid, euid, exe, strings.Repeat("a", 64), func() (string, error) {
				calls++
				if kind == "hash" || kind == "drift" && calls == 2 {
					return strings.Repeat("b", 64), nil
				}
				return strings.Repeat("a", 64), nil
			}, func(context.Context) error {
				if kind == "signature" {
					return errors.New("wrong team")
				}
				return nil
			})
			if (err == nil) != (kind == "allowed") {
				t.Fatal(err)
			}
			if (kind == "user" || kind == "setuid_only" || kind == "bundle" || kind == "cancel") && calls != 0 {
				t.Fatal("unqualified caller reached artifacts")
			}
		})
	}
}
