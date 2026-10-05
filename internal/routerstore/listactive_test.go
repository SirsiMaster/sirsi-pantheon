package routerstore

import (
	"context"
	"testing"
)

// ListActive returns open work and the closed items open work depends on, but not
// unrelated history; CountClosed counts the history without reading it (both
// directions, agreeing with ListAll).
func TestListActiveExcludesUnrelatedHistoryKeepsDependencies(t *testing.T) {
	s := newTestStore(t)
	mk := func(to string) string {
		id, err := s.Send("sender", to, "t-"+to, "proposal", "body")
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	openA := mk("a")
	dep := mk("b") // will be closed, but openA depends on it
	oldHistory := mk("c")
	waiting := mk("d")
	for _, id := range []string{dep, oldHistory} {
		if err := s.CloseItem(id, "done"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetBlockedBy(waiting, dep); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListActive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, it := range got {
		have[it.ID] = true
	}
	if !have[openA] || !have[waiting] {
		t.Fatalf("open items missing: %v", have)
	}
	if !have[dep] {
		t.Fatal("a closed item that open work is blocked on must be included (dependency truth)")
	}
	if have[oldHistory] {
		t.Fatal("unrelated closed history must not be returned")
	}
	n, err := s.CountClosed(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("CountClosed = %d, %v; want 2", n, err)
	}
	all, _ := s.ListAll(context.Background())
	closedInAll := 0
	for _, it := range all {
		if it.Status == "closed" {
			closedInAll++
		}
	}
	if closedInAll != n {
		t.Fatalf("CountClosed %d disagrees with ListAll %d", n, closedInAll)
	}
}
