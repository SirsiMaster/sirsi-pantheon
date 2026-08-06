// Package work — prune.go
//
// Retention for the pull-model work queue. Closed items accumulate forever
// under items/ because the router is deliberately daemonless — nothing ever
// owned their lifecycle end. Prune compacts closed items whose `closed:`
// timestamp precedes a cutoff into tombstones, and NEVER touches an open item
// or one whose close date cannot be parsed (fail safe: keep, don't delete).
package work

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	After  int64
}

// PruneItems compacts closed items whose close timestamp is strictly before
// cutoff. When dryRun is true nothing is written — the returned slice reports
// what would have been compacted. Open items, already-tombstoned items, and
// closed items with a missing or unparseable `closed:` field are always kept.
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
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return pruned, readErr
		}
		if isTombstone(data) {
			continue
		}
		size := int64(len(data))
		tombstone, compactErr := compactItemTombstone(it, data)
		if compactErr != nil {
			return pruned, compactErr
		}
		after := int64(len(tombstone))
		if !dryRun {
			if writeErr := os.WriteFile(path, tombstone, 0o644); writeErr != nil {
				return pruned, writeErr
			}
		}
		pruned = append(pruned, PrunedItem{ID: it.ID, Closed: closedAt, Bytes: size, After: after})
	}
	return pruned, nil
}

func isTombstone(data []byte) bool {
	fm, _, ok := splitItem(data)
	return ok && strings.Contains(fm, "tombstoned: true")
}

func compactItemTombstone(it Item, data []byte) ([]byte, error) {
	fm, _, ok := splitItem(data)
	if !ok {
		return nil, fmt.Errorf("item %s: missing frontmatter", it.ID)
	}
	sum := sha256.Sum256(data)
	now := time.Now().UTC().Format(time.RFC3339)
	lines := strings.Split(fm, "\n")
	lines = upsertFrontmatter(lines, "tombstoned", "true")
	lines = upsertFrontmatter(lines, "tombstoned_at", quoteYAML(now))
	lines = upsertFrontmatter(lines, "content_hash_sha256", quoteYAML(hex.EncodeToString(sum[:])))

	body := fmt.Sprintf(`## Tombstone

Payload compacted by router retention. The item id, frontmatter provenance,
close timestamp, and original content hash are retained.

- item_id: %s
- original_hash_sha256: %s
`, it.ID, hex.EncodeToString(sum[:]))

	return []byte("---\n" + strings.Join(lines, "\n") + "\n---\n\n" + body), nil
}

func splitItem(data []byte) (frontmatter, body string, ok bool) {
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return "", "", false
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return "", "", false
	}
	return content[4 : 4+end], content[4+end+5:], true
}

func upsertFrontmatter(lines []string, key, value string) []string {
	prefix := key + ":"
	out := make([]string, 0, len(lines)+1)
	replaced := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			if !replaced {
				out = append(out, key+": "+value)
				replaced = true
			}
			continue
		}
		out = append(out, line)
	}
	if !replaced {
		out = append(out, key+": "+value)
	}
	return out
}
