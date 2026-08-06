package router

import (
	"testing"
	"time"
)

// H6 acceptance: a resident consumer publishes and heartbeats its OWN
// consumer-capable thread, and one that stops publishing stops being credited.
//
// The subtle half is PROMOTION on the RE-register. A resident worker is
// long-lived and re-registers on restart/renew, which lands on RegisterThread's
// idempotent reuse path — not the mint path. That path REPLACES a curated list
// of fields and ignored ConsumerCapable entirely, so a thread first minted
// WITHOUT the capability could never acquire it while that PID lived: the
// worker publishes, register reports success, and the lane stays uncredited.
// Preserving an already-set capability needs no code (the reuse path returns
// the stored record); RAISING one does, which is why this test registers plain
// first and only then publishes.
func TestResidentConsumerPublishesOwnCapability(t *testing.T) {
	root := t.TempDir()

	// A plain register first — no capability claimed yet.
	first, err := RegisterThread(root, &Thread{
		AgentID: "gemma-pantheon",
		Surface: surfaceWorker,
		PID:     minAgentPID + 1,
	})
	if err != nil {
		t.Fatalf("RegisterThread: %v", err)
	}
	if first.ConsumerCapable {
		t.Fatal("a plain register must not claim consumer capability")
	}
	if first.IsInboxConsumer() {
		t.Fatal("a worker surface with no declared capability must not be credited as a consumer")
	}

	// Now the resident PUBLISHES on the same (agent, pid) — the reuse path.
	// This is the promotion the flag exists for.
	published, err := RegisterThread(root, &Thread{
		AgentID:         "gemma-pantheon",
		Surface:         surfaceWorker,
		PID:             minAgentPID + 1,
		ConsumerCapable: true,
	})
	if err != nil {
		t.Fatalf("publishing re-RegisterThread: %v", err)
	}
	if published.ThreadID != first.ThreadID {
		t.Fatalf("expected the reuse path (one live worker → one thread), got %s then %s",
			first.ThreadID, published.ThreadID)
	}
	if !published.ConsumerCapable {
		t.Error("reuse path ignored ConsumerCapable — a resident can never publish while its PID lives")
	}
	if !published.IsInboxConsumer() {
		t.Error("a published resident consumer must count as an inbox consumer")
	}

	// A bare heartbeat-style re-register (flag absent) must NOT wipe a
	// capability already proven. Credit lapses by going stale, not by omission.
	bare, err := RegisterThread(root, &Thread{
		AgentID: "gemma-pantheon",
		Surface: surfaceWorker,
		PID:     minAgentPID + 1,
	})
	if err != nil {
		t.Fatalf("bare re-RegisterThread: %v", err)
	}
	if !bare.ConsumerCapable {
		t.Error("a bare re-register wiped ConsumerCapable — omission is not a demotion")
	}

	// Acceptance clause 2: stop publishing → stop being credited. The armed
	// predicate in WakePass skips any thread that is stale, so a worker that
	// stops heartbeating lapses on its own with nothing to clear.
	now := time.Now().UTC()
	if bare.IsStale(now, DefaultThreadStaleAfter) {
		t.Error("a just-published thread must not read stale")
	}
	stopped := *bare
	stopped.LastSeenAt = now.Add(-2 * DefaultThreadStaleAfter)
	if !stopped.IsStale(now, DefaultThreadStaleAfter) {
		t.Error("a resident that stopped publishing must go stale — that IS the demotion")
	}
}

// The guard behind --consumer-capable: only an agent DECLARING a resident
// consumer may publish capability. A watch-only lane must not arm itself,
// because an armed lane suppresses its own rescue in WakePass.
func TestDeclaresResidentConsumer(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode string
		want bool
	}{
		{"resident", ConsumerModeResident, true},
		{"legacy external/resident spelling", "external/resident", true},
		{"whitespace tolerated", "  resident  ", true},
		{"spawned command is not resident", ConsumerModeCommand, false},
		{"undeclared defaults to command", "", false},
		{"unknown mode is not resident", "daemon", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := AgentConfig{Consumer: ConsumerConfig{Mode: tc.mode}}
			if got := cfg.DeclaresResidentConsumer(); got != tc.want {
				t.Errorf("mode %q: got %v, want %v", tc.mode, got, tc.want)
			}
		})
	}
}

// The SESSION-KEYED reuse branch raises capability too — and until this test
// existed, it did not have to.
//
// #598's changelog claimed the regression test "fails red and names itself when
// EITHER raise is removed". Verified by deleting each raise independently:
// removing the pid-keyed raise (~560) failed TestResidentConsumerPublishesOwnCapability
// correctly, but removing the session-keyed raise (~497) left the package GREEN.
// One of the two sites was uncovered, so the claim's scope was one raise while
// its wording was both (A35, in a change whose own changelog cites A35).
//
// A resident on an app-hosted surface registers with a SessionID and no
// ThreadID, so it lands here and never touches the pid-keyed branch — exactly
// the long-lived worker the raise exists for.
func TestResidentConsumerCapabilityRaisedOnSessionKeyedReuse(t *testing.T) {
	root := t.TempDir()
	const sid = "session-resident-1"

	// First register: plain, no capability — the ordering the promotion exists for.
	first, err := RegisterThread(root, &Thread{
		AgentID: "gemma-pantheon", Surface: "worker", SessionID: sid,
	})
	if err != nil {
		t.Fatalf("initial register: %v", err)
	}
	if first.ConsumerCapable {
		t.Fatal("setup: first register must NOT be consumer-capable")
	}

	// The live worker now publishes its own capability by re-registering on the
	// same session. No ThreadID => session-keyed reuse path.
	second, err := RegisterThread(root, &Thread{
		AgentID: "gemma-pantheon", Surface: "worker", SessionID: sid,
		ConsumerCapable: true,
	})
	if err != nil {
		t.Fatalf("republish: %v", err)
	}
	if second.ThreadID != first.ThreadID {
		t.Fatalf("expected session-keyed REUSE, got a new record %s (was %s)",
			second.ThreadID, first.ThreadID)
	}
	if !second.ConsumerCapable {
		t.Fatal("session-keyed reuse dropped the published capability — the resident " +
			"can never become creditable while this session lives")
	}

	// Raise-only: a later bare re-register must not wipe proven capability.
	third, err := RegisterThread(root, &Thread{
		AgentID: "gemma-pantheon", Surface: "worker", SessionID: sid,
	})
	if err != nil {
		t.Fatalf("bare re-register: %v", err)
	}
	if !third.ConsumerCapable {
		t.Error("a bare heartbeat-style re-register cleared a proven capability; " +
			"credit must lapse by staleness, not by being silently wiped")
	}
}
