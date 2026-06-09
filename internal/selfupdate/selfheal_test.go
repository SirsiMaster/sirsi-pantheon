package selfupdate

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// withAllowList points the allow-list at dir for the duration of the test.
func withAllowList(t *testing.T, dir string) {
	t.Helper()
	old := allowedBinDirsFn
	allowedBinDirsFn = func() []string { return []string{dir} }
	t.Cleanup(func() { allowedBinDirsFn = old })
}

// withExec swaps the codesign runner for the duration of the test.
func withExec(t *testing.T, fn func(name string, args ...string) ([]byte, error)) {
	t.Helper()
	old := healExecFn
	healExecFn = fn
	t.Cleanup(func() { healExecFn = old })
}

func writeExe(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestGuardCLIPath(t *testing.T) {
	binDir := t.TempDir()
	withAllowList(t, binDir)

	tests := []struct {
		name    string
		dst     string
		wantErr error
	}{
		{"allow-listed dir", filepath.Join(binDir, "sirsi"), nil},
		{"app bundle rejected (A19)", "/Applications/Pantheon.app/Contents/MacOS/sirsi", ErrAppBundleProtected},
		{"nested app bundle rejected", filepath.Join(binDir, "Foo.app", "sirsi"), ErrAppBundleProtected},
		{"random path rejected", "/tmp/somewhere/sirsi", ErrPathNotAllowed},
		{"subdir of allowed not allowed", filepath.Join(binDir, "nested", "sirsi"), ErrPathNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := guardCLIPath(tt.dst)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("want %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestSafeReplace_HappyPath(t *testing.T) {
	binDir := t.TempDir()
	withAllowList(t, binDir)

	var codesigned string
	withExec(t, func(name string, args ...string) ([]byte, error) {
		// Verify the contract: codesign --force --sign - <staged .new>
		if name != "codesign" || len(args) != 4 || args[0] != "--force" || args[1] != "--sign" || args[2] != "-" {
			t.Errorf("unexpected exec: %s %v", name, args)
		}
		codesigned = args[3]
		return nil, nil
	})

	src := filepath.Join(t.TempDir(), "fresh")
	writeExe(t, src, "#!/bin/sh\necho fresh\n")
	dst := filepath.Join(binDir, "sirsi")
	writeExe(t, dst, "#!/bin/sh\necho stale\n")

	if err := SafeReplace(src, dst); err != nil {
		t.Fatalf("SafeReplace: %v", err)
	}

	got, _ := os.ReadFile(dst)
	if string(got) != "#!/bin/sh\necho fresh\n" {
		t.Errorf("dst not replaced with fresh content: %q", got)
	}
	if _, err := os.Stat(dst + ".new"); !os.IsNotExist(err) {
		t.Errorf(".new staging file should be gone after rename")
	}
	if runtime.GOOS == "darwin" && codesigned != dst+".new" {
		t.Errorf("codesign should sign the staged .new inode, signed %q", codesigned)
	}
}

func TestSafeReplace_RejectsAppBundleWithoutWriting(t *testing.T) {
	withAllowList(t, t.TempDir())
	src := filepath.Join(t.TempDir(), "fresh")
	writeExe(t, src, "x")
	dst := "/Applications/Pantheon.app/Contents/MacOS/sirsi"

	err := SafeReplace(src, dst)
	if !errors.Is(err, ErrAppBundleProtected) {
		t.Fatalf("want ErrAppBundleProtected, got %v", err)
	}
	if _, statErr := os.Stat(dst + ".new"); statErr == nil {
		t.Fatal("must not stage a .new beside an app-bundle target")
	}
}

func TestSafeReplace_CodesignFailureLeavesOldBinaryAndNoStaging(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("codesign step only runs on darwin")
	}
	binDir := t.TempDir()
	withAllowList(t, binDir)
	withExec(t, func(string, ...string) ([]byte, error) {
		return []byte("simulated codesign failure"), errors.New("boom")
	})

	src := filepath.Join(t.TempDir(), "fresh")
	writeExe(t, src, "fresh")
	dst := filepath.Join(binDir, "sirsi")
	writeExe(t, dst, "stale")

	if err := SafeReplace(src, dst); err == nil {
		t.Fatal("expected codesign failure to propagate")
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "stale" {
		t.Errorf("old binary must be untouched on failure, got %q", got)
	}
	if _, err := os.Stat(dst + ".new"); !os.IsNotExist(err) {
		t.Error("staging file must be cleaned up on failure")
	}
}

func TestSafeReplace_ExecutesAfterReplace(t *testing.T) {
	// End-to-end: replace a binary and confirm the result actually runs (the
	// AMFI-137 regression guard). On darwin real codesign signs it; elsewhere
	// codesign is skipped but the staged-rename still produces a runnable file.
	binDir := t.TempDir()
	withAllowList(t, binDir)

	src := filepath.Join(t.TempDir(), "fresh")
	writeExe(t, src, "#!/bin/sh\necho OK\n")
	dst := filepath.Join(binDir, "tool")
	writeExe(t, dst, "#!/bin/sh\necho OLD\n")

	if err := SafeReplace(src, dst); err != nil {
		t.Fatalf("SafeReplace: %v", err)
	}
	out, err := exec.Command(dst).Output()
	if err != nil {
		t.Fatalf("replaced binary failed to execute (AMFI-137 regression?): %v", err)
	}
	if string(out) != "OK\n" {
		t.Errorf("ran old binary? got %q", out)
	}
}
