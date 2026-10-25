//go:build darwin

package servicedrain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaintenanceEndpointCannotBypassModernOrOccupiedPath(t *testing.T) {
	for _, kind := range []string{"absent", "occupied", "symlink", "loose_parent"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			if err := os.Chmod(parent, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(parent, "selected.sock")
			switch kind {
			case "occupied":
				if err := os.WriteFile(path, []byte("inert existing endpoint marker"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink("missing", path); err != nil {
					t.Fatal(err)
				}
			case "loose_parent":
				if err := os.Chmod(parent, 0755); err != nil {
					t.Fatal(err)
				}
			}
			err := maintenanceEndpointAbsentAt(path, os.Getuid())
			if (err != nil) != (kind != "absent") {
				t.Fatalf("%s: %v", kind, err)
			}
		})
	}
}
