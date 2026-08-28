package ra

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunNativeFleetUsesExplicitOrderedSteps(t *testing.T) {
	original := nativeRun
	t.Cleanup(func() { nativeRun = original })
	var calls []string
	nativeRun = func(_ context.Context, dir string, args ...string) ([]byte, error) {
		calls = append(calls, dir+":"+strings.Join(args, " "))
		return []byte("ok\n"), nil
	}

	results, err := RunNativeFleet(context.Background(), []NativeRepo{{
		Name: "nexus", Path: t.TempDir(), Health: []Command{
			{Args: []string{"git", "status", "--short"}},
			{Dir: "ui", Args: []string{"yarn", "lint"}},
		},
	}}, "health")
	if err != nil {
		t.Fatalf("RunNativeFleet() error = %v", err)
	}
	if results[0].Status != "pass" {
		t.Fatalf("status = %q, output=%q", results[0].Status, results[0].Output)
	}
	want := []string{results[0].Repo}
	_ = want
	if len(calls) != 2 || !strings.HasSuffix(calls[0], ":git status --short") || !strings.HasSuffix(calls[1], "/ui:yarn lint") {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestRunNativeFleetFailsClosedForProviderOperations(t *testing.T) {
	_, err := RunNativeFleet(context.Background(), nil, "task")
	if !errors.Is(err, ErrExternalProviderUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestExternalProviderIsExplicitAndDirect(t *testing.T) {
	t.Setenv(ExternalProviderEnv, "")
	if _, err := ExternalProviderFromEnv(); !errors.Is(err, ErrExternalProviderUnavailable) {
		t.Fatalf("missing provider error = %v", err)
	}

	original := nativeRun
	t.Cleanup(func() { nativeRun = original })
	providerFile := filepath.Join(t.TempDir(), "sirsi-ra-provider")
	if err := os.WriteFile(providerFile, []byte("provider"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ExternalProviderEnv, providerFile)
	provider, err := ExternalProviderFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	nativeRun = func(_ context.Context, dir string, args ...string) ([]byte, error) {
		got = append([]string{dir}, args...)
		return []byte("accepted"), nil
	}
	results, err := RunNativeFleetWithProvider(context.Background(), nil, "task", []string{"pantheon", "fix it"}, provider)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"", providerFile, "task", "pantheon", "fix it"}
	if strings.Join(got, "|") != strings.Join(want, "|") || results[0].Status != "pass" {
		t.Fatalf("got=%q results=%+v", got, results)
	}
}

func TestExternalProviderRejectsUnsafeExecutableIdentity(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "provider")
	if err := os.WriteFile(executable, []byte("provider"), 0o700); err != nil {
		t.Fatal(err)
	}
	nonExecutable := filepath.Join(dir, "non-executable")
	if err := os.WriteFile(nonExecutable, []byte("provider"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(dir, "provider-link")
	if err := os.Symlink(executable, symlink); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{
		"relative":       "relative/provider",
		"symlink":        symlink,
		"non-executable": nonExecutable,
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(ExternalProviderEnv, path)
			if _, err := ExternalProviderFromEnv(); err == nil {
				t.Fatalf("ExternalProviderFromEnv() accepted %q", path)
			}
		})
	}
}

func TestRunNativeFleetPreservesCommandFailureOutput(t *testing.T) {
	original := nativeRun
	t.Cleanup(func() { nativeRun = original })
	nativeRun = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		return []byte("broken " + strings.Join(args, " ")), fmt.Errorf("exit 1")
	}
	results, err := RunNativeFleet(context.Background(), []NativeRepo{{Name: "one", Path: t.TempDir(), Test: []Command{{Args: []string{"go", "test"}}}}}, "test")
	if err != nil {
		t.Fatalf("RunNativeFleet() error = %v", err)
	}
	if results[0].Status != "fail" || !strings.Contains(results[0].Output, "broken go test") {
		t.Fatalf("result = %+v", results[0])
	}
}

func TestDefaultFleetReposHasNoPythonOrShellCommands(t *testing.T) {
	for _, repo := range DefaultFleetRepos("/owner") {
		for _, steps := range [][]Command{repo.Health, repo.Test, repo.Lint} {
			for _, step := range steps {
				if len(step.Args) == 0 || step.Args[0] == "python3" || step.Args[0] == "sh" || step.Args[0] == "zsh" {
					t.Fatalf("unsafe command for %s: %#v", repo.Name, step.Args)
				}
			}
		}
	}
}

func TestDefaultFleetReposPreservesFormerFleetOperations(t *testing.T) {
	byName := map[string]NativeRepo{}
	for _, repo := range DefaultFleetRepos("/owner") {
		byName[repo.Name] = repo
	}
	tests := map[string][]string{
		"pantheon":    {"go test -short ./..."},
		"nexus":       {"yarn test --passWithNoTests", "npx tsc --noEmit"},
		"finalwishes": {"npm test --if-present"},
		"assiduous":   {"npm test --if-present"},
	}
	for name, want := range tests {
		repo, ok := byName[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		var got []string
		for _, step := range repo.Test {
			got = append(got, strings.Join(step.Args, " "))
		}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s test contract = %q, want %q", name, got, want)
		}
		if len(repo.Health) < 3 || strings.Join(repo.Health[0].Args, " ") != "git status --short" || strings.Join(repo.Health[1].Args, " ") != "git log --oneline -3" {
			t.Errorf("%s health preflight = %#v", name, repo.Health)
		}
	}
}

func TestRunNativeAndRecordUsesInjectedFleetWithoutPython(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()
	p := NewPipeline(repo)
	called := false
	result, err := p.RunNativeAndRecord(context.Background(), nil, Task{Subcmd: "health"}, func(_ context.Context, _ []NativeRepo, operation string) ([]NativeResult, error) {
		called = true
		if operation != "health" {
			t.Fatalf("operation = %q", operation)
		}
		return []NativeResult{{Repo: "pantheon", Operation: operation, Status: "pass", Output: "ok"}}, nil
	})
	if err != nil {
		t.Fatalf("RunNativeAndRecord() error = %v", err)
	}
	if !called || result.ItemsIngested != 1 {
		t.Fatalf("called=%v result=%+v", called, result)
	}
}
