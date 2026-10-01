package routerstore

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestReopenItem(t *testing.T) {
	s, err := OpenPath(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	id, _ := s.Send("a", "b", "t", "", "body")

	if err := s.ReopenItem(id, "\nnote\n"); err == nil {
		t.Fatal("reopening an OPEN item must be refused")
	}
	if err := s.ReopenItem("nope", "n"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v, want ErrNotFound", err)
	}
	if err := s.CloseItem(id, "the close result"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReopenItem(id, "\nnote\n"); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	it, _ := s.Get(id)
	if it.Status != "open" || it.Closed != "" || it.Result != "" || !strings.HasSuffix(it.Instructions, "body\nnote\nthe close result") {
		t.Fatalf("after reopen: %+v", it)
	}
	if err := s.ReopenItem(id, "n"); err == nil {
		t.Fatal("second reopen of an open item must be refused")
	}
	// Close works again after a reopen (the round trip is clean).
	if err := s.CloseItem(id, "closed once more"); err != nil {
		t.Fatalf("close after reopen: %v", err)
	}
}
