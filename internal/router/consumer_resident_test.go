package router

import (
	"strings"
	"testing"
	"time"
)

// H6 acceptance: "the wake watcher never borrows resident capability; a resident
// that stops publishing stops being credited."
//
// The defect these lock down: resolveResidentConsumer used to credit a lane by
// RUNNING consumer.health_check. gemma-pantheon declared
// `sirsi-gemma-worker.sh --version`, which proves a binary can print a string.
// It exits 0 identically whether the worker is draining an inbox or was stopped
// an hour ago — so the credit could never decay, and the watcher was asserting
// liveness on the resident's behalf.

func withResidentThreadFn(t *testing.T, fn func(string, string, time.Time) (bool, string)) {
	t.Helper()
	orig := residentConsumerThreadFn
	residentConsumerThreadFn = fn
	t.Cleanup(func() { residentConsumerThreadFn = orig })
}

// A resident with a published, live, consumer-capable thread IS credited — and
// notably with NO health_check declared at all, which the old code refused.
func TestResidentCreditedOnPublishedThreadWithoutHealthCheck(t *testing.T) {
	withResidentThreadFn(t, func(string, string, time.Time) (bool, string) { return true, "" })

	cfg := AgentConfig{ID: "gemma-pantheon", Consumer: ConsumerConfig{Mode: ConsumerModeResident}}
	got, reason := ResolveConsumer(cfg, "/tmp/router")
	if got == nil {
		t.Fatalf("expected resident credited on its published thread, refused: %s", reason)
	}
	if !got.Resident {
		t.Error("resolved consumer must be marked Resident")
	}
}

// The core of H6: stop publishing, stop being credited. No probe can rescue it.
func TestResidentNotCreditedWhenItStopsPublishing(t *testing.T) {
	withResidentThreadFn(t, func(string, string, time.Time) (bool, string) {
		return false, "resident's thread is not an active, heartbeat-fresh, consumer-capable record"
	})

	cfg := AgentConfig{
		ID: "gemma-pantheon",
		Consumer: ConsumerConfig{
			Mode: ConsumerModeResident,
			// The exact declaration that used to qualify the lane on its own.
			HealthCheck: []string{"echo", "gemma-pantheon"},
		},
	}
	got, reason := ResolveConsumer(cfg, "/tmp/router")
	if got != nil {
		t.Fatal("a resident that stopped publishing must NOT be credited, even with a passing health_check")
	}
	if !strings.Contains(reason, "not credited") {
		t.Errorf("refusal must say the resident was not credited, got: %q", reason)
	}
}

// A passing health_check must never by itself qualify a lane — that is the
// borrowing this task removes.
func TestPassingHealthCheckAloneCannotQualify(t *testing.T) {
	withResidentThreadFn(t, func(string, string, time.Time) (bool, string) {
		return false, "resident published no thread"
	})

	cfg := AgentConfig{
		ID:       "gemma-pantheon",
		Consumer: ConsumerConfig{Mode: ConsumerModeResident, HealthCheck: []string{"true"}},
	}
	if got, _ := ResolveConsumer(cfg, "/tmp/router"); got != nil {
		t.Fatal("`true` exits 0 — a probe that always passes must not credit a resident")
	}
}

// A declared check that FAILS is still a real negative and may disqualify.
func TestFailingHealthCheckStillDisqualifies(t *testing.T) {
	withResidentThreadFn(t, func(string, string, time.Time) (bool, string) { return true, "" })

	cfg := AgentConfig{
		ID:       "gemma-pantheon",
		Consumer: ConsumerConfig{Mode: ConsumerModeResident, HealthCheck: []string{"false"}},
	}
	got, reason := ResolveConsumer(cfg, "/tmp/router")
	if got != nil {
		t.Fatal("a failing declared health_check must still disqualify")
	}
	if !strings.Contains(reason, "health_check failed") {
		t.Errorf("expected health_check failure reason, got: %q", reason)
	}
}

// The registry lookup itself must fail CLOSED: an unreadable registry is not
// evidence of a live consumer.
func TestResidentThreadLookupFailsClosed(t *testing.T) {
	ok, why := residentConsumerThread(t.TempDir()+"/definitely-absent", "gemma-pantheon", time.Now())
	if ok {
		t.Fatal("an unreadable/absent registry must never credit a resident")
	}
	if why == "" {
		t.Error("refusal must carry an operator-readable reason")
	}
}

// Staleness is what makes the credit decay on its own — the property a probe
// could never have.
func TestPublishedThreadDecaysWhenHeartbeatGoesStale(t *testing.T) {
	now := time.Now()
	reg := &ThreadRegistry{Threads: map[string]*Thread{"thr-resident": {
		ThreadID:        "thr-resident",
		AgentID:         "gemma-pantheon",
		Surface:         "worker",
		Status:          ThreadStatusActive,
		ConsumerCapable: true,
		LastSeenAt:      now.Add(-2 * DefaultThreadStaleAfter),
	}}}
	root := t.TempDir()
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Skipf("cannot persist registry in this environment: %v", err)
	}
	if ok, _ := residentConsumerThread(root, "gemma-pantheon", now); ok {
		t.Error("a thread whose heartbeat aged past DefaultThreadStaleAfter must lose the credit")
	}
	// Same record, fresh heartbeat => credited. Proves the test can go both ways.
	reg.Threads["thr-resident"].LastSeenAt = now
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Skipf("cannot persist registry: %v", err)
	}
	if ok, why := residentConsumerThread(root, "gemma-pantheon", now); !ok {
		t.Errorf("a fresh, active, consumer-capable published thread must be credited, got: %s", why)
	}
}
