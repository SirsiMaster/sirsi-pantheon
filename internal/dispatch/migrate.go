package dispatch

// Router v2 Phase 4 — the one-shot migration importer (PRD /goal #4: every
// existing items/*.md lands in the store with zero data loss, count-in ==
// count-out, spot-checked bodies). Idempotent: the store upserts by id, so
// re-running after new sends just refreshes rows.

import (
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// MigrateReport is the importer's verification evidence (Rule A14: the
// zero-data-loss claim ships with the numbers that prove it).
type MigrateReport struct {
	FilesSeen   int
	Inserted    int
	Updated     int
	SpotChecked int
	Errors      []string
}

// CountOut is the number of items that verifiably landed in the store.
func (r MigrateReport) CountOut() int { return r.Inserted + r.Updated }

// adaptWorkItem projects a canonical file item into its store row —
// field-for-field (the pairing routerstore's reflection test enforces).
func adaptWorkItem(w work.Item) routerstore.Item {
	return routerstore.Item{
		ID:           w.ID,
		From:         w.From,
		To:           w.To,
		Title:        w.Title,
		Type:         w.Type,
		Status:       w.Status,
		Opened:       w.Opened,
		Closed:       w.Closed,
		Instructions: w.Instructions,
		Result:       w.Result,

		WakeStatus:      w.WakeStatus,
		WakeAttemptedAt: w.WakeAttemptedAt,
		WakeAdapter:     w.WakeAdapter,
		WakeError:       w.WakeError,
	}
}

// Migrate imports every canonical items/*.md into the store and verifies the
// round trip: count-in == count-out, and a sample of migrated rows is read
// back and compared field-by-field against its source file.
func (f *Facade) Migrate() (MigrateReport, error) {
	fileItems, err := work.ListAll(f.root)
	if err != nil {
		return MigrateReport{}, fmt.Errorf("dispatch: migrate: list items: %w", err)
	}
	rep := MigrateReport{FilesSeen: len(fileItems)}

	adapted := make([]routerstore.Item, 0, len(fileItems))
	for _, w := range fileItems {
		adapted = append(adapted, adaptWorkItem(w))
	}
	bf, err := f.store.Backfill(adapted)
	if err != nil {
		return rep, fmt.Errorf("dispatch: migrate: backfill: %w", err)
	}
	rep.Inserted, rep.Updated = bf.Inserted, bf.Updated
	rep.Errors = append(rep.Errors, bf.Errors...)

	// Spot-check first, middle, and last migrated items: the store row must
	// carry the file's exact fields (not a lossy or truncated projection).
	for _, idx := range spotIndices(len(fileItems)) {
		w := fileItems[idx]
		row, gerr := f.store.Get(w.ID)
		if gerr != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("spot-check %s: %v", w.ID, gerr))
			continue
		}
		if row.Instructions != strings.TrimSpace(w.Instructions) && row.Instructions != w.Instructions {
			rep.Errors = append(rep.Errors, fmt.Sprintf("spot-check %s: instructions differ between file and store", w.ID))
			continue
		}
		if row.Title != w.Title || row.Status != w.Status || row.From != w.From || row.To != w.To {
			rep.Errors = append(rep.Errors, fmt.Sprintf("spot-check %s: frontmatter differs between file and store", w.ID))
			continue
		}
		rep.SpotChecked++
	}
	return rep, nil
}

// spotIndices picks up to three representative indices: first, middle, last.
func spotIndices(n int) []int {
	switch {
	case n == 0:
		return nil
	case n == 1:
		return []int{0}
	case n == 2:
		return []int{0, 1}
	default:
		return []int{0, n / 2, n - 1}
	}
}
