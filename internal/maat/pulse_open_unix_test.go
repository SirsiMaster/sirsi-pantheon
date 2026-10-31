//go:build unix

package maat

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestCountDeities_RejectsFIFOAndSymlink(t *testing.T) {
	for _, kind := range []string{"fifo", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "internal", "deity", "registry.go")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if kind == "fifo" {
				// No writer: a blocking open must fail the deadline assertion.
				if err := unix.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				target := filepath.Join(root, "actual-registry.go")
				if err := os.WriteFile(target, []byte(pulseRosterFixture), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan int, 1)
			go func() { done <- countDeities(root) }()
			select {
			case got := <-done:
				if got != 0 {
					t.Errorf("%s roster: countDeities() = %d, want 0", kind, got)
				}
			case <-time.After(time.Second):
				t.Fatalf("%s roster: countDeities blocked beyond 1 second", kind)
			}
		})
	}
}
