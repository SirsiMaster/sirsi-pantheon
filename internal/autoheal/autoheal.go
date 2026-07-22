// Package autoheal — the autonomous monitor→identify→FIX loop (ADR-039 P3;
// router items 20260713-213507 + 20260714-170033).
//
// One bounded pass: when autonomous mode is ON (brain.AutonomousMode(), the
// #203 single source of truth, default OFF), run the health doctor and apply
// the remediation lever of every Warn+ finding — through TWO lines of defense:
//
//  1. The lever whitelist: only the doctor's own remediation catalog is ever
//     executed (every Fix is a `sirsi …` verb, itself safety-gated per A1).
//  2. router.GateAction on the action text (the previously-unwired second
//     line): a Gated decision downgrades the fix to a PROPOSAL — surfaced,
//     never applied.
//
// OFF = propose: the findings stay on every existing surface (diagnose,
// menubar, dashboard) with their Fix buttons; this pass simply does nothing.
// Every applied fix is inscribed to the Stele — no black box (A29).
package autoheal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

const (
	// maxFixesPerPass bounds one pass so a cascade of findings can never turn
	// the healer into a storm of side effects.
	maxFixesPerPass = 3
	// fixCooldown: the same lever is not re-applied within this window — a
	// relief lever that didn't clear a finding in an hour needs the owner, not
	// a hot loop re-running it every 5 minutes.
	fixCooldown = time.Hour
	execTimeout = 2 * time.Minute
)

// Injectable seams (Rule A16 + A21).
var (
	mu           sync.RWMutex
	autonomousFn = func() bool {
		cfg, err := brain.LoadConfig()
		return err == nil && cfg.AutonomousMode()
	}
	doctorFn = func() ([]guard.DiagnosticFinding, error) {
		rep, err := guard.Doctor()
		if err != nil {
			return nil, err
		}
		return rep.Findings, nil
	}
	gateFn = router.GateAction
	execFn = func(argv []string) error {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		cmd := exec.Command(self, argv[1:]...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			return err
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			return err
		case <-time.After(execTimeout):
			_ = cmd.Process.Kill()
			return fmt.Errorf("fix timed out after %s", execTimeout)
		}
	}
	statePathFn = func() string {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".sirsi", "autoheal-state.json")
	}
	// inscribeFn seams the provenance write so unit tests never pollute the
	// LIVE Stele (the first suite run wrote its fixtures into ~/.config/ra).
	inscribeFn = stele.Inscribe
)

func setSeams(auto func() bool, doc func() ([]guard.DiagnosticFinding, error), gate func(string) router.GateDecision, ex func([]string) error, state func() string) {
	mu.Lock()
	defer mu.Unlock()
	inscribeFn = func(string, string, string, map[string]string) {} // tests never write the live Stele
	if auto != nil {
		autonomousFn = auto
	}
	if doc != nil {
		doctorFn = doc
	}
	if gate != nil {
		gateFn = gate
	}
	if ex != nil {
		execFn = ex
	}
	if state != nil {
		statePathFn = state
	}
}

// Outcome of one finding in a pass.
type Outcome struct {
	Check   string `json:"check"`
	Fix     string `json:"fix"`
	Applied bool   `json:"applied"`
	Reason  string `json:"reason"` // applied | gated-proposal | cooldown | fix-failed: … | budget
}

// Run executes one bounded auto-heal pass. It is the supervisor-duty entry
// point: silent no-op when autonomous is OFF, error-isolated by the caller.
func Run(_, _ string) error {
	mu.RLock()
	auto, doc, gate, ex, statePath, inscribe := autonomousFn, doctorFn, gateFn, execFn, statePathFn(), inscribeFn
	mu.RUnlock()

	if !auto() {
		return nil // OFF = propose: existing surfaces already carry the findings
	}
	findings, err := doc()
	if err != nil {
		return fmt.Errorf("auto-heal doctor: %w", err)
	}

	lastRun := loadState(statePath)
	now := time.Now().UTC()
	applied := 0
	var outcomes []Outcome

	for _, f := range findings {
		if f.Severity < guard.SeverityWarn || f.Fix == "" {
			continue
		}
		o := Outcome{Check: f.Check, Fix: f.Fix}
		action := fmt.Sprintf("%s: %s → %s", f.Check, f.Message, f.Fix)
		switch {
		case applied >= maxFixesPerPass:
			o.Reason = "budget"
		case !strings.HasPrefix(f.Fix, "sirsi "):
			o.Reason = "not-a-sirsi-verb" // whitelist: catalog levers only
		case gate(action).Gated:
			o.Reason = "gated-proposal" // second line of defense (ADR-039 P3)
		case now.Sub(lastRun[f.Fix]) < fixCooldown:
			o.Reason = "cooldown"
		default:
			if runErr := ex(strings.Fields(f.Fix)); runErr != nil {
				o.Reason = "fix-failed: " + runErr.Error()
			} else {
				o.Applied, o.Reason = true, "applied"
				applied++
				lastRun[f.Fix] = now
			}
		}
		outcomes = append(outcomes, o)
	}

	if len(outcomes) > 0 {
		saveState(statePath, lastRun)
		for _, o := range outcomes {
			inscribe("autoheal", "auto_heal", o.Check, map[string]string{
				"fix": o.Fix, "applied": fmt.Sprintf("%t", o.Applied), "reason": o.Reason,
			})
		}
	}
	return nil
}

func loadState(path string) map[string]time.Time {
	state := map[string]time.Time{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &state)
	}
	return state
}

func saveState(path string, state map[string]time.Time) {
	if b, err := json.MarshalIndent(state, "", " "); err == nil {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, b, 0o644)
	}
}
