package main

import (
	"errors"
	"testing"
)

func TestPurgeCachesNeverPromptsUnattended(t *testing.T) {
	cases := []struct {
		name        string
		sudoFails   bool
		unattended  bool
		wantSkipped bool
		wantOsa     bool
	}{
		{"sudo ok, unattended", false, true, false, false},
		{"sudo ok, interactive", false, false, false, false},
		{"sudo fails, unattended: skip, no dialog", true, true, true, false},
		{"sudo fails, interactive: dialog fallback", true, false, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			osa := false
			run := func(name string, _ ...string) ([]byte, error) {
				if name == "osascript" {
					osa = true
					return nil, nil
				}
				if c.sudoFails {
					return []byte("sudo: a password is required"), errors.New("exit 1")
				}
				return nil, nil
			}
			skipped, _, err := purgeCaches(run, c.unattended)
			if skipped != c.wantSkipped || osa != c.wantOsa {
				t.Fatalf("skipped=%v osascript=%v, want skipped=%v osascript=%v", skipped, osa, c.wantSkipped, c.wantOsa)
			}
			if err != nil && !c.wantSkipped {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}
