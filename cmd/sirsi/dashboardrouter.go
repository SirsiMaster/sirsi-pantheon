package main

import (
	"crypto/sha256"
	"encoding/hex"
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
func collectDashboardRouter() (dashboard.RouterSnapshot, error) {
	repo, err := router.FindRepoRoot()
	if err != nil {
		return dashboard.RouterSnapshot{}, fmt.Errorf("locate repo root: %w", err)
	}
	routerRoot := filepath.Join(repo, ".agents", "idea-router")
	snap := dashboard.RouterSnapshot{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Version:     appversion.Version,
		Lanes:       dashboard.RouterLanes{Counts: map[string]int{}},
	}

	reg, err := router.LoadRegistry(routerRoot)
	if err != nil {
		return snap, fmt.Errorf("registry: %w", err)
	}
	threads, err := router.LoadThreadRegistry(routerRoot)
	if err != nil {
		return snap, fmt.Errorf("threads: %w", err)
	}
	f, err := dispatch.Open(repo)
	if err != nil {
		return snap, fmt.Errorf("dispatch: %w", err)
	}
	defer func() { _ = f.Close() }()
	aliases, _ := f.Aliases()

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
		snap.Lanes.List = append(snap.Lanes.List, dashboard.RouterLaneVerdict{Agent: id, Verdict: p.Verdict, Detail: p.Detail, WorkerThreadID: p.Thread})
	}

	items, _, err := f.ListActive()
	if err != nil {
		return snap, fmt.Errorf("queue: %w", err)
	}
	per := map[string]int{}
	byAgent := map[string][]dashboard.RouterQueueItem{}
	for _, it := range items {
		per[it.To]++
		byAgent[it.To] = append(byAgent[it.To], dashboard.RouterQueueItem{
			ID: it.ID, Recipient: it.To, Subject: it.Title, OpenedAt: it.Opened, AcknowledgedAt: it.AckedAt,
		})
	}
	for a, n := range per {
		row := dashboard.RouterQueueRow{Agent: a, Open: n}
		rowItems := byAgent[a]
		sort.Slice(rowItems, func(i, j int) bool { return rowItems[i].OpenedAt < rowItems[j].OpenedAt })
		row.Items = rowItems
		snap.Queue = append(snap.Queue, row)
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
	path, pinned, meta := router.RegistryPinStatus(routerRoot)
	snap.Registry = dashboard.RouterRegistryPin{Pinned: pinned, Source: path}
	if meta != nil {
		snap.Registry.Commit, snap.Registry.FetchedAt = meta.Commit, meta.FetchedAt
	}

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
	snap.Releases = readChangelogReleases(changelogText(repo), 5)
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
	add := func(agent, sev, title, detail, action string) {
		a := dashboard.RouterAttention{ID: attentionID(sev, title), Agent: agent, Severity: sev, Title: title, Detail: detail, Action: action}
		// Only a literal CLI invocation becomes a typed next step — prose
		// guidance ("re-authenticate that account") is never promoted into a
		// command the UI could be tempted to run.
		if strings.HasPrefix(action, "sirsi ") {
			a.NextStep = &dashboard.RouterNextStep{Kind: "command-copy", Label: "Copy command", Command: action}
		}
		out = append(out, a)
	}
	if s.Swap != nil && s.Swap.Verdict == "pressure" {
		add("", "critical", "Memory pressure with active paging", fmt.Sprintf("swap %d of %d MiB, %d%% free; a coordinated restart is proposed (nothing was restarted)", int(s.Swap.UsedMiB), int(s.Swap.TotalMiB), s.Swap.FreePct), "coordinate a restart with the owner and workload owners")
	}
	for _, l := range s.Lanes.List {
		switch {
		case l.Verdict == "AUTH_REQUIRED":
			add(l.Agent, "critical", l.Agent+": consumer cannot log in", l.Detail, "re-authenticate that account")
		case l.Verdict == "HELD" && strings.Contains(l.Detail, "quarantine"):
			add(l.Agent, "critical", l.Agent+": quarantined", l.Detail, "needs a human: see the lane log; known failures are matched automatically")
		case (l.Verdict == "WATCH_ONLY" || l.Verdict == "UNSTAFFED" || l.Verdict == "UNREACHABLE") && l.Open > 0:
			add(l.Agent, "warn", fmt.Sprintf("%s: %d open item(s), nothing will work them", l.Agent, l.Open), l.Verdict+": "+l.Detail, "staff the lane or register an attended session")
		}
	}
	for _, k := range s.KnownFailures {
		if k.Status == "open" {
			add("", "warn", "Known failure unresolved: "+k.ID, k.Title, "resolve it with a fix and a guard test")
		}
	}
	if !s.Registry.Pinned {
		add("", "warn", "Registry is not pinned to origin/main", "this host reads a shared working tree; another session's branch can change who the lanes are", "sirsi router registry sync --install")
	}
	if s.Consumers.Max > 0 && s.Consumers.Running >= s.Consumers.Max {
		add("", "info", "Consumer cap reached", fmt.Sprintf("%d of %d headless consumers running; further lanes wait their turn", s.Consumers.Running, s.Consumers.Max), "")
	}
	rank := map[string]int{"critical": 0, "warn": 1, "info": 2}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	return out
}

// attentionID is a stable selection identity for an attention row: deterministic
// from severity+title (which already encode the lane/condition), so a UI can
// select/compare rows across refreshes without parsing Title text itself.
func attentionID(severity, title string) string {
	sum := sha256.Sum256([]byte(severity + "\x00" + title))
	return hex.EncodeToString(sum[:8])
}
