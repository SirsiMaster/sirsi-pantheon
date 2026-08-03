// Package ledger builds the router's universal task read model. It owns no
// persistence: dispatch/routerstore own messages and tasks; router owns thread
// heartbeats. Ledger joins those truths once for every surface.
package ledger

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

const DefaultStaleAfter = 4 * time.Hour

type Item struct {
	ID              string   `json:"id"`
	Agent           string   `json:"agent"`
	Title           string   `json:"title"`
	From            string   `json:"from"`
	Type            string   `json:"type,omitempty"`
	Opened          string   `json:"opened"`
	AgeSeconds      float64  `json:"age_seconds"`
	Stale           bool     `json:"stale"`
	Picked          bool     `json:"picked"`
	Blocked         bool     `json:"blocked"`
	BlockedBy       string   `json:"blocked_by,omitempty"`
	DependencyChain []string `json:"dependency_chain,omitempty"`
}

type Agent struct {
	AgentID           string             `json:"agent"`
	Items             []Item             `json:"items"`
	Tasks             []routerstore.Task `json:"tasks"`
	OldestAgeSeconds  float64            `json:"oldest_item_age_seconds"`
	Stale             bool               `json:"stale"`
	BlockedCount      int                `json:"blocked_count"`
	UnblockedUnpicked int                `json:"unblocked_unpicked_count"`
	LatestHeartbeat   string             `json:"latest_heartbeat,omitempty"`
}

type Snapshot struct {
	GeneratedAt string  `json:"generated_at"`
	StaleAfter  string  `json:"stale_after"`
	Agents      []Agent `json:"agents"`
}

// Build joins open items, terminal dependency truth, thread heartbeats/current
// work, and the task registry. agent may be empty for a fleet-wide snapshot.
func Build(repoRoot, agent string, now time.Time, staleAfter time.Duration) (Snapshot, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if staleAfter <= 0 {
		staleAfter = DefaultStaleAfter
	}
	f, err := dispatch.Open(repoRoot)
	if err != nil {
		return Snapshot{}, err
	}
	defer f.Close()
	all, err := f.ListAll()
	if err != nil {
		return Snapshot{}, err
	}
	tasks, err := f.Store().ListTasks(agent)
	if err != nil {
		return Snapshot{}, err
	}
	routerRoot := repoRoot + "/.agents/idea-router"
	threads, err := router.LoadThreadRegistry(routerRoot)
	if err != nil {
		return Snapshot{}, err
	}
	return BuildFrom(all, tasks, threads, agent, now, staleAfter), nil
}

func BuildFrom(all []work.Item, tasks []routerstore.Task, threads *router.ThreadRegistry, scope string, now time.Time, staleAfter time.Duration) Snapshot {
	byID := make(map[string]work.Item, len(all))
	agents := map[string]*Agent{}
	for _, it := range all {
		byID[it.ID] = it
		if itemTerminal(it.Status) || (scope != "" && it.To != scope) {
			continue
		}
		if agents[it.To] == nil {
			agents[it.To] = &Agent{AgentID: it.To}
		}
	}
	for _, t := range tasks {
		if scope != "" && t.Agent != scope {
			continue
		}
		if agents[t.Agent] == nil {
			agents[t.Agent] = &Agent{AgentID: t.Agent}
		}
		agents[t.Agent].Tasks = append(agents[t.Agent].Tasks, t)
	}

	latest := map[string]time.Time{}
	picked := map[string]bool{}
	if threads != nil {
		for _, t := range threads.Threads {
			if t == nil || t.Status.IsTerminal() || t.Status == router.ThreadStatusSuspended {
				continue
			}
			if t.LastSeenAt.After(latest[t.AgentID]) {
				latest[t.AgentID] = t.LastSeenAt
			}
			if t.CurrentItem != "" {
				picked[t.CurrentItem] = true
			}
		}
	}

	for _, it := range all {
		if itemTerminal(it.Status) || (scope != "" && it.To != scope) {
			continue
		}
		opened, _ := time.Parse(time.RFC3339, it.Opened)
		age := now.Sub(opened).Seconds()
		if opened.IsZero() || age < 0 {
			age = 0
		}
		hb, hasHB := latest[it.To]
		stale := !hasHB || now.Sub(hb) >= staleAfter
		chain, blocked := dependencyChain(it.BlockedBy, byID)
		li := Item{ID: it.ID, Agent: it.To, Title: it.Title, From: it.From, Type: it.Type, Opened: it.Opened,
			AgeSeconds: age, Stale: stale, Picked: picked[it.ID], Blocked: blocked, BlockedBy: it.BlockedBy, DependencyChain: chain}
		a := agents[it.To]
		a.Items = append(a.Items, li)
		if age > a.OldestAgeSeconds {
			a.OldestAgeSeconds = age
		}
		a.Stale = a.Stale || stale
		if blocked {
			a.BlockedCount++
		} else if !li.Picked {
			a.UnblockedUnpicked++
		}
	}
	for id, hb := range latest {
		if a := agents[id]; a != nil {
			a.LatestHeartbeat = hb.UTC().Format(time.RFC3339)
		}
	}

	ids := make([]string, 0, len(agents))
	for id := range agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	s := Snapshot{GeneratedAt: now.UTC().Format(time.RFC3339), StaleAfter: staleAfter.String()}
	for _, id := range ids {
		a := agents[id]
		sort.Slice(a.Items, func(i, j int) bool { return a.Items[i].ID < a.Items[j].ID })
		s.Agents = append(s.Agents, *a)
	}
	return s
}

// dependencyChain follows item-to-item edges. Missing dependencies remain
// blocking (fail closed); a terminal dependency releases the dependent item.
func dependencyChain(first string, byID map[string]work.Item) ([]string, bool) {
	if strings.TrimSpace(first) == "" {
		return nil, false
	}
	var chain []string
	seen := map[string]bool{}
	cur := first
	for cur != "" {
		chain = append(chain, cur)
		if seen[cur] {
			chain = append(chain, "cycle")
			return chain, true
		}
		seen[cur] = true
		dep, ok := byID[cur]
		if !ok {
			chain = append(chain, "missing")
			return chain, true
		}
		if itemTerminal(dep.Status) {
			return chain, false
		}
		cur = dep.BlockedBy
		if cur == "" {
			return chain, true
		}
	}
	return chain, true
}

func itemTerminal(status string) bool {
	switch status {
	case "closed", "completed", "dead_letter":
		return true
	}
	return false
}

func FormatAge(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	d := time.Duration(seconds * float64(time.Second))
	if d < time.Minute {
		return "<1m"
	}
	days := int(d / (24 * time.Hour))
	hours := int(d%(24*time.Hour)) / int(time.Hour)
	minutes := int(d%time.Hour) / int(time.Minute)
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// BoardSummary is the compact owner-facing view used by the menubar, TUI, and
// MCP router_ledger tool. Derived from a Snapshot so every surface shows
// identical numbers.
type BoardSummary struct {
	// Task registry counts (durable tasks registered with `sirsi router task add`).
	TotalTasks   int `json:"total_tasks"`
	DoneTasks    int `json:"done_tasks"`
	ActiveTasks  int `json:"active_tasks"`
	BlockedTasks int `json:"blocked_tasks"`
	PctDone      int `json:"pct_done"` // DoneTasks*100/TotalTasks (0 if 0)

	// Open inbox items.
	OpenItems    int `json:"open_items"`
	BlockedItems int `json:"blocked_items"` // items with Blocked==true

	// Blockers — items waiting on owner/user or an explicit dep.
	Blockers []BoardBlocker `json:"blockers,omitempty"`

	GeneratedAt string `json:"generated_at"`
}

// BoardBlocker is one line on the blockers table.
type BoardBlocker struct {
	Agent  string `json:"agent"`
	ItemID string `json:"item_id"`
	Title  string `json:"title"`
	Age    string `json:"age"`
}

// Summarize distills a Snapshot into a BoardSummary for compact surface rendering.
func Summarize(s Snapshot) BoardSummary {
	bs := BoardSummary{GeneratedAt: s.GeneratedAt}
	for _, a := range s.Agents {
		for _, t := range a.Tasks {
			bs.TotalTasks++
			switch t.Status {
			case "done":
				bs.DoneTasks++
			case "blocked":
				bs.BlockedTasks++
				bs.ActiveTasks++
			default:
				bs.ActiveTasks++
			}
		}
		for _, it := range a.Items {
			bs.OpenItems++
			if it.Blocked {
				bs.BlockedItems++
				bs.Blockers = append(bs.Blockers, BoardBlocker{
					Agent:  a.AgentID,
					ItemID: it.ID,
					Title:  it.Title,
					Age:    FormatAge(it.AgeSeconds),
				})
			}
		}
	}
	if bs.TotalTasks > 0 {
		bs.PctDone = bs.DoneTasks * 100 / bs.TotalTasks
	}
	return bs
}

// TextBoard renders a compact multi-line ASCII board for CLI/MCP text output.
func (bs BoardSummary) TextBoard() string {
	var sb strings.Builder
	bar := progressBar(bs.PctDone, 20)
	fmt.Fprintf(&sb, "\n  ── Ledger Board ──\n")
	if bs.TotalTasks > 0 {
		fmt.Fprintf(&sb, "  Tasks     %s  %d%%\n", bar, bs.PctDone)
		fmt.Fprintf(&sb, "  Done %-4d  Active %-4d  Blocked  %d\n", bs.DoneTasks, bs.ActiveTasks, bs.BlockedTasks)
	}
	fmt.Fprintf(&sb, "  Open items %-4d  Blocked items  %d\n", bs.OpenItems, bs.BlockedItems)
	if len(bs.Blockers) > 0 {
		fmt.Fprintf(&sb, "\n  Blocked items:\n")
		for _, bl := range bs.Blockers {
			fmt.Fprintf(&sb, "    • [%s] %s  ← %s  (%s)\n",
				bl.Age, truncateLedger(bl.Title, 48), bl.Agent, bl.ItemID)
		}
	}
	fmt.Fprintln(&sb)
	return sb.String()
}

func progressBar(pct, width int) string {
	filled := pct * width / 100
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

func truncateLedger(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
