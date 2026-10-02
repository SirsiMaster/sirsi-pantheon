package routerstore

// ADR-066 (rs-31b/c): caller-authority binding + canonical-path containment
// for Stack Lab wing admission. Built against codex-apollo's SNE disposition
// on 2026-10-02 (router item 20261002-211532 / response
// 20261002-211749-codex-apollo-...): the threads/agents tables prove identity,
// never repository authority, so a SEPARATE grant table is required — never a
// self-declared field on the wing record itself (that is exactly the
// bootstrap-from-nothing shape Apollo rejected).
//
// Grant issuance (GrantWingAuthority/RevokeWingAuthority) is server-side
// only — see serve.go's notServed — mirroring MintHostToken/RevokeHostToken:
// run on the service host against its own backend, never reachable from a
// node. RegisterWing (the consumer side) stays a normal served RPC; it reads
// the grant but never writes one.

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// WingAuthorityGrant is the stored record: principal may admit wings for
// (ProjectID, RouterNamespace) whose workspace roots resolve inside
// RepositoryRoot or one of EvidenceRoots.
type WingAuthorityGrant struct {
	GrantID         string   `json:"grant_id"`
	Principal       string   `json:"principal"`
	Issuer          string   `json:"issuer"`
	ProjectID       string   `json:"project_id"`
	RouterNamespace string   `json:"router_namespace"`
	RepositoryRoot  string   `json:"repository_root"`
	EvidenceRoots   []string `json:"evidence_roots"`
	Status          string   `json:"status"`
	Created         string   `json:"created"`
	Revoked         string   `json:"revoked,omitempty"`
}

var (
	// ErrWingAuthorityMissing: no active grant exists for this (principal,
	// project, namespace) triple. Covers both "never granted" and "revoked" —
	// Apollo's disposition treats missing/ambiguous/revoked the same: fail
	// closed before persistence, not a distinct recoverable state.
	ErrWingAuthorityMissing = errors.New("routerstore: no active wing authority grant for this principal/project/namespace")
	// ErrWingAuthorityInsufficient: the issuer of a new grant does not itself
	// hold an active grant whose roots already cover the roots being granted.
	// A second owner needs delegation FROM authority that already covers the
	// requested scope — never a bare assertion.
	ErrWingAuthorityInsufficient = errors.New("routerstore: issuer holds no active grant covering the requested scope")
	// ErrWingRootNotContained: the wing record's workspace root (repository
	// root, a writable root, or the evidence root) does not canonically
	// resolve inside the grant's repository_root or evidence_roots. Covers
	// foreign roots and symlink/ancestor escape attempts alike.
	ErrWingRootNotContained = errors.New("routerstore: wing workspace root is outside the grant's authorized roots")
)

// canonicalDir resolves dir to an absolute, symlink-resolved, cleaned path.
// Requiring the path to exist (EvalSymlinks fails otherwise) is deliberate:
// containment against a root that cannot be resolved on this host fails
// closed rather than comparing unresolved strings.
func canonicalDir(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("routerstore: empty path")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("routerstore: resolve %q: %w", dir, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("routerstore: resolve %q: %w", dir, err)
	}
	return filepath.Clean(real), nil
}

// containsPath reports whether target canonically resolves inside root —
// component-wise via filepath.Rel, never a string prefix (so /tmp/repo-evil
// is not "contained" by /tmp/repo), and symlink-resolved on both sides so an
// escape via a symlinked ancestor is caught rather than compared as text.
func containsPath(root, target string) (bool, error) {
	rootReal, err := canonicalDir(root)
	if err != nil {
		return false, err
	}
	targetReal, err := canonicalDir(target)
	if err != nil {
		return false, err
	}
	if rootReal == targetReal {
		return true, nil
	}
	rel, err := filepath.Rel(rootReal, targetReal)
	if err != nil {
		return false, err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false, nil
	}
	return true, nil
}

// containedInAny reports whether target is contained in root or any of alts.
func containedInAny(target, root string, alts []string) (bool, error) {
	ok, err := containsPath(root, target)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	for _, alt := range alts {
		ok, err := containsPath(alt, target)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func marshalRoots(roots []string) (string, error) {
	b, err := json.Marshal(roots)
	if err != nil {
		return "", fmt.Errorf("routerstore: marshal evidence roots: %w", err)
	}
	return string(b), nil
}

func unmarshalRoots(raw string) ([]string, error) {
	var roots []string
	if raw == "" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(raw), &roots); err != nil {
		return nil, fmt.Errorf("routerstore: unmarshal evidence roots: %w", err)
	}
	return roots, nil
}

func scanGrant(scan func(dest ...any) error) (WingAuthorityGrant, error) {
	var g WingAuthorityGrant
	var evidenceJSON string
	if err := scan(&g.GrantID, &g.Principal, &g.Issuer, &g.ProjectID, &g.RouterNamespace,
		&g.RepositoryRoot, &evidenceJSON, &g.Status, &g.Created, &g.Revoked); err != nil {
		return WingAuthorityGrant{}, err
	}
	roots, err := unmarshalRoots(evidenceJSON)
	if err != nil {
		return WingAuthorityGrant{}, err
	}
	g.EvidenceRoots = roots
	return g, nil
}

// activeWingGrant returns the one active grant for (principal, projectID,
// namespace). More than one active row for the same triple is a fleet
// anomaly (grants are superseded by revoking, never duplicated) — treated as
// ErrWingAuthorityMissing: ambiguous authority fails closed exactly like no
// authority, per Apollo's disposition.
func (s *SQLiteStore) activeWingGrant(principal, projectID, namespace string) (WingAuthorityGrant, error) {
	rows, err := s.db.Query(
		`SELECT grant_id,principal,issuer,project_id,router_namespace,repository_root,evidence_roots_json,status,created,revoked
		 FROM wing_authority_grants WHERE principal=? AND project_id=? AND router_namespace=? AND status='active'`,
		principal, projectID, namespace)
	if err != nil {
		return WingAuthorityGrant{}, fmt.Errorf("routerstore: activeWingGrant: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var found []WingAuthorityGrant
	for rows.Next() {
		g, err := scanGrant(rows.Scan)
		if err != nil {
			return WingAuthorityGrant{}, fmt.Errorf("routerstore: activeWingGrant: scan: %w", err)
		}
		found = append(found, g)
	}
	if err := rows.Err(); err != nil {
		return WingAuthorityGrant{}, fmt.Errorf("routerstore: activeWingGrant: %w", err)
	}
	if len(found) != 1 {
		return WingAuthorityGrant{}, ErrWingAuthorityMissing
	}
	return found[0], nil
}

// activeWingGrantTx is activeWingGrant run against an open transaction, so
// RegisterWing can check authority and persist the admitted wing atomically
// — closing the validate-then-use race Apollo's disposition names explicitly
// (a grant revoked between the check and the write must not admit).
func activeWingGrantTx(tx *txHandle, principal, projectID, namespace string) (WingAuthorityGrant, error) {
	rows, err := tx.Query(
		`SELECT grant_id,principal,issuer,project_id,router_namespace,repository_root,evidence_roots_json,status,created,revoked
		 FROM wing_authority_grants WHERE principal=? AND project_id=? AND router_namespace=? AND status='active'`,
		principal, projectID, namespace)
	if err != nil {
		return WingAuthorityGrant{}, fmt.Errorf("routerstore: activeWingGrantTx: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var found []WingAuthorityGrant
	for rows.Next() {
		g, err := scanGrant(rows.Scan)
		if err != nil {
			return WingAuthorityGrant{}, fmt.Errorf("routerstore: activeWingGrantTx: scan: %w", err)
		}
		found = append(found, g)
	}
	if err := rows.Err(); err != nil {
		return WingAuthorityGrant{}, fmt.Errorf("routerstore: activeWingGrantTx: %w", err)
	}
	if len(found) != 1 {
		return WingAuthorityGrant{}, ErrWingAuthorityMissing
	}
	return found[0], nil
}

// GrantWingAuthority issues a new grant binding principal to
// (projectID, namespace) over repositoryRoot + evidenceRoots.
//
// Bootstrap (the table holds no rows at all) is the one ungated root of
// trust: issuing it requires direct backend access, which this method
// already requires (it is listed in serve.go's notServed and only reachable
// via `sirsi router wing authority grant` run on the service host). Every
// later grant requires issuer to already hold an active grant whose roots
// contain both repositoryRoot and every evidenceRoot — delegation from
// covering authority, never a bare assertion.
func (s *SQLiteStore) GrantWingAuthority(issuer, principal, projectID, namespace, repositoryRoot string, evidenceRoots []string) (WingAuthorityGrant, error) {
	if strings.TrimSpace(principal) == "" || strings.TrimSpace(projectID) == "" || strings.TrimSpace(namespace) == "" {
		return WingAuthorityGrant{}, fmt.Errorf("routerstore: GrantWingAuthority: principal, project id and namespace are all required")
	}
	repoCanon, err := canonicalDir(repositoryRoot)
	if err != nil {
		return WingAuthorityGrant{}, fmt.Errorf("routerstore: GrantWingAuthority: repository root: %w", err)
	}
	evCanon := make([]string, 0, len(evidenceRoots))
	for _, r := range evidenceRoots {
		c, err := canonicalDir(r)
		if err != nil {
			return WingAuthorityGrant{}, fmt.Errorf("routerstore: GrantWingAuthority: evidence root: %w", err)
		}
		evCanon = append(evCanon, c)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return WingAuthorityGrant{}, fmt.Errorf("routerstore: GrantWingAuthority: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var total int
	if err := tx.QueryRow(`SELECT count(*) FROM wing_authority_grants`).Scan(&total); err != nil {
		return WingAuthorityGrant{}, fmt.Errorf("routerstore: GrantWingAuthority: count: %w", err)
	}
	if total > 0 {
		issuerGrant, err := activeWingGrantTx(tx, issuer, projectID, namespace)
		if err != nil {
			return WingAuthorityGrant{}, ErrWingAuthorityInsufficient
		}
		ok, err := containedInAny(repoCanon, issuerGrant.RepositoryRoot, issuerGrant.EvidenceRoots)
		if err != nil {
			return WingAuthorityGrant{}, fmt.Errorf("routerstore: GrantWingAuthority: %w", err)
		}
		if !ok {
			return WingAuthorityGrant{}, ErrWingAuthorityInsufficient
		}
		for _, ev := range evCanon {
			ok, err := containedInAny(ev, issuerGrant.RepositoryRoot, issuerGrant.EvidenceRoots)
			if err != nil {
				return WingAuthorityGrant{}, fmt.Errorf("routerstore: GrantWingAuthority: %w", err)
			}
			if !ok {
				return WingAuthorityGrant{}, ErrWingAuthorityInsufficient
			}
		}
	}

	id, err := randomHex(8)
	if err != nil {
		return WingAuthorityGrant{}, err
	}
	evJSON, err := marshalRoots(evCanon)
	if err != nil {
		return WingAuthorityGrant{}, err
	}
	now := s.clock().Format(time.RFC3339)
	grant := WingAuthorityGrant{
		GrantID: id, Principal: principal, Issuer: issuer, ProjectID: projectID,
		RouterNamespace: namespace, RepositoryRoot: repoCanon, EvidenceRoots: evCanon,
		Status: "active", Created: now,
	}
	if _, err := tx.Exec(
		`INSERT INTO wing_authority_grants(grant_id,principal,issuer,project_id,router_namespace,repository_root,evidence_roots_json,status,created,revoked)
		 VALUES(?,?,?,?,?,?,?,?,?,'')`,
		id, principal, issuer, projectID, namespace, repoCanon, evJSON, "active", now,
	); err != nil {
		return WingAuthorityGrant{}, fmt.Errorf("routerstore: GrantWingAuthority: insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return WingAuthorityGrant{}, fmt.Errorf("routerstore: GrantWingAuthority: commit: %w", err)
	}
	return grant, nil
}

// RevokeWingAuthority marks grantID revoked. Idempotent: revoking an
// already-revoked grant is a no-op, not an error — mirrors
// RevokeHostToken's shape.
func (s *SQLiteStore) RevokeWingAuthority(grantID string) error {
	now := s.clock().Format(time.RFC3339)
	if _, err := s.exec(
		`UPDATE wing_authority_grants SET status='revoked', revoked=? WHERE grant_id=? AND status='active'`,
		now, grantID,
	); err != nil {
		return fmt.Errorf("routerstore: RevokeWingAuthority: %w", err)
	}
	return nil
}
