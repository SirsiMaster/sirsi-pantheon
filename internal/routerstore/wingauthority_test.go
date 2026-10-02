package routerstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGrantWingAuthorityBootstrapRequiresNoIssuer(t *testing.T) {
	s := newTestStore(t)
	repo := t.TempDir()
	g, err := s.GrantWingAuthority("", "ra", "sirsi-pantheon", "router", repo, nil)
	if err != nil {
		t.Fatalf("bootstrap grant: %v", err)
	}
	if g.Principal != "ra" || g.Issuer != "" || g.Status != "active" || g.GrantID == "" {
		t.Fatalf("bootstrap grant incomplete: %+v", g)
	}
}

func TestGrantWingAuthorityUnauthorizedDelegationRejected(t *testing.T) {
	// The table is non-empty (a bootstrap grant exists for someone else), so
	// this grant is no longer bootstrap: issuer MUST already hold a covering
	// active grant. An issuer with nothing on file cannot delegate.
	s := newTestStore(t)
	repo := t.TempDir()
	grantFor(t, s, "ra", "sirsi-pantheon", "router", repo)

	other := t.TempDir()
	if _, err := s.GrantWingAuthority("nobody", "codex-apollo", "sirsi-pantheon", "router", other, nil); !errors.Is(err, ErrWingAuthorityInsufficient) {
		t.Fatalf("unauthorized delegation: want ErrWingAuthorityInsufficient, got %v", err)
	}
}

func TestGrantWingAuthorityBroaderThanIssuerRejected(t *testing.T) {
	// ra holds a grant rooted at a narrow subdirectory; delegating a grant
	// rooted at the subdirectory's PARENT (broader than what ra itself holds)
	// must fail — delegation can only narrow, never widen, scope.
	s := newTestStore(t)
	parent := t.TempDir()
	narrow := filepath.Join(parent, "narrow")
	if err := os.MkdirAll(narrow, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	grantFor(t, s, "ra", "sirsi-pantheon", "router", narrow)

	if _, err := s.GrantWingAuthority("ra", "codex-apollo", "sirsi-pantheon", "router", parent, nil); !errors.Is(err, ErrWingAuthorityInsufficient) {
		t.Fatalf("broader-than-issuer: want ErrWingAuthorityInsufficient, got %v", err)
	}
}

func TestGrantWingAuthorityValidDelegationSucceeds(t *testing.T) {
	// ra holds a grant rooted at parent; delegating a grant rooted at a
	// subdirectory of parent (narrower or equal scope) succeeds.
	s := newTestStore(t)
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	grantFor(t, s, "ra", "sirsi-pantheon", "router", parent)

	g, err := s.GrantWingAuthority("ra", "codex-apollo", "sirsi-pantheon", "router", child, nil)
	if err != nil {
		t.Fatalf("valid delegation: %v", err)
	}
	if g.Issuer != "ra" || g.Principal != "codex-apollo" {
		t.Fatalf("delegated grant wrong shape: %+v", g)
	}
}

func TestGrantWingAuthorityDelegationViaEvidenceRootSucceeds(t *testing.T) {
	// Delegation may be covered by the issuer's EVIDENCE roots, not only its
	// repository root.
	s := newTestStore(t)
	repoRoot := t.TempDir()
	evidenceRoot := t.TempDir()
	if _, err := s.GrantWingAuthority("", "ra", "sirsi-pantheon", "router", repoRoot, []string{evidenceRoot}); err != nil {
		t.Fatalf("bootstrap grant: %v", err)
	}

	if _, err := s.GrantWingAuthority("ra", "codex-apollo", "sirsi-pantheon", "router", evidenceRoot, nil); err != nil {
		t.Fatalf("delegation via evidence root: %v", err)
	}
}

func TestRevokeWingAuthorityIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	repo := t.TempDir()
	g := grantFor(t, s, "ra", "sirsi-pantheon", "router", repo)
	if err := s.RevokeWingAuthority(g.GrantID); err != nil {
		t.Fatalf("first revoke: %v", err)
	}
	if err := s.RevokeWingAuthority(g.GrantID); err != nil {
		t.Fatalf("second revoke (idempotent): %v", err)
	}
	if err := s.RevokeWingAuthority("never-issued"); err != nil {
		t.Fatalf("revoke unknown id (idempotent no-op): %v", err)
	}
}

func TestContainsPathRejectsStringPrefixWithoutSeparator(t *testing.T) {
	// /tmp/repo-evil must NOT be considered contained in /tmp/repo — a naive
	// strings.HasPrefix check would wrongly accept it.
	root := t.TempDir()
	evilSibling := root + "-evil"
	if err := os.MkdirAll(evilSibling, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(evilSibling) })

	ok, err := containsPath(root, evilSibling)
	if err != nil {
		t.Fatalf("containsPath: %v", err)
	}
	if ok {
		t.Fatalf("string-prefix sibling %q must not be contained in %q", evilSibling, root)
	}
}
