package packageinventorycmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestVerifyRejectsDeclaredVersionBuildMismatch(t *testing.T) {
	app, info, pkgInfo, launchAgent := makeCommandBundle(t)
	_, err := Verify(Inputs{App: app, Version: "0.23.9-beta", Build: "wrong", InfoPlist: info, PkgInfo: pkgInfo, LaunchAgent: launchAgent})
	if err == nil || !strings.Contains(err.Error(), "build") {
		t.Fatalf("mismatched build accepted: %v", err)
	}
}

func TestVerifyAcceptsCanonicalPayloadReferences(t *testing.T) {
	app, info, pkgInfo, launchAgent := makeCommandBundle(t)
	if _, err := Verify(Inputs{App: app, Version: "0.23.9-beta", Build: "20260908", InfoPlist: info, PkgInfo: pkgInfo, LaunchAgent: launchAgent}); err != nil {
		t.Fatalf("canonical payload references rejected: %v", err)
	}
}

func TestVerifyRejectsSelfConsistentNoncanonicalPayloadReferences(t *testing.T) {
	t.Run("PkgInfo", func(t *testing.T) {
		app, info, pkgInfo, launchAgent := makeCommandBundle(t)
		wrong := []byte("APPLNOPE\n")
		if err := os.WriteFile(pkgInfo, wrong, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(app, "Contents", "PkgInfo"), wrong, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(Inputs{App: app, Version: "0.23.9-beta", Build: "20260908", InfoPlist: info, PkgInfo: pkgInfo, LaunchAgent: launchAgent}); err == nil || !strings.Contains(err.Error(), "canonical Pantheon APPL signature") {
			t.Fatalf("self-consistent noncanonical PkgInfo was accepted: %v", err)
		}
	})
	t.Run("LaunchAgent engine path", func(t *testing.T) {
		app, info, pkgInfo, launchAgent := makeCommandBundle(t)
		original, err := os.ReadFile(launchAgent)
		if err != nil {
			t.Fatal(err)
		}
		wrong := []byte(strings.Replace(string(original), "/Applications/Pantheon.app/Contents/MacOS/sirsi-menubar", "/tmp/unrelated-engine", 1))
		if string(wrong) == string(original) {
			t.Fatal("LaunchAgent fixture did not contain the canonical executable path")
		}
		if err := os.WriteFile(launchAgent, wrong, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(app, "Contents", "Resources", "ai.sirsi.pantheon.plist"), wrong, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(Inputs{App: app, Version: "0.23.9-beta", Build: "20260908", InfoPlist: info, PkgInfo: pkgInfo, LaunchAgent: launchAgent}); err == nil || !strings.Contains(err.Error(), "do not target the canonical menu-bar executable") {
			t.Fatalf("self-consistent noncanonical LaunchAgent was accepted: %v", err)
		}
	})
}

func TestValidateLaunchAgentRejectsMalformedAndConflictingRecords(t *testing.T) {
	_, _, _, launchAgent := makeCommandBundle(t)
	valid, err := os.ReadFile(launchAgent)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(string) string{
		"duplicate key": func(value string) string {
			return strings.Replace(value, "</dict>", "<key>Label</key><string>other.service</string></dict>", 1)
		},
		"unknown key": func(value string) string {
			return strings.Replace(value, "</dict>", "<key>EnvironmentVariables</key><dict/></dict>", 1)
		},
		"false RunAtLoad": func(value string) string {
			return strings.Replace(value, "<key>RunAtLoad</key><true/>", "<key>RunAtLoad</key><false/>", 1)
		},
		"attributed key": func(value string) string {
			return strings.Replace(value, "<key>Label</key>", `<key unexpected="1">Label</key>`, 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := []byte(mutate(string(valid)))
			if string(candidate) == string(valid) {
				t.Fatal("fixture mutation did not change the LaunchAgent")
			}
			if err := validateLaunchAgent(candidate); err == nil {
				t.Fatal("malformed or conflicting LaunchAgent record was accepted")
			}
		})
	}
}

func TestReadCanonicalFileRejectsSymlinkedAncestor(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realFile := filepath.Join(realDir, "Info.plist")
	if err := os.WriteFile(realFile, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCanonicalFile(filepath.Join(link, "Info.plist")); err == nil {
		t.Fatal("symlinked ancestor was accepted")
	}
}

func TestCanonicalFileRevalidationRejectsLeafReplacement(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "Info.plist")
	if err := os.WriteFile(path, []byte("captured bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := captureCanonicalFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.close()
	backup := filepath.Join(root, "Info.plist.original")
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(backup) })
	if err := os.WriteFile(path, []byte("captured bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := file.revalidate(); err == nil || !strings.Contains(err.Error(), "name identity changed") {
		t.Fatalf("same-content leaf replacement was accepted: %v", err)
	}
}

func TestCanonicalFileOpenRejectsSymlinkToMovedOriginal(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "PkgInfo")
	backup := filepath.Join(root, "PkgInfo.original")
	if err := os.WriteFile(path, []byte("APPL????"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = captureCanonicalFileWithHook(path, func() {
		if err := os.Rename(path, backup); err != nil {
			t.Fatalf("move original input: %v", err)
		}
		if err := os.Symlink(backup, path); err != nil {
			t.Fatalf("install symlink at original input name: %v", err)
		}
	})
	if err == nil {
		t.Fatal("symlink to moved original was accepted")
	}
}

func TestCanonicalFileRevalidationRejectsParentReplacement(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(root, "real")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "Info.plist")
	if err := os.WriteFile(path, []byte("captured bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := captureCanonicalFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.close()
	backup := filepath.Join(root, "real.original")
	if err := os.Rename(parent, backup); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(backup) })
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("captured bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := file.revalidate(); err == nil || !strings.Contains(err.Error(), "parent path continuity failed") {
		t.Fatalf("replacement parent was accepted: %v", err)
	}
}

func TestCanonicalFileRevalidationAllowsSiblingDirectoryChange(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(root, "inputs")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "Info.plist")
	if err := os.WriteFile(path, []byte("captured bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	var before unix.Stat_t
	if err := unix.Lstat(parent, &before); err != nil {
		t.Fatal(err)
	}
	file, err := captureCanonicalFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.close()
	if err := os.Mkdir(filepath.Join(parent, "unrelated"), 0o755); err != nil {
		t.Fatal(err)
	}
	var after unix.Stat_t
	if err := unix.Lstat(parent, &after); err != nil {
		t.Fatal(err)
	}
	if before.Dev != after.Dev || before.Ino != after.Ino {
		t.Fatalf("test changed parent identity: before=%+v after=%+v", before, after)
	}
	if before.Nlink == after.Nlink && before.Size == after.Size {
		t.Skip("filesystem did not expose sibling addition in directory metadata")
	}
	if err := file.revalidate(); err != nil {
		t.Fatalf("unrelated sibling directory metadata change rejected stable input: %v", err)
	}
}

func TestCanonicalFileRevalidationRejectsSameInodeContentChange(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "Info.plist")
	if err := os.WriteFile(path, []byte("original bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := captureCanonicalFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.close()
	if err := os.WriteFile(path, []byte("changed  bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := file.revalidate(); err == nil || !strings.Contains(err.Error(), "bytes changed") {
		t.Fatalf("same-inode content mutation was accepted: %v", err)
	}
}

func TestValidateInfoPlistRejectsWrapperAndTrailingContent(t *testing.T) {
	valid := `<plist><dict><key>CFBundleIdentifier</key><string>ai.sirsi.pantheon</string><key>CFBundleShortVersionString</key><string>0.23.9-beta</string><key>CFBundleVersion</key><string>20260908</string></dict></plist>`
	for name, value := range map[string]string{
		"missing plist wrapper": `<dict><key>CFBundleIdentifier</key><string>ai.sirsi.pantheon</string></dict>`,
		"trailing XML":          valid + `<extra/>`,
		"trailing text":         valid + `unexpected`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateInfoPlist([]byte(value), "0.23.9-beta", "20260908"); err == nil {
				t.Fatal("malformed plist wrapper was accepted")
			}
		})
	}
}

func makeCommandBundle(t *testing.T) (string, string, string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "Pantheon.app")
	for _, dir := range []string{"Contents/MacOS", "Contents/Resources"} {
		if err := os.MkdirAll(filepath.Join(app, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	info := filepath.Join(root, "Info.plist")
	pkgInfo := filepath.Join(root, "PkgInfo")
	launchAgent := filepath.Join(root, "LaunchAgent.plist")
	infoBytes := []byte(`<?xml version="1.0"?>
<!-- production-shaped whitespace and non-string values -->
<plist version="1.0">
  <dict>
    <key>CFBundleIdentifier</key>
    <string>ai.sirsi.pantheon</string>
    <key>LSUIElement</key>
    <true/>
    <key>CFBundleShortVersionString</key>
    <string>0.23.9-beta</string>
    <key>CFBundleDocumentTypes</key>
    <array><string>public.data</string></array>
    <key>CFBundleVersion</key>
    <string>20260908</string>
  </dict>
</plist>`)
	if err := os.WriteFile(info, infoBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	pkgInfoBytes := []byte("APPL????\n")
	if err := os.WriteFile(pkgInfo, pkgInfoBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	launchAgentBytes := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>ai.sirsi.pantheon</string>
<key>ProgramArguments</key><array><string>/Applications/Pantheon.app/Contents/MacOS/sirsi-menubar</string></array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
<key>ProcessType</key><string>Interactive</string>
</dict></plist>
`)
	if err := os.WriteFile(launchAgent, launchAgentBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, data := range map[string][]byte{
		"Contents/Info.plist":                        infoBytes,
		"Contents/PkgInfo":                           pkgInfoBytes,
		"Contents/MacOS/sirsi":                       []byte("cli"),
		"Contents/MacOS/sirsi-menubar":               []byte("menu"),
		"Contents/Resources/ai.sirsi.pantheon.plist": launchAgentBytes,
	} {
		mode := os.FileMode(0o644)
		if rel == "Contents/MacOS/sirsi" || rel == "Contents/MacOS/sirsi-menubar" {
			mode = 0o755
		}
		if err := os.WriteFile(filepath.Join(app, filepath.FromSlash(rel)), data, mode); err != nil {
			t.Fatal(err)
		}
	}
	return app, info, pkgInfo, launchAgent
}
