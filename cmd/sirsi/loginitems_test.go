package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func validPlist(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
` + body + `
</plist>
`
}

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
		if err := os.WriteFile(filepath.Join(dir, name), []byte(validPlist(content)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("com.google.keystone.agent.plist", `<dict/>`)                                                                                                   // empty: dead
	write("com.google.GoogleUpdater.wake.plist", `<dict><key>ProgramArguments</key><array><string>/does/not/exist/GoogleUpdater</string></array></dict>`) // missing binary: dead
	write("com.other.live.plist", `<dict><key>Program</key><string>/bin/echo</string></dict>`)                                                            // existing binary: alive
	write("ai.sirsi.pantheon.plist", `<dict/>`)                                                                                                           // ours: ignored even though empty

	got := deadThirdPartyPlists([]string{dir})
	if len(got) != 2 {
		t.Fatalf("deadThirdPartyPlists() = %v, want 2 entries", got)
	}
}

// TestDeadThirdPartyPlistsHandlesBinaryFormat is the regression test for the
// P2 codex found in review: a binary-encoded plist was reported "(empty)"
// because the old parser only matched the literal "<dict>" XML substring.
func TestDeadThirdPartyPlistsHandlesBinaryFormat(t *testing.T) {
	if _, err := exec.LookPath("plutil"); err != nil {
		t.Skip("plutil not available")
	}
	dir := t.TempDir()
	live := filepath.Join(dir, "com.review.live.plist")
	if err := os.WriteFile(live, []byte(validPlist(`<dict><key>Program</key><string>/bin/echo</string></dict>`)), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("plutil", "-convert", "binary1", "-o", live, live).CombinedOutput(); err != nil {
		t.Fatalf("plutil convert to binary1 failed: %v: %s", err, out)
	}

	dead := filepath.Join(dir, "com.review.dead.plist")
	if err := os.WriteFile(dead, []byte(validPlist(`<dict><key>ProgramArguments</key><array><string>/does/not/exist/binary</string></array></dict>`)), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("plutil", "-convert", "binary1", "-o", dead, dead).CombinedOutput(); err != nil {
		t.Fatalf("plutil convert to binary1 failed: %v: %s", err, out)
	}

	got := deadThirdPartyPlists([]string{dir})
	if len(got) != 1 || got[0] != "com.review.dead.plist (missing binary: /does/not/exist/binary)" {
		t.Fatalf("deadThirdPartyPlists() = %v, want only com.review.dead.plist flagged (com.review.live.plist must not read as empty)", got)
	}
}

// TestDeadThirdPartyPlistsSkipsMalformedPlist asserts a plist that fails to
// parse (truncated/corrupt) is treated as unknown, never as "empty" — the
// same bug class as the binary case, triggered by XML that isn't well-formed.
func TestDeadThirdPartyPlistsSkipsMalformedPlist(t *testing.T) {
	if _, err := exec.LookPath("plutil"); err != nil {
		t.Skip("plutil not available")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "com.review.corrupt.plist"), []byte("not a plist at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := deadThirdPartyPlists([]string{dir})
	if len(got) != 0 {
		t.Fatalf("deadThirdPartyPlists() = %v, want malformed plist skipped as unknown, not flagged dead", got)
	}
}

// TestDeadThirdPartyPlistsUnreadableBinaryIsUnknown asserts that a Stat
// failure other than not-exist (e.g. permission denied) on the referenced
// executable is treated as unknown, not as proof the binary is missing.
func TestDeadThirdPartyPlistsUnreadableBinaryIsUnknown(t *testing.T) {
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "blocked-binary")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Deny traversal into binDir so os.Stat(binPath) fails with permission
	// denied rather than not-exist.
	if err := os.Chmod(binDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(binDir, 0o755) })

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "com.review.blocked.plist"), []byte(validPlist(`<dict><key>Program</key><string>`+binPath+`</string></dict>`)), 0o644); err != nil {
		t.Fatal(err)
	}

	got := deadThirdPartyPlists([]string{dir})
	if len(got) != 0 {
		t.Fatalf("deadThirdPartyPlists() = %v, want permission-denied Stat treated as unknown, not dead", got)
	}
}
