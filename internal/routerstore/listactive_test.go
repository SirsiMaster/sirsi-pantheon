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

// ListSince returns open work and recent closures but not old history, and a
// recently closed item is present exactly when its closed time is within the window.
func TestListSinceBoundsHistoryByClosedTime(t *testing.T) {
	s := newTestStore(t)
	mk := func(to string) string {
		id, err := s.Send("sender", to, "t-"+to, "proposal", "body")
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	openID, recent := mk("a"), mk("b")
	if err := s.CloseItem(recent, "done"); err != nil {
		t.Fatal(err)
	}
	future := "2999-01-01T00:00:00Z"
	got, err := s.ListSince(context.Background(), "2000-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, it := range got {
		have[it.ID] = true
	}
	if !have[openID] || !have[recent] {
		t.Fatalf("open and recently closed must both be returned: %v", have)
	}
	got, _ = s.ListSince(context.Background(), future)
	have = map[string]bool{}
	for _, it := range got {
		have[it.ID] = true
	}
	if !have[openID] || have[recent] {
		t.Fatalf("a window after the closure must drop the closed item but keep open work: %v", have)
	}
}

// ListSinceHeaders is ListSince's exact row set and header fields with the bodies
// empty; ListSince itself keeps the bodies (history readback depends on them).
func TestListSinceHeadersDropsOnlyTheBodies(t *testing.T) {
	s := newTestStore(t)
	openID, err := s.Send("sender", "a", "t-open", "proposal", "open body")
	if err != nil {
		t.Fatal(err)
	}
	closedID, err := s.Send("sender", "b", "t-closed", "proposal", "closed body")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CloseItem(closedID, "the result"); err != nil {
		t.Fatal(err)
	}
	ctx, since := context.Background(), "2000-01-01T00:00:00Z"
	full, err := s.ListSince(ctx, since)
	if err != nil {
		t.Fatal(err)
	}
	lean, err := s.ListSinceHeaders(ctx, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(lean) != len(full) || len(full) != 2 {
		t.Fatalf("row sets differ: headers %d, full %d (want 2)", len(lean), len(full))
	}
	for i := range full {
		f, l := full[i], lean[i]
		if f.Instructions == "" {
			t.Fatalf("ListSince must keep bodies: %s has no instructions", f.ID)
		}
		if l.Instructions != "" || l.Result != "" {
			t.Fatalf("ListSinceHeaders leaked a body for %s: %q / %q", l.ID, l.Instructions, l.Result)
		}
		f.Instructions, f.Result = "", ""
		if f != l {
			t.Fatalf("header fields differ for %s:\nfull %+v\nlean %+v", f.ID, f, l)
		}
	}
	if full[0].ID != openID && full[1].ID != openID {
		t.Fatal("open item missing")
	}
}
