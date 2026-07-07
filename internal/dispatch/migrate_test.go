package dispatch

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// TestMigrateZeroLossAndIdempotent: PRD /goal #4 — every file item lands in
// the store (count-in == count-out), bodies spot-checked, and a re-run
// updates instead of duplicating.
func TestMigrateZeroLossAndIdempotent(t *testing.T) {
	f := testFacade(t)
	if err := work.EnsureRoot(f.root); err != nil {
		t.Fatal(err)
	}
	// Legacy-writer items (never touched the store), one of them closed.
	ids := make([]string, 0, 5)
	for i, title := range []string{"alpha", "bravo", "charlie", "delta", "echo"} {
		id, err := work.SendTyped(f.root, "legacy", "claude-pantheon", title, "", strings.Repeat("body ", i+1))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := work.Close(f.root, ids[1], "done early"); err != nil {
		t.Fatal(err)
	}

	rep, err := f.Migrate()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Errors) != 0 {
		t.Fatalf("migrate errors: %v", rep.Errors)
	}
	if rep.FilesSeen != 5 || rep.CountOut() != 5 || rep.Inserted != 5 {
		t.Fatalf("zero-loss violated: %+v", rep)
	}
	if rep.SpotChecked == 0 {
		t.Fatal("spot-check ran zero comparisons — the zero-loss claim is unverified")
	}

	// Idempotent re-run: updates, never duplicates.
	rep2, err := f.Migrate()
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Inserted != 0 || rep2.Updated != 5 {
		t.Fatalf("re-run must update-not-duplicate: %+v", rep2)
	}
}

// TestDualReadSurfacesStoreOnlyRows: a store row whose audit file is missing
// (failed write, hand-deleted file) still appears in the merged inbox —
// §2b axiom 8: a missing file cannot hide dispatched work.
func TestDualReadSurfacesStoreOnlyRows(t *testing.T) {
	f := testFacade(t)
	if err := work.EnsureRoot(f.root); err != nil {
		t.Fatal(err)
	}
	fileID, err := work.SendTyped(f.root, "a", "codex-pantheon", "file item", "", "x")
	if err != nil {
		t.Fatal(err)
	}
	storeID, _, err := f.store.SendGuarded(routerstore.SendReq{From: "a", To: "codex-pantheon", Title: "store only item", Instructions: "y"})
	if err != nil {
		t.Fatal(err)
	}

	items, err := f.Inbox("codex-pantheon")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, it := range items {
		got[it.ID] = true
	}
	if !got[fileID] || !got[storeID] {
		t.Fatalf("dual-read must union file+store items, got %v", got)
	}
	if len(items) != 2 {
		t.Fatalf("expected exactly 2 merged items, got %d", len(items))
	}
}
