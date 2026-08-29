package router

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scaffoldDutyScripts writes the two legacy shell duty scripts under a temp router root
// so runSupervisorDuties finds them (stat-only — dutyRunFn is stubbed).
func scaffoldDutyScripts(t *testing.T) (routerRoot, repoRoot string) {
	t.Helper()
	repoRoot = t.TempDir()
	routerRoot = filepath.Join(repoRoot, ".agents", "idea-router")
	if err := os.MkdirAll(filepath.Join(routerRoot, "police"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"run-on-event.sh", "sweep.sh"} {
		if err := os.WriteFile(filepath.Join(routerRoot, rel), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return routerRoot, repoRoot
}

func stubRegistryPolice(t *testing.T) *int {
	t.Helper()
	runs := 0
	orig := getRegistryPoliceFn()
	SetRegistryPoliceFn(func(string, string) error { runs++; return nil })
	t.Cleanup(func() { SetRegistryPoliceFn(orig) })
	return &runs
}

// stubDutyRun replaces dutyRunFn for one test, recording invoked scripts.
func stubDutyRun(t *testing.T, fn func(script, repoRoot string) error) *[]string {
	t.Helper()
	var calls []string
	orig := dutyRunFn
	dutyRunFn = func(script, repoRoot string) error {
		calls = append(calls, filepath.Base(script))
		if fn != nil {
			return fn(script, repoRoot)
		}
		return nil
	}
	t.Cleanup(func() { dutyRunFn = orig })
	return &calls
}

// stubSessionReaper replaces the native session-reaper duty body for the
// duration of a test — the duty table must be executable in tests WITHOUT
// shelling ps/SIGTERM on the host (the #151 test-side-effects class; the
// first run of this suite executed the real reaper before this stub existed).
func stubSessionReaper(t *testing.T) *int {
	t.Helper()
	oldEnum := enumerateProcsFn
	enumerateProcsFn = func() ([]CensusProc, error) { return nil, nil }
	t.Cleanup(func() { enumerateProcsFn = oldEnum })
	runs := 0
	orig := getSessionReaperImpl()
	setSessionReaperImpl(func(routerRoot, repoRoot string) error { runs++; return nil })
	t.Cleanup(func() { setSessionReaperImpl(orig) })
	return &runs
}

// TestRunSupervisorDuties_CadenceGating proves the fold's scheduler contract:
// the dispatch pump runs every tick; sweep and registry-police run once and
// are then cadence-skipped until their interval elapses — the same cadences
// the three legacy LaunchAgents carried (single-backstop ruling).
func TestRunSupervisorDuties_CadenceGating(t *testing.T) {
	routerRoot, repoRoot := scaffoldDutyScripts(t)
	calls := stubDutyRun(t, nil)
	stubSessionReaper(t)
	policeRuns := stubRegistryPolice(t)
	now := time.Now()

	first := runSupervisorDuties(routerRoot, repoRoot, now)
	if len(first) != 10 {
		t.Fatalf("first pass duties = %d, want 10 (2 scripts + native registry-police + auto-heal + census + gemma-liveness + session-reaper + launchd-kickstart + stale-thread-reconcile + spotlight-markers)", len(first))
	}
	for _, d := range first {
		if !d.Ran || d.Skipped != "" || d.Error != "" {
			t.Errorf("first pass %s = %+v, want ran with no skip/error", d.Name, d)
		}
	}
	if len(*calls) != 2 {
		t.Fatalf("first pass invoked %d scripts, want 2: %v", len(*calls), *calls)
	}
	if *policeRuns != 1 {
		t.Fatalf("registry police runs = %d, want 1", *policeRuns)
	}

	// Second tick, 61s later (the resident loop's default interval order):
	// dispatch pumps again; sweep (1h) and police (10m) are cadence-gated.
	*calls = nil
	second := runSupervisorDuties(routerRoot, repoRoot, now.Add(61*time.Second))
	byName := map[string]DutyResult{}
	for _, d := range second {
		byName[d.Name] = d
	}
	if d := byName["dispatch-pump"]; !d.Ran {
		t.Errorf("dispatch-pump must run every tick, got %+v", d)
	}
	for _, name := range []string{"sweep", "registry-police"} {
		if d := byName[name]; d.Ran || d.Skipped != "cadence" {
			t.Errorf("%s should be cadence-skipped on the next tick, got %+v", name, d)
		}
	}
	if len(*calls) != 1 {
		t.Fatalf("second pass invoked %d scripts, want 1 (dispatch only): %v", len(*calls), *calls)
	}

	// 11 minutes later the police cadence (10m) has elapsed; sweep (1h) has not.
	*calls = nil
	third := runSupervisorDuties(routerRoot, repoRoot, now.Add(11*time.Minute))
	byName = map[string]DutyResult{}
	for _, d := range third {
		byName[d.Name] = d
	}
	if d := byName["registry-police"]; !d.Ran {
		t.Errorf("registry-police due after 11m, got %+v", d)
	}
	if *policeRuns != 2 {
		t.Fatalf("registry police runs = %d, want 2", *policeRuns)
	}
	if d := byName["sweep"]; d.Ran || d.Skipped != "cadence" {
		t.Errorf("sweep must still be cadence-gated at 11m, got %+v", d)
	}
}

// TestRunSupervisorDuties_ErrorIsolated proves one failing duty never stops
// the others and never fails the pass — its error is reported on the result.
func TestRunSupervisorDuties_ErrorIsolated(t *testing.T) {
	routerRoot, repoRoot := scaffoldDutyScripts(t)
	calls := stubDutyRun(t, func(script, _ string) error {
		if filepath.Base(script) == "run-on-event.sh" {
			return errors.New("dispatch exploded")
		}
		return nil
	})
	stubRegistryPolice(t)

	results := runSupervisorDuties(routerRoot, repoRoot, time.Now())
	if len(*calls) != 2 {
		t.Fatalf("a failing duty must not stop the rest: invoked %v", *calls)
	}
	byName := map[string]DutyResult{}
	for _, d := range results {
		byName[d.Name] = d
	}
	if d := byName["dispatch-pump"]; !d.Ran || d.Error != "dispatch exploded" {
		t.Errorf("dispatch-pump = %+v, want ran with recorded error", d)
	}
	for _, name := range []string{"sweep", "registry-police"} {
		if d := byName[name]; !d.Ran || d.Error != "" {
			t.Errorf("%s = %+v, want clean run despite sibling failure", name, d)
		}
	}
}

// TestRunSupervisorDuties_MissingScriptsSkipCleanly covers dev clones and test
// scaffolds without the router scripts: every duty skips, nothing executes.
func TestRunSupervisorDuties_MissingScriptsSkipCleanly(t *testing.T) {
	repoRoot := t.TempDir()
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	if err := os.MkdirAll(routerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	calls := stubDutyRun(t, nil)
	reaperRuns := stubSessionReaper(t)
	policeRuns := stubRegistryPolice(t)

	results := runSupervisorDuties(routerRoot, repoRoot, time.Now())
	if len(*calls) != 0 {
		t.Fatalf("no script should run when none exist: %v", *calls)
	}
	for _, d := range results {
		if d.Name == "registry-police" || d.Name == "session-reaper" || d.Name == "auto-heal" || d.Name == "thread-census" || d.Name == "gemma-liveness" || d.Name == "launchd-kickstart" || d.Name == "stale-thread-reconcile" || d.Name == "spotlight-markers" {
			// Native duties: no script to be missing — they run (auto-heal is an
			// inert no-op until cmd/sirsi injects the real pass).
			if !d.Ran || d.Skipped != "" {
				t.Errorf("%s = %+v, want ran (native, script-independent)", d.Name, d)
			}
			continue
		}
		if d.Ran || d.Skipped != "script missing" {
			t.Errorf("%s = %+v, want skipped (script missing)", d.Name, d)
		}
	}
	if *reaperRuns != 1 {
		t.Fatalf("stubbed reaper runs = %d, want 1", *reaperRuns)
	}
	if *policeRuns != 1 {
		t.Fatalf("registry police runs = %d, want 1", *policeRuns)
	}
}

// TestSuperviseOnce_ReportsDuties proves the fold is wired into the supervisor
// pass itself (not a separate scheduler): a SuperviseOnce tick carries the
// duty results on its report.
func TestSuperviseOnce_ReportsDuties(t *testing.T) {
	routerRoot, repoRoot := scaffoldDutyScripts(t)
	if err := os.MkdirAll(filepath.Join(routerRoot, "items"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSupervisorRegistry(t, routerRoot, repoRoot)
	calls := stubDutyRun(t, nil)
	stubSessionReaper(t)
	stubRegistryPolice(t)

	report, err := SuperviseOnce(SuperviseOptions{
		RepoRoot: repoRoot,
		AgentID:  "horus-supervisor-test",
		PID:      os.Getpid(),
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Duties) != 10 {
		t.Fatalf("report.Duties = %d, want 10: %+v", len(report.Duties), report.Duties)
	}
	if len(*calls) != 2 {
		t.Fatalf("SuperviseOnce should have run both shell duties, invoked %v", *calls)
	}
	// The full work board is broadcast on every pass and persisted to board.json
	// (owner directive 2026-07-17) — every thread reads the same file.
	if report.WorkBoard == nil {
		t.Fatal("report.WorkBoard is nil — the board must broadcast every pass")
	}
	raw, err := os.ReadFile(filepath.Join(routerRoot, BoardFileName))
	if err != nil {
		t.Fatalf("board.json not written: %v", err)
	}
	if !strings.Contains(string(raw), "\"work_board\"") {
		t.Fatal("board.json missing the broadcast work_board block")
	}
}
