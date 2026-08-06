package routerstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestAllocateIdentifierNeverCrossClaims is the point of the whole allocator.
//
// The failure it prevents is real and live: origin/main carries two distinct
// ADR-054 documents because two agents each read the filesystem, each saw 053
// as the highest, and each concluded 054 was free. Any allocator that reads a
// high-water mark and then writes MUST hold a write lock across both, or it
// reproduces exactly that bug.
//
// Concurrency here is not decoration — a serial test passes even against a
// naive read-then-write implementation, so it would not catch the regression.
func TestAllocateIdentifierNeverCrossClaims(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "router.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	const n = 25
	var wg sync.WaitGroup
	got := make([]Identifier, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i], errs[i] = s.AllocateIdentifier("ADR", "concurrent claim", "agent")
		}(i)
	}
	wg.Wait()

	seen := map[int]bool{}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("allocation %d failed: %v", i, errs[i])
		}
		if seen[got[i].Number] {
			t.Fatalf("CROSS-CLAIM: number %d issued twice — this is the two-ADR-054 bug", got[i].Number)
		}
		seen[got[i].Number] = true
	}
	if len(seen) != n {
		t.Fatalf("want %d distinct numbers, got %d", n, len(seen))
	}
	// Numbers must be a dense 1..n range: gaps would mean a lost allocation.
	for i := 1; i <= n; i++ {
		if !seen[i] {
			t.Fatalf("gap at %d — an allocation was lost", i)
		}
	}
}

func TestWithdrawnNumberIsNeverReissued(t *testing.T) {
	s := newTestStore(t)

	first, err := s.AllocateIdentifier("ADR", "doomed", "claude-home")
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if err = s.WithdrawIdentifier("ADR", first.Number); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	next, err2 := s.AllocateIdentifier("ADR", "successor", "claude-home")
	err = err2
	if err != nil {
		t.Fatalf("allocate after withdraw: %v", err)
	}
	if next.Number == first.Number {
		t.Fatalf("withdrawn number %d was reissued — every existing citation of it now silently retargets", first.Number)
	}
}

func TestClaimSpecificNumberRefusesAnotherOwner(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.ClaimIdentifierNumber("ADR", 54, "contracts and identity", "docs/ADR-054-CONTRACTS.md", "claude-pantheon"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	// Second agent tries to take the same number — the live bug, replayed.
	_, err := s.ClaimIdentifierNumber("ADR", 54, "one horus", "docs/ADR-054-ONE-HORUS.md", "claude-home")
	if !errors.Is(err, ErrIdentifierTaken) {
		t.Fatalf("want ErrIdentifierTaken, got %v", err)
	}
	if !strings.Contains(err.Error(), "claude-pantheon") {
		t.Fatalf("error must name the current holder so the loser knows who to talk to, got: %v", err)
	}

	// Idempotent for the SAME owner, so backfill can be re-run.
	if _, err := s.ClaimIdentifierNumber("ADR", 54, "contracts and identity", "docs/ADR-054-CONTRACTS.md", "claude-pantheon"); err != nil {
		t.Fatalf("re-claim by same owner must be idempotent, got: %v", err)
	}
}

func TestAllocateRequiresOwnerAndTitle(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AllocateIdentifier("ADR", "titled", ""); err == nil {
		t.Fatal("want error for empty owner")
	}
	if _, err := s.AllocateIdentifier("ADR", "", "claude-home"); err == nil {
		t.Fatal("want error for empty title")
	}
	if _, err := s.AllocateIdentifier("not-a-namespace", "t", "o"); err == nil {
		t.Fatal("want error for invalid namespace")
	}
}

func TestNamespacesAreIndependent(t *testing.T) {
	s := newTestStore(t)
	adr, err := s.AllocateIdentifier("ADR", "an adr", "claude-home")
	if err != nil {
		t.Fatalf("adr: %v", err)
	}
	req, err := s.AllocateIdentifier("REQ", "a requirement", "claude-home")
	if err != nil {
		t.Fatalf("req: %v", err)
	}
	if adr.Number != 1 || req.Number != 1 {
		t.Fatalf("namespaces must not share a counter: ADR=%d REQ=%d", adr.Number, req.Number)
	}
	if adr.Name() != "ADR-001" || req.Name() != "REQ-001" {
		t.Fatalf("citation form wrong: %q %q", adr.Name(), req.Name())
	}
}

// Guards the backfill script's contract: every ADR file on disk must be
// claimable, and a second file claiming the same number must fail loudly.
func TestBackfillDetectsRealCollisionShape(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()
	// Two files, same number — the exact shape on origin/main today.
	for _, name := range []string{"ADR-054-CONTRACTS-IDENTITY.md", "ADR-054-ONE-HORUS.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var failures int
	for _, name := range []string{"ADR-054-CONTRACTS-IDENTITY.md", "ADR-054-ONE-HORUS.md"} {
		if _, err := s.ClaimIdentifierNumber("ADR", 54, name, name, "owner-of-"+name); err != nil {
			failures++
		}
	}
	if failures != 1 {
		t.Fatalf("exactly one of two same-numbered documents must fail to claim, got %d failures", failures)
	}
}
