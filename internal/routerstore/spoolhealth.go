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
	Observed       bool   `json:"observed"`
	Readable       bool   `json:"readable"`
	QueuedForRetry int    `json:"queued_for_retry"`
	Unreadable     bool   `json:"unreadable"`
	Error          string `json:"error,omitempty"`
}

// SpoolOutboxHealth reports every observed agent directory under spoolRoot,
// sorted by agent id. Only a listed outbox directory has Readable=true; a
// missing directory remains an explicit observation with Readable=false so a
// consumer cannot mistake absence for an empty readable queue. This only reads
// the filesystem; it never drains, retries, or deletes anything. Call it only
// on a host that actually runs the relay (routerstore.SpoolDir(SIRSI_ROUTER_URL)
// != ""); elsewhere no outbox observation is emitted.
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
				out = append(out, SpoolAgentOutbox{
					Agent: e.Name(), Observed: true, Readable: false,
					Error: "outbox directory is missing",
				})
				continue
			}
			out = append(out, SpoolAgentOutbox{
				Agent: e.Name(), Observed: true, Readable: false,
				Unreadable: true, Error: rErr.Error(),
			})
			continue
		}
		count := 0
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
				count++
			}
		}
		out = append(out, SpoolAgentOutbox{
			Agent: e.Name(), Observed: true, Readable: true, QueuedForRetry: count,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Agent < out[j].Agent })
	return out, nil
}
