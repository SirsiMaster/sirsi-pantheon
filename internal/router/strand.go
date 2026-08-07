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

// threadArmedForNewest is the single liveness predicate for one thread.
// The agent-keyed watcher probe is credited ONLY when isNewest=true (A35: scope the check to its claim).
//
// After a thread re-registers, the watcher script retains the OLD thread id in
// argv — WatcherAlive(newID) misses it. WatcherAliveByAgent finds the live
// script by its stable name. But crediting that for EVERY thread of the agent
// makes the oldest-stale record appear armed as long as the newest thread's
// watcher is alive — one PID vouching for N unrelated records. Only the newest
// non-terminal thread is a plausible beneficiary; all others are previous
// registrations whose watcher, if still alive, is serving the new session.
//
// Call sites that own the registry precompute newestByAgent via
// NewestActiveByAgent and pass isNewest=true for the matching thread only.
func threadArmedForNewest(thr *Thread, now time.Time, isNewest bool) bool {
	wtype := WatcherFor(thr.Surface, thr.AgentID, thr.ThreadID).Type
	switch {
	case requiresThreadIDLoop(wtype):
		return WatcherAliveForThread(thr, isNewest)
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
	threads := reg.SortedThreads()
	newestByAgent := NewestActiveByAgent(threads)
	for _, t := range threads {
		if t == nil || t.AgentID != agentID {
			continue
		}
		if t.Status.IsTerminal() || t.Status == ThreadStatusSuspended {
			continue
		}
		if threadArmedForNewest(t, now, newestByAgent[agentID] == t.ThreadID) {
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

// liveWakeAgents returns the set of agents whose per-agent launchd wake job
// (`ai.sirsi.router.wake.<agent>`) is loaded. After the store cutover this job —
// not a registry thread — is the durable inbox consumer for a background agent,
// so it MUST count as "watched". `launchctl list <label>` exits 0 iff loaded
// (the same domain-correct probe CollectNodeStatus/CollectLaunchAgents use). A
// nil checker (tests) yields an empty set, preserving registry-only semantics.
func liveWakeAgents(agents []string, launchctlCheck LaunchctlChecker) map[string]bool {
	live := map[string]bool{}
	if launchctlCheck == nil {
		return live
	}
	for _, a := range agents {
		if launchctlCheck("list", "ai.sirsi.router.wake."+a) == nil {
			live[a] = true
		}
	}
	return live
}

// noWakeAgents builds the set of agents whose wake mechanism is WakeNone —
// agents that have explicitly opted out of automatic waking. Pass this to
// computeStranded so those agents are excluded from the stranded report.
// Stranding is expected and not actionable for mechanism:none agents; including
// them only produces noise (the same category error as stranding "user").
func noWakeAgents(reg *Registry) map[string]bool {
	out := make(map[string]bool, len(reg.Agents))
	for id, cfg := range reg.Agents {
		if cfg.WakeMechanism() == WakeNone {
			out[id] = true
		}
	}
	return out
}

// computeStranded returns the agents that have pending items but no watcher
// consuming them, sorted by descending backlog. Reuses the already-computed
// PendingByAgent, so it adds no extra item scan. `liveWake` is the set of agents
// whose launchd wake job is loaded (see liveWakeAgents) — pass nil for
// registry-only classification. `noWake` is the set of agents with mechanism:none;
// they are excluded because stranding is expected and not actionable for them.
func computeStranded(routerRoot string, pendingByAgent map[string][]string, liveWake map[string]bool, noWake map[string]bool) []StrandedAgent {
	var stranded []StrandedAgent
	for agent, items := range pendingByAgent {
		if len(items) == 0 {
			continue
		}
		// The owner queue ("user") has no session/loop watcher — it is consumed by
		// the owner reading the menubar. Telling the owner to "arm a watcher" for
		// their own inbox is a category error, so `user` is never stranded.
		if agent == "user" {
			continue
		}
		// Agents with mechanism:none have explicitly declared "do not auto-wake me"
		// — operator must start them manually. Stranding is expected; no automated
		// action can resolve it.
		if noWake[agent] {
			continue
		}
		// Watched = a live consumer: an armed registry thread (a running /loop
		// session) OR the agent's launchd wake-loop. Crediting only the registry
		// false-strands every wake-loop agent (the #298 class — proven live: the
		// screenshot flagged claude-pantheon stranded while its wake job was up).
		if AgentArmed(routerRoot, agent) || liveWake[agent] {
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
