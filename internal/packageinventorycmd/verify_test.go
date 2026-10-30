package packageinventorycmd

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashCanonicalFileUsesStableNoFollowDescriptor(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "bundle", "Contents", "MacOS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sirsi")
	content := []byte("synthetic engine bytes")
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%x", sha256.Sum256(content))
	got, err := HashCanonicalFile(path)
	if err != nil || got != want {
		t.Fatalf("stable descriptor hash=%q err=%v, want %q", got, err, want)
	}
	linked := filepath.Join(dir, "sirsi-hardlink")
	if err := os.Link(path, linked); err != nil {
		t.Fatal(err)
	}
	if _, err := HashCanonicalFile(path); err == nil || !strings.Contains(err.Error(), "singly-linked") {
		t.Fatalf("hard-linked engine accepted: %v", err)
	}
	symlinkedParent := filepath.Join(root, "Contents")
	if err := os.Symlink(filepath.Join(root, "bundle", "Contents"), symlinkedParent); err != nil {
		t.Fatal(err)
	}
	if _, err := HashCanonicalFile(filepath.Join(symlinkedParent, "MacOS", "sirsi")); err == nil {
		t.Fatal("symlinked engine ancestor accepted")
	}
}

func TestVerifyRejectsDeclaredVersionBuildMismatch(t *testing.T) {
	app, info, pkgInfo, launchAgent, brandLogo := makeCommandBundle(t)
	_, err := Verify(Inputs{App: app, Version: "0.23.9-beta", Build: "wrong", InfoPlist: info, PkgInfo: pkgInfo, LaunchAgent: launchAgent, BrandLogo: brandLogo, ApolloCapability: "legacy-development"})
	if err == nil || !strings.Contains(err.Error(), "build") {
		t.Fatalf("mismatched build accepted: %v", err)
	}
}

func TestVerifyUsesIndependentExpectedCapabilityNotBundleSelfDeclaration(t *testing.T) {
	app, info, pkgInfo, launchAgent, brandLogo := makeCommandBundle(t)
	for _, capability := range []string{"enabled", "disabled"} {
		_, err := Verify(Inputs{App: app, Version: "0.23.9-beta", Build: "20260908", InfoPlist: info,
			PkgInfo: pkgInfo, LaunchAgent: launchAgent, BrandLogo: brandLogo, ApolloCapability: capability})
		if err == nil || !strings.Contains(err.Error(), "Info.plist Apollo capability/profile mismatch") {
			t.Fatalf("expected capability %q trusted the bundle's self-supplied plist bytes: %v", capability, err)
		}
	}
}

func TestVerifyRejectsUnsafeApolloProfileInput(t *testing.T) {
	app, info, pkgInfo, launchAgent, brandLogo := makeCommandBundle(t)
	profileTarget := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(profileTarget, []byte(`{"fixture":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	profileLink := filepath.Join(t.TempDir(), "profile-link.json")
	if err := os.Symlink(profileTarget, profileLink); err != nil {
		t.Fatal(err)
	}
	_, err := Verify(Inputs{App: app, Version: "0.23.9-beta", Build: "20260908", InfoPlist: info,
		PkgInfo: pkgInfo, LaunchAgent: launchAgent, BrandLogo: brandLogo,
		ApolloCapability: "enabled", ApolloProfile: profileLink})
	if err == nil || !strings.Contains(err.Error(), "protected Apollo profile") {
		t.Fatalf("symlinked Apollo profile accepted or unrelated refusal: %v", err)
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
	if err := validateInfoPlist(info, "0.24.14", "0.24.14", "legacy-development", false); err != nil {
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
	if err := validateInfoPlist(info, "0.24.14", "0.24.14", "legacy-development", false); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate optional plist key accepted: %v", err)
	}
}

func TestApolloCapabilityPlistReleaseStatesAreClosed(t *testing.T) {
	base := `<?xml version="1.0"?><plist><dict><key>CFBundleIdentifier</key><string>ai.sirsi.pantheon</string><key>CFBundleShortVersionString</key><string>0.24.147</string><key>CFBundleVersion</key><string>0.24.147</string>%s</dict></plist>`
	for _, tc := range []struct {
		name, capability, declaration string
		profile                       bool
		wantErr                       string
	}{
		{"enabled complete", "enabled", `<key>SirsiApolloNativeCapability</key><string>enabled</string>`, true, ""},
		{"disabled complete", "disabled", `<key>SirsiApolloNativeCapability</key><string>disabled</string>`, false, ""},
		{"enabled missing key", "enabled", "", true, "mismatch"},
		{"disabled missing key", "disabled", "", false, "mismatch"},
		{"enabled missing profile", "enabled", `<key>SirsiApolloNativeCapability</key><string>enabled</string>`, false, "mismatch"},
		{"disabled conflicting profile", "disabled", `<key>SirsiApolloNativeCapability</key><string>disabled</string>`, true, "mismatch"},
		{"unknown declaration", "enabled", `<key>SirsiApolloNativeCapability</key><string>automatic</string>`, true, "mismatch"},
		{"missing expected mode", "", `<key>SirsiApolloNativeCapability</key><string>disabled</string>`, false, "expected Apollo capability"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := []byte(fmt.Sprintf(base, tc.declaration))
			err := validateInfoPlist(info, "0.24.147", "0.24.147", tc.capability, tc.profile)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("valid state rejected: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("invalid state accepted or unrelated refusal: %v", err)
			}
		})
	}
}

func TestApolloCapabilityRuntimeParserDoesNotInferMissingAsDisabled(t *testing.T) {
	info := []byte(`<?xml version="1.0"?><plist><dict><key>CFBundleIdentifier</key><string>ai.sirsi.pantheon</string></dict></plist>`)
	if _, err := ApolloCapabilityFromInfoPlist(info); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing capability was treated as disabled: %v", err)
	}
	info = []byte(`<?xml version="1.0"?><plist><dict><key>SirsiApolloNativeCapability</key><string>disabled</string><key>SirsiApolloNativeCapability</key><string>enabled</string></dict></plist>`)
	if _, err := ApolloCapabilityFromInfoPlist(info); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate capability accepted: %v", err)
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
