package main

import (
	"fmt"
	"os"
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
		snap.Lanes.List = append(snap.Lanes.List, dashboard.RouterLaneVerdict{Agent: id, Verdict: p.Verdict, Detail: p.Detail})
	}

	items, _, err := f.ListActive()
	if err != nil {
		return snap, fmt.Errorf("queue: %w", err)
	}
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
	snap.Releases = readChangelogReleases(filepath.Join(repo, "CHANGELOG.md"), 4)
	return snap, nil
}

var changelogHead = regexp.MustCompile(`^## \[([^\]]+)\](?: — (\d{4}-\d{2}-\d{2}))?`)

// readChangelogReleases returns the Unreleased section and the next n released
// sections with one line per bullet, so the panel shows what each release added.
func readChangelogReleases(path string, n int) []dashboard.RouterRelease {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []dashboard.RouterRelease
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
			out = append(out, dashboard.RouterRelease{Version: m[1], Date: m[2]})
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
