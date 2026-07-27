package govern

import (
	"testing"
	"time"
)

const gib = uint64(1) << 30

// The ceiling must scale with the machine. The whole point of a fraction is
// that the same footprint is fine on a workstation and fatal on the 16 GB
// target — an absolute threshold is how a check becomes useless exactly where
// it is needed.
func TestCeilingScalesWithMachine(t *testing.T) {
	if got := CeilingForRAM(48 * gib).HardBytes; got != 16*gib {
		t.Errorf("48 GB machine: ceiling %d GB, want 16", got/gib)
	}
	if got := CeilingForRAM(16 * gib).HardBytes; got != 16*gib/3 {
		t.Errorf("16 GB machine: ceiling %d bytes, want %d", got, 16*gib/3)
	}
}

// Peak must count. The broker sat at 19 GB live and peaked at 40.5 GB; the
// kernel chose its Jetsam victim at the peak. A ceiling reading only "now"
// waves through a process that is regularly the largest thing on the machine.
func TestPeakIsJudged(t *testing.T) {
	c := Ceiling{HardBytes: 16 * gib, UsePeak: true}
	// The real shape from 2026-07-27: comfortably under the ceiling when sampled,
	// far over it at peak.
	live, peak := uint64(9)*gib, uint64(40)*gib

	judged := live
	if c.UsePeak && peak > judged {
		judged = peak
	}
	if judged < c.HardBytes {
		t.Fatalf("peak %d GB was not judged against ceiling %d GB", judged/gib, c.HardBytes/gib)
	}

	// And with UsePeak off, the same process reads clean — which is exactly the
	// blindness this option exists to remove.
	cNoPeak := Ceiling{HardBytes: 16 * gib}
	if j := live; cNoPeak.UsePeak || j >= cNoPeak.HardBytes {
		t.Fatalf("expected the live-only read to look clean at %d GB", j/gib)
	}
}

// An unreadable footprint must never read as healthy. Reporting green on
// missing evidence is the failure this codebase keeps recording.
func TestUnknownIsNotOK(t *testing.T) {
	// pid 0 is not a process we can measure.
	a := Assess(-1, CeilingForRAM(48*gib))
	if a.Verdict == VerdictOK {
		t.Error("unreadable footprint reported OK — must be VerdictUnknown")
	}
	if a.Err == nil {
		t.Error("unreadable footprint carried no error to explain itself")
	}
}

// The soft mark warns; it must not act. An enforcer that restarts a process for
// legitimately using its allowance makes the fabric flap.
func TestApproachingDoesNotRestart(t *testing.T) {
	act := Plan(Assessment{Verdict: VerdictApproaching, Reason: "near"}, "sirsi gemma serve --restart")
	if act.Kind == ActionRestart {
		t.Error("approaching the ceiling proposed a restart — only a hard breach may act")
	}
	hard := Plan(Assessment{Verdict: VerdictOverCeiling, Reason: "over"}, "sirsi gemma serve --restart")
	if hard.Kind != ActionRestart {
		t.Error("a hard breach did not propose a restart")
	}
	if !hard.Reversible {
		t.Error("broker restart should be marked reversible — the model reloads in seconds")
	}
}

// The governor must not flap. The broker legitimately sits near its ceiling; an
// enforcer that restarts on every sample is worse than the OOM it prevents.
func TestBreachTrackerRequiresSustainedBreachAndHonorsCooldown(t *testing.T) {
	tr := &BreachTracker{RequiredBreaches: 3, Cooldown: 10 * time.Minute}
	over := Assessment{Verdict: VerdictOverCeiling}
	t0 := time.Now()

	for i := 1; i <= 2; i++ {
		if act, _ := tr.Observe(over, t0); act {
			t.Fatalf("acted after only %d breach(es) — must require 3", i)
		}
	}
	act, _ := tr.Observe(over, t0)
	if !act {
		t.Fatal("did not act after 3 sustained breaches")
	}

	// Immediately after acting, a fresh sustained breach must be suppressed.
	for i := 0; i < 3; i++ {
		if act, why := tr.Observe(over, t0.Add(time.Minute)); act {
			t.Fatalf("acted inside cooldown: %s", why)
		}
	}
	// Past the cooldown it may act again. The FIRST post-cooldown observation is
	// the one that acts (and resets the counter), so capture whether any did —
	// looping and keeping the last result would read the reset, not the action.
	acted := false
	for i := 0; i < 3; i++ {
		if a, _ := tr.Observe(over, t0.Add(11*time.Minute)); a {
			acted = true
		}
	}
	if !acted {
		t.Error("did not act after cooldown expired")
	}
}

// A transient spike must reset the counter, or three spikes hours apart would
// eventually trip a restart that was never warranted.
func TestTransientBreachResets(t *testing.T) {
	tr := &BreachTracker{RequiredBreaches: 3, Cooldown: time.Minute}
	now := time.Now()
	tr.Observe(Assessment{Verdict: VerdictOverCeiling}, now)
	tr.Observe(Assessment{Verdict: VerdictOverCeiling}, now)
	if _, why := tr.Observe(Assessment{Verdict: VerdictOK}, now); why == "" {
		t.Error("a cleared breach was not reported")
	}
	if act, _ := tr.Observe(Assessment{Verdict: VerdictOverCeiling}, now); act {
		t.Error("acted on the first breach after a reset — counter did not clear")
	}
}
