package govern

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Action is what the governor proposes doing about a ceiling breach.
//
// Deliberately a PROPOSAL, not an execution. The process most likely to breach
// a ceiling on a developer's machine is Sirsi's own model broker, and silently
// killing someone's local AI is not a remedy they should discover afterwards
// (Rule A1: preview mutates nothing).
type Action struct {
	Kind    ActionKind
	PID     int
	Command string // the exact command an operator or an autonomous mode would run
	Why     string
	// Reversible reports whether the action loses state. A broker restart is
	// reversible — the model reloads in seconds. It is recorded so a surface can
	// distinguish "safe to automate" from "ask first".
	Reversible bool
}

type ActionKind int

const (
	ActionNone ActionKind = iota
	ActionObserve
	ActionRestart
)

// Plan converts an assessment into a proposed action.
//
// The escalation is deliberately gentle. VerdictApproaching does NOT act: a
// process legitimately near its ceiling is doing its job, and an enforcer that
// restarts on the soft mark makes the fabric flap. Only a hard breach proposes
// a restart, because at that point the alternative is the kernel choosing a
// victim — and it will not choose the broker just because the broker is at
// fault. On 2026-07-27 the three Jetsams could have taken any load-bearing
// process on the machine.
func Plan(a Assessment, restartCmd string) Action {
	switch a.Verdict {
	case VerdictOverCeiling:
		return Action{
			Kind:       ActionRestart,
			PID:        a.PID,
			Command:    restartCmd,
			Why:        a.Reason + " — restarting now is cheaper than letting the kernel pick a victim",
			Reversible: true,
		}
	case VerdictApproaching:
		return Action{
			Kind: ActionObserve,
			PID:  a.PID,
			Why:  a.Reason + " — watching; not acting on the soft mark",
		}
	case VerdictUnknown:
		return Action{
			Kind: ActionObserve,
			PID:  a.PID,
			Why:  a.Reason + " — cannot judge, so not acting",
		}
	default:
		return Action{Kind: ActionNone, PID: a.PID, Why: a.Reason}
	}
}

// Apply executes an action. Callers must gate this behind explicit consent —
// autonomous mode, an operator confirm, or a policy tier. Apply itself does not
// ask; it is the hands, not the judgement.
func Apply(a Action, timeout time.Duration) error {
	if a.Kind != ActionRestart || strings.TrimSpace(a.Command) == "" {
		return nil
	}
	parts := strings.Fields(a.Command)
	cmd := exec.Command(parts[0], parts[1:]...) //nolint:gosec // command is constructed by the caller, never user input
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("govern: start %q: %w", a.Command, err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("govern: %q: %w", a.Command, err)
		}
		return nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return fmt.Errorf("govern: %q timed out after %s", a.Command, timeout)
	}
}
