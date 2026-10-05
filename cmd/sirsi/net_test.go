package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInspectBuildLogCountsHeadingAtStartOfFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "BUILD_LOG.md")
	content := []byte("## First\nbody\n### Nested\n## Second\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	sections, modified, err := inspectBuildLog(path)
	if err != nil {
		t.Fatalf("inspectBuildLog: %v", err)
	}
	if sections != 2 {
		t.Fatalf("section count = %d, want 2", sections)
	}
	if modified.IsZero() {
		t.Fatal("regular build log returned a zero modification time")
	}
}

func TestInspectBuildLogRejectsDirectory(t *testing.T) {
	path := t.TempDir()
	if _, _, err := inspectBuildLog(path); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory accepted as build log: %v", err)
	}
}

func TestRunNetRepositoryChecksReportsMeasuredResultsOnly(t *testing.T) {
	var calls []string
	runner := func(_ context.Context, root, name string, args ...string) ([]byte, error) {
		if root != "/repo" {
			t.Fatalf("check root = %q, want /repo", root)
		}
		call := append([]string{name}, args...)
		calls = append(calls, strings.Join(call, " "))
		switch {
		case name == "go" && reflect.DeepEqual(args, []string{"vet", "./..."}):
			return nil, nil
		case name == "go" && reflect.DeepEqual(args, []string{"build", "./..."}):
			return []byte("compile failure"), errors.New("exit 1")
		case name == "gofmt" && reflect.DeepEqual(args, []string{"-l", "./internal/", "./cmd/"}):
			return []byte("cmd/example.go\n"), nil
		default:
			t.Fatalf("unexpected repository check: %s %v", name, args)
			return nil, nil
		}
	}

	results, err := runNetRepositoryChecks(context.Background(), "/repo", runner)
	if err != nil {
		t.Fatalf("runNetRepositoryChecks: %v", err)
	}
	wantCalls := []string{"go vet ./...", "go build ./...", "gofmt -l ./internal/ ./cmd/"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("commands = %v, want %v", calls, wantCalls)
	}
	if len(results) != 3 {
		t.Fatalf("got %d checks, want exactly 3 measured checks", len(results))
	}
	if !results[0].passed() || results[1].passed() || results[2].passed() {
		t.Fatalf("check pass states = [%t %t %t], want [true false false]", results[0].passed(), results[1].passed(), results[2].passed())
	}
}

func TestRunNetRepositoryChecksStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	runner := func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
		calls++
		cancel()
		return nil, nil
	}
	results, err := runNetRepositoryChecks(ctx, "/repo", runner)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v, want context.Canceled", err)
	}
	if calls != 1 || len(results) != 1 {
		t.Fatalf("calls/results = %d/%d, want 1/1", calls, len(results))
	}
}
