package maat

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fakeGoCoverageOutput = "ok  \tgithub.com/SirsiMaster/sirsi-pantheon/internal/fixture\t0.001s\tcoverage: 63.0% of statements\n"

func installFakeGo(t *testing.T, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		`[ "$1" = test ] && [ "$2" = -cover ] && [ "$3" = ./... ] || exit 91` + "\n" +
		`printf '%s\n' '` + strings.TrimSuffix(fakeGoCoverageOutput, "\n") + "'\n" +
		fmt.Sprintf("exit %d\n", exitCode)
	path := filepath.Join(dir, "go")
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestRunFullCoverageBufferedPropagatesFailureWithRawOutput(t *testing.T) {
	installFakeGo(t, 23)
	got, err := (&CoverageAssessor{ProjectRoot: t.TempDir()}).runFullCoverage()
	if got != fakeGoCoverageOutput {
		t.Fatalf("buffered output = %q, want raw fixture %q", got, fakeGoCoverageOutput)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("buffered child error = %v, want exit status 23", err)
	}
}

func TestRunFullCoverageStreamingPropagatesFailureAndUnknownTotal(t *testing.T) {
	installFakeGo(t, 23)
	var progress []PackageProgress
	assessor := &CoverageAssessor{
		ProjectRoot: t.TempDir(),
		Thresholds:  []CoverageThreshold{{Module: "policy-module", MinCoverage: 50}},
		ProgressFn:  func(p PackageProgress) { progress = append(progress, p) },
	}
	got, err := assessor.runFullCoverage()
	if got != fakeGoCoverageOutput {
		t.Fatalf("streamed output = %q, want raw fixture %q", got, fakeGoCoverageOutput)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("streamed child error = %v, want exit status 23", err)
	}
	if len(progress) != 1 || progress[0].Package != "fixture" || progress[0].Current != 1 || progress[0].Total != 0 {
		t.Fatalf("streamed progress = %+v, want ordinal 1 with unknown total", progress)
	}
}

func TestRunFullCoverageAcceptsZeroExitInBothModes(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("streaming=%t", streaming), func(t *testing.T) {
			installFakeGo(t, 0)
			assessor := &CoverageAssessor{ProjectRoot: t.TempDir()}
			if streaming {
				assessor.ProgressFn = func(PackageProgress) {}
			}
			got, err := assessor.runFullCoverage()
			if err != nil || got != fakeGoCoverageOutput {
				t.Fatalf("output = %q, error = %v", got, err)
			}
		})
	}
}
