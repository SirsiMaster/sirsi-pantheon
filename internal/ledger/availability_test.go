package ledger

import (
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestAttachAvailabilityUsesConsumerAndActorTruth(t *testing.T) {
	now := time.Now()
	snap := Snapshot{Agents: []Agent{{AgentID: "lane"}, {AgentID: "owner"}, {AgentID: "missing"}}}
	reg := &router.Registry{Agents: map[string]router.AgentConfig{"lane": {Type: "codex"}, "owner": {Type: "human"}}}
	threads := &router.ThreadRegistry{Threads: map[string]*router.Thread{"live": {ThreadID: "live", AgentID: "lane", Surface: "codex", Status: router.ThreadStatusActive, LastSeenAt: now}}}
	AttachAvailability(&snap, reg, threads, now)
	if snap.Agents[0].Availability.Verdict != router.VerdictLive || !snap.Agents[0].Availability.Workable {
		t.Fatal("attended session must override wake-none")
	}
	if snap.Agents[1].RecipientType != "human" || snap.Agents[1].Availability != nil {
		t.Fatal("human must not be diagnosed as an unstaffed model worker")
	}
	if snap.Agents[2].Availability.Verdict != "UNKNOWN" {
		t.Fatal("missing declaration must remain unknown")
	}
	threads.Threads["live"].Status = router.ThreadStatusClosed
	AttachAvailability(&snap, reg, threads, now)
	if snap.Agents[0].Availability.Workable {
		t.Fatal("closed session must stop being routable on next observation")
	}
}
