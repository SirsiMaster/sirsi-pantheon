package reason

import (
	"context"
	"strings"
	"testing"
)

// codex-pantheon blocked PR #340 until "restart verification proves a new PID
// plus a live endpoint". The original Verify only checked pid > 0, so it passed
// in both failure modes that actually occur:
//
//  1. the restart did nothing and the OLD process is still running
//  2. the process is alive but its HTTP server is wedged or still loading
//
// These tests call restartVerdict — the PRODUCTION decision rule, extracted from
// the polling loop so it can be exercised without a live broker. They do not
// restate the rule: a test that reimplements the logic proves only that the copy
// agrees with itself, which is the same defect shape the rule exists to catch.

func TestRestartVerifyRejectsUnchangedPID(t *testing.T) {
	// The restart command exited 0 but replaced nothing.
	ok, reason := restartVerdict(4242, 4242, "gemma-3-12b", nil)
	if ok {
		t.Fatal("a broker with the SAME pid must not verify as restarted — the process was never replaced")
	}
	if !strings.Contains(reason, "pid unchanged") {
		t.Fatalf("reason = %q, want the unchanged-pid rejection", reason)
	}
}

func TestRestartVerifyRejectsLivePIDWithDeadEndpoint(t *testing.T) {
	// New process, but the server is wedged or still loading weights. This is
	// the green-surface-over-a-dead-thing case: a pid is not a service.
	ok, reason := restartVerdict(4242, 5150, "", context.DeadlineExceeded)
	if ok {
		t.Fatal("a new pid whose endpoint does not serve must not verify — a live process is not a live model")
	}
	if !strings.Contains(reason, "endpoint not serving") {
		t.Fatalf("reason = %q, want the dead-endpoint rejection", reason)
	}
}

func TestRestartVerifyRejectsEndpointServingNoModel(t *testing.T) {
	// The server answers but has no model loaded — it can accept a connection
	// and still be useless, which reads as healthy to any transport-only probe.
	ok, _ := restartVerdict(4242, 5150, "   ", nil)
	if ok {
		t.Fatal("an endpoint that names no model must not verify as a healthy broker")
	}
}

func TestRestartVerifyRejectsAbsentProcess(t *testing.T) {
	if ok, _ := restartVerdict(4242, 0, "", nil); ok {
		t.Fatal("no broker process must never verify")
	}
}

func TestRestartVerifyAcceptsNewPIDServingAModel(t *testing.T) {
	ok, reason := restartVerdict(4242, 5150, "gemma-3-12b-8bit", nil)
	if !ok {
		t.Fatalf("a NEW pid serving a named model is a verified restart, got %q", reason)
	}
}

// If nothing was running before, any live serving broker is a valid outcome —
// the rule must not demand a pid CHANGE when there was no prior pid to change.
func TestRestartVerifyAcceptsColdStart(t *testing.T) {
	if ok, reason := restartVerdict(0, 5150, "gemma-3-12b", nil); !ok {
		t.Fatalf("cold start (no prior pid) must verify, got %q", reason)
	}
}
