// Package router — discover.go
//
// Reconciliation logic for `sirsi thread discover`: given the running agent
// processes on this host, decide which should be registered as live threads.
//
// This file holds ONLY the pure decision logic (no process enumeration, no
// filesystem side effects) so it is fully unit-testable with injected inputs
// per Rule A16. The CLI layer (cmd/sirsi) supplies the real process list and
// applies the resulting registrations.
//
// Design (agreed with codex-pantheon 2026-05-31): a session is mappable only
// when its working directory resolves to exactly one agent in agents.json of
// the matching surface. Home-launched sessions (no repo binding) and sessions
// whose cwd matches multiple agents are reported, never guessed.
package router

import (
	"path/filepath"
	"strings"
)

// DiscoveredProc is a running agent process found by enumeration on this host.
type DiscoveredProc struct {
	PID     int    `json:"pid"`     // the live session process id
	Surface string `json:"surface"` // claude|codex|gemini|... (the binary we matched)
	Cwd     string `json:"cwd"`     // resolved working directory; "" when unresolved
}

// DiscoverOutcome classifies the decision reconcile made for one process.
type DiscoverOutcome string

const (
	// OutcomeRegister: a new, mappable, not-yet-registered session.
	OutcomeRegister DiscoverOutcome = "register"
	// OutcomeSkip: an active thread already exists for this live PID.
	OutcomeSkip DiscoverOutcome = "skip"
	// OutcomeUnmappable: cwd does not resolve to any agent of this surface
	// (e.g. home-launched sessions). Correctly NOT registered.
	OutcomeUnmappable DiscoverOutcome = "unmappable"
	// OutcomeAmbiguous: cwd resolves to more than one agent of this surface.
	// We refuse to guess; the operator must disambiguate agents.json.
	OutcomeAmbiguous DiscoverOutcome = "ambiguous"
)

// DiscoverAction is reconcile's decision for a single discovered process.
type DiscoverAction struct {
	Proc    DiscoveredProc  `json:"proc"`
	Outcome DiscoverOutcome `json:"outcome"`
	AgentID string          `json:"agent_id,omitempty"` // set for register/skip
	Repo    string          `json:"repo,omitempty"`     // matched agent cwd
	Reason  string          `json:"reason,omitempty"`   // human explanation
}

// ReconcileDiscovery is pure: given the agent registry, the current thread
// registry, and the discovered live processes, it returns one action per
// process. It performs NO side effects — the caller registers threads for
// actions whose Outcome is OutcomeRegister.
//
// host scopes the "already registered" check to threads on this machine;
// remote-host threads are never matched against local PIDs.
func ReconcileDiscovery(reg *Registry, threads *ThreadRegistry, procs []DiscoveredProc, host string) []DiscoverAction {
	// Index active threads on this host by live PID, so a re-run of discover
	// recognizes sessions it (or `register`) already enrolled.
	activeByPID := make(map[int]*Thread)
	if threads != nil {
		for _, t := range threads.Threads {
			if t == nil || t.Status == ThreadStatusClosed {
				continue
			}
			if t.Host == host && t.PID > 0 {
				activeByPID[t.PID] = t
			}
		}
	}

	actions := make([]DiscoverAction, 0, len(procs))
	for _, p := range procs {
		if t, ok := activeByPID[p.PID]; ok {
			actions = append(actions, DiscoverAction{
				Proc: p, Outcome: OutcomeSkip, AgentID: t.AgentID, Repo: t.Repo,
				Reason: "already registered as " + t.ThreadID,
			})
			continue
		}

		matches := matchAgentsByCwd(reg, p.Surface, p.Cwd)
		switch len(matches) {
		case 0:
			reason := "cwd not under any " + p.Surface + " agent repo"
			if p.Cwd == "" {
				reason = "working directory could not be resolved"
			}
			actions = append(actions, DiscoverAction{Proc: p, Outcome: OutcomeUnmappable, Reason: reason})
		case 1:
			actions = append(actions, DiscoverAction{
				Proc: p, Outcome: OutcomeRegister, AgentID: matches[0].id, Repo: matches[0].cwd,
			})
		default:
			ids := make([]string, len(matches))
			for i, m := range matches {
				ids[i] = m.id
			}
			actions = append(actions, DiscoverAction{
				Proc: p, Outcome: OutcomeAmbiguous, Repo: p.Cwd,
				Reason: "cwd matches multiple agents (" + strings.Join(ids, ", ") + ") — disambiguate agents.json",
			})
		}
	}
	return actions
}

type agentMatch struct {
	id  string
	cwd string
}

// matchAgentsByCwd returns every agent of the given surface whose configured
// cwd equals, or is the nearest ancestor of, procCwd. Only the most-specific
// (longest) ancestor depth is returned: a session in repo/sub matches the
// agent rooted at repo, not at the parent of repo. If two agents share that
// most-specific cwd, both are returned and the caller treats it as ambiguous.
func matchAgentsByCwd(reg *Registry, surface, procCwd string) []agentMatch {
	if reg == nil || procCwd == "" || surface == "" {
		return nil
	}
	cleanCwd := filepath.Clean(procCwd)
	bestLen := -1
	var best []agentMatch
	for id, a := range reg.Agents {
		if a.Type != surface || a.Cwd == "" {
			continue
		}
		ac := filepath.Clean(a.Cwd)
		if cleanCwd != ac && !strings.HasPrefix(cleanCwd, ac+string(filepath.Separator)) {
			continue
		}
		switch {
		case len(ac) > bestLen:
			bestLen = len(ac)
			best = []agentMatch{{id: id, cwd: ac}}
		case len(ac) == bestLen:
			best = append(best, agentMatch{id: id, cwd: ac})
		}
	}
	return best
}
