package routerstore

import (
	"encoding/json"
	"errors"
	"testing"
)

// validWingFixture mirrors a real schema-valid record (docs/router-service/
// stacklab/router-wing-ra-v1.json) so the test exercises ValidateWing's actual
// required-field shape rather than a hand-trimmed stand-in.
const validWingFixture = `{
  "schema": "sirsi.stacklab.wing.v1",
  "id": "stacklab.wing.rs31a-test",
  "owner": "ra",
  "project_id": "sirsi-pantheon",
  "router_namespace": "router",
  "class": "control-plane",
  "scope": "rs-31a register-verb test fixture.",
  "status": "active",
  "first_gate": "RS31A-TEST.G1",
  "workspace": {
    "repository_root": "/tmp/repo",
    "writable_roots": ["/tmp/repo"],
    "evidence_root": "/tmp/repo/docs/evidence",
    "shared_payload_access": "none",
    "boundary_policy": "default-deny"
  },
  "handoffs": {
    "inbound": "router-receipt-only",
    "outbound": "router-receipt-only",
    "allowed_peer_wings": []
  },
  "provenance": {
    "lifecycle_task_id": "rs-31a-wing-register-test",
    "component_catalog": "test-fixture",
    "receipt_links": ["test:fixture"]
  },
  "mirrors": {
    "repository": "current",
    "desktop": "pending",
    "workspace": "pending"
  },
  "next_action": "none — test fixture"
}`

func TestRegisterWingPersistsAndReturnsReceipt(t *testing.T) {
	s := newTestStore(t)
	receipt, err := s.RegisterWing([]byte(validWingFixture))
	if err != nil {
		t.Fatalf("RegisterWing: %v", err)
	}
	if receipt.WingID != "stacklab.wing.rs31a-test" || receipt.ProjectID != "sirsi-pantheon" ||
		receipt.RouterNamespace != "router" || receipt.ContentHash == "" || receipt.Created == "" {
		t.Fatalf("receipt incomplete: %+v", receipt)
	}
}

func TestRegisterWingIdempotentOnIdenticalBytes(t *testing.T) {
	s := newTestStore(t)
	first, err := s.RegisterWing([]byte(validWingFixture))
	if err != nil {
		t.Fatalf("first RegisterWing: %v", err)
	}
	second, err := s.RegisterWing([]byte(validWingFixture))
	if err != nil {
		t.Fatalf("second RegisterWing (idempotent): %v", err)
	}
	if second != first {
		t.Fatalf("idempotent re-register must return the identical receipt: first=%+v second=%+v", first, second)
	}
}

func TestRegisterWingRejectsConflictingIdentity(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.RegisterWing([]byte(validWingFixture)); err != nil {
		t.Fatalf("initial RegisterWing: %v", err)
	}

	var rec map[string]any
	if err := json.Unmarshal([]byte(validWingFixture), &rec); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	rec["scope"] = "a different scope string makes the content hash differ"
	mutated, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal mutated fixture: %v", err)
	}

	if _, err := s.RegisterWing(mutated); !errors.Is(err, ErrWingConflict) {
		t.Fatalf("same wing id, different bytes: want ErrWingConflict, got %v", err)
	}
}

func TestRegisterWingRejectsSchemaInvalidRecord(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.RegisterWing([]byte(`{"schema":"not-the-right-const"}`)); err == nil {
		t.Fatal("schema-invalid record: want an error, got nil")
	}
}
