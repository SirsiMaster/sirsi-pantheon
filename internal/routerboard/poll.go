package routerboard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Poll builds one payload and publishes it if it changed.
//
// Publishing only on CHANGE is what makes the SSE stream honest: a client that
// sees an event knows the fleet moved, rather than that a timer fired.
func (b *Board) Poll(ctx context.Context) {
	var errs []string

	var ledger struct {
		GeneratedAt string `json:"generated_at"`
		Agents      []struct {
			Agent string    `json:"agent"`
			Items []Item    `json:"items"`
			Tasks []rawTask `json:"tasks"`
		} `json:"agents"`
	}
	b.runJSON(ctx, 8*time.Second, []string{"router", "ledger", "--json"}, &ledger, &errs)

	agentsCfg := b.registeredAgents(&errs)
	threads := b.allThreads(ctx, &errs)
	consumers := b.consumerLiveness(ctx, &errs)
	armed := armedLabels(ctx)

	threadsByAgent := map[string][]Thread{}
	freshestIdle := map[string]float64{}
	for _, t := range threads {
		if t.Agent == "" {
			continue
		}
		threadsByAgent[t.Agent] = append(threadsByAgent[t.Agent], t)
		if t.IdleSeconds != nil {
			if cur, ok := freshestIdle[t.Agent]; !ok || *t.IdleSeconds < cur {
				freshestIdle[t.Agent] = *t.IdleSeconds
			}
		}
	}

	// ONE READ MODEL. An earlier version iterated agents.json and made a
	// separate task-list call per agent, so any agent present in the store but
	// absent from that file was invisible — the board read 196 tasks while the
	// ledger read 205. The ledger already embeds every agent's tasks; agents.json
	// is consulted only for wake/type metadata.
	ledgerAgents := map[string]int{}
	byAgentItems := map[string][]Item{}
	tasksByAgent := map[string][]rawTask{}
	for i, a := range ledger.Agents {
		ledgerAgents[a.Agent] = i
		byAgentItems[a.Agent] = a.Items
		tasksByAgent[a.Agent] = a.Tasks
	}
	ids := map[string]bool{}
	for id := range agentsCfg {
		ids[id] = true
	}
	for id := range ledgerAgents {
		ids[id] = true
	}
	allIDs := make([]string, 0, len(ids))
	for id := range ids {
		allIDs = append(allIDs, id)
	}
	sort.Strings(allIDs)

	var fleet []Lane
	for _, aid := range allIDs {
		tasks := tasksByAgent[aid]
		b.diffAndLog(aid, tasks)

		counts := map[string]int{"done": 0, "in-progress": 0, "pending": 0, "blocked": 0}
		lastTouch := ""
		unblockedOpen := 0
		for _, t := range tasks {
			st := t.str("status")
			if st == "" {
				st = "pending"
			}
			counts[st]++
			if u := t.str("updated"); u > lastTouch {
				lastTouch = u
			}
			if (st == "pending" || st == "in-progress") && t.str("blocked_by") == "" {
				unblockedOpen++
			}
		}

		cfg := agentsCfg[aid]
		wakeMech, wakeLabel := wakeOf(cfg)
		wakeState := "n/a"
		if wakeMech == "launchagent" {
			wakeState = "disarmed"
			if armed[wakeLabel] {
				wakeState = "armed"
			}
		}

		taskAge, taskAgeOK := ageSeconds(lastTouch)
		// Alive-and-working if EITHER the thread heartbeat is fresh OR a task was
		// touched recently: some agents update tasks via one-shot calls with no
		// persistent thread record, so thread-idle alone under-counts them.
		best, haveBest := taskAge, taskAgeOK
		if idle, ok := freshestIdle[aid]; ok && (!haveBest || idle < best) {
			best, haveBest = idle, true
		}
		activity := "unknown"
		switch {
		case !haveBest:
		case best < 600:
			activity = "active now"
		case best < 14400:
			activity = "recently active"
		default:
			activity = "quiet"
		}

		// Keyed on TASK-TOUCH age, never session heartbeat: a live session with a
		// stale task registry is exactly the "alive but not working" state.
		idleWithWork := unblockedOpen > 0 && (!taskAgeOK || taskAge > 14400)

		items := byAgentItems[aid]
		if len(items) > 60 {
			items = items[:60]
		}
		if items == nil {
			items = []Item{}
		}
		oldestStale := false
		for _, i := range byAgentItems[aid] {
			if i.Stale {
				oldestStale = true
				break
			}
		}

		// Absent from live_threads means NO THREAD RECORD — not "definitively
		// unwatched". Defaulting to armed:false rendered those lanes as
		// "NO WATCHER", which is absence of evidence dressed as evidence of
		// absence. A lane with no record reports null and the UI says so.
		var cons *Consumer
		if consumers != nil {
			if c, ok := consumers[aid]; ok {
				cons = &c
			} else {
				cons = &Consumer{}
			}
		}

		if tasks == nil {
			tasks = []rawTask{}
		}
		fleet = append(fleet, Lane{
			Agent: aid, Type: typeOf(cfg), WakeState: wakeState, Activity: activity,
			OpenItems: len(byAgentItems[aid]), Items: items, OldestMsgStale: oldestStale,
			TasksTotal: len(tasks), Counts: counts,
			UnblockedOpen: unblockedOpen, IdleWithWork: idleWithWork,
			Registered: Registered{Router: true, Thread: len(threadsByAgent[aid]) > 0, Ledger: len(tasks) > 0},
			LastTouch:  lastTouch, Tasks: tasks, Consumer: cons,
		})
	}
	sort.Slice(fleet, func(i, j int) bool {
		if fleet[i].OpenItems != fleet[j].OpenItems {
			return fleet[i].OpenItems > fleet[j].OpenItems
		}
		return fleet[i].Agent < fleet[j].Agent
	})

	// in-progress is a LIVE GAUGE across the fleet right now, not a cumulative
	// transition tally: it is a non-terminal state, so counting only arrivals
	// undercounts whenever work is already in it or leaving it. It once read 0
	// while real in-progress work was visibly underway.
	var c Counters
	for _, f := range fleet {
		c.InProgressNow += f.Counts["in-progress"]
		c.Total += f.TasksTotal
		c.Blocked += f.Counts["blocked"]
		c.Pending += f.Counts["pending"]
		c.Done += f.Counts["done"]
	}

	b.mu.Lock()
	c.Completed = b.completed
	act := make([]Event, 0, 40)
	for i, e := range b.activity {
		if i >= 40 {
			break
		}
		act = append(act, e)
	}
	b.mu.Unlock()

	var gaps []string
	for _, f := range fleet {
		if !(f.Registered.Thread && f.Registered.Ledger) {
			gaps = append(gaps, f.Agent)
		}
	}
	if gaps == nil {
		gaps = []string{}
	}
	if errs == nil {
		errs = []string{}
	}
	sort.Strings(errs)
	if len(errs) > 5 {
		errs = errs[:5]
	}

	p := Payload{
		Build: b.buildID, DataErrors: errs, GeneratedAt: ledger.GeneratedAt,
		Counters: c, Activity: act, Fleet: fleet,
		Board:   summarize(fleet),
		Ledger:  summarize(fleet),
		Threads: threads, RegistrationGaps: gaps, Tasks: taskDetails(fleet),
	}
	body, err := json.Marshal(p)
	if err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	// Version bumps ONLY on real change, so an SSE event means the fleet moved.
	if b.version == 0 || string(body) != string(b.payload) {
		b.payload = body
		b.version++
	}
}

// diffAndLog emits an event for every real status change.
//
// A task's FIRST sighting seeds silently. Without that, every restart replays
// the whole ledger as a burst of fake transitions, and the feed's entire value
// is that movement in it means movement in the ledger.
func (b *Board) diffAndLog(agent string, tasks []rawTask) {
	now := time.Now().Format("15:04:05")
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, t := range tasks {
		id := t.str("task_id")
		if id == "" {
			continue
		}
		key := agent + "\x00" + id
		cur := t.str("status")
		prev, seen := b.prev[key]
		if !seen {
			b.prev[key] = cur
			continue
		}
		if prev == cur {
			continue
		}
		b.activity = append([]Event{{
			At: now, Agent: agent, TaskID: id,
			Subject: t.str("subject"), From: prev, To: cur,
		}}, b.activity...)
		if len(b.activity) > activityCap {
			b.activity = b.activity[:activityCap]
		}
		if cur == "done" {
			b.completed++
		}
		b.prev[key] = cur
	}
}

func summarize(fleet []Lane) BoardSummary {
	var s BoardSummary
	for _, f := range fleet {
		s.TotalTasks += f.TasksTotal
		s.DoneTasks += f.Counts["done"]
		s.BlockedTasks += f.Counts["blocked"]
		// blocked is a SUBSET of active, never a third independent segment.
		s.ActiveTasks += f.Counts["in-progress"] + f.Counts["pending"] + f.Counts["blocked"]
		s.OpenItems += f.OpenItems
	}
	s.PctDone = pct(s.DoneTasks, s.TotalTasks)

	// Group by the task's OWN --phase label, not by agent. "Phase" here means
	// what an agent declared work to belong to ("Host Stability", "Model
	// Router"), not "which lane owns it" — those are different questions and
	// collapsing them into one row per agent buried a handful of freshly
	// worked tasks under a lane's entire multi-week backlog, making real
	// progress invisible (owner, 2026-08-07: "I see no progress on any
	// burndown"). A task with no --phase set falls back to its agent name,
	// prefixed so it never collides with that agent's own real phase names.
	type acc struct{ total, done, active, blocked int }
	order := []string{}
	byPhase := map[string]*acc{}
	for _, f := range fleet {
		for _, t := range f.Tasks {
			label := t.str("phase")
			if label == "" {
				label = f.Agent + " (no phase set)"
			}
			a, ok := byPhase[label]
			if !ok {
				a = &acc{}
				byPhase[label] = a
				order = append(order, label)
			}
			a.total++
			switch t.str("status") {
			case "done":
				a.done++
			case "blocked":
				a.blocked++
				a.active++
			case "in-progress", "pending":
				a.active++
			}
		}
	}
	sort.Slice(order, func(i, j int) bool { return byPhase[order[i]].total > byPhase[order[j]].total })
	s.Phases = []Phase{}
	for _, label := range order {
		a := byPhase[label]
		if a.total == 0 {
			continue
		}
		s.Phases = append(s.Phases, Phase{
			Name: label, Total: a.total, Done: a.done, Active: a.active, Blocked: a.blocked,
			PctDone: pct(a.done, a.total),
		})
	}

	s.Blockers = []Blocker{}
	for _, f := range fleet {
		for _, t := range f.Tasks {
			if t.str("status") != "blocked" {
				continue
			}
			title := t.str("blocked_by")
			if title == "" {
				title = t.str("subject")
			}
			if len(title) > 120 {
				title = title[:120]
			}
			s.Blockers = append(s.Blockers, Blocker{
				Agent: f.Agent, ItemID: t.str("task_id"), Title: title, Age: ageStr(t.str("updated")),
			})
		}
	}
	s.BlockedItems = len(s.Blockers)
	if len(s.Blockers) > 20 {
		s.Blockers = s.Blockers[:20]
	}
	return s
}

func pct(n, d int) float64 {
	if d < 1 {
		d = 1
	}
	return float64(int(1000.0*float64(n)/float64(d)+0.5)) / 10.0
}

// taskDetails flattens tasks and derives liveness AT READ TIME per the v7
// contract — never stored, so it cannot go stale.
func taskDetails(fleet []Lane) []TaskDetail {
	out := []TaskDetail{}
	for _, f := range fleet {
		for _, t := range f.Tasks {
			age, ok := ageSeconds(t.str("updated"))
			status := t.str("status")
			blockedBy := t.str("blocked_by")
			var bp *string
			if blockedBy != "" {
				bp = &blockedBy
			}
			liveness := "unknown"
			switch {
			case blockedBy != "":
				liveness = "blocked"
			case status == "in-progress" && ok && age < stallSeconds:
				liveness = "active"
			case (status == "pending" || status == "in-progress") && (!ok || age >= stallSeconds):
				liveness = "stalled"
			}
			out = append(out, TaskDetail{
				TaskID: t.str("task_id"), Agent: f.Agent, Subject: t.str("subject"),
				Status: status, BlockedBy: bp, ResponsibleParty: t["responsible_party"],
				Updated: t.str("updated"), Age: ageStr(t.str("updated")), Liveness: liveness,
				Charter: t["charter"], CommissionedAt: t["commissioned_at"],
				CommissionedBy: t["commissioned_by"], Outline: t["outline"],
				Timeline: sliceOf(t["timeline"]), Links: sliceOf(t["links"]),
				TestState: strOr(t["test_state"], "untested"), Stage: strOr(t["stage"], "spec"),
				TokensConsumed: intOr(t["tokens_consumed"]), DurationSeconds: intOr(t["duration_seconds"]),
			})
		}
	}
	return out
}

func sliceOf(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}
	return []interface{}{}
}
func strOr(v interface{}, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}
func intOr(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

func (b *Board) registeredAgents(errs *[]string) map[string]interface{} {
	raw, err := os.ReadFile(b.agentsJSON)
	if err != nil {
		*errs = append(*errs, "agents.json: "+err.Error())
		return map[string]interface{}{}
	}
	var d map[string]interface{}
	if err := json.Unmarshal(raw, &d); err != nil {
		*errs = append(*errs, "agents.json: "+err.Error())
		return map[string]interface{}{}
	}
	if a, ok := d["agents"].(map[string]interface{}); ok {
		return a
	}
	return d
}

func typeOf(cfg interface{}) string {
	if m, ok := cfg.(map[string]interface{}); ok {
		if s, ok := m["type"].(string); ok {
			return s
		}
	}
	return "?"
}

func wakeOf(cfg interface{}) (mech, label string) {
	m, ok := cfg.(map[string]interface{})
	if !ok {
		return "", ""
	}
	w, ok := m["wake"].(map[string]interface{})
	if !ok {
		return "", ""
	}
	mech, _ = w["mechanism"].(string)
	label, _ = w["launch_agent_label"].(string)
	return mech, label
}

func (b *Board) allThreads(ctx context.Context, errs *[]string) []Thread {
	var raw []struct {
		Thread struct {
			ThreadID   string `json:"thread_id"`
			ID         string `json:"id"`
			AgentID    string `json:"agent_id"`
			Workstream string `json:"workstream"`
			Repo       string `json:"repo"`
		} `json:"thread"`
		IdleSeconds *float64 `json:"idle_seconds"`
		Stale       bool     `json:"stale"`
	}
	out := []Thread{}
	if !b.runJSON(ctx, 8*time.Second, []string{"thread", "list", "--json"}, &raw, errs) {
		return out
	}
	for _, t := range raw {
		id := t.Thread.ThreadID
		if id == "" {
			id = t.Thread.ID
		}
		ws := t.Thread.Workstream
		if ws == "" {
			ws = t.Thread.Repo
		}
		out = append(out, Thread{
			ThreadID: id, Agent: t.Thread.AgentID, Workstream: ws,
			IdleSeconds: t.IdleSeconds, Stale: t.Stale,
		})
	}
	return out
}

// consumerLiveness returns nil (not an empty map) when node-status is
// unavailable, so callers can distinguish "no record" from "not armed".
func (b *Board) consumerLiveness(ctx context.Context, errs *[]string) map[string]Consumer {
	var d struct {
		LiveThreads []struct {
			AgentID   string `json:"agent_id"`
			Armed     bool   `json:"armed"`
			LoopState string `json:"loop_state"`
		} `json:"live_threads"`
		StrandedInbox []struct {
			AgentID string `json:"agent_id"`
		} `json:"stranded_inbox"`
	}
	if !b.runJSON(ctx, 45*time.Second, []string{"router", "node-status", "--json"}, &d, errs) {
		return nil
	}
	out := map[string]Consumer{}
	for _, t := range d.LiveThreads {
		if t.AgentID == "" {
			continue
		}
		c := out[t.AgentID]
		if t.Armed {
			c.Armed = true
		}
		if t.LoopState != "" {
			c.Loop = t.LoopState
		}
		out[t.AgentID] = c
	}
	for _, s := range d.StrandedInbox {
		if _, ok := out[s.AgentID]; !ok && s.AgentID != "" {
			out[s.AgentID] = Consumer{Loop: "none"}
		}
	}
	return out
}

func armedLabels(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := exec.CommandContext(c, "launchctl", "list").Output()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, "ai.sirsi.router.wake.") {
			continue
		}
		f := strings.Fields(line)
		if len(f) > 0 {
			out[f[len(f)-1]] = true
		}
	}
	return out
}

var _ = fmt.Sprintf
