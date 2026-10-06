package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knownfail"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/swaphygiene"
	appversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

// collectDashboardRouter builds the router panel from the same sources the CLI
// uses (ping, status, registry pin, known-failure catalog, swap receipt, changelog),
// so the dashboard cannot disagree with `sirsi router ping --all`.
func collectDashboardRouter() (result dashboard.RouterSnapshot, _ error) {
	repo, err := router.FindRepoRoot()
	if err != nil {
		return dashboard.RouterSnapshot{}, fmt.Errorf("locate repo root: %w", err)
	}
	routerRoot := filepath.Join(repo, ".agents", "idea-router")
	started := time.Now()
	snap := dashboard.RouterSnapshot{
		GeneratedAt: started.UTC().Format(time.RFC3339),
		Version:     appversion.Version,
		// Every verdict is present, zero included: a verdict absent from the map reads
		// as "unknown" to a UI, and zero lanes LIVE is a fact, not a gap.
		Lanes:   dashboard.RouterLanes{Counts: zeroVerdictCounts()},
		Timings: map[string]int64{},
	}
	last := started
	lap := func(stage string) {
		now := time.Now()
		snap.Timings[stage] = now.Sub(last).Milliseconds()
		last = now
	}
	defer func() { result.BuiltMs = time.Since(started).Milliseconds() }()

	reg, err := router.LoadRegistry(routerRoot)
	if err != nil {
		return snap, fmt.Errorf("registry: %w", err)
	}
	lap("registry")
	threads, err := router.LoadThreadRegistry(routerRoot)
	if err != nil {
		return snap, fmt.Errorf("threads: %w", err)
	}
	lap("threads")
	f, err := dispatch.Open(repo)
	if err != nil {
		return snap, fmt.Errorf("dispatch: %w", err)
	}
	defer func() { _ = f.Close() }()
	aliases, _ := f.Aliases()
	lap("open_and_aliases")

	now := time.Now().UTC()
	ids := make([]string, 0, len(reg.Agents))
	for id, cfg := range reg.Agents {
		if _, retired := aliases[id]; retired || cfg.Type == "human" || cfg.Type == "service" {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := router.PingLane(threads, reg.Agents[id], id, now)
		snap.Lanes.Counts[p.Verdict]++
		snap.Lanes.List = append(snap.Lanes.List, dashboard.RouterLaneVerdict{
			Agent: id, Verdict: p.Verdict, Detail: p.Detail,
			ObservedAt: now.Format(time.RFC3339), WorkerThreadID: p.Thread,
			LastReportAt: p.ReportedAt, LastReportSummary: p.ReportSummary,
		})
	}
	lap("lane_verdicts")

	items, _, err := f.ListActive()
	if err != nil {
		return snap, fmt.Errorf("queue: %w", err)
	}
	lap("list_active")
	per := map[string]int{}
	for _, it := range items {
		per[it.To]++
	}
	for a, n := range per {
		snap.Queue = append(snap.Queue, dashboard.RouterQueueRow{Agent: a, Open: n})
	}
	sort.Slice(snap.Queue, func(i, j int) bool {
		if snap.Queue[i].Open != snap.Queue[j].Open {
			return snap.Queue[i].Open > snap.Queue[j].Open
		}
		return snap.Queue[i].Agent < snap.Queue[j].Agent
	})

	for i := range snap.Lanes.List {
		snap.Lanes.List[i].Open = per[snap.Lanes.List[i].Agent]
	}
	snap.Consumers.Running, snap.Consumers.Max = router.ConsumerSlotUsage(routerRoot)
	lap("consumer_slots")
	path, pinned, meta := router.RegistryPinStatus(routerRoot)
	snap.Registry = dashboard.RouterRegistryPin{Pinned: pinned, Source: path}
	if meta != nil {
		snap.Registry.Commit, snap.Registry.FetchedAt = meta.Commit, meta.FetchedAt
	}

	lap("registry_pin")
	if c, kerr := knownfail.Load(); kerr == nil {
		for _, e := range c.Entries {
			snap.KnownFailures = append(snap.KnownFailures, dashboard.RouterKnownFail{ID: e.ID, Title: e.Title, Status: e.Status, FixedIn: e.Fix.FixedIn, Guard: e.Guard.Ref})
		}
	}
	if home, herr := os.UserHomeDir(); herr == nil {
		if r, serr := swaphygiene.Last(home); serr == nil {
			snap.Swap = &dashboard.RouterSwap{At: r.At.Format(time.RFC3339), Verdict: r.Verdict, UsedMiB: r.SwapUsedMiB, TotalMiB: r.SwapTotalMiB,
				FreePct: r.FreePct, DeltaPages: r.DeltaSwapPages, Correctness: r.CorrectnessOnlyOK, Timing: r.ReleaseTimingOK, Restart: r.RestartProposed}
		}
	}
	lap("known_failures_and_swap")
	snap.Releases = readChangelogReleases(changelogText(repo), 5)
	lap("changelog")
	snap.Attention = routerAttention(snap)
	return snap, nil
}

var changelogHead = regexp.MustCompile(`^## \[([^\]]+)\](?: — (\d{4}-\d{2}-\d{2}))?`)

// readChangelogReleases returns the Unreleased section and the next n released
// sections with one line per bullet, so the panel shows what each release added.
func readChangelogReleases(text string, n int) []dashboard.RouterRelease {
	if text == "" {
		return nil
	}
	b := []byte(text)
	var out []dashboard.RouterRelease
	seenVersion := map[string]int{}
	var cur *dashboard.RouterRelease
	var bullet []string
	flush := func() {
		if cur != nil && len(bullet) > 0 {
			cur.Items = append(cur.Items, summarizeBullet(strings.Join(bullet, " ")))
		}
		bullet = nil
	}
	for _, line := range strings.Split(string(b), "\n") {
		if m := changelogHead.FindStringSubmatch(line); m != nil {
			flush()
			if len(out) > n {
				break
			}
			if idx, dup := seenVersion[m[1]]; dup { // several [Unreleased] headings merge into one
				cur = &out[idx]
				continue
			}
			out = append(out, dashboard.RouterRelease{Version: m[1], Date: m[2]})
			seenVersion[m[1]] = len(out) - 1
			cur = &out[len(out)-1]
			continue
		}
		if cur == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "- "):
			flush()
			bullet = []string{strings.TrimSpace(strings.TrimPrefix(line, "- "))}
		case bullet != nil && (strings.HasPrefix(line, "  ") || strings.TrimSpace(line) == ""):
			if t := strings.TrimSpace(line); t != "" {
				bullet = append(bullet, t)
			}
		default:
			flush()
		}
	}
	flush()
	if len(out) > n+1 {
		out = out[:n+1]
	}
	return out
}

// summarizeBullet keeps the bold lead sentence of a changelog bullet (its headline).
func summarizeBullet(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	if i := strings.Index(s, "**"); i == 0 {
		if j := strings.Index(s[2:], "**"); j >= 0 {
			head := strings.TrimSpace(s[2 : 2+j])
			rest := strings.TrimSpace(s[2+j+2:])
			if len(rest) > 140 {
				rest = rest[:140] + "…"
			}
			return strings.TrimSuffix(head, ".") + " — " + rest
		}
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// changelogText prefers origin/main's CHANGELOG (the released truth) over whatever
// branch the shared checkout happens to be on, falling back to the file.
func changelogText(repo string) string {
	cmd := exec.Command("git", "-C", repo, "show", "origin/main:CHANGELOG.md")
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		return string(out)
	}
	if b, err := os.ReadFile(filepath.Join(repo, "CHANGELOG.md")); err == nil {
		return string(b)
	}
	return ""
}

// routerAttention derives what needs attention from the snapshot, deterministically,
// most severe first. It claims only what the data shows.
func routerAttention(s dashboard.RouterSnapshot) []dashboard.RouterAttention {
	var out []dashboard.RouterAttention
	add := func(id, agent, sev, title, detail, action string, next *dashboard.RouterNext) {
		out = append(out, dashboard.RouterAttention{ID: id, Agent: agent, Severity: sev, Title: title, Detail: detail, Action: action, Next: next})
	}
	cmd := func(label, command string) *dashboard.RouterNext {
		return &dashboard.RouterNext{Kind: "command-copy", Label: label, Command: command}
	}
	if s.Swap != nil && s.Swap.Verdict == "pressure" {
		add("swap-pressure", "", "critical", "Memory pressure with active paging", fmt.Sprintf("swap %d of %d MiB, %d%% free; a coordinated restart is proposed (nothing was restarted)", int(s.Swap.UsedMiB), int(s.Swap.TotalMiB), s.Swap.FreePct), "coordinate a restart with the owner and workload owners", cmd("Show the swap assessment", "sirsi swap-hygiene --status"))
	}
	for _, l := range s.Lanes.List {
		switch {
		case l.Verdict == "AUTH_REQUIRED":
			add("auth:"+l.Agent, l.Agent, "critical", l.Agent+": consumer cannot log in", l.Detail, "re-authenticate that account", cmd("Check the lane", "sirsi router ping "+l.Agent))
		case l.Verdict == "HELD" && strings.Contains(l.Detail, "quarantine"):
			add("quarantine:"+l.Agent, l.Agent, "critical", l.Agent+": quarantined", l.Detail, "needs a human: see the lane log; known failures are matched automatically", cmd("Check the lane", "sirsi router ping "+l.Agent))
		case (l.Verdict == "WATCH_ONLY" || l.Verdict == "UNSTAFFED" || l.Verdict == "UNREACHABLE") && l.Open > 0:
			next := cmd("Install its wake loop on the lane's host", "sirsi router wake-install "+l.Agent)
			if l.Verdict == "WATCH_ONLY" {
				next = cmd("See why it has no working consumer", "sirsi router ping "+l.Agent)
			}
			add("unworked:"+l.Agent, l.Agent, "warn", fmt.Sprintf("%s: %d open item(s), nothing will work them", l.Agent, l.Open), l.Verdict+": "+l.Detail, "staff the lane or register an attended session", next)
		}
	}
	for _, k := range s.KnownFailures {
		if k.Status == "open" {
			add("knownfail:"+k.ID, "", "warn", "Known failure unresolved: "+k.ID, k.Title, "resolve it with a fix and a guard test", cmd("List known failures", "sirsi maat known-failures list"))
		}
	}
	if !s.Registry.Pinned {
		add("registry-unpinned", "", "warn", "Registry is not pinned to origin/main", "this host reads a shared working tree; another session's branch can change who the lanes are", "sirsi router registry sync --install", cmd("Pin this host", "sirsi router registry sync --install"))
	}
	if s.Consumers.Max > 0 && s.Consumers.Running >= s.Consumers.Max {
		add("consumer-cap", "", "info", "Consumer cap reached", fmt.Sprintf("%d of %d headless consumers running; further lanes wait their turn", s.Consumers.Running, s.Consumers.Max), "", nil)
	}
	rank := map[string]int{"critical": 0, "warn": 1, "info": 2}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	return out
}

// zeroVerdictCounts has every ping verdict at zero.
func zeroVerdictCounts() map[string]int {
	return map[string]int{
		router.VerdictLive: 0, router.VerdictWakeable: 0, router.VerdictHeld: 0, router.VerdictAuthRequired: 0,
		router.VerdictWatchOnly: 0, router.VerdictUnstaffed: 0, router.VerdictUnreachable: 0,
	}
}
