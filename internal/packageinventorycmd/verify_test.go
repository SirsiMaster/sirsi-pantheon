package packageinventorycmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyRejectsDeclaredVersionBuildMismatch(t *testing.T) {
	app, info, pkgInfo, launchAgent, brandLogo := makeCommandBundle(t)
	_, err := Verify(Inputs{App: app, Version: "0.23.9-beta", Build: "wrong", InfoPlist: info, PkgInfo: pkgInfo, LaunchAgent: launchAgent, BrandLogo: brandLogo})
	if err == nil || !strings.Contains(err.Error(), "build") {
		t.Fatalf("mismatched build accepted: %v", err)
	}
}

func TestValidateInfoPlistAcceptsTypedOptionalValues(t *testing.T) {
	info := []byte(`<?xml version="1.0"?><plist><dict>
	<key>CFBundleIdentifier</key>
	<string>ai.sirsi.pantheon</string>
	<key>CFBundleShortVersionString</key>
	<string>0.24.14</string>
	<key>CFBundleVersion</key>
	<string>0.24.14</string>
	<key>LSUIElement</key>
	<true/>
	<key>NSHighResolutionCapable</key>
	<true/>
	</dict></plist>`)
	if err := validateInfoPlist(info, "0.24.14", "0.24.14"); err != nil {
		t.Fatalf("typed optional plist values rejected: %v", err)
	}
}

func TestValidateInfoPlistRejectsDuplicateOptionalKey(t *testing.T) {
	info := []byte(`<?xml version="1.0"?><plist><dict>
	<key>CFBundleIdentifier</key><string>ai.sirsi.pantheon</string>
	<key>CFBundleShortVersionString</key><string>0.24.14</string>
	<key>CFBundleVersion</key><string>0.24.14</string>
	<key>LSUIElement</key><true/>
	<key>LSUIElement</key><false/>
	</dict></plist>`)
	if err := validateInfoPlist(info, "0.24.14", "0.24.14"); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate optional plist key accepted: %v", err)
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

func makeCommandBundle(t *testing.T) (string, string, string, string, string) {
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
	brandLogo := filepath.Join(root, "sirsi-logo-white.png")
	infoBytes := []byte(`<?xml version="1.0"?><plist><dict><key>CFBundleIdentifier</key><string>ai.sirsi.pantheon</string><key>CFBundleShortVersionString</key><string>0.23.9-beta</string><key>CFBundleVersion</key><string>20260908</string></dict></plist>`)
	if err := os.WriteFile(info, infoBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pkgInfo, []byte("APPL????"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launchAgent, []byte("Label=ai.sirsi.pantheon\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(brandLogo, []byte("canonical Sirsi application-mark bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, data := range map[string][]byte{
		"Contents/Info.plist":                        infoBytes,
		"Contents/PkgInfo":                           []byte("APPL????"),
		"Contents/MacOS/sirsi":                       []byte("cli"),
		"Contents/MacOS/sirsi-menubar":               []byte("menu"),
		"Contents/Resources/ai.sirsi.pantheon.plist": []byte("Label=ai.sirsi.pantheon\n"),
		"Contents/Resources/sirsi-logo-white.png":    []byte("canonical Sirsi application-mark bytes\n"),
	} {
		if err := os.WriteFile(filepath.Join(app, filepath.FromSlash(rel)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return app, info, pkgInfo, launchAgent, brandLogo
}
