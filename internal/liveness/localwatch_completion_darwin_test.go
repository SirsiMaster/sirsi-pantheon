package liveness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalPostconditionRequiresOwnedExactTargetAndAuthoritativeNegative(t *testing.T) {
	for _, mode := range []string{"known-absent", "missing-parent", "symlink-parent", "writable-parent", "symlink-leaf", "writable-leaf", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			home, engine := localInstallFixture(t)
			if err := os.Chmod(home, 0o700); err != nil {
				t.Fatal(err)
			}
			agents := filepath.Join(home, "Library", "LaunchAgents")
			if mode != "missing-parent" {
				if err := os.MkdirAll(agents, 0o755); err != nil {
					t.Fatal(err)
				}
				// Explicit modes make these fixtures independent of 022/077.
				for _, dir := range []string{filepath.Dir(agents), agents} {
					if err := os.Chmod(dir, 0o755); err != nil {
						t.Fatal(err)
					}
				}
			}
			path := filepath.Join(agents, Label+".plist")
			switch mode {
			case "symlink-parent":
				if err := os.Remove(agents); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), agents); err != nil {
					t.Fatal(err)
				}
			case "writable-parent":
				if err := os.Chmod(agents, 0o777); err != nil {
					t.Fatal(err)
				}
			case "symlink-leaf":
				if err := os.Symlink(engine, path); err != nil {
					t.Fatal(err)
				}
			case "writable-leaf":
				if err := os.WriteFile(path, []byte(LocalPlistContent(engine, home)), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o666); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			calls := []string{}
			observation, err := InspectLocalWatch(ctx, engine, home, localFixtureCommand(engine, home, false, &calls))
			if mode == "known-absent" {
				if err != nil || !observation.Concern || observation.WatchPostcondition != "known-absent" {
					t.Fatalf("owned exact missing target not classified: %+v %v", observation, err)
				}
			} else {
				if observation.WatchPostcondition != "" {
					t.Fatalf("unsafe/unknown metadata became an eligible failure: %+v %v", observation, err)
				}
				if mode != "missing-parent" && err == nil {
					t.Fatalf("unsafe/canceled metadata was not refused: %s", mode)
				}
				if mode == "canceled" && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
			}
			for _, command := range calls {
				if command != fmt.Sprintf("/bin/launchctl print-disabled gui/%d", os.Getuid()) {
					t.Fatalf("negative inspection invoked unexpected command: %s", command)
				}
			}
		})
	}
}
