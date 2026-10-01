package routerstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SpoolAgentOutbox is one agent's held (queued-for-retry) outbox depth on a
// spool relay host — a read-only count, never a mutation. An item lands here
// when the relay could not reach the service and is holding the request for
// drainOutbox's next scheduled retry (ADR-069).
//
// Unreadable (with Error set) means the outbox directory exists but could not
// be listed (permission denied, not-a-directory, ...) — this is NOT the same
// as "no held items" and must render as unknown/error, never as quiet
// (codex-pantheon review of PR #931, item 20261001-010622: filepath.Glob
// silently swallows directory read errors, reporting a chmod-0000 outbox as
// nil/nil — zero held items, indistinguishable from genuinely empty).
type SpoolAgentOutbox struct {
	Agent          string `json:"agent"`
	QueuedForRetry int    `json:"queued_for_retry,omitempty"`
	Unreadable     bool   `json:"unreadable,omitempty"`
	Error          string `json:"error,omitempty"`
}

// SpoolOutboxHealth lists every agent under spoolRoot that has a held outbox
// item OR an unreadable outbox directory, sorted by agent id. It only reads
// <spoolRoot>/<agent>/outbox/ — it never drains, retries, or deletes
// anything. An absent outbox directory is legitimately quiet (no entry); any
// other read failure (permission denied, not a directory, ...) is reported
// as Unreadable, never silently counted as zero. Call it only on a host that
// actually runs the relay (routerstore.SpoolDir(SIRSI_ROUTER_URL) != "");
// elsewhere spoolRoot will not exist and this returns that error honestly.
func SpoolOutboxHealth(spoolRoot string) ([]SpoolAgentOutbox, error) {
	entries, err := os.ReadDir(spoolRoot)
	if err != nil {
		return nil, fmt.Errorf("read spool root %s: %w", spoolRoot, err)
	}
	var out []SpoolAgentOutbox
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, rErr := os.ReadDir(filepath.Join(spoolRoot, e.Name(), "outbox"))
		if rErr != nil {
			if errors.Is(rErr, os.ErrNotExist) {
				continue // no outbox at all — legitimately quiet
			}
			out = append(out, SpoolAgentOutbox{Agent: e.Name(), Unreadable: true, Error: rErr.Error()})
			continue
		}
		count := 0
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
				count++
			}
		}
		if count == 0 {
			continue
		}
		out = append(out, SpoolAgentOutbox{Agent: e.Name(), QueuedForRetry: count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Agent < out[j].Agent })
	return out, nil
}
