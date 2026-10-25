package ledger

import (
	"sort"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func captureFabricMetadata(reg *router.Registry, threads *router.ThreadRegistry, node *router.NodeStatus) FabricMetadata {
	m := FabricMetadata{Lanes: map[string]FabricLaneMetadata{}, Threads: []FabricThreadMetadata{}, Errors: []string{}}
	if reg == nil {
		m.Errors = append(m.Errors, "registry-metadata-unavailable")
	} else {
		for id, cfg := range reg.Agents {
			m.Lanes[id] = FabricLaneMetadata{Type: cfg.Type, WakeMechanism: cfg.Wake.Mechanism}
		}
	}
	observed := map[string]router.ThreadSummary{}
	if node == nil {
		m.Errors = append(m.Errors, "node-status-unavailable")
	} else {
		for _, t := range node.LiveThreads {
			observed[t.ThreadID] = t
		}
		for _, t := range node.StaleThreads {
			observed[t.ThreadID] = t
		}
	}
	if threads == nil {
		m.Errors = append(m.Errors, "threads-unavailable")
		return m
	}
	missing := false
	for _, thread := range threads.Threads {
		if thread == nil {
			continue
		}
		workstream := thread.Workstream
		if workstream == "" {
			workstream = thread.Repo
		}
		row := FabricThreadMetadata{ID: thread.ThreadID, Agent: thread.AgentID, Workstream: workstream, Stale: true}
		if t, ok := observed[thread.ThreadID]; ok && t.AgentID == thread.AgentID {
			idle := t.IdleSeconds
			row.IdleSeconds = &idle
			row.Stale = t.Stale
		} else {
			missing = true
		}
		m.Threads = append(m.Threads, row)
	}
	if missing {
		m.Errors = append(m.Errors, "thread-observation-unavailable")
	}
	sort.Slice(m.Threads, func(i, j int) bool { return m.Threads[i].ID < m.Threads[j].ID })
	return m
}
