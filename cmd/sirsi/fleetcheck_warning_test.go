package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFleetCheckUnavailableWarnings(t *testing.T) {
	for _, kind := range []string{"missing-script", "missing-bash", "empty-failure"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			if kind != "missing-script" {
				if err := os.Mkdir(filepath.Join(root, "scripts"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "scripts", "fleet-check.sh"), []byte("exit 7\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "missing-bash" {
				t.Setenv("PATH", t.TempDir())
			}
			got := fleetCheckWarning(root)
			if !strings.Contains(got, "UNKNOWN") {
				t.Fatalf("unavailable evidence silently omitted: %q", got)
			}
		})
	}
}
func TestFleetCheckHealthyAndDiagnosticOutput(t *testing.T) {
	for _, tc := range []struct{ body, want string }{{"exit 0\n", ""}, {"echo 'queue depth unknown'; exit 1\n", "queue depth unknown"}} {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "scripts"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "scripts", "fleet-check.sh"), []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		if got := fleetCheckWarning(root); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
}
