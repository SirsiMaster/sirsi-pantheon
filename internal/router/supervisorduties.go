// Package router — supervisorduties.go
//
// Single-backstop duty passes (backlog ruling 20260629-230327): the duties the
// three legacy router LaunchAgents used to carry (com.sirsi.idea-router
// dispatch pump, com.sirsi.idea-router-sweep hourly sweep,
// ai.sirsi.registry-police registry reap) are FOLDED into the one resident
// supervisor loop instead of living as separate launchd jobs. SuperviseOnce
// runs them as cadence-gated, bounded, error-isolated passes each tick.
//
// Phase-1 folding: each duty invokes the existing shell script shipped under
// .agents/idea-router/ as a subroutine. The scripts stay the single source of
// duty logic; only the SCHEDULER moves from three LaunchAgents into the
// supervisor. A missing script skips its duty cleanly (dev clones and test
// scaffolds don't carry them), and a failing script never fails the
// supervisor pass — its error is reported on the duty result and the loop
// moves on.
package router

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// dutyTimeout bounds one duty script run so a hung script can never wedge the
// supervisor loop (bounded, per the single-backstop ruling).
const dutyTimeout = 2 * time.Minute

// autoHealFn is the injected auto-heal pass (Rule A16/A21 accessor pattern).
// internal/autoheal imports this package, so the duty reaches it through this
// seam, wired by cmd/sirsi at startup; the default is an inert no-op.
var (
	autoHealMu sync.RWMutex
	autoHealFn = func(routerRoot, repoRoot string) error { return nil }
)

func getAutoHealFn() func(string, string) error {
	autoHealMu.RLock()
	defer autoHealMu.RUnlock()
	return autoHealFn
}

// SetAutoHealFn installs the auto-heal pass the supervisor's auto-heal duty runs.
func SetAutoHealFn(fn func(routerRoot, repoRoot string) error) {
	autoHealMu.Lock()
	defer autoHealMu.Unlock()
	if fn != nil {
		autoHealFn = fn
	}
}

// SupervisorDuty describes one folded duty pass.
type SupervisorDuty struct {
	Name      string                                  // duty id, stable (used for the cadence stamp file)
	ScriptRel string                                  // script path relative to the router root ("" for Go duties)
	GoRun     func(routerRoot, repoRoot string) error // native duty; takes precedence over ScriptRel
	Cadence   time.Duration                           // 0 = every tick
}

// supervisorDuties returns the three folded duties. Cadences mirror the legacy
// LaunchAgents they replace: dispatch pumped every tick (was FSEvents-driven),
// sweep hourly (was StartInterval 3600), registry police every 10 minutes
// (was StartInterval 600).
func supervisorDuties() []SupervisorDuty {
	return []SupervisorDuty{
		{Name: "dispatch-pump", ScriptRel: "run-on-event.sh", Cadence: 0},
		{Name: "sweep", ScriptRel: "sweep.sh", Cadence: time.Hour},
		{Name: "registry-police", ScriptRel: filepath.Join("police", "registry-police.sh"), Cadence: 10 * time.Minute},
		// Auto-heal (ADR-039 P3): the monitor→identify→FIX pass, gated on
		// autonomous mode + GateAction inside the hook. Injected from cmd/sirsi
		// (internal/autoheal imports this package — a direct import would cycle).
		// The default no-ops, so library consumers without the wiring are inert.
		{Name: "auto-heal", GoRun: func(routerRoot, repoRoot string) error {
			return getAutoHealFn()(routerRoot, repoRoot)
		}, Cadence: 5 * time.Minute},
		// Universal Thread Census (A33): every agent-class infra process —
		// including GPU servers — becomes a registered thread, no misses;
		// a future process is caught within one cadence of first launch.
		{Name: "thread-census", GoRun: RunCensusDuty, Cadence: 10 * time.Minute},
		// Gemma liveness (A32, owner directive 2026-07-17): the local LLM must
		// survive on Pantheon's survival, not the IDE. This resident supervisor
		// (always-on, IDE-independent) probes the broker and RESTORES it — the
		// `ai.sirsi.gemma` LaunchAgent only starts it at boot. 2-min cadence
		// because gemma is the Tier-0 substrate everything depends on.
		{Name: "gemma-liveness", GoRun: func(routerRoot, repoRoot string) error {
			return getGemmaLivenessFn()(routerRoot, repoRoot)
		}, Cadence: 2 * time.Minute},
		// Session reaper (2026-07-08 wakeup-leak RCA): native Go — it needs
		// process grouping + SIGTERM, not a shell subroutine. See sessionreaper.go.
		{Name: "session-reaper", GoRun: runSessionReaperDuty, Cadence: 10 * time.Minute},
		// Dead-label kickstart (local sovereignty P1, owner directive
		// 2026-07-23): every managed LaunchAgent plist on disk must be loaded
		// in launchd — the reboot proved a label that dies at boot stays dead
		// until a CLOUD agent notices, which inverts the sovereignty order.
		// Deterministic, no LLM; inert seam wired by cmd/sirsi (tests never
		// shell real launchctl). See launchdkickstart.go.
		{Name: "launchd-kickstart", GoRun: func(routerRoot, repoRoot string) error {
			return getLaunchdKickstartFn()(routerRoot, repoRoot)
		}, Cadence: 5 * time.Minute},
	}
}

// DutyResult reports one duty pass on the SuperviseReport.
type DutyResult struct {
	Name       string `json:"name"`
	Ran        bool   `json:"ran"`
	Skipped    string `json:"skipped,omitempty"` // "cadence" | "script missing"
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// dutyRunFn indirects the script execution so tests can stub it (Rule A16).
// Production runs the real script, bounded by dutyTimeout.
var dutyRunFn = runDutyScript

// runDutyScript executes one duty script from the repo root with a bounded
// deadline. Output is discarded — the scripts keep their own logs under
// .agents/idea-router/logs (unchanged from their LaunchAgent life).
func runDutyScript(script, repoRoot string) error {
	ctx, cancel := context.WithTimeout(context.Background(), dutyTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, script)
	cmd.Dir = repoRoot
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timed out after %s", dutyTimeout)
		}
		return err
	}
	return nil
}

// dutyStampPath is the cadence stamp for one duty. The stamp's mtime is the
// last attempt time; file-based so the cadence survives supervisor restarts
// (a relaunched loop must not re-fire an hourly sweep it ran 5 minutes ago).
func dutyStampPath(routerRoot, name string) string {
	return filepath.Join(routerRoot, "logs", "duty-"+name+".stamp")
}

// dutyDue reports whether a duty's cadence has elapsed since its last attempt.
func dutyDue(routerRoot string, d SupervisorDuty, now time.Time) bool {
	if d.Cadence <= 0 {
		return true
	}
	info, err := os.Stat(dutyStampPath(routerRoot, d.Name))
	if err != nil {
		return true // never attempted
	}
	return now.Sub(info.ModTime()) >= d.Cadence
}

// touchDutyStamp records an attempt (success or failure) so a failing script
// retries on its cadence, not hot-looped every tick.
func touchDutyStamp(routerRoot string, d SupervisorDuty, now time.Time) {
	if d.Cadence <= 0 {
		return // every-tick duties need no stamp
	}
	path := dutyStampPath(routerRoot, d.Name)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(now.UTC().Format(time.RFC3339)+"\n"), 0o644); err == nil {
		_ = os.Chtimes(path, now, now)
	}
}

// runSupervisorDuties executes every due duty, error-isolated: one duty's
// failure is recorded on its result and the next duty still runs. It never
// returns an error — the supervisor pass must not die because a subroutine
// script did.
func runSupervisorDuties(routerRoot, repoRoot string, now time.Time) []DutyResult {
	duties := supervisorDuties()
	results := make([]DutyResult, 0, len(duties))
	for _, d := range duties {
		res := DutyResult{Name: d.Name}
		if d.GoRun == nil {
			if _, err := os.Stat(filepath.Join(routerRoot, d.ScriptRel)); err != nil {
				res.Skipped = "script missing"
				results = append(results, res)
				continue
			}
		}
		if !dutyDue(routerRoot, d, now) {
			res.Skipped = "cadence"
			results = append(results, res)
			continue
		}
		start := time.Now()
		if d.GoRun != nil {
			if err := d.GoRun(routerRoot, repoRoot); err != nil {
				res.Error = err.Error()
			}
		} else if err := dutyRunFn(filepath.Join(routerRoot, d.ScriptRel), repoRoot); err != nil {
			res.Error = err.Error()
		}
		res.Ran = true
		res.DurationMS = time.Since(start).Milliseconds()
		touchDutyStamp(routerRoot, d, now)
		results = append(results, res)
	}
	return results
}
