package packageinventorycmd

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type frozenReceiptWriterFixture struct {
	writeCount int
	writeErr   error
	closeErr   error
	contents   []byte
	closed     bool
}

func (f *frozenReceiptWriterFixture) Write(contents []byte) (int, error) {
	f.contents = append([]byte(nil), contents...)
	if f.writeCount < 0 {
		return len(contents), f.writeErr
	}
	return f.writeCount, f.writeErr
}

func (f *frozenReceiptWriterFixture) Close() error {
	f.closed = true
	return f.closeErr
}

func TestWriteFrozenDigestsChecksFullWriteAndCloseFailures(t *testing.T) {
	digests := FrozenDigests{
		InfoPlist:     strings.Repeat("a", 64),
		ApolloProfile: strings.Repeat("b", 64),
	}
	openFailure := errors.New("fixture open failure")
	writeFailure := errors.New("fixture write failure")
	closeFailure := errors.New("fixture close failure")
	for _, tc := range []struct {
		name       string
		writeCount int
		openErr    error
		writeErr   error
		closeErr   error
		wantErr    error
	}{
		{name: "complete write", writeCount: -1},
		{name: "open error", writeCount: -1, openErr: openFailure, wantErr: openFailure},
		{name: "short write", writeCount: 1, wantErr: io.ErrShortWrite},
		{name: "write error", writeCount: 1, writeErr: writeFailure, wantErr: writeFailure},
		{name: "close error", writeCount: -1, closeErr: closeFailure, wantErr: closeFailure},
		{name: "write and close errors", writeCount: 1, writeErr: writeFailure, closeErr: closeFailure, wantErr: writeFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := &frozenReceiptWriterFixture{writeCount: tc.writeCount, writeErr: tc.writeErr, closeErr: tc.closeErr}
			path := filepath.Join(t.TempDir(), "apollo-digests.fixture")
			var openCalled bool
			err := writeFrozenDigestsWith(path, digests,
				func(path string, flags int, mode os.FileMode) (frozenDigestReceiptFile, error) {
					openCalled = true
					if flags&(os.O_CREATE|os.O_EXCL) != (os.O_CREATE|os.O_EXCL) || mode != 0o600 {
						t.Fatalf("receipt open flags=%#x mode=%#o", flags, mode)
					}
					if tc.openErr != nil {
						return nil, tc.openErr
					}
					return file, nil
				})
			if !openCalled || file.closed != (tc.openErr == nil) {
				t.Fatalf("receipt I/O open/close mismatch: open=%t close=%t openErr=%v", openCalled, file.closed, tc.openErr)
			}
			if tc.wantErr == nil {
				if err != nil || string(file.contents) != "info_sha256="+digests.InfoPlist+"\napollo_profile_sha256="+digests.ApolloProfile+"\n" {
					t.Fatalf("complete receipt result err=%v contents=%q", err, file.contents)
				}
				return
			}
			if tc.openErr != nil {
				if err == nil || !strings.Contains(err.Error(), "create-only") || strings.Contains(err.Error(), "retained for reconciliation") || !errors.Is(err, tc.wantErr) {
					t.Fatalf("receipt open failure=%v, want create-only open error without post-create reconciliation", err)
				}
				if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
					t.Fatalf("failed receipt open left an artifact: lstat err=%v", statErr)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "retained for reconciliation") || !errors.Is(err, tc.wantErr) {
				t.Fatalf("receipt failure=%v, want retained reconciliation error wrapping %v", err, tc.wantErr)
			}
		})
	}
}

func TestWriteFrozenDigestsUsesCreateOnlyReceiptPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frozen-digests.txt")
	original := []byte("preexisting receipt")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	err := WriteFrozenDigests(path, FrozenDigests{InfoPlist: strings.Repeat("a", 64), ApolloProfile: ApolloProfileAbsent})
	if err == nil || !strings.Contains(err.Error(), "create-only") {
		t.Fatalf("existing receipt was not refused: %v", err)
	}
	retained, readErr := os.ReadFile(path)
	if readErr != nil || string(retained) != string(original) {
		t.Fatalf("preexisting receipt changed or disappeared: bytes=%q err=%v", retained, readErr)
	}
}

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

func TestReleaseInventoryRequiresFrozenDigests(t *testing.T) {
	app, info, pkgInfo, launchAgent, brandLogo := makeCommandBundle(t)
	_, err := Verify(Inputs{App: app, Version: "0.23.9-beta", Build: "20260908", InfoPlist: info,
		PkgInfo: pkgInfo, LaunchAgent: launchAgent, BrandLogo: brandLogo,
		ApolloCapability: "disabled", RequireCodeSignature: true, RequireFrozenDigests: true})
	if err == nil || !strings.Contains(err.Error(), "release inventory requires frozen Info.plist and Apollo profile digests") {
		t.Fatalf("release inventory accepted without its frozen digest pair: %v", err)
	}
}

func TestSignedDevelopmentInventoryKeepsStructureOnlyContract(t *testing.T) {
	app, info, pkgInfo, launchAgent, brandLogo := makeCommandBundle(t)
	infoBytes := []byte(strings.Replace(string(mustReadFile(t, info)), "</dict>",
		"<key>SirsiApolloNativeCapability</key><string>disabled</string></dict>", 1))
	if err := os.WriteFile(info, infoBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), infoBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(app, "Contents", "_CodeSignature"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "_CodeSignature", "CodeResources"), []byte("fixture signature layout"), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := minimalSignedMachOFixture()
	for _, path := range []string{filepath.Join(app, "Contents", "MacOS", "sirsi"), filepath.Join(app, "Contents", "MacOS", "sirsi-menubar")} {
		if err := os.WriteFile(path, engine, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_, err := Verify(Inputs{App: app, Version: "0.23.9-beta", Build: "20260908", InfoPlist: info,
		PkgInfo: pkgInfo, LaunchAgent: launchAgent, BrandLogo: brandLogo,
		ApolloCapability: "disabled", RequireCodeSignature: true})
	if err != nil {
		t.Fatalf("signed development inventory without release digest flags failed: %v", err)
	}
}

func minimalSignedMachOFixture() []byte {
	const headerSize = 32
	const dylibHeaderSize = 24
	const loadDylib = 0xc
	name := "/usr/lib/libSystem.B.dylib"
	commandSize := dylibHeaderSize + len(name) + 1
	for commandSize%8 != 0 {
		commandSize++
	}
	data := make([]byte, headerSize+commandSize)
	binary.LittleEndian.PutUint32(data[0:], 0xfeedfacf)
	binary.LittleEndian.PutUint32(data[4:], 0x0100000c)
	binary.LittleEndian.PutUint32(data[12:], 2)
	binary.LittleEndian.PutUint32(data[16:], 1)
	binary.LittleEndian.PutUint32(data[20:], uint32(commandSize))
	binary.LittleEndian.PutUint32(data[headerSize:], loadDylib)
	binary.LittleEndian.PutUint32(data[headerSize+4:], uint32(commandSize))
	binary.LittleEndian.PutUint32(data[headerSize+8:], dylibHeaderSize)
	copy(data[headerSize+dylibHeaderSize:], name)
	return data
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
	profileRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	profileTarget := filepath.Join(profileRoot, "profile.json")
	if err := os.WriteFile(profileTarget, []byte(`{"fixture":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	profileLink := filepath.Join(profileRoot, "profile-link.json")
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

func TestApolloProfileDescriptorProtection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mode  os.FileMode
		owner uint32
		ok    bool
	}{
		{name: "private owner readable", mode: 0o600, owner: uint32(os.Getuid()), ok: true},
		{name: "root owned readable", mode: 0o640, owner: 0, ok: true},
		{name: "group writable", mode: 0o620, owner: uint32(os.Getuid())},
		{name: "other writable", mode: 0o602, owner: uint32(os.Getuid())},
		{name: "world writable", mode: 0o666, owner: uint32(os.Getuid())},
		{name: "foreign owner", mode: 0o600, owner: uint32(os.Getuid()) + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := apolloProfileDescriptorAllowed(tc.owner, uint32(tc.mode.Perm()), uint32(os.Getuid()))
			if got != tc.ok {
				t.Fatalf("profile descriptor policy=%t, want %t", got, tc.ok)
			}
		})
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "profile.json")
	if err := os.WriteFile(path, []byte(`{"fixture":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadProtectedApolloProfile(path); err != nil {
		t.Fatalf("private profile rejected: %v", err)
	}
	for _, mode := range []os.FileMode{0o666, 0o620, 0o602} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadProtectedApolloProfile(path); err == nil || !strings.Contains(err.Error(), "owner or write permissions") {
			t.Fatalf("unsafe mode %04o accepted or unrelated refusal: %v", mode, err)
		}
		if mode == 0o666 {
			if _, err := ReadCanonicalFile(path); err != nil {
				t.Fatalf("generic canonical reader changed its permissive mode contract: %v", err)
			}
		}
	}
}

func TestFrozenApolloInputDigestsRejectReferenceAndCandidateSubstitution(t *testing.T) {
	initialInfo := []byte("initial Info.plist")
	initialProfile := []byte("initial protected profile")
	infoDigest := fmt.Sprintf("%x", sha256.Sum256(initialInfo))
	profileDigest := fmt.Sprintf("%x", sha256.Sum256(initialProfile))
	if err := verifyFrozenDigest("Info.plist", []byte("substituted Info.plist"), infoDigest); err == nil {
		t.Fatal("changed Info.plist reference and candidate passed frozen initial digest")
	}
	if err := verifyFrozenProfileDigest([]byte("substituted profile"), true, profileDigest); err == nil {
		t.Fatal("changed profile reference and candidate passed frozen initial digest")
	}
	if err := verifyFrozenDigest("Info.plist", initialInfo, infoDigest); err != nil {
		t.Fatalf("unchanged initial Info.plist rejected: %v", err)
	}
	if err := verifyFrozenProfileDigest(initialProfile, true, profileDigest); err != nil {
		t.Fatalf("unchanged initial profile rejected: %v", err)
	}
	if err := verifyFrozenProfileDigest(nil, false, ApolloProfileAbsent); err != nil {
		t.Fatalf("frozen disabled-profile absence rejected: %v", err)
	}
}

func TestVerifyFreezesInitialInfoDigestBeforeMatchingCandidateCanChange(t *testing.T) {
	app, info, pkgInfo, launchAgent, brandLogo := makeCommandBundle(t)
	makeCommandBundleEnginesExecutable(t, app)
	inputs := Inputs{App: app, Version: "0.23.9-beta", Build: "20260908", InfoPlist: info,
		PkgInfo: pkgInfo, LaunchAgent: launchAgent, BrandLogo: brandLogo, ApolloCapability: "legacy-development"}
	_, frozen, err := VerifyWithFrozenDigests(inputs)
	if err != nil {
		t.Fatalf("initial inventory failed: %v", err)
	}
	if frozen.InfoPlist != fmt.Sprintf("%x", sha256.Sum256(mustReadFile(t, info))) || frozen.ApolloProfile != ApolloProfileAbsent {
		t.Fatalf("initial digest pair=%+v, want exact Info.plist digest and absent profile", frozen)
	}

	changed, err := os.ReadFile(info)
	if err != nil {
		t.Fatal(err)
	}
	changed = append(changed, '\n')
	if err := os.WriteFile(info, changed, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), changed, 0o644); err != nil {
		t.Fatal(err)
	}
	inputs.ExpectedInfoSHA256 = frozen.InfoPlist
	inputs.ExpectedProfileSHA256 = frozen.ApolloProfile
	if _, err := Verify(inputs); err == nil || !strings.Contains(err.Error(), "frozen pre-sign digest") {
		t.Fatalf("matching changed reference and candidate bypassed initial digest: %v", err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestVerifyFreezesInitialProfileDigestBeforeMatchingCandidateCanChange(t *testing.T) {
	app, info, pkgInfo, launchAgent, brandLogo := makeCommandBundle(t)
	makeCommandBundleEnginesExecutable(t, app)
	infoBytes := mustReadFile(t, info)
	infoBytes = []byte(strings.Replace(string(infoBytes), "</dict>", "<key>SirsiApolloNativeCapability</key><string>enabled</string></dict>", 1))
	if err := os.WriteFile(info, infoBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), infoBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	profileRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(profileRoot, "native-profile.json")
	profileBytes := []byte(`{"fixture":"initial"}`)
	if err := os.WriteFile(profilePath, profileBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	profileResource := filepath.Join(app, "Contents", "Resources", "Apollo", "native-profile.json")
	if err := os.MkdirAll(filepath.Dir(profileResource), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profileResource, profileBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	inputs := Inputs{App: app, Version: "0.23.9-beta", Build: "20260908", InfoPlist: info,
		PkgInfo: pkgInfo, LaunchAgent: launchAgent, BrandLogo: brandLogo,
		ApolloCapability: "enabled", ApolloProfile: profilePath}
	_, frozen, err := VerifyWithFrozenDigests(inputs)
	if err != nil {
		t.Fatalf("initial inventory failed: %v", err)
	}
	if frozen.ApolloProfile != fmt.Sprintf("%x", sha256.Sum256(profileBytes)) {
		t.Fatalf("initial profile digest=%q", frozen.ApolloProfile)
	}
	changed := []byte(`{"fixture":"changed"}`)
	if err := os.WriteFile(profilePath, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profileResource, changed, 0o644); err != nil {
		t.Fatal(err)
	}
	inputs.ExpectedInfoSHA256 = frozen.InfoPlist
	inputs.ExpectedProfileSHA256 = frozen.ApolloProfile
	if _, err := Verify(inputs); err == nil || !strings.Contains(err.Error(), "frozen pre-sign digest") {
		t.Fatalf("matching changed profile reference and candidate bypassed initial digest: %v", err)
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

func makeCommandBundleEnginesExecutable(t *testing.T, app string) {
	t.Helper()
	for _, name := range []string{"sirsi", "sirsi-menubar"} {
		path := filepath.Join(app, "Contents", "MacOS", name)
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}
