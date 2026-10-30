package stacklab

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveGitHubCLIFromUsesPathBeforeMacOSFallback(t *testing.T) {
	root := t.TempDir()
	pathDir := filepath.Join(root, "path")
	fallbackDir := filepath.Join(root, "fallback")
	if err := os.MkdirAll(pathDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(fallbackDir, 0o700); err != nil {
		t.Fatal(err)
	}
	pathGH := filepath.Join(pathDir, "gh")
	fallbackGH := filepath.Join(fallbackDir, "gh")
	writeExecutable(t, pathGH)
	writeExecutable(t, fallbackGH)

	got, err := resolveGitHubCLIFrom(pathDir, "darwin", []string{fallbackGH})
	if err != nil {
		t.Fatal(err)
	}
	if got != pathGH {
		t.Fatalf("resolved executable = %q, want PATH executable %q", got, pathGH)
	}
}

func TestResolveGitHubCLIFromUsesMacOSFallbackAndCanonicalSymlink(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "Cellar", "gh", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(binDir, "gh")
	link := filepath.Join(root, "opt", "homebrew", "bin", "gh")
	writeExecutable(t, target)
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	got, err := resolveGitHubCLIFrom("", "darwin", []string{link})
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("resolved executable = %q, want canonical target %q", got, target)
	}
}

func TestResolveGitHubCLIFromSkipsCurrentDirectoryAndUsesMacOSFallback(t *testing.T) {
	root := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	cwdGH := filepath.Join(root, "gh")
	fallbackGH := filepath.Join(root, "homebrew", "bin", "gh")
	writeExecutable(t, cwdGH)
	if err := os.MkdirAll(filepath.Dir(fallbackGH), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, fallbackGH)

	got, err := resolveGitHubCLIFrom(".", "darwin", []string{fallbackGH})
	if err != nil {
		t.Fatal(err)
	}
	if got != fallbackGH {
		t.Fatalf("resolved executable = %q, want fixed macOS fallback %q; cwd gh must never run", got, fallbackGH)
	}
}

func TestResolveGitHubCLIFromLimitsFallbackToMacOSAndRejectsInvalidFiles(t *testing.T) {
	root := t.TempDir()
	nonExecutable := filepath.Join(root, "gh")
	if err := os.WriteFile(nonExecutable, []byte("not executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveGitHubCLIFrom("", "linux", []string{nonExecutable}); err == nil {
		t.Fatal("resolver accepted a non-executable file or a non-macOS fallback")
	}
	var missing MissingGitHubCLIError
	if _, err := resolveGitHubCLIFrom("", "darwin", nil); !errors.As(err, &missing) {
		t.Fatalf("missing executable error = %T %v, want MissingGitHubCLIError", err, err)
	}
}

func TestGHRemoteReaderUsesResolvedExecutableAndDoesNotExposeStderr(t *testing.T) {
	var gotBinary string
	var gotArgs []string
	reader := newGHRemoteReader(
		func() (string, error) { return "/canonical/gh", nil },
		func(binary string, args ...string) ([]byte, []byte, error) {
			gotBinary = binary
			gotArgs = append([]string(nil), args...)
			return nil, []byte("private auth detail"), errors.New("exit status 1")
		},
	)
	_, exists, err := reader.ReadFile("owner/repo", "wing.json", "main")
	if err == nil || exists {
		t.Fatalf("ReadFile = exists %v, err %v; want failed, unknown read", exists, err)
	}
	if gotBinary != "/canonical/gh" {
		t.Fatalf("command binary = %q, want canonical executable", gotBinary)
	}
	wantArgs := []string{"api", "-H", "Accept: application/vnd.github.raw", "repos/owner/repo/contents/wing.json?ref=main"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", gotArgs, wantArgs)
	}
	if strings.Contains(err.Error(), "private auth detail") {
		t.Fatalf("error exposed stderr: %v", err)
	}
}

func TestGHRemoteReaderReturnsTypedMissingDependencyWithoutRunningCommand(t *testing.T) {
	reader := newGHRemoteReader(
		func() (string, error) { return "", MissingGitHubCLIError{} },
		func(string, ...string) ([]byte, []byte, error) {
			t.Fatal("command ran without a resolved GitHub CLI")
			return nil, nil, nil
		},
	)
	_, exists, err := reader.ReadFile("owner/repo", "wing.json", "main")
	var missing MissingGitHubCLIError
	if exists || !errors.As(err, &missing) {
		t.Fatalf("ReadFile = exists %v, err %T %v; want typed missing dependency and no report", exists, err, err)
	}
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}
