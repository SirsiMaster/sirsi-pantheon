package packageinventory

import (
	"encoding/binary"
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
	if len(report.Entries) != 19 {
		t.Fatalf("entry count = %d, want unsigned payload including Stack Lab contracts", len(report.Entries))
	}
	for i := 1; i < len(report.Entries); i++ {
		if report.Entries[i-1].Path >= report.Entries[i].Path {
			t.Fatalf("entries are not strictly sorted: %+v", report.Entries)
		}
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

func TestVerifyRejectsPythonMachOLinkageInSignedPayload(t *testing.T) {
	app, expected := makeBundle(t)
	if err := os.Mkdir(filepath.Join(app, "Contents", "_CodeSignature"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "_CodeSignature", "CodeResources"), []byte("signature"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected.RequireCodeSignature = true
	if err := os.WriteFile(filepath.Join(app, "Contents", "MacOS", "sirsi"), machoWithDylib("/usr/lib/libpython3.13.dylib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(app, expected); err == nil || !strings.Contains(err.Error(), "Python linkage") {
		t.Fatalf("Python-linked Mach-O payload was accepted: %v", err)
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
	rootFD, err := unix.Open(app, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(rootFD)
	snapshot := &scanSnapshot{entries: make(map[string]Entry), bytes: make(map[string][]byte)}
	if err := scanDir(rootFD, "", snapshot); err != nil {
		t.Fatal(err)
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
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "PkgInfo.old")
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original, 0o755); err != nil {
		t.Fatal(err)
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
	files := map[string][]byte{
		"Contents/Info.plist":                                                  info,
		"Contents/PkgInfo":                                                     pkgInfo,
		"Contents/MacOS/sirsi":                                                 []byte("go cli bytes"),
		"Contents/MacOS/sirsi-menubar":                                         []byte("go menubar bytes"),
		"Contents/Resources/ai.sirsi.pantheon.plist":                           launchAgent,
		"Contents/Resources/StackLab/apollo-sne-telemetry-v1.json":             []byte(`{"schema":"sirsi.stacklab.apollo-telemetry.v1"}`),
		"Contents/Resources/StackLab/maat-system-one-recipe-v1.json":           []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/maat-wing-v1.json":                        []byte(`{"schema":"sirsi.stacklab.wing.v1"}`),
		"Contents/Resources/StackLab/native-stacklab-surface-recipe-v1.json":   []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/pantheon-release-artifact-recipe-v1.json": []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/ra-horus-fabric-recipe-v1.json":           []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/ra-horus-fabric-wing-v1.json":             []byte(`{"schema":"sirsi.stacklab.wing.v1"}`),
		"Contents/Resources/StackLab/v2/PROVENANCE.md":                         []byte("Stack Lab provenance\n"),
		"Contents/Resources/StackLab/v2/wing.schema.json":                      []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema"}`),
	}
	for rel, data := range files {
		if err := os.WriteFile(filepath.Join(app, filepath.FromSlash(rel)), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return app, Expectations{Version: "0.23.9-beta", Build: "20260908", InfoPlist: info, PkgInfo: pkgInfo, LaunchAgent: launchAgent}
}
