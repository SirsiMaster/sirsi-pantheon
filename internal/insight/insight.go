// Package insight produces a PLATFORM-WIDE "state of the union" across all of
// Pantheon's deities — Anubis (hygiene), Horus (ops/health), Thoth (memory),
// the router (multi-agent collaboration), and per-deity run state — and a
// deterministic, prioritized recommendation of what to do next.
//
// AI-OPTIONAL CONTRACT: Build() is 100% deterministic and never needs a model.
// When the optional local Gemma backend is present (internal/gemma.Available),
// the caller may enrich the recommendation with a natural-language narration —
// but its absence is normal and changes nothing about correctness. Pantheon
// operates fully without an AI backend; it includes one when there is one.
package insight

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/deity"
	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// DeitySignal is one deity's deterministic, at-a-glance status.
type DeitySignal struct {
	Deity    string `json:"deity"`
	Glyph    string `json:"glyph"`
	Status   string `json:"status"`
	Severity int    `json:"severity"` // 0 ok · 1 attention · 2 critical
}

// Action is a concrete, deterministic next step with the exact command.
type Action struct {
	Priority int    `json:"priority"` // lower = do first
	Title    string `json:"title"`
	Why      string `json:"why"`
	Command  string `json:"command"`
}

// Platform is the aggregated state of the union.
type Platform struct {
	Signals   []DeitySignal `json:"signals"`
	Actions   []Action      `json:"actions"`
	Worst     int           `json:"worst"`
	Narrative string        `json:"narrative,omitempty"` // optional Gemma enrichment
	Source    string        `json:"source"`              // "rules" or "rules+gemma"
}

// Build assembles the platform state deterministically. repoRoot scopes the
// router/Thoth signals (use the current project root; "" disables them).
func Build(repoRoot string) Platform {
	p := Platform{Source: "rules"}

	p.addHorus()  // ops / health
	p.addAnubis() // hygiene
	p.addThoth(repoRoot)
	p.addRouter(repoRoot)
	p.addRunStates() // any deity without a richer signal, from last-run state

	// Worst severity across signals drives the headline.
	for _, s := range p.Signals {
		if s.Severity > p.Worst {
			p.Worst = s.Severity
		}
	}
	// Prioritized, stable order.
	sort.SliceStable(p.Actions, func(i, j int) bool { return p.Actions[i].Priority < p.Actions[j].Priority })
	return p
}

// ── Horus — ops / health ─────────────────────────────────────────────────────

func (p *Platform) addHorus() {
	rep, err := guard.Doctor()
	if err != nil || rep == nil {
		p.Signals = append(p.Signals, DeitySignal{"Horus — Ops", "𓂀", "health unavailable", 1})
		return
	}
	worst, issues := 0, 0
	for _, f := range rep.Findings {
		sev := mapDiagSeverity(int(f.Severity))
		if sev >= 1 {
			issues++
		}
		if sev > worst {
			worst = sev
		}
		// Turn known critical/warn checks into concrete, prioritized actions.
		p.Actions = append(p.Actions, horusActions(f, sev)...)
	}
	status := "all healthy"
	if issues > 0 {
		status = pluralIssues(issues)
	}
	p.Signals = append(p.Signals, DeitySignal{"Horus — Ops", "𓂀", status, worst})
}

// horusActions maps one diagnose finding to the concrete, prioritized relief
// action(s) it warrants. Pure (no I/O) so the detect→relief mapping is unit
// tested directly. Lower Priority = surfaced first.
func horusActions(f guard.DiagnosticFinding, sev int) []Action {
	var out []Action
	switch {
	case f.Check == "App Crashes (7d)" && sev >= 1:
		out = append(out, Action{20, "Clear the crash backlog", f.Message, "sirsi clean"})
		if containsSelfCrash(f) {
			out = append(out, Action{10, "Heal the sirsi binary (it is its own #1 crasher)", "binary drift on disk → AMFI SIGKILL on exec", "sirsi self-update"})
		}
	case f.Check == "Spotlight Storm" && sev >= 1:
		out = append(out, Action{30, "Stop the Spotlight write-storm", f.Message, "sirsi spotlight-exclude ~/Development"})
	case f.Check == "Jetsam Events (7d)" && sev >= 2:
		out = append(out, Action{15, "Relieve sustained RAM pressure", f.Message, "sirsi diagnose"})
	case f.Check == "App Hangs (7d)" && sev >= 1:
		// Detection → relief. Pick the SAFE existing remediation by who is
		// saturating: a root-owned Spotlight indexer can't be reniced by the
		// user (and Spotlight-exclude is the real fix); a normal CPU hog is
		// deprioritized by guard's renice throttler. Never a kill.
		title, cmd := "Calm the freeze / stutter", "sirsi guard"
		if isSpotlightOffender(f.Detail) {
			title, cmd = "Stop the Spotlight storm (top hang cause)", "sirsi spotlight-exclude ~/Development"
		}
		out = append(out, Action{18, title, f.Message, cmd})
	}
	return out
}

func containsSelfCrash(f guard.DiagnosticFinding) bool {
	return strings.Contains(f.Message, "sirsi") || strings.Contains(f.Detail, "sirsi")
}

// isSpotlightOffender reports whether the App-Hangs offender detail names a
// macOS Spotlight/metadata indexer — a root-owned daemon best relieved by
// Spotlight Privacy exclusion, not a (permission-denied, often harmful) renice.
func isSpotlightOffender(detail string) bool {
	d := strings.ToLower(detail)
	for _, n := range []string{"spotlightknowledged", "mds_stores", "mdworker", "mds ", "mdsync"} {
		if strings.Contains(d, n) {
			return true
		}
	}
	return false
}

// ── Anubis — hygiene ─────────────────────────────────────────────────────────

func (p *Platform) addAnubis() {
	ps, err := jackal.LoadLatest()
	if err != nil || ps == nil {
		p.Signals = append(p.Signals, DeitySignal{"Anubis — Hygiene", "🐺", "no scan yet", 0})
		return
	}
	var safeBytes int64
	for _, f := range ps.Findings {
		if f.Severity == jackal.SeveritySafe {
			safeBytes += f.SizeBytes
		}
	}
	const gb = 1 << 30
	sev := 0
	status := "clean"
	if safeBytes > 0 {
		status = jackal.FormatSize(safeBytes) + " reclaimable"
	}
	if safeBytes >= 2*gb {
		sev = 1
		p.Actions = append(p.Actions, Action{40, "Reclaim safe disk space", jackal.FormatSize(safeBytes) + " of regenerable caches", "sirsi clean"})
	}
	p.Signals = append(p.Signals, DeitySignal{"Anubis — Hygiene", "🐺", status, sev})
}

// ── Thoth — memory ───────────────────────────────────────────────────────────

func (p *Platform) addThoth(repoRoot string) {
	if repoRoot == "" {
		return
	}
	mem := filepath.Join(repoRoot, ".thoth", "memory.yaml")
	info, err := os.Stat(mem)
	if err != nil {
		p.Signals = append(p.Signals, DeitySignal{"Thoth — Memory", "𓁟", "no project memory in this folder", 0})
		return
	}
	age := nowFn().Sub(info.ModTime())
	status := "memory current"
	sev := 0
	if age > 14*24*time.Hour {
		status = "memory stale — sync recommended"
		sev = 1
		p.Actions = append(p.Actions, Action{60, "Refresh project memory", "memory.yaml not synced in 14+ days", "sirsi thoth sync"})
	}
	p.Signals = append(p.Signals, DeitySignal{"Thoth — Memory", "𓁟", status, sev})
}

// ── Router — multi-agent collaboration ───────────────────────────────────────

func (p *Platform) addRouter(repoRoot string) {
	if repoRoot == "" {
		return
	}
	items := filepath.Join(repoRoot, ".agents", "idea-router", "items")
	entries, err := os.ReadDir(items)
	if err != nil {
		return // router not in use here — not a signal
	}
	open := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".md" {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(items, e.Name())); err == nil {
			if strings.Contains(string(data), "status: open") {
				open++
			}
		}
	}
	status := "no open hand-offs"
	if open > 0 {
		status = strconv.Itoa(open) + " open router item(s)"
	}
	p.Signals = append(p.Signals, DeitySignal{"Router — Collaboration", "𓆑", status, 0})
}

// ── per-deity run state (fills gaps for deities without a richer signal) ──────

func (p *Platform) addRunStates() {
	st, err := deity.LoadState()
	if err != nil {
		return
	}
	// Deities without a richer live signal above — surfaced from last-run state
	// so the platform view is complete (Pantheon is more than hygiene).
	known := []struct{ key, name, glyph string }{
		{"maat", "Ma'at — Governance", "𓆄"},
		{"ra", "Ra — Fleet", "𓇶"},
	}
	for _, k := range known {
		status := "ready"
		if rs, ok := st.DeityState[k.key]; ok {
			status = runStateLabel(rs)
		}
		p.Signals = append(p.Signals, DeitySignal{k.name, k.glyph, status, 0})
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// nowFn is injectable for deterministic tests.
var nowFn = time.Now

func mapDiagSeverity(d int) int {
	// guard: 0 OK, 1 Info, 2 Warn, 3 Critical → insight: 0 ok, 1 attention, 2 critical
	switch {
	case d >= 3:
		return 2
	case d >= 2:
		return 1
	default:
		return 0
	}
}

func runStateLabel(rs deity.RunState) string {
	switch rs {
	case deity.StateSucceeded:
		return "last run ok"
	case deity.StateFailed:
		return "last run failed"
	case deity.StateHasData:
		return "has actionable data"
	default:
		return "never run"
	}
}

func pluralIssues(n int) string {
	if n == 1 {
		return "1 issue"
	}
	return strconv.Itoa(n) + " issues"
}
