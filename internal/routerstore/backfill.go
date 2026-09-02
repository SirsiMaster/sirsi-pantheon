package routerstore

import "fmt"

// BackfillReport summarizes a Backfill run so a future `sirsi router migrate`
// (PRD Phase 4) can prove zero data loss: count-in == Inserted+Updated, and a
// caller can spot-check bodies via Get.
type BackfillReport struct {
	Seen     int      // items handed to Backfill
	Inserted int      // ids not previously present
	Updated  int      // ids that already existed and were refreshed
	Errors   []string // per-item failures (id: reason); Backfill is best-effort
}

// Backfill mirrors a slice of items into the store. It is the store-side spine
// of the Phase 4 one-shot importer: the caller reads every <root>/items/*.md
// via internal/work.ListAll, adapts each work.Item to a routerstore.Item, and
// hands the slice here. Keeping the input a plain []Item (not a work.Item)
// avoids an import cycle and keeps this Phase-1 package free of any dependency
// on the live router — it stays wired to nothing.
//
// Backfill is idempotent: re-running it over the same items updates rows in
// place rather than duplicating them (each Put upserts on id), so `sirsi router
// migrate` can be run repeatedly safely (PRD /goal #4 idempotency).
//
// It is best-effort: a single malformed item is recorded in Errors and skipped
// rather than aborting the whole migration, so one bad file never strands the
// rest of the queue.
func (s *SQLiteStore) Backfill(items []Item) (BackfillReport, error) {
	rep := BackfillReport{Seen: len(items)}
	for _, it := range items {
		existed := false
		if _, err := s.Get(it.ID); err == nil {
			existed = true
		}
		if err := s.Put(it); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", it.ID, err))
			continue
		}
		if existed {
			rep.Updated++
		} else {
			rep.Inserted++
		}
	}
	return rep, nil
}
