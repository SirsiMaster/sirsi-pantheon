package routerstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// SpoolAgentOutbox is one agent's held (queued-for-retry) outbox depth on a
// spool relay host — a read-only count, never a mutation. An item lands here
// when the relay could not reach the service and is holding the request for
// drainOutbox's next scheduled retry (ADR-069).
type SpoolAgentOutbox struct {
	Agent          string `json:"agent"`
	QueuedForRetry int    `json:"queued_for_retry"`
}

// SpoolOutboxHealth lists every agent under spoolRoot with at least one held
// outbox item, sorted by agent id. It only reads <spoolRoot>/<agent>/outbox/ —
// it never drains, retries, or deletes anything. Call it only on a host that
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
		held, _ := filepath.Glob(filepath.Join(spoolRoot, e.Name(), "outbox", "*.json"))
		if len(held) == 0 {
			continue
		}
		out = append(out, SpoolAgentOutbox{Agent: e.Name(), QueuedForRetry: len(held)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Agent < out[j].Agent })
	return out, nil
}
