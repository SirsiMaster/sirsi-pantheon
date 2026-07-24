// Package liveness — the OS-level (launchd) liveness watch.
//
// The router conduit, the gemma generation broker, and the menubar are watched
// today only by Claude-app scheduled tasks: those survive a session close but
// NOT a full OS reboot until a human relaunches the app — and (worse) every
// scheduled run leaks a resident claude-desktop session, the very leak that
// swap-killed the broker on 2026-07-17. This LaunchAgent closes both gaps: it is
// OS-managed (RunAtLoad + StartInterval), needs no Claude app, and spawns NO
// claude session, so it is zero-leak by construction. Routed by claude-home;
// canon A32 (overseer) + the launchd-durability item. Decision: this is the thin
// liveness+alert safety net ONLY — it never runs the router's heavy passes.
//
// Load-bearing safety (A32/ADR-040): Run() is strictly read-and-route. It NEVER
// signals, kills, resizes, or restarts any process, never touches /Applications,
// and never acts on an unverified PID. Remediation stays with the owner (routed
// as one non-duplicate item) and the existing governors — the watch only sees.
package liveness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/reaper"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// Label is the LaunchAgent label for the liveness watch.
const Label = "ai.sirsi.liveness-watch"

// StartInterval is how often launchd fires the watch (~15 min): frequent enough
// to catch a wedge within one cadence, rare enough to cost nothing.
const StartInterval = 900

// probeTimeout bounds the gemma generation probe. Longer than a healthy answer,
// short enough that a wedged broker (>30s to first token) reads as wedged.
const probeTimeout = 30 * time.Second

// PlistPath is where the LaunchAgent is written (per-user).
func PlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
}

// Installed reports whether the LaunchAgent plist is in place. macOS only.
func Installed() bool {
	return runtime.GOOS == "darwin" && fileExists(PlistPath())
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// PlistContent renders the LaunchAgent. RunAtLoad fires it once at login/load;
// StartInterval repeats it. KeepAlive is deliberately ABSENT: the watch runs and
// exits, so KeepAlive would restart it in a tight loop. WorkingDirectory pins the
// repo root so the router root under .agents/idea-router/ resolves.
func PlistContent(sirsiBin, workDir string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%[1]s</string>
	<key>WorkingDirectory</key>
	<string>%[3]s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%[2]s</string>
		<string>liveness-watch</string>
		<string>run</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>StartInterval</key>
	<integer>%[4]d</integer>
	<key>StandardOutPath</key>
	<string>/tmp/sirsi-liveness-watch.log</string>
	<key>StandardErrorPath</key>
	<string>/tmp/sirsi-liveness-watch.err</string>
	<key>ProcessType</key>
	<string>Background</string>
</dict>
</plist>
`, Label, sirsiBin, workDir, StartInterval)
}

// Install writes the LaunchAgent and loads it so the watch runs now and at every
// login. macOS only; idempotent (a re-install reloads). Returns a status line.
func Install(sirsiBin, workDir string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "liveness watch is macOS only", nil
	}
	if sirsiBin == "" {
		return "", fmt.Errorf("sirsi binary path could not be resolved")
	}
	path := PlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(PlistContent(sirsiBin, workDir)), 0o644); err != nil {
		return "", err
	}
	// Reload: unload first so a re-install picks up changes (ignore the not-loaded error).
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := exec.Command("launchctl", "load", path).Run(); err != nil {
		return "", fmt.Errorf("wrote LaunchAgent but launchctl load failed: %w", err)
	}
	return "liveness watch installed and started (fires now + every " + strconv.Itoa(StartInterval/60) + " min)", nil
}

// Uninstall unloads and removes the LaunchAgent. Idempotent.
func Uninstall() (string, error) {
	path := PlistPath()
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return "liveness watch uninstalled", nil
}

// Finding is one liveness signal read by Run.
type Finding struct {
	Check   string // "gemma-broker" | "menubar" | "memory-death"
	OK      bool
	Detail  string
	Fixable bool   // an owner-fixable, currently-actionable blocker (route it)
	Title   string // deterministic router title for dedup (empty when OK)
	Body    string // routed instructions when !OK && Fixable
}

// Run performs the liveness checks and routes ONE non-duplicate item to `user`
// for the worst current owner-fixable blocker. routerRoot is the idea-router
// root (…/.agents/idea-router). Read-and-route only — never kills a process.
func Run(routerRoot string, w io.Writer) error {
	home, _ := os.UserHomeDir()
	// Order = severity: a wedged broker strands every agent; a memory spiral
	// swap-kills the broker; a leaked-session pileup is the precursor that CAUSES
	// that spiral; a dead menubar only loses the operator surface.
	findings := []Finding{
		probeGemma(home),
		probeMemoryDeath(),
		probeSessionLeak(),
		probeMenubar(),
	}

	for _, f := range findings {
		status := "ok"
		if !f.OK {
			status = "WEDGED"
		}
		fmt.Fprintf(w, "%-14s %-7s %s\n", f.Check, status, f.Detail)
	}

	// Route the single worst blocker (order = severity: broker, then memory, then
	// menubar). One item per pass, deduplicated against the open `user` inbox so a
	// persistent wedge does not flood the router every 15 minutes.
	worst := pickWorst(findings)
	if worst == nil {
		return nil
	}
	// Route to the agent that OWNS the remediation, not the owner queue (owner
	// standing correction 2026-07-24: liveness + machine-health remediation
	// belongs to Pantheon, not `user`). Every machine-health condition here —
	// a wedged broker, a memory spiral, a leaked-session pileup, a dead menubar
	// — is one claude-pantheon now remediates under A32/ADR-040 (broker
	// right-size/restart, safe caller-protected reap, launchd kickstart). Only
	// a genuinely owner-only condition (none today) would fall through to user.
	recipient := recipientFor(worst.Check)
	if hasOpen(routerRoot, recipient, worst.Title) {
		fmt.Fprintf(w, "route          skip    already open: %q\n", worst.Title)
		return nil
	}
	id, err := work.Send(routerRoot, "liveness-watch", recipient, worst.Title, worst.Body)
	if err != nil {
		return fmt.Errorf("route blocker: %w", err)
	}
	fmt.Fprintf(w, "route          sent    %s → %s (%s)\n", worst.Title, recipient, id)
	return nil
}

// recipientFor maps a finding's Check to the agent that owns its remediation.
// Machine-health conditions go to claude-pantheon (which fixes them under
// A32/ADR-040); anything unclassified falls through to `user` so a novel
// condition still reaches the owner rather than being silently misrouted.
func recipientFor(check string) string {
	switch check {
	case "gemma-broker", "memory-death", "session-leak", "menubar":
		return "claude-pantheon"
	default:
		return "user"
	}
}

// pickWorst returns the highest-severity fixable non-OK finding, or nil.
// Severity order is the findings slice order in Run: broker > memory > menubar.
func pickWorst(fs []Finding) *Finding {
	for i := range fs {
		if !fs[i].OK && fs[i].Fixable {
			return &fs[i]
		}
	}
	return nil
}

// hasOpen reports whether `user` already has an open item with this title.
func hasOpen(root, agent, title string) bool {
	items, err := work.ListInbox(root, agent)
	if err != nil {
		return false // can't read the inbox → don't suppress the alert
	}
	for _, it := range items {
		if it.Title == title {
			return true
		}
	}
	return false
}

// GemmaStatus classifies the broker's liveness for consumers that need to ACT on
// it (the router's self-healing gemma-liveness duty), not just alert.
type GemmaStatus int

const (
	// GemmaHealthy — answered a real generation.
	GemmaHealthy GemmaStatus = iota
	// GemmaDown — nothing is serving: no port file, or the connection is refused.
	// The correct restore is to START the broker.
	GemmaDown
	// GemmaWedged — the port responds but the broker is not generating (non-200,
	// empty content, or slower than probeTimeout). "Health lies" — /v1/models can
	// return 200 while completions return nothing (2026-07-17). The correct restore
	// is a GRACEFUL restart (stop + start), never a SIGKILL (A32/ADR-040).
	GemmaWedged
)

// ProbeGemmaState runs ONE real chat completion against the warm broker and
// classifies the result. Health lies, so it generates and inspects the answer
// rather than trusting a 200. Read-only. Shared by the read-only liveness watch
// (probeGemma) and the router's self-healing duty so both agree on "wedged".
func ProbeGemmaState(home string) (GemmaStatus, string) {
	portRaw, err := os.ReadFile(filepath.Join(home, ".sirsi/gemma-server.port"))
	if err != nil {
		return GemmaDown, "no port file (broker not running)"
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(portRaw)))
	if err != nil || port == 0 {
		return GemmaDown, "port file unreadable"
	}
	body, _ := json.Marshal(map[string]any{
		"model":       resolveModel(home),
		"messages":    []map[string]string{{"role": "user", "content": "Reply with the single word: OK"}},
		"max_tokens":  128, // reasoning model burns budget thinking; too-low a cap yields empty content
		"temperature": 0,
	})
	start := time.Now()
	cl := &http.Client{Timeout: probeTimeout}
	resp, err := cl.Post(fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", port), "application/json", bytes.NewReader(body))
	if err != nil {
		// A timeout means the server is there but hung (wedged); anything else
		// (connection refused, no listener) means nothing is serving (down).
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return GemmaWedged, fmt.Sprintf("no answer in %s (timed out — up but hung)", time.Since(start).Round(time.Second))
		}
		return GemmaDown, fmt.Sprintf("connection failed in %s (%v)", time.Since(start).Round(time.Second), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return GemmaWedged, fmt.Sprintf("HTTP %d (up but erroring)", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return GemmaWedged, "unparseable response (up but not generating)"
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return GemmaWedged, fmt.Sprintf("empty content after %s (up but not generating)", time.Since(start).Round(time.Second))
	}
	return GemmaHealthy, fmt.Sprintf("answered in %s", time.Since(start).Round(time.Second))
}

// probeGemma is the read-only liveness-watch wrapper over ProbeGemmaState.
func probeGemma(home string) Finding {
	f := Finding{Check: "gemma-broker", Fixable: true,
		Title: "liveness-watch: gemma broker wedged",
		Body: "The launchd liveness watch generated against the warm gemma broker and it did not answer " +
			"(no port, connection error, non-200, empty content, or >30s). The broker is the Tier-0 substrate " +
			"the router/reconcile/gemma-builder depend on. The router's gemma-liveness duty auto-restores it " +
			"(load-bearing-safe, A32/ADR-040: right-size, never SIGKILL — `sirsi gemma serve --stop && sirsi gemma serve`); " +
			"this alert fires when a restore did not stick (e.g. RAM won't fit the model — free memory)."}
	status, detail := ProbeGemmaState(home)
	f.OK = status == GemmaHealthy
	f.Detail = detail
	return f
}

// resolveModel mirrors the worker/CLI: env > ~/.sirsi/gemma-model.conf > fallback.
func resolveModel(home string) string {
	if m := os.Getenv("GEMMA_MODEL"); m != "" {
		return m
	}
	if b, err := os.ReadFile(filepath.Join(home, ".sirsi/gemma-model.conf")); err == nil {
		if m := strings.TrimSpace(string(b)); m != "" {
			return m
		}
	}
	return "mlx-community/gemma-2-27b-it-bf16-4bit"
}

// sessionLeakThreshold is how many leaked claude-desktop sessions must
// accumulate before the watch proactively alerts. Matches the `sirsi diagnose`
// leaked-sessions lever so the two surfaces agree.
const sessionLeakThreshold = 8

// sessionLeakCountFn returns (candidate count, reclaimable MB) for leaked
// sessions — the caller's own ancestry is never counted (reaper.Plan). Injectable
// (A16/A21) so the threshold branch is testable without a live process table.
var (
	sessionLeakMu    sync.RWMutex
	sessionLeakCount = func() (int, int) {
		p, err := reaper.Plan(reaper.Options{}, reaper.RealDeps())
		if err != nil {
			return 0, 0
		}
		return len(p.Candidates), p.ReclaimMBEst
	}
)

func getSessionLeakCount() func() (int, int) {
	sessionLeakMu.RLock()
	defer sessionLeakMu.RUnlock()
	return sessionLeakCount
}

func setSessionLeakCount(fn func() (int, int)) {
	sessionLeakMu.Lock()
	defer sessionLeakMu.Unlock()
	sessionLeakCount = fn
}

// probeSessionLeak surfaces a leaked-session pileup PROACTIVELY (before an
// operator runs `sirsi diagnose`) so it can be reclaimed before it swap-kills
// the broker. Alert-ONLY: it never reaps (the hard reap stays owner/doctor-
// invoked via `sirsi reap-sessions --apply` — no auto-kill from a headless net).
func probeSessionLeak() Finding {
	n, mb := getSessionLeakCount()()
	f := Finding{Check: "session-leak", Fixable: true,
		Title: "liveness-watch: leaked claude-desktop sessions piling up",
		Body: fmt.Sprintf("The launchd liveness watch found %d leaked claude-desktop task-runner sessions "+
			"(~%d MB reclaimable). These accumulate from scheduled-task/wakeup runs that never self-exit and "+
			"drive the swap-death that wedges the gemma broker (2026-07-17). Reclaim them safely — your live "+
			"session is protected: `sirsi reap-sessions` (dry-run) then `sirsi reap-sessions --apply`. Alert-only; "+
			"nothing was killed.", n, mb)}
	if n < sessionLeakThreshold {
		f.OK = true
		f.Detail = fmt.Sprintf("%d leaked (below %d threshold)", n, sessionLeakThreshold)
		return f
	}
	f.Detail = fmt.Sprintf("%d leaked sessions (~%d MB) — reap to prevent swap-death", n, mb)
	return f
}

// probeMenubar checks the SwiftUI menubar is alive by its exact executable path
// (never a bare "SirsiMenubar" that could match a build process).
func probeMenubar() Finding {
	f := Finding{Check: "menubar", Fixable: true,
		Title: "liveness-watch: menubar not running",
		Body: "The launchd liveness watch found no running SwiftUI menubar " +
			"(Sirsi Menubar.app/Contents/MacOS/SirsiMenubar). Relaunch it: open the installed " +
			"~/Applications/Sirsi Menubar.app (or rebuild via macapp/build-app.sh, then `open -n`)."}
	if runtime.GOOS != "darwin" {
		f.OK, f.Fixable, f.Detail = true, false, "not macOS"
		return f
	}
	if exec.Command("pgrep", "-f", "Sirsi Menubar.app/Contents/MacOS/SirsiMenubar").Run() == nil {
		f.OK, f.Detail = true, "running"
		return f
	}
	f.Detail = "no SwiftUI menubar process"
	return f
}

// probeMemoryDeath reads the swap/free-RAM spiral signals via guard (one shared
// definition of "dying"). It routes only the live-critical spiral; ordinary
// pressure is not a currently-owner-fixable emergency (A32: alarm only when a
// current fixable condition exists).
func probeMemoryDeath() Finding {
	md := guard.SampleMemoryDeath()
	f := Finding{Check: "memory-death", Fixable: true,
		Title: "liveness-watch: memory death spiral",
		Body: fmt.Sprintf("The launchd liveness watch measured a memory death spiral: swap %.0f%% (%.1f GB), "+
			"free %.2f GB, load %.1f on %d cores. The machine is paging itself to death — this swap-kills the "+
			"gemma broker (2026-07-17). Fix: close/restart the heaviest leaked sessions (leaked claude-desktop "+
			"scheduled-task sessions have NO safe auto-reap signature — restart Claude.app to reap them) and "+
			"right-size the broker; never SIGKILL a load-bearing serving process (A32/ADR-040).",
			md.SwapPct, md.SwapUsedGB, md.FreeGB, md.Load1, md.Cores)}
	if !md.Readable {
		f.OK, f.Fixable, f.Detail = true, false, "no signal readable"
		return f
	}
	if md.Dying {
		f.Detail = fmt.Sprintf("swap %.0f%% / free %.2f GB / load %.1f — SPIRAL", md.SwapPct, md.FreeGB, md.Load1)
		return f
	}
	f.OK = true
	f.Detail = fmt.Sprintf("swap %.0f%% / free %.2f GB / load %.1f/%d", md.SwapPct, md.FreeGB, md.Load1, md.Cores)
	return f
}
