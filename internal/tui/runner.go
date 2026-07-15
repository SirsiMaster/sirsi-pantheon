package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Command-runner seam (Rule A16 side-effect injection).
//
// Every screen reads live workstation state through ONE injectable seam: it
// runs `sirsi <verb> --json` and decodes the contract. Tests swap the runner
// for a canned-JSON stub so the model is exercised deterministically without
// ever shelling out. The TUI is a projection of these four merged --json
// contracts (vitals, scan, ghosts, activity, diagnose) — Go stays the brain.
//
// Concurrency (Rule A21): the package-level default runner is guarded by a
// RWMutex and swapped only through setRunner/getRunner. Tests that swap it MUST
// NOT use t.Parallel() (package-global swap; repo lessons #129/#131/#139).

// Runner executes a sirsi subcommand with --json and returns its raw stdout.
// verb is the leading token ("vitals", "scan", …); args are the remaining
// flags (already excluding --json, which the runner appends). A Runner MUST be
// safe to call from a bubbletea tea.Cmd goroutine.
type Runner interface {
	RunJSON(ctx context.Context, verb string, args ...string) ([]byte, error)
}

// RunnerFunc adapts a function to the Runner interface.
type RunnerFunc func(ctx context.Context, verb string, args ...string) ([]byte, error)

// RunJSON implements Runner.
func (f RunnerFunc) RunJSON(ctx context.Context, verb string, args ...string) ([]byte, error) {
	return f(ctx, verb, args...)
}

// execRunner shells out to the sirsi binary that is currently executing, so the
// TUI always drives the exact same engine it was launched from (no PATH drift).
type execRunner struct{ self string }

// newExecRunner resolves the running binary. It falls back to "sirsi" on PATH
// when the executable path cannot be determined.
func newExecRunner() execRunner {
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "sirsi"
	}
	return execRunner{self: self}
}

// RunJSON runs `<self> <verb> [args…] --json` and returns stdout. Stderr is
// folded into the error so a failure surfaces its reason, never a silent empty
// frame.
func (r execRunner) RunJSON(ctx context.Context, verb string, args ...string) ([]byte, error) {
	full := append([]string{verb}, args...)
	full = append(full, "--json")
	cmd := exec.CommandContext(ctx, r.self, full...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("sirsi %s: %s", verb, string(ee.Stderr))
		}
		return nil, fmt.Errorf("sirsi %s: %w", verb, err)
	}
	return out, nil
}

var (
	runnerMu      sync.RWMutex
	defaultRunner Runner = newExecRunner()
)

// getRunner returns the active runner under the read lock (Rule A21).
func getRunner() Runner {
	runnerMu.RLock()
	defer runnerMu.RUnlock()
	return defaultRunner
}

// setRunner installs r as the active runner under the write lock. Tests use it
// to inject canned JSON; production never calls it.
func setRunner(r Runner) {
	runnerMu.Lock()
	defer runnerMu.Unlock()
	defaultRunner = r
}

// runTimeout bounds every contract read. vitals is built for a <100ms tick;
// scan/diagnose are seconds. The ceiling stops a hung child from freezing the
// runloop — a timeout surfaces as an error banner the operator can retry.
const runTimeout = 30 * time.Second

// decode runs a verb and unmarshals its JSON into v. It centralizes the
// timeout, the run, and the decode so every screen's load path is identical.
func decode(verb string, v any, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	out, err := getRunner().RunJSON(ctx, verb, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, v); err != nil {
		return fmt.Errorf("sirsi %s: decode: %w", verb, err)
	}
	return nil
}
