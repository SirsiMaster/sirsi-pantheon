// Package router — wake.go
//
// Wake-or-declare-unavailable (PR#2 router hardening). The honest-liveness work
// (#79/#80) and the stranded-inbox surface (#85) tell us WHICH agents have work
// waiting and whether they are armed. This file closes the loop: on a SUPERVISOR
// or DOCTOR tick (never on `router send`), it walks every open inbox item and for
// each one either (a) confirms the target is armed and leaves it, (b) wakes the
// target via an explicitly-configured, reachable adapter exactly once, or (c)
// records wake_status:wake-unavailable on the item itself — so a stranded item is
// never silent and never blind-spawned.
//
// Design constraints (claude-home bind 2026-06-19, codex SME):
//  1. Wake fires from a supervisor/doctor pass, not inside router send.
//  2. LaunchAgent pull-loops are pull-loop watchers, armed by HEARTBEAT freshness
//     — never the loop-monitor pgrep gate (#79/#80 stays Claude-only).
//  3. Interactive claude-* are never daemonized/blind-spawned — if unarmed they
//     are routed via the claude-home conduit or marked wake-unavailable.
//  4. cli-spawn is EXPLICIT-only: a legacy Command array (WakeMechanism()'s
//     cli-spawn default) must NOT trigger a wake.
//  5. The mechanism enum carries launchagent + none (wakemechanism.go).
//  6. Delivery truth is additive item frontmatter, not a sidecar (work.SetWake).
//  7. Idempotency: item wake_attempted_at + (for LaunchAgent) label/pidfile/flock.
package router

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// Wake status values recorded on an item's wake_status frontmatter field.
const (
	WakeStatusPending     = "pending"
	WakeStatusAttempted   = "wake-attempted"
	WakeStatusUnavailable = "wake-unavailable"
	WakeStatusArmed       = "armed"
)

// DefaultWakeRetryAfter bounds re-invocation: once an item records a
// wake_attempted_at, the pass will not invoke its adapter again until this long
// has elapsed. Keeps repeated supervisor ticks from spawning a worker per tick.
const DefaultWakeRetryAfter = 10 * time.Minute

// inboxUnion lists open items for agent (all agents if agent==""), unioning the
// store with the files when the ADR-036/037 cutover is active so store-only
// items are visible to the internal/router surfaces (node-status, supervisor,
// wake pass). Default-off it is exactly work.ListInbox — byte-identical — and if
// the store cannot open it degrades to files rather than failing the surface.
func inboxUnion(routerRoot, agent string) ([]work.Item, error) {
	if !routercfg.StoreWake() {
		return work.ListInbox(routerRoot, agent)
	}
	repoRoot := filepath.Dir(filepath.Dir(routerRoot)) // <repo> from <repo>/.agents/idea-router
	f, err := dispatch.Open(repoRoot)
	if err != nil {
		return work.ListInbox(routerRoot, agent)
	}
	defer func() { _ = f.Close() }()
	return f.Inbox(agent)
}

// AgentWakeHealth (the readiness verdict) is defined in nodestatus.go — it is the
// per-agent wake-readiness type the node-status surface already exposes. The wake
// pass reuses it so the surfaced readiness and the acted-on readiness are one type.

// ExplicitWakeMechanism returns ONLY the explicitly-configured wake mechanism.
// Unlike AgentConfig.WakeMechanism(), it never infers cli-spawn from a bare
// Command array: a legacy command agent has declared no wake intent and must
// never be blind-spawned on a tick (constraint 4). Returns "" for such agents.
func ExplicitWakeMechanism(cfg AgentConfig) string {
	return strings.TrimSpace(cfg.Wake.Mechanism)
}

// isInteractiveSpawnType reports whether spawning the agent's launch command
// would start an INTERACTIVE REPL session rather than a headless worker. cli-spawn
// is never an honest wake for these (constraint 3): a fresh `claude` process is a
// new conversation, not a nudge to the running one.
func isInteractiveSpawnType(agentType string) bool {
	return strings.EqualFold(strings.TrimSpace(agentType), "claude")
}

// ProbeWakeReadiness probes whether cfg can be woken right now without
// blind-spawning. It is the HONEST, explicit-only readiness the wake pass acts
// on (and node-status can surface): unlike the legacy permissive view, a bare
// command array is NOT treated as wakeable.
func ProbeWakeReadiness(cfg AgentConfig) AgentWakeHealth {
	mech := ExplicitWakeMechanism(cfg)
	h := AgentWakeHealth{AgentID: cfg.ID, Mechanism: mech}
	switch mech {
	case "":
		h.Detail = "no explicit wake mechanism — legacy command agents are never blind-spawned (set wake.mechanism to enable)"
	case WakeNone:
		h.Detail = "wake disabled (mechanism: none)"
	case WakeCLISpawn:
		switch {
		case isInteractiveSpawnType(cfg.Type):
			h.Detail = fmt.Sprintf("interactive %s agent — not blind-spawned; arm its /loop or route via claude-home conduit", cfg.Type)
		case len(cfg.Command) == 0:
			h.Detail = "cli-spawn configured but command array is empty"
		default:
			if _, err := exec.LookPath(cfg.Command[0]); err != nil {
				h.Detail = fmt.Sprintf("cli-spawn command %q not found in PATH", cfg.Command[0])
			} else {
				h.Ready, h.Adapter, h.Detail = true, WakeCLISpawn, "explicit cli-spawn ready"
			}
		}
	case WakeAPICall:
		if strings.TrimSpace(cfg.Wake.Endpoint) == "" {
			h.Detail = "api-call configured but wake.endpoint is empty"
		} else {
			h.Ready, h.Adapter, h.Detail = true, WakeAPICall, cfg.Wake.Endpoint
		}
	case WakeMCPNotification:
		// MCP wake is not yet wired in the pull-model CLI — there is no invoker
		// that can deliver an MCP notification, so readiness MUST report not-ready.
		// The doctor's report view and the acted-on pass must agree; claiming
		// "ready" here while the invoker always fails is dishonest readiness
		// (codex SME #89, finding 3). Wire an adapter before flipping this.
		h.Detail = "mcp-notification wake is not yet wired in the pull-model CLI — use launchagent or api-call"
	case WakeLaunchAgent:
		label := WakeLaunchAgentLabel(cfg.ID)
		if !launchAgentInstalled(label) {
			h.Detail = fmt.Sprintf("launchagent %s not installed — run `sirsi router wake-install %s`", label, cfg.ID)
		} else {
			h.Ready, h.Adapter, h.Detail = true, WakeLaunchAgent, label
		}
	default:
		h.Detail = fmt.Sprintf("unsupported wake mechanism %q", mech)
	}
	return h
}

// WakeInvoke performs the side effect of waking an agent via the named adapter.
// Injectable (Rule A16) + mutex-guarded (Rule A21) so the wake pass is testable
// without spawning real processes or making network calls.
type WakeInvoke func(cfg AgentConfig, adapter string) error

var (
	wakeInvokeMu sync.RWMutex
	wakeInvokeFn WakeInvoke = defaultWakeInvoke
)

func getWakeInvoke() WakeInvoke {
	wakeInvokeMu.RLock()
	defer wakeInvokeMu.RUnlock()
	return wakeInvokeFn
}

func setWakeInvoke(fn WakeInvoke) {
	wakeInvokeMu.Lock()
	defer wakeInvokeMu.Unlock()
	if fn == nil {
		fn = defaultWakeInvoke
	}
	wakeInvokeFn = fn
}

func defaultWakeInvoke(cfg AgentConfig, adapter string) error {
	switch adapter {
	case WakeCLISpawn:
		if len(cfg.Command) == 0 {
			return fmt.Errorf("cli-spawn: empty command")
		}
		cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
		cmd.Dir = cfg.Cwd
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		// Detach (Unix Setsid) + Release so the nudged worker survives this
		// short-lived doctor tick and leaves no zombie — the established
		// cmd/sirsi router-event spawn pattern (codex SME #89, finding 2).
		cmd.SysProcAttr = detachedSysProcAttr()
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	case WakeAPICall:
		req, err := http.NewRequest(http.MethodPost, cfg.Wake.Endpoint, nil)
		if err != nil {
			return err
		}
		if cfg.Wake.Auth != "" {
			req.Header.Set("Authorization", cfg.Wake.Auth)
		}
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("api-call: wake endpoint returned %s", resp.Status)
		}
		return nil
	case WakeLaunchAgent:
		// The pull-loop is resident; kick it to poll immediately.
		label := WakeLaunchAgentLabel(cfg.ID)
		target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
		return exec.Command("launchctl", "kickstart", "-k", target).Run()
	default:
		// Includes mcp-notification: ProbeWakeReadiness never yields it as a
		// ready adapter (not yet wired), so this is the honest failure if reached.
		return fmt.Errorf("no invoker for adapter %q", adapter)
	}
}

// WakeOutcome is one item's result in a wake pass.
type WakeOutcome struct {
	ItemID  string `json:"item_id"`
	AgentID string `json:"agent_id"`
	Adapter string `json:"adapter,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// WakePassReport summarizes one wake-or-declare-unavailable pass.
type WakePassReport struct {
	Armed       []WakeOutcome `json:"armed,omitempty"`
	Attempted   []WakeOutcome `json:"attempted,omitempty"`
	Unavailable []WakeOutcome `json:"unavailable,omitempty"`
}

// WakePass runs one wake-or-declare-unavailable pass over every OPEN inbox item.
// This is the supervisor/doctor-tick wake (constraint 1): it is never called from
// `router send`. For each item, by its target agent:
//   - armed (a non-terminal, heartbeat-FRESH thread exists) → annotate armed, no spawn.
//   - unarmed + WakeHealth.Ready → invoke the adapter ONCE (idempotent via the
//     item's wake_attempted_at), annotate wake-attempted.
//   - unarmed + !Ready → annotate wake-unavailable + wake_error (never blind-spawn).
//
// "Armed" is heartbeat freshness only (constraint 2) — it deliberately does NOT
// consult the loop-monitor pgrep gate, so it never widens #79/#80 to pull-loops.
func WakePass(routerRoot string, now time.Time) (WakePassReport, error) {
	return WakePassFiltered(routerRoot, now, nil)
}

// WakePassFiltered is WakePass restricted to the items `allow` accepts. A nil
// filter is allow-all, so WakePass is byte-identical. The continuous work loop
// (ADR-039 P3) passes a filter of exactly the gate-cleared, dispatch-authorized
// item ids, so a wake pass can never touch an owner-gated item.
func WakePassFiltered(routerRoot string, now time.Time, allow func(work.Item) bool) (WakePassReport, error) {
	var rep WakePassReport
	if now.IsZero() {
		now = time.Now().UTC()
	}

	reg, err := LoadRegistry(routerRoot)
	if err != nil {
		return rep, fmt.Errorf("load agents: %w", err)
	}
	threadReg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return rep, fmt.Errorf("load threads: %w", err)
	}

	armed := map[string]bool{}
	for _, t := range threadReg.SortedThreads() {
		if t.Status.IsTerminal() || t.IsStale(now, DefaultThreadStaleAfter) {
			continue
		}
		armed[t.AgentID] = true
	}

	// Post-cutover (ADR-036/037) open items live only as store rows, and a
	// store-only item has no file to annotate — so both the READ (which items are
	// waiting) and the WRITE (idempotent wake_status) must go through the facade,
	// which unions files+store and routes SetWake to whichever holds the item.
	// Default-off stays byte-identical: f == nil → the exact legacy file calls.
	var f *dispatch.Facade
	if routercfg.StoreWake() {
		repoRoot := filepath.Dir(filepath.Dir(routerRoot)) // <repo> from <repo>/.agents/idea-router
		if fac, ferr := dispatch.Open(repoRoot); ferr == nil {
			f = fac
			defer func() { _ = f.Close() }()
		}
	}
	setWake := func(id string, ann work.WakeAnnotation) {
		if f != nil {
			_ = f.SetWake(id, ann)
			return
		}
		_ = work.SetWake(routerRoot, id, ann)
	}

	var items []work.Item
	if f != nil {
		items, err = f.Inbox("") // all open items (store ∪ files)
	} else {
		items, err = work.ListInbox(routerRoot, "") // all open items
	}
	if err != nil {
		return rep, fmt.Errorf("list inbox: %w", err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })

	invoke := getWakeInvoke()
	for _, item := range items {
		agentID := item.To
		if agentID == "" {
			continue
		}
		// The continuous loop's gate/authorization filter: skip any item not
		// explicitly cleared for dispatch (owner-gated items never pass).
		if allow != nil && !allow(item) {
			continue
		}

		if armed[agentID] {
			// Idempotent: only WRITE when the status actually changes to armed.
			// Re-writing an already-armed item every pass bumps its mtime, and
			// when items/ is a launchd WatchPath (the conduit mesh) that would
			// self-trigger an endless tick loop. A steady state produces no writes.
			if item.WakeStatus != WakeStatusArmed {
				setWake(item.ID, work.WakeAnnotation{Status: WakeStatusArmed})
			}
			rep.Armed = append(rep.Armed, WakeOutcome{ItemID: item.ID, AgentID: agentID})
			continue
		}

		var health AgentWakeHealth
		if cfg, lerr := reg.Lookup(agentID); lerr != nil {
			health = AgentWakeHealth{Detail: fmt.Sprintf("agent %q not registered", agentID)}
		} else {
			health = ProbeWakeReadiness(*cfg)
		}

		if !health.Ready {
			// Idempotent (same reason as the armed branch): only write on change.
			if item.WakeStatus != WakeStatusUnavailable {
				setWake(item.ID, work.WakeAnnotation{Status: WakeStatusUnavailable, Error: health.Detail})
			}
			rep.Unavailable = append(rep.Unavailable, WakeOutcome{ItemID: item.ID, AgentID: agentID, Detail: health.Detail})
			continue
		}

		// Idempotency: don't re-invoke while a prior attempt is still fresh.
		if item.WakeStatus == WakeStatusAttempted && item.WakeAttemptedAt != "" {
			if at, perr := time.Parse(time.RFC3339, item.WakeAttemptedAt); perr == nil && now.Sub(at) < DefaultWakeRetryAfter {
				rep.Attempted = append(rep.Attempted, WakeOutcome{ItemID: item.ID, AgentID: agentID, Adapter: health.Adapter, Detail: "already attempted (idempotent skip)"})
				continue
			}
		}

		cfg, _ := reg.Lookup(agentID) // Ready implies registered
		ann := work.WakeAnnotation{Status: WakeStatusAttempted, AttemptedAt: now.Format(time.RFC3339), Adapter: health.Adapter}
		if ierr := invoke(*cfg, health.Adapter); ierr != nil {
			ann = work.WakeAnnotation{Status: WakeStatusUnavailable, Error: fmt.Sprintf("%s adapter failed: %v", health.Adapter, ierr)}
			setWake(item.ID, ann)
			rep.Unavailable = append(rep.Unavailable, WakeOutcome{ItemID: item.ID, AgentID: agentID, Adapter: health.Adapter, Detail: ann.Error})
			continue
		}
		setWake(item.ID, ann)
		rep.Attempted = append(rep.Attempted, WakeOutcome{ItemID: item.ID, AgentID: agentID, Adapter: health.Adapter})
	}
	return rep, nil
}

// DefaultWakeLoopInterval is the pull-loop heartbeat cadence (A27 bounded ≥60s).
const DefaultWakeLoopInterval = 60 * time.Second

// RunWakeLoop is the foreground bounded pull-loop for a worker/headless agent
// (A27: "a bounded headless pull loop over items/ plus sirsi thread heartbeat").
// It registers a concrete pull-loop thread (surface=worker, bound to THIS
// process's pid, so it stays armed by heartbeat freshness and is not OS-reaped
// between heartbeats), then each interval pulls the agent's inbox and heartbeats.
// It runs until ctx is canceled (SIGTERM/SIGINT from launchd), then closes the
// thread. It is meant to be RUN BY the wake LaunchAgent — external automation
// (A26 Automation Boundary) — and does NOT self-daemonize: it is a plain
// foreground loop, not a reintroduced daemon verb.
func RunWakeLoop(ctx context.Context, routerRoot, agentID string, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultWakeLoopInterval
	}
	host, _ := os.Hostname()
	thr, err := RegisterThread(routerRoot, &Thread{
		AgentID:       agentID,
		Surface:       surfaceWorker,
		WakeMechanism: WakeLaunchAgent,
		PID:           os.Getpid(),
		Host:          host,
		Status:        ThreadStatusActive,
	})
	if err != nil {
		return fmt.Errorf("register wake-loop thread: %w", err)
	}
	defer func() { _, _ = CloseThread(routerRoot, thr.ThreadID) }()

	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		// Surface the inbox depth into the heartbeat status; the worker surface's
		// own logic processes the items — this loop only proves liveness + watches.
		status := ThreadStatusIdle
		if items, lerr := work.ListInbox(routerRoot, agentID); lerr == nil && len(items) > 0 {
			status = ThreadStatusActive
		}
		_, _ = Heartbeat(routerRoot, thr.ThreadID, HeartbeatUpdate{Status: status})

		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// ── LaunchAgent pull-loop install (constraint 2 + 7) ─────────────────────────

const wakeLaunchAgentPrefix = "ai.sirsi.router.wake."

var labelUnsafe = regexp.MustCompile(`[^a-zA-Z0-9.-]+`)

// launchAgentsDirOverride lets tests point the installer at a temp dir instead of
// the real ~/Library/LaunchAgents. Empty in production.
var launchAgentsDirOverride string

func launchAgentsDir() string {
	if launchAgentsDirOverride != "" {
		return launchAgentsDirOverride
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents")
}

// WakeLaunchAgentLabel returns the launchd label for an agent's wake pull-loop.
func WakeLaunchAgentLabel(agentID string) string {
	return wakeLaunchAgentPrefix + labelUnsafe.ReplaceAllString(strings.TrimSpace(agentID), "-")
}

func launchAgentInstalled(label string) bool {
	_, err := os.Stat(filepath.Join(launchAgentsDir(), label+".plist"))
	return err == nil
}

// wakeLaunchAgentPlist renders the per-agent pull-loop plist. The loop is the
// LaunchAgent's StartInterval: every interval it pulls the agent's inbox and
// heartbeats — a pull-loop watcher armed by heartbeat freshness, never a
// loop-monitor (constraint 2). Deterministic for a given (label, agent, bin) so
// InstallWakeLaunchAgent is idempotent by content comparison.
//
// ProgramArguments is a DIRECT argv (no `/bin/sh -c`): the loop is the real
// `sirsi router wake-loop <agent>` verb, so there is no shell to break on a path
// with spaces or to inject via a metacharacter-bearing agent id (codex SME #89,
// finding 4 — and finding 1: the prior `thread heartbeat --agent` flag did not
// exist). KeepAlive (not StartInterval) keeps ONE long-lived process whose stable
// pid is not OS-reaped between heartbeats; launchd restarts it if it exits. Both
// values are XML-escaped for the plist text.
func wakeLaunchAgentPlist(label string, cfg AgentConfig, sirsiBin string) string {
	// Observability + crash-loop bounds, learned the hard way (2026-07-07:
	// nine loops dead-respawning ~15k times with NO log file to say why):
	//   - StandardOut/ErrorPath: every start/exit leaves evidence under
	//     ~/.sirsi/logs/wake-<agent>.log.
	//   - ThrottleInterval 60: a crashing loop respawns once a minute, not
	//     at launchd's default burn rate.
	//   - The binary path is absolute (InstallWakeLaunchAgent enforces it).
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, ".sirsi", "logs", "wake-"+slugifyLabelPart(cfg.ID)+".log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>router</string>
    <string>wake-loop</string>
    <string>%s</string>
  </array>
  <key>KeepAlive</key>
  <true/>
  <key>RunAtLoad</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>60</integer>
  <key>ProcessType</key>
  <string>Background</string>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, escapeXML(label), escapeXML(sirsiBin), escapeXML(cfg.ID), escapeXML(logPath), escapeXML(logPath))
}

// slugifyLabelPart mirrors WakeLaunchAgentLabel's id sanitization for log
// filenames, so the label and its log always correspond.
func slugifyLabelPart(id string) string {
	return labelUnsafe.ReplaceAllString(strings.TrimSpace(id), "-")
}

// escapeXML escapes the five XML predefined entities so a sirsi path or agent id
// containing &, <, >, ", or ' cannot break (or inject into) the plist text.
func escapeXML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(s)
}

// WakeLaunchAgentInstalled reports whether the per-agent pull-loop LaunchAgent
// plist for agentID is present on disk. Exported so thread-scoped callers can show
// install status without re-deriving the label/dir convention.
func WakeLaunchAgentInstalled(agentID string) bool {
	return launchAgentInstalled(WakeLaunchAgentLabel(agentID))
}

// UninstallWakeLaunchAgent removes the per-agent pull-loop LaunchAgent installed by
// InstallWakeLaunchAgent — the clean-off path (A27: install AND clean uninstall).
// It is the exact inverse of Install: it best-effort boots out the running launchd
// job (so the resident wake-loop stops immediately) and then removes the plist.
// Idempotent: removed=false when there is nothing installed, so re-running is a
// no-op. The bootout is best-effort — an unloaded-but-present plist still gets its
// file removed, and a missing plist never errors.
func UninstallWakeLaunchAgent(cfg AgentConfig) (removed bool, path string, err error) {
	label := WakeLaunchAgentLabel(cfg.ID)
	path = filepath.Join(launchAgentsDir(), label+".plist")
	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, path, nil // nothing installed — clean no-op
		}
		return false, path, fmt.Errorf("stat wake LaunchAgent: %w", statErr)
	}
	// Best-effort: stop the resident loop before deleting its definition. Ignore
	// the error — the job may already be unloaded, and file removal is what makes
	// the uninstall durable across a relaunch. Guarded so tests that redirect
	// launchAgentsDirOverride to a temp dir don't touch the real launchd domain.
	if launchAgentsDirOverride == "" {
		target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
		_ = exec.Command("launchctl", "bootout", target).Run()
	}
	if rmErr := os.Remove(path); rmErr != nil {
		return false, path, fmt.Errorf("remove wake LaunchAgent plist: %w", rmErr)
	}
	return true, path, nil
}

// InstallWakeLaunchAgent writes (idempotently) the per-agent pull-loop LaunchAgent
// for a worker/headless agent. Returns changed=false when the plist already exists
// with identical content — re-running setup is a no-op (constraint 7 idempotency
// at the install layer; flock/pidfile keep the loop single-instance at runtime).
func InstallWakeLaunchAgent(cfg AgentConfig, sirsiBin string) (changed bool, path string, err error) {
	if strings.TrimSpace(sirsiBin) == "" {
		// Resolution order: the RUNNING binary first (always absolute, always
		// exists — and it is exactly the code the caller just exercised),
		// then PATH. A bare name in ProgramArguments[0] is unspawnable under
		// launchd (its PATH has no ~/.local/bin): the job "runs", dies
		// instantly, and KeepAlive respawns it forever — 15,189 silent
		// restarts per loop before anyone noticed (2026-07-07). PATH lookup
		// alone is also the ADR-023 D3 drift vector. Refuse loudly rather
		// than write an unspawnable plist.
		if self, serr := os.Executable(); serr == nil && strings.Contains(filepath.Base(self), "sirsi") {
			sirsiBin = self
		} else if p, lerr := exec.LookPath("sirsi"); lerr == nil {
			sirsiBin = p
		} else {
			return false, "", fmt.Errorf("cannot resolve an absolute sirsi binary for the LaunchAgent (launchd cannot spawn a bare name): %w", lerr)
		}
	}
	if !filepath.IsAbs(sirsiBin) {
		if abs, aerr := filepath.Abs(sirsiBin); aerr == nil {
			sirsiBin = abs
		}
	}
	label := WakeLaunchAgentLabel(cfg.ID)
	dir := launchAgentsDir()
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return false, "", fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	// launchd creates the log FILES but not their parent directory — a
	// missing ~/.sirsi/logs would turn StandardErrorPath into another
	// silent-death mode.
	if home, herr := os.UserHomeDir(); herr == nil {
		if err = os.MkdirAll(filepath.Join(home, ".sirsi", "logs"), 0o755); err != nil {
			return false, "", fmt.Errorf("create wake log dir: %w", err)
		}
	}
	path = filepath.Join(dir, label+".plist")
	content := wakeLaunchAgentPlist(label, cfg, sirsiBin)
	if existing, rerr := os.ReadFile(path); rerr == nil && string(existing) == content {
		return false, path, nil
	}
	if err = os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, path, fmt.Errorf("write plist: %w", err)
	}
	return true, path, nil
}
