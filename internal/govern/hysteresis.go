package govern

import "time"

// Breach tracking exists so the governor cannot flap.
//
// A single over-ceiling reading is not grounds to act. The broker sits at
// ~19 GB against a 16 GB ceiling as normal operation for a 13 GB model, and an
// enforcer that restarts on every sample would put the local AI in a permanent
// reboot loop — strictly worse than the OOM it is preventing.
//
// This mirrors the repeat-OOM doctrine already in this fabric: a second event
// is an investigation, not a restart. Here it becomes mechanical — act only on
// SUSTAINED breach, then stand down for a cooldown so a restart is never
// immediately followed by another.
type BreachTracker struct {
	// Consecutive breaches required before acting. Two samples of a spike is
	// noise; sustained breach across N samples is a condition.
	RequiredBreaches int
	// Cooldown after acting, during which no further action is proposed. The
	// broker needs time to reload its model and settle before being judged again
	// — measured at ~5s to serve, longer to reach steady state.
	Cooldown time.Duration

	consecutive int
	lastAction  time.Time
}

// NewBreachTracker returns a tracker with defaults tuned for a model server:
// three sustained breaches, ten-minute cooldown.
func NewBreachTracker() *BreachTracker {
	return &BreachTracker{RequiredBreaches: 3, Cooldown: 10 * time.Minute}
}

// Observe records one assessment and reports whether the governor should act
// NOW. It is the only place allowed to say yes.
func (b *BreachTracker) Observe(a Assessment, now time.Time) (act bool, why string) {
	if b.RequiredBreaches <= 0 {
		b.RequiredBreaches = 3
	}
	if a.Verdict != VerdictOverCeiling {
		if b.consecutive > 0 {
			b.consecutive = 0
			return false, "breach cleared before it became sustained"
		}
		return false, ""
	}

	b.consecutive++
	if b.consecutive < b.RequiredBreaches {
		return false, "over ceiling, but not yet sustained"
	}
	if !b.lastAction.IsZero() && now.Sub(b.lastAction) < b.Cooldown {
		return false, "sustained breach, but within cooldown from the last action"
	}
	b.lastAction = now
	b.consecutive = 0
	return true, "sustained breach past cooldown"
}
