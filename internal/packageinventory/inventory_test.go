package packageinventory

import (
	"errors"
	"io"
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
	if report.Schema != Schema || !report.PythonFree || report.EngineCount != 1 || report.ExecutableCount != 2 {
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

func TestVerifyRejectsExecutableMetadataPayload(t *testing.T) {
	app, expected := makeBundle(t)
	if err := os.Chmod(filepath.Join(app, "Contents", "Info.plist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(app, expected); err == nil || !strings.Contains(err.Error(), "resource payload has executable mode") {
		t.Fatalf("executable Info.plist was accepted: %v", err)
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

func TestVerifyRejectsHardLinkedPayload(t *testing.T) {
	app, expected := makeBundle(t)
	sirsi := filepath.Join(app, "Contents", "MacOS", "sirsi")
	linked := filepath.Join(filepath.Dir(app), "sirsi-linked-outside-bundle")
	if err := os.Link(sirsi, linked); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(linked) })
	if _, err := Verify(app, expected); err == nil || !strings.Contains(err.Error(), "nlink=1") {
		t.Fatalf("hard-linked package payload was accepted: %v", err)
	}
}

func TestValidateReportRejectsMalformedRegularMetadataAndDigest(t *testing.T) {
	app, expected := makeBundle(t)
	report, err := Verify(app, expected)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Entry){
		"hardlink":      func(entry *Entry) { entry.Nlink = 2 },
		"out-of-range":  func(entry *Entry) { entry.Size = (32 << 20) + 1 },
		"nonhex digest": func(entry *Entry) { entry.SHA256 = strings.Repeat("z", 64) },
		"mode type":     func(entry *Entry) { entry.Mode = uint32(unix.S_IFDIR | 0o755) },
	} {
		t.Run(name, func(t *testing.T) {
			malformed := report
			malformed.Entries = append([]Entry(nil), report.Entries...)
			for i := range malformed.Entries {
				if malformed.Entries[i].Path == "Contents/MacOS/sirsi" {
					mutate(&malformed.Entries[i])
					break
				}
			}
			if err := validateReport(malformed, nil, expected); err == nil {
				t.Fatal("malformed report was accepted")
			}
		})
	}
}

func TestValidateReportRejectsInconsistentEngineAndExecutableCounts(t *testing.T) {
	app, expected := makeBundle(t)
	report, err := Verify(app, expected)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Report){
		"engine count":     func(candidate *Report) { candidate.EngineCount++ },
		"executable count": func(candidate *Report) { candidate.ExecutableCount-- },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := report
			mutate(&candidate)
			if err := validateReport(candidate, nil, expected); err == nil || !strings.Contains(err.Error(), "malformed report") {
				t.Fatalf("inconsistent %s was accepted: %v", name, err)
			}
		})
	}
}

func TestVerifyRejectsPartialOptionalCodeSignaturePayload(t *testing.T) {
	app, expected := makeBundle(t)
	if err := os.Mkdir(filepath.Join(app, "Contents", "_CodeSignature"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(app, expected); err == nil || !strings.Contains(err.Error(), "incomplete _CodeSignature payload") {
		t.Fatalf("partial optional code signature payload was accepted: %v", err)
	}
}

func TestVerifyAcceptsCompleteCodeSignaturePayload(t *testing.T) {
	app, expected := makeBundle(t)
	signatureDir := filepath.Join(app, "Contents", "_CodeSignature")
	if err := os.Mkdir(signatureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(signatureDir, "CodeResources"), []byte("resource seal"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected.RequireCodeSignature = true
	if _, err := Verify(app, expected); err != nil {
		t.Fatalf("complete code-signature payload was rejected: %v", err)
	}
}

func TestVerifyRejectsNonExecutableEnginePayload(t *testing.T) {
	for _, rel := range []string{"Contents/MacOS/sirsi", "Contents/MacOS/sirsi-menubar"} {
		t.Run(rel, func(t *testing.T) {
			app, expected := makeBundle(t)
			engine := filepath.Join(app, filepath.FromSlash(rel))
			if err := os.Chmod(engine, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(app, expected); err == nil || !strings.Contains(err.Error(), "product executable payload is not executable") {
				t.Fatalf("non-executable engine payload was accepted: %v", err)
			}
		})
	}
}

func TestVerifyRejectsAppLeafReplacementBeforeAcceptance(t *testing.T) {
	app, expected := makeBundle(t)
	replacement := filepath.Join(filepath.Dir(app), "Replacement.app")
	writeBundleAt(t, replacement, expected)
	backup := filepath.Join(filepath.Dir(app), "Pantheon.original.app")

	_, err := verifyWithHook(app, expected, func() {
		if err := os.Rename(app, backup); err != nil {
			t.Fatalf("move verified bundle aside: %v", err)
		}
		if err := os.Rename(replacement, app); err != nil {
			t.Fatalf("install replacement at governed app name: %v", err)
		}
	})
	if err == nil || !strings.Contains(err.Error(), "app path identity changed") {
		t.Fatalf("replacement bundle at the governed path was accepted: %v", err)
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("original bundle was not preserved: %v", err)
	}
	if _, err := os.Stat(app); err != nil {
		t.Fatalf("replacement bundle was unexpectedly removed: %v", err)
	}
}

func TestVerifyRejectsAppParentReplacementBeforeAcceptance(t *testing.T) {
	app, expected := makeBundle(t)
	parent := filepath.Dir(app)
	backup := parent + ".original"
	t.Cleanup(func() { _ = os.RemoveAll(backup) })

	_, err := verifyWithHook(app, expected, func() {
		if err := os.Rename(parent, backup); err != nil {
			t.Fatalf("move verified app parent aside: %v", err)
		}
		if err := os.Mkdir(parent, 0o755); err != nil {
			t.Fatalf("replace governed app parent: %v", err)
		}
	})
	if err == nil || !strings.Contains(err.Error(), "parent path continuity failed") {
		t.Fatalf("replacement parent at the governed path was accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backup, filepath.Base(app))); err != nil {
		t.Fatalf("original bundle was not preserved with its parent: %v", err)
	}
}

func TestVerifyAllowsUnrelatedSiblingDirectoryChange(t *testing.T) {
	app, expected := makeBundle(t)
	parent := filepath.Dir(app)
	var before unix.Stat_t
	if err := unix.Lstat(parent, &before); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(parent, "unrelated-sibling")
	_, err := verifyWithHook(app, expected, func() {
		if err := os.Mkdir(extra, 0o755); err != nil {
			t.Fatalf("create unrelated sibling directory: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("unrelated sibling directory metadata change rejected valid bundle: %v", err)
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
}

func TestVerifyRejectsSymlinkedAppAncestor(t *testing.T) {
	app, expected := makeBundle(t)
	aliasRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(aliasRoot, "bundle-parent-alias")
	if err := os.Symlink(filepath.Dir(app), alias); err != nil {
		t.Fatal(err)
	}
	aliasedApp := filepath.Join(alias, filepath.Base(app))
	if _, err := Verify(aliasedApp, expected); err == nil || !strings.Contains(err.Error(), "app parent is not a real directory") {
		t.Fatalf("symlinked app ancestor was accepted: %v", err)
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

func TestReadOnceRequiresExactObservedSizeAndEnforcesBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("exact size", func(t *testing.T) {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		got, err := readOnce(int(file.Fd()), int64(len("payload")))
		if err != nil || string(got) != "payload" {
			t.Fatalf("readOnce = %q, %v", got, err)
		}
	})

	t.Run("truncated below observed size", func(t *testing.T) {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := readOnce(int(file.Fd()), int64(len("payload")+1)); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("short read error = %v, want unexpected EOF", err)
		}
	})

	t.Run("grew beyond observed size", func(t *testing.T) {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := readOnce(int(file.Fd()), int64(len("pay"))); err == nil || !strings.Contains(err.Error(), "grew beyond observed size") {
			t.Fatalf("growth error = %v, want exact-size rejection", err)
		}
	})

	t.Run("rejects size above maximum before reading", func(t *testing.T) {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := readOnce(int(file.Fd()), (32<<20)+1); err == nil {
			t.Fatal("oversized observed file was accepted")
		}
	})
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

func TestVerifyRejectsPayloadAddedBeforeFinalNamespaceRescan(t *testing.T) {
	app, expected := makeBundle(t)
	_, err := verifyWithHook(app, expected, func() {
		codeSignature := filepath.Join(app, "Contents", "_CodeSignature")
		if err := os.Mkdir(codeSignature, 0o755); err != nil {
			t.Fatalf("add late allowed directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(codeSignature, "CodeResources"), []byte("late payload"), 0o644); err != nil {
			t.Fatalf("add late allowed payload: %v", err)
		}
	})
	if err == nil || !strings.Contains(err.Error(), "namespace changed") {
		t.Fatalf("payload added before final namespace rescan was accepted: %v", err)
	}
}

func TestVerifyRejectsSameInodeSameSizePayloadMutation(t *testing.T) {
	app, expected := makeBundle(t)
	payload := filepath.Join(app, "Contents", "MacOS", "sirsi")
	var before unix.Stat_t
	if err := unix.Lstat(payload, &before); err != nil {
		t.Fatal(err)
	}
	_, err := verifyWithHook(app, expected, func() {
		mutated := strings.Repeat("x", int(before.Size))
		if err := os.WriteFile(payload, []byte(mutated), 0o755); err != nil {
			t.Fatalf("mutate payload without replacing inode: %v", err)
		}
		var after unix.Stat_t
		if err := unix.Lstat(payload, &after); err != nil {
			t.Fatalf("stat mutated payload: %v", err)
		}
		if before.Dev != after.Dev || before.Ino != after.Ino || before.Size != after.Size || before.Mode != after.Mode || before.Nlink != after.Nlink {
			t.Fatalf("fixture did not preserve metadata identity: before=%+v after=%+v", before, after)
		}
	})
	if err == nil || !strings.Contains(err.Error(), "content changed after file read") {
		t.Fatalf("same-inode, same-size payload mutation was accepted: %v", err)
	}
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
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "Pantheon.app")
	expected := Expectations{
		Version:     "0.23.9-beta",
		Build:       "20260908",
		InfoPlist:   []byte("CFBundleShortVersionString=0.23.9-beta\nCFBundleVersion=20260908\n"),
		PkgInfo:     []byte("APPL????"),
		LaunchAgent: []byte("Label=ai.sirsi.pantheon\n"),
	}
	writeBundleAt(t, app, expected)
	return app, expected
}

func writeBundleAt(t *testing.T, app string, expected Expectations) {
	t.Helper()
	for _, dir := range []string{
		"Contents/MacOS",
		"Contents/Resources",
	} {
		if err := os.MkdirAll(filepath.Join(app, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string][]byte{
		"Contents/Info.plist":                        expected.InfoPlist,
		"Contents/PkgInfo":                           expected.PkgInfo,
		"Contents/MacOS/sirsi":                       []byte("go cli bytes"),
		"Contents/MacOS/sirsi-menubar":               []byte("go menubar bytes"),
		"Contents/Resources/ai.sirsi.pantheon.plist": expected.LaunchAgent,
	}
	for rel, data := range files {
		mode := os.FileMode(0o644)
		if rel == "Contents/MacOS/sirsi" || rel == "Contents/MacOS/sirsi-menubar" {
			mode = 0o755
		}
		if err := os.WriteFile(filepath.Join(app, filepath.FromSlash(rel)), data, mode); err != nil {
			t.Fatal(err)
		}
	}
}
