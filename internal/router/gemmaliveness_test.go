package router

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/liveness"
)

// installGemmaFakes swaps the probe + serve seams and returns a restore fn and a
// pointer to the recorded serve calls ("start" or "restart").
func installGemmaFakes(t *testing.T, status liveness.GemmaStatus) *[]string {
	t.Helper()
	oldProbe := getGemmaProbeFn()
	oldServe := getGemmaServeFn()
	var calls []string
	setGemmaProbeFn(func(string) (liveness.GemmaStatus, string) { return status, "test" })
	setGemmaServeFn(func(restart bool) error {
		if restart {
			calls = append(calls, "restart")
		} else {
			calls = append(calls, "start")
		}
		return nil
	})
	gemmaWedgeStrikes = 0
	t.Cleanup(func() {
		setGemmaProbeFn(oldProbe)
		setGemmaServeFn(oldServe)
		gemmaWedgeStrikes = 0
	})
	return &calls
}

// TestGemmaLiveness_HealthyDoesNothing: a healthy broker is never touched.
func TestGemmaLiveness_HealthyDoesNothing(t *testing.T) {
	calls := installGemmaFakes(t, liveness.GemmaHealthy)
	if err := RunGemmaLivenessDuty("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Errorf("healthy broker triggered serve calls %v, want none", *calls)
	}
}

// TestGemmaLiveness_DownStartsImmediately: a down broker is started at once (no
// process to protect — the common post-crash gap).
func TestGemmaLiveness_DownStartsImmediately(t *testing.T) {
	calls := installGemmaFakes(t, liveness.GemmaDown)
	if err := RunGemmaLivenessDuty("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0] != "start" {
		t.Errorf("down broker → calls %v, want [start]", *calls)
	}
}

// TestGemmaLiveness_WedgedRestartsOnlyAfterConfirmation is the anti-thrash
// guard: a wedged broker is NOT restarted on the first observation (could be a
// transient), only after the strike threshold — and the restart is graceful.
func TestGemmaLiveness_WedgedRestartsOnlyAfterConfirmation(t *testing.T) {
	calls := installGemmaFakes(t, liveness.GemmaWedged)

	// First tick: one strike, no action.
	if err := RunGemmaLivenessDuty("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Fatalf("first wedged tick restarted the broker (%v) — must wait for confirmation", *calls)
	}
	// Second tick: threshold reached → graceful restart.
	if err := RunGemmaLivenessDuty("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0] != "restart" {
		t.Errorf("confirmed wedge → calls %v, want [restart] (graceful stop+start, never SIGKILL)", *calls)
	}
}

// TestGemmaLiveness_TransientWedgeResets: a wedge that recovers before the
// threshold never triggers a restart (the strike counter resets on healthy).
func TestGemmaLiveness_TransientWedgeResets(t *testing.T) {
	oldProbe := getGemmaProbeFn()
	oldServe := getGemmaServeFn()
	var calls []string
	state := liveness.GemmaWedged
	setGemmaProbeFn(func(string) (liveness.GemmaStatus, string) { return state, "test" })
	setGemmaServeFn(func(restart bool) error { calls = append(calls, "call"); return nil })
	gemmaWedgeStrikes = 0
	t.Cleanup(func() { setGemmaProbeFn(oldProbe); setGemmaServeFn(oldServe); gemmaWedgeStrikes = 0 })

	_ = RunGemmaLivenessDuty("", "") // strike 1
	state = liveness.GemmaHealthy
	_ = RunGemmaLivenessDuty("", "") // recovered → reset
	state = liveness.GemmaWedged
	_ = RunGemmaLivenessDuty("", "") // strike 1 again, not 2

	if len(calls) != 0 {
		t.Errorf("a transient wedge that recovered triggered %v, want no restart", calls)
	}
}
