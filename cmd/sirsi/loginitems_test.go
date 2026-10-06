package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBareShellLoginItemsFlagsMissingBundleID(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Bare interpreter, no AssociatedBundleIdentifiers: should be flagged.
	write("ai.sirsi.host-readiness-watch.plist", `<string>/bin/zsh</string>`)
	// Bare interpreter, but already carries the key: should not be flagged.
	write("ai.sirsi.pantheon.plist", `<string>/bin/zsh</string><key>AssociatedBundleIdentifiers</key>`)
	// Direct argv, no shell at all: should not be flagged.
	write("ai.sirsi.swap-hygiene.plist", `<string>/opt/homebrew/bin/sirsi</string>`)
	// Not ours: should be ignored even though it matches the pattern.
	write("com.other.thing.plist", `<string>/bin/zsh</string>`)
	// Right prefix, wrong extension: should be ignored.
	write("ai.sirsi.notes.txt", `<string>/bin/zsh</string>`)

	got := bareShellLoginItems([]string{dir})
	want := []string{"ai.sirsi.host-readiness-watch"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("bareShellLoginItems() = %v, want %v", got, want)
	}
}

func TestBareShellLoginItemsSkipsUnreadableDir(t *testing.T) {
	got := bareShellLoginItems([]string{filepath.Join(t.TempDir(), "does-not-exist")})
	if len(got) != 0 {
		t.Fatalf("expected no findings for an unreadable dir, got %v", got)
	}
}

func TestDeadThirdPartyPlistsFlagsEmptyAndMissingBinary(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("com.google.keystone.agent.plist", ``)                                                                                                          // empty: dead
	write("com.google.GoogleUpdater.wake.plist", `<dict><key>ProgramArguments</key><array><string>/does/not/exist/GoogleUpdater</string></array></dict>`) // missing binary: dead
	write("com.other.live.plist", `<dict><key>Program</key><string>/bin/echo</string></dict>`)                                                            // existing binary: alive
	write("ai.sirsi.pantheon.plist", ``)                                                                                                                  // ours: ignored even though empty

	got := deadThirdPartyPlists([]string{dir})
	if len(got) != 2 {
		t.Fatalf("deadThirdPartyPlists() = %v, want 2 entries", got)
	}
}
