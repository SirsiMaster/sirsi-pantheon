package router

import (
	"os"
	"strings"
	"time"
)

// Last-consumer outcomes a wake loop publishes (LaneState.LastOutcome).
const (
	OutcomeOK           = "ok"            // the last consumer drained or reduced the inbox
	OutcomeNoProgress   = "no_progress"   // it ran and changed nothing
	OutcomeAuthRequired = "auth_required" // the consumer cannot log in
	OutcomeRelay        = "relay_unreach" // the consumer cannot reach the router relay
	OutcomeStartFailed  = "start_failed"  // the consumer command would not start
	OutcomeExitedError  = "exited_error"  // it started and exited with an error
	OutcomeNoneYet      = ""              // no consumer has run since this loop started
	HoldWindow          = "window"        // a Ma'at measurement window is open
	HoldLoad            = "load"          // host load is at or above the core count
	HoldBackoff         = "backoff"       // waiting out a no-progress back-off
	HoldQuarantine      = "quarantine"    // fabric or lane quarantined: needs a human
	HoldCeiling         = "ceiling"       // hourly spawn ceiling reached
)

// LaneState is a worker's self-reported readiness, published on every heartbeat.
type LaneState struct {
	ConsumerDeclared bool      `json:"consumer_declared"`
	LastOutcome      string    `json:"last_outcome,omitempty"`
	LastOutcomeAt    time.Time `json:"last_outcome_at,omitempty"`
	LastDetail       string    `json:"last_detail,omitempty"`
	Hold             string    `json:"hold,omitempty"`
	HoldUntil        time.Time `json:"hold_until,omitempty"` // zero = until the condition clears
	LastProgressAt   time.Time `json:"last_progress_at,omitempty"`
	PublishedAt      time.Time `json:"published_at,omitempty"`
}

// ClassifyConsumerFailure turns a failed consumer's tail output into one of the
// outcomes above. It is deliberately a short list of phrases the fabric has
// actually produced (2026-09-30/10-01), not a guess at every failure.
func ClassifyConsumerFailure(tail string) string {
	if k := KnownFailure(tail); k != "" {
		return k
	}
	return OutcomeExitedError
}

// KnownFailure returns the outcome for a recognized failure phrase in a
// consumer's tail output, or "" when nothing matches (so the caller can fall
// back to no_progress / exited_error from the process facts).
func KnownFailure(tail string) string {
	t := strings.ToLower(tail)
	switch {
	case strings.Contains(t, "not logged in"), strings.Contains(t, "please run /login"),
		strings.Contains(t, "oauth access token is invalid"), strings.Contains(t, "failed to authenticate"):
		return OutcomeAuthRequired
	case strings.Contains(t, "requests in flight"), strings.Contains(t, "relay stalled"),
		strings.Contains(t, "spool:"):
		return OutcomeRelay
	}
	return ""
}

// Ping verdicts.
const (
	VerdictLive         = "LIVE"          // an attended session is consuming now
	VerdictWakeable     = "WAKEABLE"      // a worker loop with a working consumer; it will start one
	VerdictHeld         = "HELD"          // would work, but is gated right now
	VerdictAuthRequired = "AUTH_REQUIRED" // the consumer cannot log in
	VerdictWatchOnly    = "WATCH_ONLY"    // a loop exists but declares no consumer
	VerdictUnstaffed    = "UNSTAFFED"     // no worker path at all
	VerdictUnreachable  = "UNREACHABLE"   // a worker existed but has stopped reporting
)

// PingResult is what a sender is told about a recipient before sending.
type PingResult struct {
	Agent   string `json:"agent"`
	Verdict string `json:"verdict"`
	Detail  string `json:"detail"`
	// Workable is true when sending will lead to the item being worked without
	// anyone doing anything else (LIVE, WAKEABLE, or a HELD that will clear).
	Workable bool   `json:"workable"`
	Thread   string `json:"thread,omitempty"`
}

// attendedSurface reports whether a thread surface is an interactive session
// (it IS the consumer) rather than a headless worker loop.
func attendedSurface(surface string) bool {
	switch surface {
	case "claude", "codex", "gemini", "qwen":
		return true
	}
	return false
}

// PingLane computes the honest availability of agentID from the thread registry
// and its declared config. It reads state only: no model call, no process
// spawn, no network. A static "launch job installed + heartbeat fresh" is NOT
// enough for WAKEABLE — the loop must also declare a working consumer.
func PingLane(reg *ThreadRegistry, cfg AgentConfig, agentID string, now time.Time) PingResult {
	res := PingResult{Agent: agentID}
	var worker, attended, stale *Thread
	for _, t := range reg.SortedThreads() {
		watches := t.AgentID == agentID
		for _, w := range t.Watches {
			if w == agentID {
				watches = true
			}
		}
		if !watches || t.Status.IsTerminal() || t.Status == ThreadStatusSuspended {
			continue
		}
		if now.Sub(t.LastSeenAt) > DefaultThreadStaleAfter {
			if stale == nil || t.LastSeenAt.After(stale.LastSeenAt) {
				stale = t
			}
			continue
		}
		if attendedSurface(t.Surface) {
			attended = t
		} else if worker == nil || t.LastSeenAt.After(worker.LastSeenAt) {
			worker = t
		}
	}
	switch {
	case attended != nil:
		res.Verdict, res.Workable, res.Thread = VerdictLive, true, attended.ThreadID
		res.Detail = "an attended " + attended.Surface + " session is consuming this inbox"
	case worker != nil:
		res.Thread = worker.ThreadID
		l := worker.Lane
		switch {
		case !worker.ConsumerCapable || (l != nil && !l.ConsumerDeclared):
			res.Verdict = VerdictWatchOnly
			res.Detail = "a wake loop is running but declares no working consumer, so it will watch the inbox and never work it"
		case l != nil && l.LastOutcome == OutcomeAuthRequired:
			res.Verdict = VerdictAuthRequired
			res.Detail = "the consumer cannot log in: " + trimDetail(l.LastDetail)
		case l != nil && l.Hold != "":
			res.Verdict, res.Workable = VerdictHeld, l.Hold != HoldQuarantine
			res.Detail = "held: " + l.Hold
			if !l.HoldUntil.IsZero() {
				res.Detail += " until " + l.HoldUntil.Local().Format("15:04:05")
			}
			if l.Hold == HoldQuarantine {
				res.Detail += " (needs a human)"
			}
		case l != nil && l.LastOutcome == OutcomeRelay:
			res.Verdict = VerdictHeld
			res.Detail = "the consumer cannot reach the router relay: " + trimDetail(l.LastDetail)
		default:
			res.Verdict, res.Workable = VerdictWakeable, true
			res.Detail = "a worker loop with a working consumer will start a session"
			switch {
			case l == nil:
				res.Detail += " (this loop predates lane-state publishing: readiness declared, not proven)"
			case l.LastOutcome == OutcomeOK:
				res.Detail += " (last run made progress " + ago(now, l.LastProgressAt) + ")"
			case l.LastOutcome == OutcomeNoneYet:
				res.Detail += " (no consumer run since this loop started: not yet proven)"
			}
		}
	case stale != nil:
		res.Verdict, res.Thread = VerdictUnreachable, stale.ThreadID
		res.Detail = "its worker last reported " + ago(now, stale.LastSeenAt) + " on " + hostOrUnknown(stale.Host)
	default:
		res.Verdict = VerdictUnstaffed
		mech := ExplicitWakeMechanism(cfg)
		switch mech {
		case "", WakeNone:
			res.Detail = "no worker path: wake mechanism is none and no session is registered"
		default:
			res.Detail = "wake mechanism " + mech + " is declared but no worker is reporting"
		}
	}
	return res
}

func trimDetail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	if s == "" {
		return "(no detail)"
	}
	return s
}

func hostOrUnknown(h string) string {
	if h == "" {
		return "an unknown host"
	}
	return h
}

func ago(now, t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t).Round(time.Second)
	if d < 0 {
		d = 0
	}
	return d.String() + " ago"
}

// currentHold reports, without logging or side effects, what (if anything) is
// holding this lane's dispatch right now. Order mirrors the dispatch gate.
func currentHold(agentID string, fruitless int, nextDispatchAllowed time.Time) (string, time.Time) {
	if fruitless >= wakeLoopFruitlessQuarantine {
		return HoldQuarantine, time.Time{}
	}
	if home, err := os.UserHomeDir(); err == nil && IsFabricQuarantined(home) {
		return HoldQuarantine, time.Time{}
	}
	if _, open := railsLockHolder(); open {
		return HoldWindow, time.Time{}
	}
	if hold, _, _ := shouldDeferDispatch(); hold {
		return HoldLoad, time.Time{}
	}
	if time.Now().Before(nextDispatchAllowed) {
		return HoldBackoff, nextDispatchAllowed
	}
	if over, _ := spawnCeilingReached(agentID, time.Now()); over {
		return HoldCeiling, time.Time{}
	}
	return "", time.Time{}
}
