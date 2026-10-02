package routerstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// wingFixture builds a schema-valid wing record (shape mirrors a real record
// at docs/router-service/stacklab/router-wing-ra-v1.json) rooted at repoRoot,
// so containment tests exercise real, resolvable directories rather than a
// static "/tmp/repo" string.
func wingFixture(t *testing.T, id, projectID, namespace, repoRoot string) []byte {
	t.Helper()
	evidence := filepath.Join(repoRoot, "docs", "evidence")
	if err := os.MkdirAll(evidence, 0o755); err != nil {
		t.Fatalf("mkdir evidence: %v", err)
	}
	rec := map[string]any{
		"schema":           "sirsi.stacklab.wing.v1",
		"id":               id,
		"owner":            "ra",
		"project_id":       projectID,
		"router_namespace": namespace,
		"class":            "control-plane",
		"scope":            "rs-31 test fixture.",
		"status":           "active",
		"first_gate":       "RS31-TEST.G1",
		"workspace": map[string]any{
			"repository_root":       repoRoot,
			"writable_roots":        []string{repoRoot},
			"evidence_root":         evidence,
			"shared_payload_access": "none",
			"boundary_policy":       "default-deny",
		},
		"handoffs": map[string]any{
			"inbound":            "router-receipt-only",
			"outbound":           "router-receipt-only",
			"allowed_peer_wings": []string{},
		},
		"provenance": map[string]any{
			"lifecycle_task_id": "rs-31-wing-register-test",
			"component_catalog": "test-fixture",
			"receipt_links":     []string{"test:fixture"},
		},
		"mirrors": map[string]any{
			"repository": "current",
			"desktop":    "pending",
			"workspace":  "pending",
		},
		"next_action": "none — test fixture",
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return raw
}

// grantFor issues a bootstrap grant binding principal to (projectID,
// namespace) over repoRoot, failing the test on error.
func grantFor(t *testing.T, s *SQLiteStore, principal, projectID, namespace, repoRoot string) WingAuthorityGrant {
	t.Helper()
	g, err := s.GrantWingAuthority("", principal, projectID, namespace, repoRoot, nil)
	if err != nil {
		t.Fatalf("GrantWingAuthority: %v", err)
	}
	return g
}

func TestRegisterWingPersistsAndReturnsReceipt(t *testing.T) {
	s := newTestStore(t)
	repo := t.TempDir()
	grantFor(t, s, "ra", "sirsi-pantheon", "router", repo)
	raw := wingFixture(t, "stacklab.wing.rs31-test", "sirsi-pantheon", "router", repo)

	receipt, err := s.RegisterWing("ra", raw)
	if err != nil {
		t.Fatalf("RegisterWing: %v", err)
	}
	if receipt.WingID != "stacklab.wing.rs31-test" || receipt.ProjectID != "sirsi-pantheon" ||
		receipt.RouterNamespace != "router" || receipt.ContentHash == "" || receipt.Created == "" {
		t.Fatalf("receipt incomplete: %+v", receipt)
	}
}

func TestRegisterWingIdempotentOnIdenticalBytes(t *testing.T) {
	s := newTestStore(t)
	repo := t.TempDir()
	grantFor(t, s, "ra", "sirsi-pantheon", "router", repo)
	raw := wingFixture(t, "stacklab.wing.rs31-test", "sirsi-pantheon", "router", repo)

	first, err := s.RegisterWing("ra", raw)
	if err != nil {
		t.Fatalf("first RegisterWing: %v", err)
	}
	second, err := s.RegisterWing("ra", raw)
	if err != nil {
		t.Fatalf("second RegisterWing (idempotent): %v", err)
	}
	if second != first {
		t.Fatalf("idempotent re-register must return the identical receipt: first=%+v second=%+v", first, second)
	}
}

func TestRegisterWingRejectsConflictingIdentity(t *testing.T) {
	s := newTestStore(t)
	repo := t.TempDir()
	grantFor(t, s, "ra", "sirsi-pantheon", "router", repo)
	raw := wingFixture(t, "stacklab.wing.rs31-test", "sirsi-pantheon", "router", repo)
	if _, err := s.RegisterWing("ra", raw); err != nil {
		t.Fatalf("initial RegisterWing: %v", err)
	}

	var rec map[string]any
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	rec["scope"] = "a different scope string makes the content hash differ"
	mutated, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal mutated fixture: %v", err)
	}

	if _, err := s.RegisterWing("ra", mutated); !errors.Is(err, ErrWingConflict) {
		t.Fatalf("same wing id, different bytes: want ErrWingConflict, got %v", err)
	}
}

func TestRegisterWingRejectsSchemaInvalidRecord(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.RegisterWing("ra", []byte(`{"schema":"not-the-right-const"}`)); err == nil {
		t.Fatal("schema-invalid record: want an error, got nil")
	}
}

// ── rs-31b/c: authority binding + path containment ────────────────────────

func TestRegisterWingRejectsMissingAuthority(t *testing.T) {
	s := newTestStore(t)
	repo := t.TempDir()
	raw := wingFixture(t, "stacklab.wing.rs31-test", "sirsi-pantheon", "router", repo)

	// No grant issued at all.
	if _, err := s.RegisterWing("ra", raw); !errors.Is(err, ErrWingAuthorityMissing) {
		t.Fatalf("no grant: want ErrWingAuthorityMissing, got %v", err)
	}
}

func TestRegisterWingRejectsOwnerSubstitution(t *testing.T) {
	// A record's self-declared "owner" field (and the acting principal having
	// ANY grant at all elsewhere) must not substitute for a grant covering
	// THIS principal + THIS project/namespace — the exact shape codex-apollo's
	// disposition rejected.
	s := newTestStore(t)
	repo := t.TempDir()
	grantFor(t, s, "codex-apollo", "sirsi-pantheon", "router", repo) // a different principal's grant
	raw := wingFixture(t, "stacklab.wing.rs31-test", "sirsi-pantheon", "router", repo)

	if _, err := s.RegisterWing("ra", raw); !errors.Is(err, ErrWingAuthorityMissing) {
		t.Fatalf("principal substitution: want ErrWingAuthorityMissing, got %v", err)
	}
}

func TestRegisterWingRejectsRevokedAuthority(t *testing.T) {
	s := newTestStore(t)
	repo := t.TempDir()
	grant := grantFor(t, s, "ra", "sirsi-pantheon", "router", repo)
	if err := s.RevokeWingAuthority(grant.GrantID); err != nil {
		t.Fatalf("RevokeWingAuthority: %v", err)
	}
	raw := wingFixture(t, "stacklab.wing.rs31-test", "sirsi-pantheon", "router", repo)

	if _, err := s.RegisterWing("ra", raw); !errors.Is(err, ErrWingAuthorityMissing) {
		t.Fatalf("revoked grant: want ErrWingAuthorityMissing, got %v", err)
	}
}

func TestRegisterWingRejectsForeignRoot(t *testing.T) {
	s := newTestStore(t)
	grantedRoot := t.TempDir()
	foreignRoot := t.TempDir()
	grantFor(t, s, "ra", "sirsi-pantheon", "router", grantedRoot)
	// Schema-valid record whose workspace roots point OUTSIDE the grant.
	raw := wingFixture(t, "stacklab.wing.rs31-test", "sirsi-pantheon", "router", foreignRoot)

	if _, err := s.RegisterWing("ra", raw); !errors.Is(err, ErrWingRootNotContained) {
		t.Fatalf("foreign root: want ErrWingRootNotContained, got %v", err)
	}
}

func TestRegisterWingRejectsSymlinkEscape(t *testing.T) {
	s := newTestStore(t)
	grantedRoot := t.TempDir()
	outside := t.TempDir()
	// A subdirectory of the granted root that is actually a symlink pointing
	// outside it: component-wise string containment would wrongly accept
	// this as "under grantedRoot"; EvalSymlinks must catch the escape.
	escape := filepath.Join(grantedRoot, "escape")
	if err := os.Symlink(outside, escape); err != nil {
		t.Skipf("symlink unsupported in this environment: %v", err)
	}
	grantFor(t, s, "ra", "sirsi-pantheon", "router", grantedRoot)
	raw := wingFixture(t, "stacklab.wing.rs31-test", "sirsi-pantheon", "router", escape)

	if _, err := s.RegisterWing("ra", raw); !errors.Is(err, ErrWingRootNotContained) {
		t.Fatalf("symlink escape: want ErrWingRootNotContained, got %v", err)
	}
}

func TestRegisterWingAcceptsRootsUnderEvidenceRoot(t *testing.T) {
	// Separately-granted evidence roots are honored on their own, not only as
	// a subdirectory of repository_root.
	s := newTestStore(t)
	repoRoot := t.TempDir()
	separateEvidence := t.TempDir()
	if _, err := s.GrantWingAuthority("", "ra", "sirsi-pantheon", "router", repoRoot, []string{separateEvidence}); err != nil {
		t.Fatalf("GrantWingAuthority: %v", err)
	}
	evidence := filepath.Join(separateEvidence, "docs", "evidence")
	if err := os.MkdirAll(evidence, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw := wingFixture(t, "stacklab.wing.rs31-test", "sirsi-pantheon", "router", repoRoot)
	var rec map[string]any
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rec["workspace"].(map[string]any)["evidence_root"] = evidence
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if _, err := s.RegisterWing("ra", raw); err != nil {
		t.Fatalf("evidence-root-contained record: want admission, got %v", err)
	}
}

func TestRegisterWingIgnoresAllowedPeerWings(t *testing.T) {
	// handoffs.allowed_peer_wings is declarative metadata only: naming a peer
	// that was never admitted or authorized must not block (or grant)
	// anything — it is never consulted here.
	s := newTestStore(t)
	repo := t.TempDir()
	grantFor(t, s, "ra", "sirsi-pantheon", "router", repo)
	raw := wingFixture(t, "stacklab.wing.rs31-test", "sirsi-pantheon", "router", repo)
	var rec map[string]any
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rec["handoffs"].(map[string]any)["allowed_peer_wings"] = []string{"stacklab.wing.never-admitted"}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if _, err := s.RegisterWing("ra", raw); err != nil {
		t.Fatalf("unrelated allowed_peer_wings entry: want admission, got %v", err)
	}
}

func TestRegisterWingRejectionWritesNoRow(t *testing.T) {
	// A rejected RegisterWing (containment failure here, but the same
	// defer-tx.Rollback() path covers every rejection reason) must leave no
	// row behind: a corrected retry with the same wing id is admitted fresh,
	// not treated as a conflict against a partial prior write.
	s := newTestStore(t)
	grantedRoot := t.TempDir()
	foreignRoot := t.TempDir()
	grantFor(t, s, "ra", "sirsi-pantheon", "router", grantedRoot)
	bad := wingFixture(t, "stacklab.wing.rs31-test", "sirsi-pantheon", "router", foreignRoot)
	if _, err := s.RegisterWing("ra", bad); !errors.Is(err, ErrWingRootNotContained) {
		t.Fatalf("foreign root: want ErrWingRootNotContained, got %v", err)
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM wings WHERE wing_id=?`, "stacklab.wing.rs31-test").Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("rejected registration left %d row(s) behind, want 0", count)
	}

	good := wingFixture(t, "stacklab.wing.rs31-test", "sirsi-pantheon", "router", grantedRoot)
	if _, err := s.RegisterWing("ra", good); err != nil {
		t.Fatalf("corrected retry after rejection: want admission, got %v", err)
	}
}
