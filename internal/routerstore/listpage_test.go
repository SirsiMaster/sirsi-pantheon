package routerstore

import (
	"context"
	"fmt"
	"testing"
)

// Paging the ledger visits every item exactly once, in the same order as ListAll, for page sizes that
// divide the ledger evenly, leave a short tail, exceed it, and for out-of-range sizes (clamped).
func TestListAllPagedEqualsListAllForEveryPageSize(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 11; i++ {
		id, err := s.Send("sender", "a", fmt.Sprintf("t%02d", i), "proposal", "body")
		if err != nil {
			t.Fatal(err)
		}
		if i%3 == 0 {
			if err := s.CloseItem(id, "done"); err != nil {
				t.Fatal(err)
			}
		}
	}
	want, err := s.ListAll(context.Background())
	if err != nil || len(want) != 11 {
		t.Fatalf("ListAll = %d, %v; want 11", len(want), err)
	}
	for _, size := range []int{1, 2, 5, 11, 12, 100, 0, -3, ListPageMax + 1} {
		got, err := ListAllPaged(context.Background(), s, size)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if len(got) != len(want) {
			t.Fatalf("size %d: %d items, want %d (a cursor bug duplicates or drops)", size, len(got), len(want))
		}
		for i := range want {
			if got[i].ID != want[i].ID {
				t.Fatalf("size %d: position %d is %s, want %s", size, i, got[i].ID, want[i].ID)
			}
		}
	}
}

// One page is bounded by its limit and starts strictly after the cursor.
func TestListPageIsBoundedAndStrictlyAfterTheCursor(t *testing.T) {
	s := newTestStore(t)
	var ids []string
	for i := 0; i < 6; i++ {
		id, err := s.Send("sender", "a", fmt.Sprintf("t%02d", i), "proposal", "body")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	p1, err := s.ListPage(context.Background(), "", 4)
	if err != nil || len(p1) != 4 {
		t.Fatalf("first page = %d, %v; want 4", len(p1), err)
	}
	p2, err := s.ListPage(context.Background(), p1[3].ID, 4)
	if err != nil || len(p2) != 2 || p2[0].ID == p1[3].ID {
		t.Fatalf("second page = %d (first id %v), %v; want the 2 items after the cursor", len(p2), func() string {
			if len(p2) > 0 {
				return p2[0].ID
			}
			return ""
		}(), err)
	}
	if p, _ := s.ListPage(context.Background(), ids[len(ids)-1], 4); len(p) != 0 {
		t.Fatalf("page after the last id = %d, want 0", len(p))
	}
}

// The wire carries the paged read: a RemoteStore walking pages over the real handler returns exactly
// what the backend holds, in order (this is the path the spool relay forwards for `router dump`).
func TestRemoteListAllPagedOverTheWire(t *testing.T) {
	backend, rs := remoteHarness(t)
	for i := 0; i < 7; i++ {
		if _, err := backend.Send("sender", "a", fmt.Sprintf("w%02d", i), "proposal", "body"); err != nil {
			t.Fatal(err)
		}
	}
	want, err := backend.ListAll(context.Background())
	if err != nil || len(want) != 7 {
		t.Fatalf("backend ListAll = %d, %v", len(want), err)
	}
	got, err := ListAllPaged(context.Background(), rs, 3)
	if err != nil {
		t.Fatalf("paged over the wire: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("over the wire %d items, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID {
			t.Fatalf("position %d: %s, want %s", i, got[i].ID, want[i].ID)
		}
	}
}
