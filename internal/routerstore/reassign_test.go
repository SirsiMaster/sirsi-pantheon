package routerstore

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReassignItem(t *testing.T) {
	s, err := OpenPath(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	id, err := s.Send("a", "old", "move me", "", "body")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReassignItem(id, "old", "new", "\nnote"); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	it, _ := s.Get(id)
	if it.To != "new" || it.From != "a" || !strings.HasSuffix(it.Instructions, "body\nnote") {
		t.Fatalf("after reassign: %+v", it)
	}
	// A stale caller (still thinks it is addressed to "old") loses: guarded on from.
	if err := s.ReassignItem(id, "old", "other", ""); err == nil {
		t.Fatal("reassign with a stale from must be refused")
	}
	// Unknown id → ErrNotFound.
	if err := s.ReassignItem("nope", "x", "y", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v, want ErrNotFound", err)
	}
	// Closed items never move.
	if err := s.CloseItem(id, "done"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReassignItem(id, "new", "other", ""); err == nil {
		t.Fatal("reassign of a closed item must be refused")
	}
	// Claimed (leased) items never move.
	id2, _ := s.Send("a", "new", "claim me", "", "b")
	if _, err := s.ClaimNext("new", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.ReassignItem(id2, "new", "other", ""); err == nil {
		t.Fatal("reassign of a claimed item must be refused")
	}
}
