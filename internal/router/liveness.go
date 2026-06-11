// Package router — liveness.go
//
// OS-truth PID liveness for the CTR thread registry. A registered thread
// records the PID of its long-lived agent process; this is the single source
// that answers "is that process actually running right now?" against the live
// OS process table — not against a heartbeat timestamp.
//
// Why this exists: the registry classified threads as live/stale purely on
// heartbeat recency, and the original reaper used `kill -0` (signal 0), which
// a DEFUNCT (zombie, state Z) process answers successfully because it still
// occupies a slot in the table until its parent reaps it. The result was dead
// and zombie PIDs presenting as `active` forever (the CTR false-active bug).
// This primitive distinguishes alive / gone / defunct so the reaper and Horus
// agree with `ps`.
package router

import "sync"

const minAgentPID = 2

// PIDState is the OS-truth liveness of a recorded PID.
type PIDState string

const (
	// PIDAlive: the process exists and is not defunct — a real live agent.
	PIDAlive PIDState = "alive"
	// PIDGone: no such process in the table.
	PIDGone PIDState = "gone"
	// PIDDefunct: the process exists but is a zombie (state Z) awaiting reaping
	// by its parent. It answers `kill -0` but is NOT a live agent.
	PIDDefunct PIDState = "defunct"
	// PIDRecycled: a process exists at this PID, but its start time differs from
	// the one recorded at registration — the OS recycled the PID number onto a
	// DIFFERENT process (ADR-024 Amendment 1). The recorded thread's process is
	// gone; this is dead-for-reaping, distinguished from PIDGone so operators can
	// see that PID reuse (not a clean exit) is why the record was retired.
	PIDRecycled PIDState = "recycled"
	// PIDMismatched: a process exists at this PID with the expected start time,
	// but its command line no longer matches the registered agent surface. This
	// catches false-alive records where PID existence alone is not enough to
	// prove the thread still belongs to the recorded agent.
	PIDMismatched PIDState = "mismatched"
	// PIDUnknown: pid not recorded (<=0). Cannot verify — never treat as dead.
	PIDUnknown PIDState = "unknown"
)

// pidStateFn is the injectable liveness prober (Rule A16). Guarded by a
// RWMutex (Rule A21) so tests can install gone/defunct stubs without racing
// any concurrent reader.
var (
	pidStateMu sync.RWMutex
	pidStateFn = defaultPIDState
)

func getPIDStateFn() func(int) PIDState {
	pidStateMu.RLock()
	defer pidStateMu.RUnlock()
	return pidStateFn
}

// setPIDStateFn installs a prober (nil restores the platform default).
func setPIDStateFn(fn func(int) PIDState) {
	pidStateMu.Lock()
	defer pidStateMu.Unlock()
	if fn == nil {
		fn = defaultPIDState
	}
	pidStateFn = fn
}

// pidStartFn is the injectable process-start-time prober (Rule A16), the cheap
// generation discriminator that makes a PID a stable identity (ADR-024
// Amendment 1). Guarded by its own RWMutex (Rule A21). Returns the recorded
// process's start signature (e.g. `ps -o lstart=`), or "" when unavailable.
var (
	pidStartMu sync.RWMutex
	pidStartFn = defaultPIDStart
)

func getPIDStartFn() func(int) string {
	pidStartMu.RLock()
	defer pidStartMu.RUnlock()
	return pidStartFn
}

// setPIDStartFn installs a start-time prober (nil restores the platform default).
func setPIDStartFn(fn func(int) string) {
	pidStartMu.Lock()
	defer pidStartMu.Unlock()
	if fn == nil {
		fn = defaultPIDStart
	}
	pidStartFn = fn
}

// PIDStartTimeOf returns the OS start signature of pid (for capture at register
// time). Empty when the pid is invalid or the start time cannot be read.
func PIDStartTimeOf(pid int) string {
	if pid < minAgentPID {
		return ""
	}
	return getPIDStartFn()(pid)
}

// pidCommandFn is the injectable command-line prober (Rule A16). It gives the
// reaper a cheap identity check beyond "some process exists at this PID".
var (
	pidCommandMu sync.RWMutex
	pidCommandFn = defaultPIDCommand
)

func getPIDCommandFn() func(int) string {
	pidCommandMu.RLock()
	defer pidCommandMu.RUnlock()
	return pidCommandFn
}

func setPIDCommandFn(fn func(int) string) {
	pidCommandMu.Lock()
	defer pidCommandMu.Unlock()
	if fn == nil {
		fn = defaultPIDCommand
	}
	pidCommandFn = fn
}

// PIDStateOf returns the OS-truth liveness of pid, keyed on the COMPOSITE
// identity (pid, startedAt) — never a bare PID (ADR-024 Amendment 1). startedAt
// is the start signature recorded at registration; pass "" for legacy records
// (or callers that never captured one), which falls back to bare-PID semantics
// with zero behavior change. When the live process at pid has a DIFFERENT start
// time than startedAt, the PID was recycled onto another process and the result
// is PIDRecycled (dead-for-reaping). A non-positive pid is PIDUnknown.
func PIDStateOf(pid int, startedAt string) PIDState {
	if pid < minAgentPID {
		return PIDUnknown
	}
	state := getPIDStateFn()(pid)
	if state != PIDAlive {
		return state // gone/defunct/unknown already terminal — no need to disambiguate
	}
	if startedAt == "" {
		return PIDAlive // legacy record: no discriminator captured, keep bare-PID behavior
	}
	live := getPIDStartFn()(pid)
	if live == "" {
		return PIDAlive // cannot read start time — never falsely reap a live PID
	}
	if live != startedAt {
		return PIDRecycled // same PID number, different process — the recorded one is gone
	}
	return PIDAlive
}

// PIDStateOfThread extends PIDStateOf with a surface command-line check for
// known agent CLIs. It intentionally skips unknown/worker surfaces and empty
// command lines so the reaper never kills a legitimate custom integration just
// because this host cannot prove its command shape.
func PIDStateOfThread(t *Thread) PIDState {
	if t == nil {
		return PIDUnknown
	}
	state := PIDStateOf(t.PID, t.StartTime)
	if state != PIDAlive {
		return state
	}
	want := expectedAgentCommandNeedle(t.Surface, t.AgentID)
	if want == "" {
		return PIDAlive
	}
	cmd := getPIDCommandFn()(t.PID)
	if cmd == "" {
		return PIDAlive
	}
	if !containsFold(cmd, want) {
		return PIDMismatched
	}
	return PIDAlive
}

func expectedAgentCommandNeedle(surface, agentID string) string {
	switch surface {
	case "claude":
		return "claude"
	case "codex":
		return "codex"
	case "gemini":
		return "gemini"
	case "gemma":
		return "gemma"
	case "qwen":
		return "qwen"
	}
	switch {
	case containsFold(agentID, "claude"):
		return "claude"
	case containsFold(agentID, "codex"):
		return "codex"
	case containsFold(agentID, "gemini"):
		return "gemini"
	case containsFold(agentID, "gemma"):
		return "gemma"
	case containsFold(agentID, "qwen"):
		return "qwen"
	default:
		return ""
	}
}

func containsFold(s, substr string) bool {
	if substr == "" {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if equalFoldASCII(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// DeadByOSTruth reports whether a recorded PID is confirmed not running: gone
// entirely, defunct (Z) awaiting reaping, or recycled onto a different process.
// PIDUnknown is NOT dead — an unverifiable PID must never be reaped.
func DeadByOSTruth(state PIDState) bool {
	return state == PIDGone || state == PIDDefunct || state == PIDRecycled || state == PIDMismatched
}
