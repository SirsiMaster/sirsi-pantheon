package dispatch

import (
	"strings"
	"testing"
)

// TestReopenUndoesClose: a closed item returns to open, its close result is
// preserved in the body with who/why, and authority matches close. Both
// directions (A35): allowed paths succeed, every refusal path refuses.
func TestReopenUndoesClose(t *testing.T) {
	f := testFacade(t)
	res, err := f.Send("a", "b", "closed by mistake", "", "original body")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.CloseItem("b", res.ID, "wrongly closed"); err != nil {
		t.Fatal(err)
	}

	// Refusals first: no reason, a stranger, an open item.
	if err = f.Reopen("b", res.ID, "  "); err == nil {
		t.Fatal("reopen without a reason must be refused")
	}
	if err = f.Reopen("claude-home", res.ID, "not mine"); err == nil {
		t.Fatal("a stranger without close:any must be refused")
	}

	// The recipient reopens: status open, close result kept, reason recorded.
	if err = f.Reopen("b", res.ID, "closed in error"); err != nil {
		t.Fatalf("recipient reopen: %v", err)
	}
	it, err := f.store.Get(res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if it.Status != "open" || it.Closed != "" || it.Result != "" {
		t.Fatalf("after reopen: status=%q closed=%q result=%q", it.Status, it.Closed, it.Result)
	}
	for _, want := range []string{"original body", "reopened by b", "closed in error", "wrongly closed"} {
		if !strings.Contains(it.Instructions, want) {
			t.Errorf("body missing %q: %q", want, it.Instructions)
		}
	}

	// Reopening an open item is refused (no double-reopen).
	if err = f.Reopen("b", res.ID, "again"); err == nil {
		t.Fatal("reopening an already-open item must be refused")
	}

	// close:any holder may reopen someone else's closed item.
	if err = f.CloseItem("b", res.ID, "closed again"); err != nil {
		t.Fatal(err)
	}
	if err = f.Reopen("supervisor", res.ID, "supervisor undo"); err != nil {
		t.Fatalf("close:any reopen: %v", err)
	}
}

// TestReopenOwnerItemOnlyByOwner: an agent must not put work back on the owner's
// board; the owner can.
func TestReopenOwnerItemOnlyByOwner(t *testing.T) {
	f := testFacade(t)
	res, err := f.Send("a", "owner", "decision", "", "x")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.CloseItem(res.ID, "dismissed"); err != nil {
		t.Fatal(err)
	}
	if err = f.Reopen("supervisor", res.ID, "agent trying"); err == nil {
		t.Fatal("an agent (even close:any) must not reopen an owner-addressed item")
	}
	if err = f.Reopen("owner", res.ID, "owner wants it back"); err != nil {
		t.Fatalf("owner reopen: %v", err)
	}
}
