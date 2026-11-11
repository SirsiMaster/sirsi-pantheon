package routerstore

import (
	"errors"
	"testing"
)

func TestIngestDecisionRequiresAuthenticatedCaller(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.IngestDecision("", "sess-1", DecisionIngest{RawBytes: []byte(`{"kind":"grant"}`)}); err == nil {
		t.Fatal("ingest with empty host must fail")
	}
	if _, err := s.IngestDecision("host-1", "", DecisionIngest{RawBytes: []byte(`{"kind":"grant"}`)}); err == nil {
		t.Fatal("ingest with empty session must fail")
	}
}

func TestIngestDecisionRejectsEmptyBytes(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.IngestDecision("host-1", "sess-1", DecisionIngest{}); err == nil {
		t.Fatal("ingest with no raw bytes must fail")
	}
}

func TestIngestDecisionIsIdempotentOnIdenticalBytes(t *testing.T) {
	s := newTestStore(t)
	rec := DecisionIngest{SourceTime: "2026-10-09T18:00:00Z", RawBytes: []byte(`{"kind":"grant","requester":"codex"}`)}

	first, err := s.IngestDecision("host-1", "sess-1", rec)
	if err != nil {
		t.Fatal(err)
	}
	if first.Duplicate {
		t.Fatal("first ingest must not report duplicate")
	}
	if first.ID == "" {
		t.Fatal("expected a non-empty content-digest id")
	}

	second, err := s.IngestDecision("host-1", "sess-1", rec)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate {
		t.Fatal("resubmitting identical bytes must be reported as duplicate")
	}
	if second.ID != first.ID {
		t.Fatalf("same bytes must yield the same content-digest id: %q vs %q", first.ID, second.ID)
	}
}

func TestIngestDecisionDifferentBytesGetDistinctIDs(t *testing.T) {
	s := newTestStore(t)
	a, err := s.IngestDecision("host-1", "sess-1", DecisionIngest{RawBytes: []byte(`{"kind":"grant"}`)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.IngestDecision("host-1", "sess-1", DecisionIngest{RawBytes: []byte(`{"kind":"refuse"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Fatal("different bytes must not collide on id")
	}
	if b.Duplicate {
		t.Fatal("distinct content must not be reported as duplicate")
	}
}

func TestIngestDecisionRejectsProducerMismatchOnSameContent(t *testing.T) {
	s := newTestStore(t)
	rec := DecisionIngest{RawBytes: []byte(`{"kind":"grant","requester":"codex"}`)}

	if _, err := s.IngestDecision("host-1", "sess-1", rec); err != nil {
		t.Fatal(err)
	}
	if _, err := s.IngestDecision("host-2", "sess-2", rec); !errors.Is(err, ErrDecisionProducerMismatch) {
		t.Fatalf("expected ErrDecisionProducerMismatch for a different producer submitting identical bytes, got %v", err)
	}
	// Same producer, same bytes, submitted again: still idempotent.
	second, err := s.IngestDecision("host-1", "sess-1", rec)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate {
		t.Fatal("same producer resubmitting identical bytes must stay idempotent")
	}
}

// TestIngestDecisionNotServedUntilDispatchWiringLands guards the
// serve.go-side half of the SSA gate's binding requirement #1: the generic
// /v1/call/ dispatcher (serve.go) reaches ANY Store method by reflection
// unless it is deny-listed in notServed. IngestDecision's host/session must
// be server-stamped, not caller-supplied, so it stays blocked until a
// dedicated handler injects the authenticated session in their place.
func TestIngestDecisionNotServedUntilDispatchWiringLands(t *testing.T) {
	if !notServed["IngestDecision"] {
		t.Fatal("IngestDecision must stay in notServed until serve.go injects the authenticated host/session in place of caller-supplied values")
	}
}

func TestIngestDecisionDefaultsPayloadVersion(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.IngestDecision("host-1", "sess-1", DecisionIngest{RawBytes: []byte(`{"kind":"grant"}`)}); err != nil {
		t.Fatal(err)
	}
	var version string
	row := s.db.QueryRow(`SELECT payload_version FROM maat_decisions LIMIT 1`)
	if err := row.Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "v1" {
		t.Fatalf("expected default payload_version v1, got %q", version)
	}
}
