// Package work — prune.go
//
// Retention for the pull-model work queue. Closed items accumulate forever
// under items/ because the router is deliberately daemonless — nothing ever
// owned their lifecycle end. Prune deletes closed items whose `closed:`
// timestamp precedes a cutoff, and NEVER touches an open item or one whose
// close date cannot be parsed (fail safe: keep, don't delete).
package work

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PrunedItem records a single item removed (or that would be removed under
// --dry-run) by Prune.
type PrunedItem struct {
	ID     string
	Closed time.Time
	Bytes  int64
}

// PruneItems removes closed items whose close timestamp is strictly before
// cutoff. When dryRun is true nothing is deleted — the returned slice reports
// what would have been removed. Open items, and closed items with a missing or
// unparseable `closed:` field, are always kept.
func PruneItems(root string, cutoff time.Time, dryRun bool) ([]PrunedItem, error) {
	items, err := ListAll(root)
	if err != nil {
		return nil, err
	}
	var pruned []PrunedItem
	for _, it := range items {
		if it.Status != "closed" {
			continue
		}
		closedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(it.Closed))
		if err != nil {
			// No parseable close date → never delete. A closed item without a
			// timestamp is a data anomaly, not a retention candidate.
			continue
		}
		if !closedAt.Before(cutoff) {
			continue
		}
		path := filepath.Join(itemsDir(root), it.ID+".md")
		var size int64
		if fi, statErr := os.Stat(path); statErr == nil {
			size = fi.Size()
		}
		if !dryRun {
			if rmErr := os.Remove(path); rmErr != nil {
				return pruned, rmErr
			}
		}
		pruned = append(pruned, PrunedItem{ID: it.ID, Closed: closedAt, Bytes: size})
	}
	return pruned, nil
}
