package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteMenubarAppBundle(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "sirsi-menubar-build")
	if err := os.WriteFile(src, []byte("BINARY-CONTENT"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(dir, "Sirsi Menubar.app")

	execPath, err := writeMenubarAppBundle(bundle, src)
	if err != nil {
		t.Fatalf("writeMenubarAppBundle: %v", err)
	}

	wantExec := filepath.Join(bundle, "Contents", "MacOS", "sirsi-menubar")
	if execPath != wantExec {
		t.Errorf("exec path = %q, want %q", execPath, wantExec)
	}
	got, err := os.ReadFile(execPath)
	if err != nil || string(got) != "BINARY-CONTENT" {
		t.Errorf("bundled binary content = %q (err %v)", got, err)
	}
	info, err := os.Stat(execPath)
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("bundled binary should be executable, mode=%v err=%v", info.Mode(), err)
	}

	plist, err := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<key>CFBundleIdentifier</key>",
		menubarTCCIdentifier, // ai.sirsi.pantheon — the stable TCC key
		"<key>CFBundleExecutable</key>",
		"sirsi-menubar",
		"<key>LSUIElement</key>", // agent app, no Dock icon
	} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("Info.plist missing %q", want)
		}
	}
}

func TestWriteMenubarAppBundle_ReinstallReplacesCleanly(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "sirsi-menubar-build")
	bundle := filepath.Join(dir, "Sirsi Menubar.app")

	// First install.
	if err := os.WriteFile(src, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	execPath, err := writeMenubarAppBundle(bundle, src)
	if err != nil {
		t.Fatal(err)
	}

	// Reinstall with new content. The exec is os.Remove'd before the write so it
	// lands on a fresh inode (the AMFI stale-cdhash class fix) — assert the
	// reinstall path replaces cleanly with the new content, no error.
	if err := os.WriteFile(src, []byte("v2-newer-and-longer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := writeMenubarAppBundle(bundle, src); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	got, _ := os.ReadFile(execPath)
	if string(got) != "v2-newer-and-longer" {
		t.Errorf("reinstall content = %q, want v2-newer-and-longer", got)
	}
}

func TestMenubarAppInfoPlist_StableIdentity(t *testing.T) {
	// The whole point: the bundle id is the stable ai.sirsi.pantheon so TCC
	// treats reinstalls as the same app (matches the Pantheon.app + LaunchAgent
	// label).
	plist := menubarAppInfoPlist()
	if !strings.Contains(plist, "<string>ai.sirsi.pantheon</string>") {
		t.Error("Info.plist must carry the stable CFBundleIdentifier ai.sirsi.pantheon")
	}
	if menubarTCCIdentifier != menubarPlistLabel {
		t.Errorf("bundle id %q should match LaunchAgent label %q for one TCC identity",
			menubarTCCIdentifier, menubarPlistLabel)
	}
}
