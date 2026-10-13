package ledger

import (
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// AttachAvailability uses the same observed consumer truth as router ping.
// Human/service recipients are actors, not model consumers. Missing registry
// entries remain unknown rather than inheriting a fabricated wake path.
func AttachAvailability(snap *Snapshot, reg *router.Registry, threads *router.ThreadRegistry, now time.Time) {
	for i := range snap.Agents {
		a := &snap.Agents[i]
		a.Availability = nil
		a.RecipientType = ""
		cfg, known := reg.Agents[a.AgentID]
		if !known {
			a.Availability = &router.PingResult{Agent: a.AgentID, Verdict: "UNKNOWN", Detail: "recipient absent from current registry"}
			continue
		}
		a.RecipientType = cfg.Type
		if cfg.Type == "human" || cfg.Type == "service" {
			continue
		}
		result := router.PingLane(threads, cfg, a.AgentID, now)
		a.Availability = &result
	}
}
