// Package router — strand.go
//
// Strand visibility (owner priority 2026-06-19, router hardening PR #2). Work
// routed to an agent with no ARMED thread used to sit silently until someone
// noticed. node-status now surfaces "stranded inboxes" — agents that have open
// items but no live watcher consuming them — so stranded work is visible instead
// of lost.
//
// This reuses the honest-liveness verdict (#79/#80): an agent is armed when at
// least one of its non-terminal threads is armed, classified by watcher_type
// (loop-monitor needs a live thr-id loop; every other surface is armed by a fresh
// heartbeat). The wake ACTION (invoke a configured adapter, or install a per-agent
// LaunchAgent pull-loop) is the next slice; this one adds zero spawn/orphan risk —
// it only makes the stranding visible.
package router

import (
	"sort"
	"time"
)

// threadArmed computes whether ONE thread is armed, using the SAME classification
// as CollectNodeStatus's per-thread block (#79/#80). Kept in lockstep with that
// switch: loop-monitor surfaces (Claude /loop) require a live thr-id loop; every
// other surface is armed by a fresh (non-stale) heartbeat.
func threadArmed(thr *Thread, now time.Time) bool {
	wtype := WatcherFor(thr.Surface, thr.AgentID, thr.ThreadID).Type
	switch {
	case requiresThreadIDLoop(wtype):
		return thr.ThreadID != "" && WatcherAlive(thr.ThreadID)
	case thr.IsStale(now, DefaultThreadStaleAfter):
		return false
	default: // app-heartbeat / native-runloop / surface-loop / pull-loop — heartbeat is the proof
		return true
	}
}

// AgentArmed reports whether agentID has at least one ARMED thread — a live watcher
// actually consuming its inbox. Terminal and suspended records never count. A
// missing/unreadable registry reads as unarmed (fail-closed: better to flag a
// possible strand than to hide it).
func AgentArmed(routerRoot, agentID string) bool {
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return false
	}
	now := time.Now().UTC()
	for _, t := range reg.Threads {
		if t == nil || t.AgentID != agentID {
			continue
		}
		if t.Status.IsTerminal() || t.Status == ThreadStatusSuspended {
			continue
		}
		if threadArmed(t, now) {
			return true
		}
	}
	return false
}

// AgentHasLiveThread reports whether agentID has at least one LIVE (non-terminal,
// non-suspended, fresh-heartbeat) thread — a running session/loop, whether or not
// it is "armed" in the loop-monitor sense. Used to guard `wake-install`: arming a
// background wake LaunchAgent for an agent that already has a live session spawns
// duplicate processes each tick (the 2026-07-08 wake-loop leak,
// reference_schedulewakeup_process_leak). A live interactive session is already
// handling the inbox; a background channel on top of it is the leak.
func AgentHasLiveThread(routerRoot, agentID string) bool {
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return false
	}
	now := time.Now().UTC()
	for _, t := range reg.Threads {
		if t == nil || t.AgentID != agentID {
			continue
		}
		if t.Status.IsTerminal() || t.Status == ThreadStatusSuspended {
			continue
		}
		if !t.IsStale(now, DefaultThreadStaleAfter) {
			return true
		}
	}
	return false
}

// AgentLoopDead reports whether agentID actually needs a loop-dead alarm: it
// has open inbox items AND zero armed live threads. An agent needs ONE armed
// watcher to consume its inbox — extra live sessions of the same agent (the
// CCD duplicate-record artifact: several concurrent claude.app sessions, none
// running /loop) are not each obligated to run a loop, and flagging every one
// of them over-fires per-session (router item 20260714-210359). An agent with
// no open items has nothing stranding either way.
func AgentLoopDead(routerRoot, agentID string, pendingByAgent map[string][]string) bool {
	return len(pendingByAgent[agentID]) > 0 && !AgentArmed(routerRoot, agentID)
}

// StrandedAgent is one agent with open inbox items but no armed thread watching
// them — work that sits until the agent is (re)armed.
type StrandedAgent struct {
	AgentID   string `json:"agent_id"`
	OpenItems int    `json:"open_items"`
}

// computeStranded returns the agents that have pending items but no armed thread,
// sorted by descending backlog. Reuses the already-computed PendingByAgent, so it
// adds no extra item scan.
func computeStranded(routerRoot string, pendingByAgent map[string][]string) []StrandedAgent {
	var stranded []StrandedAgent
	for agent, items := range pendingByAgent {
		if len(items) == 0 || AgentArmed(routerRoot, agent) {
			continue
		}
		stranded = append(stranded, StrandedAgent{AgentID: agent, OpenItems: len(items)})
	}
	sort.Slice(stranded, func(i, j int) bool {
		if stranded[i].OpenItems != stranded[j].OpenItems {
			return stranded[i].OpenItems > stranded[j].OpenItems
		}
		return stranded[i].AgentID < stranded[j].AgentID
	})
	return stranded
}
