package packageinventory

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestVerifyBuildsDeterministicPythonFreeInventory(t *testing.T) {
	app, expected := makeBundle(t)
	report, err := Verify(app, expected)
	if err != nil {
		t.Fatal(err)
	}
	if report.Schema != Schema || !report.PythonFree || report.EngineCount != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.RootMode&0o7777 != 0o755 {
		t.Fatalf("public root mode omitted: %04o", report.RootMode)
	}
	if len(report.Entries) != 26 {
		t.Fatalf("entry count = %d, want unsigned payload including Stack Lab contracts", len(report.Entries))
	}
	mercuryRecipe := "Contents/Resources/StackLab/mercury-consumer-recipe-v1.json"
	foundMercuryRecipe := false
	trashRecipe := "Contents/Resources/StackLab/anubis-trash-receipt-v1.json"
	foundTrashRecipe := false
	modelRouterRecipe := "Contents/Resources/StackLab/model-router-v1-recipe-v1.json"
	foundModelRouterRecipe := false
	for i := 1; i < len(report.Entries); i++ {
		if report.Entries[i-1].Path >= report.Entries[i].Path {
			t.Fatalf("entries are not strictly sorted: %+v", report.Entries)
		}
	}
	for _, entry := range report.Entries {
		if entry.Path == modelRouterRecipe {
			foundModelRouterRecipe = true
			want := []byte(`{"schema":"sirsi.stacklab.recipe.v1","id":"stacklab.recipe.model-router-v1"}`)
			if entry.Type != "regular" || entry.Size != int64(len(want)) || entry.Nlink != 1 || entry.SHA256 != fmt.Sprintf("%x", sha256.Sum256(want)) {
				t.Fatalf("Model-router recipe inventory entry = %+v, want exact nonempty regular fixture bytes", entry)
			}
		}
		if entry.Path == trashRecipe {
			foundTrashRecipe = true
			if entry.Type != "regular" || entry.Size == 0 {
				t.Fatalf("Trash recipe inventory entry = %+v, want nonempty regular file", entry)
			}
		}
		if entry.Path == mercuryRecipe {
			foundMercuryRecipe = true
			if entry.Type != "regular" || entry.Size == 0 {
				t.Fatalf("Mercury recipe inventory entry = %+v, want nonempty regular file", entry)
			}
		}
	}
	if !foundMercuryRecipe {
		t.Fatalf("inventory omitted %q", mercuryRecipe)
	}
	if !foundTrashRecipe {
		t.Fatalf("inventory omitted %q", trashRecipe)
	}
	if !foundModelRouterRecipe {
		t.Fatalf("inventory omitted %q", modelRouterRecipe)
	}
}

func TestVerifyModelRouterRecipeRemainsClosedRegularPayload(t *testing.T) {
	const rel = "Contents/Resources/StackLab/model-router-v1-recipe-v1.json"
	for _, tc := range []struct {
		name   string
		mutate func(string) error
		want   string
	}{
		{"missing", os.Remove, "missing required entry"},
		{"directory", func(path string) error {
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Mkdir(path, 0o755)
		}, "type mismatch"},
		{"symlink", func(path string) error {
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Symlink("mercury-consumer-recipe-v1.json", path)
		}, "symlink rejected"},
		{"unexpected-sibling", func(path string) error {
			return os.WriteFile(filepath.Join(filepath.Dir(path), "model-router-v2-recipe-v1.json"), []byte(`{}`), 0o644)
		}, "unexpected entry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, expected := makeBundle(t)
			if err := tc.mutate(filepath.Join(app, filepath.FromSlash(rel))); err != nil {
				t.Fatal(err)
			}
			_, err := Verify(app, expected)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "Contents/Resources/StackLab/model-router-v") {
				t.Fatalf("%s model-router payload accepted or unrelated refusal: %v", tc.name, err)
			}
		})
	}
}

func TestApolloProfileRequiresExactExplicitCanonicalBytes(t *testing.T) {
	app, expected := makeBundle(t)
	dir := filepath.Join(app, "Contents/Resources/Apollo")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	profile := []byte(`{"schema":"sirsi.apollo.native-profile.v2"}`)
	file := filepath.Join(dir, "native-profile.json")
	if err := os.WriteFile(file, profile, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(app, expected); err == nil {
		t.Fatal("unbound Apollo profile accepted")
	}
	expected.ApolloProfile = profile
	if _, err := Verify(app, expected); err != nil {
		t.Fatalf("explicit exact profile rejected: %v", err)
	}
	if err := os.WriteFile(file, append(profile, ' '), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(app, expected); err == nil {
		t.Fatal("changed Apollo profile bytes accepted")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(app, expected); err == nil {
		t.Fatal("missing required Apollo profile accepted")
	}
}

func TestVerifyRejectsSymlinkedPayload(t *testing.T) {
	app, expected := makeBundle(t)
	sirsi := filepath.Join(app, "Contents", "MacOS", "sirsi")
	if err := os.Remove(sirsi); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../Resources/ai.sirsi.pantheon.plist", sirsi); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(app, expected); err == nil || !strings.Contains(err.Error(), "symlink rejected") {
		t.Fatalf("symlink payload was accepted: %v", err)
	}
}

func TestVerifyDoesNotTreatTextAsPythonLinkage(t *testing.T) {
	app, expected := makeBundle(t)
	info := filepath.Join(app, "Contents", "Info.plist")
	if err := os.WriteFile(info, []byte("libpython3.13.dylib"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected.InfoPlist = []byte("libpython3.13.dylib")
	if _, err := Verify(app, expected); err != nil {
		t.Fatalf("ordinary diagnostic text was treated as Python linkage: %v", err)
	}
}

func TestReadOnceRejectsPayloadOverBoundBeforeAllocation(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "oversized-payload")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := file.Truncate(maxPayloadFileSize + 1); err != nil {
		t.Fatal(err)
	}
	if _, err := readOnce(int(file.Fd()), maxPayloadFileSize+1); err == nil || !strings.Contains(err.Error(), "invalid file size") {
		t.Fatalf("oversized payload was accepted: %v", err)
	}
}

func TestVerifyRejectsPythonMachOLinkageInSignedPayload(t *testing.T) {
	app, expected := makeBundle(t)
	if err := os.Mkdir(filepath.Join(app, "Contents", "_CodeSignature"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "_CodeSignature", "CodeResources"), []byte("signature"), 0o644); err != nil {
		t.Fatal(err)
	}
	normalizeSignatureFixture(t, app)
	expected.RequireCodeSignature = true
	if err := os.WriteFile(filepath.Join(app, "Contents", "MacOS", "sirsi"), machoWithDylib("/usr/lib/libpython3.13.dylib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(app, expected); err == nil || !strings.Contains(err.Error(), "Python linkage") {
		t.Fatalf("Python-linked Mach-O payload was accepted: %v", err)
	}
}

func TestVerifyAcceptsContentsCodeResourcesSignatureLayout(t *testing.T) {
	app, expected := makeBundle(t)
	if err := os.WriteFile(filepath.Join(app, "Contents", "CodeResources"), []byte("signature"), 0o644); err != nil {
		t.Fatal(err)
	}
	normalizeSignatureFixture(t, app)
	if _, err := Verify(app, expected); err != nil {
		t.Fatalf("Contents/CodeResources layout was rejected: %v", err)
	}
}

func TestVerifyAcceptsBothCodeResourcesSignatureLayouts(t *testing.T) {
	app, expected := makeBundle(t)
	if err := os.Mkdir(filepath.Join(app, "Contents", "_CodeSignature"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(app, "Contents", "_CodeSignature", "CodeResources"),
		filepath.Join(app, "Contents", "CodeResources"),
	} {
		if err := os.WriteFile(path, []byte("signature"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	normalizeSignatureFixture(t, app)
	if _, err := Verify(app, expected); err != nil {
		t.Fatalf("dual CodeResources layout was rejected: %v", err)
	}
}

func normalizeSignatureFixture(t *testing.T, app string) {
	t.Helper()
	for _, relative := range []string{"Contents/_CodeSignature", "Contents/_CodeSignature/CodeResources", "Contents/CodeResources"} {
		path := filepath.Join(app, relative)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if info.IsDir() {
			mode = 0o755
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
}

func machoWithDylib(name string) []byte {
	const headerSize = 32
	const dylibHeaderSize = 24
	const loadDylib = 0xc
	cmdSize := dylibHeaderSize + len(name) + 1
	for cmdSize%8 != 0 {
		cmdSize++
	}
	data := make([]byte, headerSize+cmdSize)
	binary.LittleEndian.PutUint32(data[0:], 0xfeedfacf) // MH_MAGIC_64
	binary.LittleEndian.PutUint32(data[4:], 0x0100000c) // CPU_TYPE_ARM64
	binary.LittleEndian.PutUint32(data[12:], 2)         // MH_EXECUTE
	binary.LittleEndian.PutUint32(data[16:], 1)         // ncmds
	binary.LittleEndian.PutUint32(data[20:], uint32(cmdSize))
	binary.LittleEndian.PutUint32(data[headerSize:], loadDylib)
	binary.LittleEndian.PutUint32(data[headerSize+4:], uint32(cmdSize))
	binary.LittleEndian.PutUint32(data[headerSize+8:], dylibHeaderSize)
	copy(data[headerSize+dylibHeaderSize:], name)
	return data
}

func TestVerifyRejectsUnexpectedStackLabPayload(t *testing.T) {
	app, expected := makeBundle(t)
	path := filepath.Join(app, "Contents", "Resources", "StackLab", "unreviewed.py")
	if err := os.WriteFile(path, []byte("print('not a product payload')"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(app, expected); err == nil || !strings.Contains(err.Error(), "unexpected entry") {
		t.Fatalf("unexpected Stack Lab payload was accepted: %v", err)
	}
}

func TestFinalNamespaceRescanRejectsLateAllowedEntry(t *testing.T) {
	app, expected := makeBundle(t)
	if err := os.Mkdir(filepath.Join(app, "Contents", "_CodeSignature"), 0o755); err != nil {
		t.Fatal(err)
	}
	rootFD, openErr := unix.Open(app, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer unix.Close(rootFD)
	snapshot := &scanSnapshot{entries: make(map[string]Entry), bytes: make(map[string][]byte)}
	if scanErr := scanDir(rootFD, "", snapshot); scanErr != nil {
		t.Fatal(scanErr)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "_CodeSignature", "CodeResources"), []byte("late"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := finalNamespaceRescan(rootFD, snapshot); err == nil || !strings.Contains(err.Error(), "namespace changed") {
		t.Fatalf("late namespace addition was accepted: %v", err)
	}
	_ = expected
}

func TestFinalNamespaceRescanRejectsSameContentDifferentInode(t *testing.T) {
	app, expected := makeBundle(t)
	rootFD, err := unix.Open(app, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(rootFD)
	snapshot := &scanSnapshot{entries: make(map[string]Entry), bytes: make(map[string][]byte)}
	if err := scanDir(rootFD, "", snapshot); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(app, "Contents", "PkgInfo")
	original, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	backup := filepath.Join(t.TempDir(), "PkgInfo.old")
	if renameErr := os.Rename(path, backup); renameErr != nil {
		t.Fatal(renameErr)
	}
	if writeErr := os.WriteFile(path, original, 0o755); writeErr != nil {
		t.Fatal(writeErr)
	}
	if err := finalNamespaceRescan(rootFD, snapshot); err == nil || !strings.Contains(err.Error(), "entry identity changed") {
		t.Fatalf("same-content replacement was accepted: %v", err)
	}
	_ = os.Remove(backup)
	_ = expected
}

func TestFinalNamespaceRescanRejectsAllowedFileDirectorySubstitution(t *testing.T) {
	app, expected := makeBundle(t)
	rootFD, err := unix.Open(app, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(rootFD)
	snapshot := &scanSnapshot{entries: make(map[string]Entry), bytes: make(map[string][]byte)}
	if err := scanDir(rootFD, "", snapshot); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(app, "Contents", "PkgInfo")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := finalNamespaceRescan(rootFD, snapshot); err == nil || !strings.Contains(err.Error(), "rescan type mismatch") {
		t.Fatalf("file-to-directory substitution was accepted: %v", err)
	}
	_ = expected
}

func makeBundle(t *testing.T) (string, Expectations) {
	t.Helper()
	app := filepath.Join(t.TempDir(), "Pantheon.app")
	for _, dir := range []string{
		"Contents/MacOS",
		"Contents/Resources",
		"Contents/Resources/StackLab/v2",
	} {
		if err := os.MkdirAll(filepath.Join(app, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	info := []byte("CFBundleShortVersionString=0.23.9-beta\nCFBundleVersion=20260908\n")
	pkgInfo := []byte("APPL????")
	launchAgent := []byte("Label=ai.sirsi.pantheon\n")
	brandLogo := []byte("canonical Sirsi application-mark bytes\n")
	files := map[string][]byte{
		"Contents/Info.plist":                                                   info,
		"Contents/PkgInfo":                                                      pkgInfo,
		"Contents/MacOS/sirsi":                                                  []byte("go cli bytes"),
		"Contents/MacOS/sirsi-menubar":                                          []byte("go menubar bytes"),
		"Contents/Resources/ai.sirsi.pantheon.plist":                            launchAgent,
		"Contents/Resources/sirsi-logo-white.png":                               brandLogo,
		"Contents/Resources/StackLab/anubis-trash-receipt-v1.json":              []byte(`{"schema":"sirsi.stacklab.recipe.v1","id":"stacklab.recipe.anubis-trash-receipt"}`),
		"Contents/Resources/StackLab/apollo-sne-telemetry-v1.json":              []byte(`{"schema":"sirsi.stacklab.apollo-telemetry.v1"}`),
		"Contents/Resources/StackLab/maat-system-one-recipe-v1.json":            []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/maat-pod-authority-service-recipe-v1.json": []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/maat-wing-v1.json":                         []byte(`{"schema":"sirsi.stacklab.wing.v1"}`),
		"Contents/Resources/StackLab/mercury-consumer-recipe-v1.json":           []byte(`{"schema":"sirsi.stacklab.recipe.v1","id":"stacklab.recipe.mercury-consumer"}`),
		"Contents/Resources/StackLab/model-router-v1-recipe-v1.json":            []byte(`{"schema":"sirsi.stacklab.recipe.v1","id":"stacklab.recipe.model-router-v1"}`),
		"Contents/Resources/StackLab/native-stacklab-surface-recipe-v1.json":    []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/pantheon-native-workspace-v1.json":         []byte(`{"schema":"sirsi.stacklab.recipe.v1","id":"stacklab.recipe.pantheon-native-workspace"}`),
		"Contents/Resources/StackLab/pantheon.pt-wing-001-wing-v1.json":         []byte(`{"schema":"sirsi.stacklab.wing.v1"}`),
		"Contents/Resources/StackLab/pantheon-release-artifact-recipe-v1.json":  []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/ra-horus-fabric-recipe-v1.json":            []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/ra-horus-fabric-wing-v1.json":              []byte(`{"schema":"sirsi.stacklab.wing.v1"}`),
		"Contents/Resources/StackLab/v2/PROVENANCE.md":                          []byte("Stack Lab provenance\n"),
		"Contents/Resources/StackLab/v2/wing.schema.json":                       []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema"}`),
	}
	for rel, data := range files {
		mode := os.FileMode(0o644)
		if rel == "Contents/MacOS/sirsi" || rel == "Contents/MacOS/sirsi-menubar" {
			mode = 0o755
		}
		file := filepath.Join(app, filepath.FromSlash(rel))
		if err := os.WriteFile(file, data, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(file, mode); err != nil {
			t.Fatal(err)
		}
	}
	// Fixture modes are intentional, independent of an admitted runner's umask.
	if err := filepath.WalkDir(app, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return app, Expectations{Version: "0.23.9-beta", Build: "20260908", InfoPlist: info, PkgInfo: pkgInfo, LaunchAgent: launchAgent, BrandLogo: brandLogo}
}

func TestVerifyRejectsNonPublicModesIncludingRoot(t *testing.T) {
	for _, tc := range []struct {
		name, relative string
		mode           os.FileMode
	}{
		{"private-root", ".", 0o700},
		{"private-contents", "Contents", 0o700},
		{"private-info", "Contents/Info.plist", 0o600},
		{"private-signature", "Contents/_CodeSignature/CodeResources", 0o600},
		{"private-flat-signature", "Contents/CodeResources", 0o600},
		{"private-resource", "Contents/Resources/sirsi-logo-white.png", 0o600},
		{"nonexecutable-cli", "Contents/MacOS/sirsi", 0o644},
		{"writable-directory", "Contents/Resources", 0o777},
		{"executable-resource", "Contents/PkgInfo", 0o755},
		{"setuid-cli", "Contents/MacOS/sirsi", 0o755 | os.ModeSetuid},
		{"setgid-resource", "Contents/PkgInfo", 0o644 | os.ModeSetgid},
		{"sticky-directory", "Contents", 0o755 | os.ModeSticky},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, expected := makeBundle(t)
			if strings.Contains(tc.relative, "CodeResources") {
				parent := filepath.Dir(filepath.Join(app, tc.relative))
				if err := os.MkdirAll(parent, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(parent, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(app, tc.relative), []byte("inert signature"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chmod(filepath.Join(app, tc.relative), tc.mode); err != nil {
				t.Fatal(err)
			}
			_, err := Verify(app, expected)
			if err == nil || !strings.Contains(err.Error(), "public mode mismatch") {
				t.Fatalf("nonpublic mode accepted/unrelated refusal: %v", err)
			}
		})
	}
}
