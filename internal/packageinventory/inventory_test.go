package packageinventory

import (
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
	if len(report.Entries) != 8 {
		t.Fatalf("entry count = %d, want unsigned allowlist without signature directory", len(report.Entries))
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

func TestVerifyRejectsPythonLinkageInPayloadBytes(t *testing.T) {
	app, expected := makeBundle(t)
	info := filepath.Join(app, "Contents", "Info.plist")
	if err := os.WriteFile(info, []byte("libpython3.13.dylib"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected.InfoPlist = []byte("libpython3.13.dylib")
	if _, err := Verify(app, expected); err == nil || !strings.Contains(err.Error(), "Python linkage") {
		t.Fatalf("Python-linked payload was accepted: %v", err)
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

func makeBundle(t *testing.T) (string, Expectations) {
	t.Helper()
	app := filepath.Join(t.TempDir(), "Pantheon.app")
	for _, dir := range []string{
		"Contents/MacOS",
		"Contents/Resources",
	} {
		if err := os.MkdirAll(filepath.Join(app, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	info := []byte("CFBundleShortVersionString=0.23.9-beta\nCFBundleVersion=20260908\n")
	pkgInfo := []byte("APPL????")
	launchAgent := []byte("Label=ai.sirsi.pantheon\n")
	files := map[string][]byte{
		"Contents/Info.plist":                        info,
		"Contents/PkgInfo":                           pkgInfo,
		"Contents/MacOS/sirsi":                       []byte("go cli bytes"),
		"Contents/MacOS/sirsi-menubar":               []byte("go menubar bytes"),
		"Contents/Resources/ai.sirsi.pantheon.plist": launchAgent,
	}
	for rel, data := range files {
		if err := os.WriteFile(filepath.Join(app, filepath.FromSlash(rel)), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return app, Expectations{Version: "0.23.9-beta", Build: "20260908", InfoPlist: info, PkgInfo: pkgInfo, LaunchAgent: launchAgent}
}
