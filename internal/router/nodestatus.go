// Package router — nodestatus.go
//
// Horus local-node status aggregation. Combines router queue state,
// agent registry, and daemon health into a single operator view.
// Ra owns the queue and dispatch; Horus owns this per-desktop surface.
package router

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// NodeStatusSchemaVersion is the frozen contract version for the NodeStatus
// JSON shape (ADR-026). Surfaces decode tolerantly by checking this field; bump
// on any breaking change (renames, type changes). Additive changes (new fields)
// do NOT bump — that's the whole point of a versioned, additive contract.
const NodeStatusSchemaVersion = "1.0.0"

// NodeStatus is the aggregated Horus local-node view (ADR-026 frozen contract).
// One read-model, N read-only projections — surfaces never re-aggregate.
type NodeStatus struct {
	// Contract
	SchemaVersion string `json:"schema_version"` // = NodeStatusSchemaVersion at stamp time
	// GeneratedAt is when THIS snapshot was collected — the data's timestamp, not
	// any renderer's serving time. Observers MUST derive age from this field; a
	// missing value ranks worse than a known-old one (fail-closed: refuse, do not
	// assume fresh). Set once in CollectNodeStatus; never mutated by a surface.
	GeneratedAt string `json:"generated_at"` // RFC3339

	// Router
	RouterHome string `json:"router_home"`
	RepoRoot   string `json:"repo_root"`

	// Agents
	RegisteredAgents []string `json:"registered_agents"`
	AgentCount       int      `json:"agent_count"`

	// Queue
	PendingByAgent map[string][]string `json:"pending_by_agent"`
	TotalPending   int                 `json:"total_pending"`
	ActiveTopics   []string            `json:"active_topics"`
	CompletedCount int                 `json:"completed_count"`

	// StrandedInbox: agents with open items but NO armed thread to watch them —
	// work that sits silently until the agent is (re)armed (PR #2 strand
	// visibility). Empty when every backlogged agent is armed.
	StrandedInbox []StrandedAgent `json:"stranded_inbox,omitempty"`

	// Work items
	WorkItemSummary map[string]int `json:"work_item_summary"` // status → count

	// Daemon
	DaemonInstalled  bool                `json:"daemon_installed"`
	DaemonLoaded     bool                `json:"daemon_loaded"`
	DaemonLabel      string              `json:"daemon_label"`
	ConfiguredBinary string              `json:"configured_binary,omitempty"`
	BinaryExists     bool                `json:"binary_exists"`
	BinaryIsGoRun    bool                `json:"binary_is_go_run"`
	LaunchAgents     []LaunchAgentHealth `json:"launch_agents,omitempty"`

	// Timestamps
	LastClaudeRead string `json:"last_claude_read"`
	LastCodexRead  string `json:"last_codex_read"`

	// Agent CLI health
	AgentHealth []AgentHealthCheck `json:"agent_health,omitempty"`
	WakeHealth  []AgentWakeHealth  `json:"wake_health,omitempty"`

	// Dispatch failures (last N from work queue)
	RecentFailures []WorkItemFailure `json:"recent_failures,omitempty"`

	// Live threads (CTR thread registry). Distinct from RegisteredAgents:
	// these are the open conversations/workers that have checked in.
	LiveThreads     []ThreadSummary `json:"live_threads,omitempty"`
	StaleThreads    []ThreadSummary `json:"stale_threads,omitempty"`
	LiveThreadCount int             `json:"live_thread_count"`
}

// ThreadSummary is the operator-visible projection of a Thread record.
type ThreadSummary struct {
	ThreadID      string       `json:"thread_id"`
	AgentID       string       `json:"agent_id"`
	Surface       string       `json:"surface"`
	Status        ThreadStatus `json:"status"`
	Watches       []string     `json:"watches,omitempty"`
	WakeMechanism string       `json:"wake_mechanism,omitempty"`
	CurrentItem   string       `json:"current_item,omitempty"`
	LastError     string       `json:"last_error,omitempty"`
	StartedAt     time.Time    `json:"started_at"`
	LastSeenAt    time.Time    `json:"last_seen_at"`
	AgeSeconds    float64      `json:"age_seconds"`
	IdleSeconds   float64      `json:"idle_seconds"`
	Stale         bool         `json:"stale,omitempty"`
	PID           int          `json:"pid,omitempty"`
	OSState       PIDState     `json:"os_state,omitempty"` // OS-truth liveness of PID
	// Honest liveness (owner priority 2026-06-19, codex-validated). watcher_type is
	// the thread's canonical watcher (WatcherFor().Type). loop_state is the
	// surface-native loop-evidence verdict: "alive"/"dead" for loop-bearing surfaces
	// (loop-monitor/surface-loop/pull-loop — a pgrep `thr-<id>` loop), "na" for
	// surfaces that prove liveness by heartbeat (app-heartbeat Codex, resident
	// native-runloop UI), "unknown" when it can't be probed. armed is the truthful
	// "is the declared wake mechanism currently present?" verdict; armed_reason is
	// the short machine string behind it (loop-alive | app-heartbeat-fresh |
	// resident-runloop-fresh | loop-dead | heartbeat-stale).
	WatcherType string `json:"watcher_type,omitempty"`
	LoopState   string `json:"loop_state,omitempty"` // alive | dead | na | unknown
	Armed       bool   `json:"armed"`
	ArmedReason string `json:"armed_reason,omitempty"`
}

// AgentHealthCheck reports whether a local agent CLI is available and authenticated.
//
// The three auth outcomes are distinct on purpose (ADR-026 honest-auth):
//   - AuthOK          → the probe confirmed the CLI is logged in.
//   - NeedsLogin      → the probe saw an unambiguous "not authenticated" / /login
//     signature — the operator MUST re-auth. This is the ONLY state that alarms
//     and the ONLY state BlockedItems counts against.
//   - Degraded        → the probe could not conclude (timeout on a cold CLI start,
//     env-propagation problem, transient error). "Inconclusive", never "logged
//     out": it does NOT alarm and does NOT count blocked items. A cold Claude CLI
//     that takes >8s to answer used to be mis-reported as logged-out; a degraded
//     probe now says so honestly instead.
//
// AuthOK==false alone is ambiguous (it's true for both NeedsLogin and Degraded) —
// surfaces must branch on NeedsLogin / Degraded, never on AuthOK==false, to decide
// whether to alarm.
type AgentHealthCheck struct {
	AgentType    string `json:"agent_type"` // "claude", "codex"
	CLIFound     bool   `json:"cli_found"`
	CLIPath      string `json:"cli_path,omitempty"`
	AuthOK       bool   `json:"auth_ok"`
	AuthError    string `json:"auth_error,omitempty"`
	NeedsLogin   bool   `json:"needs_login,omitempty"`
	Degraded     bool   `json:"degraded,omitempty"` // probe inconclusive (timeout/transient) — NOT logged out
	BlockedItems int    `json:"blocked_items,omitempty"`
}

// AgentWakeHealth reports the registered wake mechanism readiness per agent.
type AgentWakeHealth struct {
	AgentID   string `json:"agent_id"`
	Mechanism string `json:"mechanism"`
	Adapter   string `json:"adapter,omitempty"` // adapter that fires when Ready (PR#2 wake pass)
	Ready     bool   `json:"ready"`
	Detail    string `json:"detail,omitempty"`
}

// LaunchAgentHealth reports installed macOS helpers that affect Pantheon
// startup, router wakeups, or noisy legacy automation.
type LaunchAgentHealth struct {
	Label        string `json:"label"`
	Role         string `json:"role"`
	PlistPath    string `json:"plist_path"`
	Installed    bool   `json:"installed"`
	Loaded       bool   `json:"loaded"`
	Program      string `json:"program,omitempty"`
	ProgramFound bool   `json:"program_found"`
	Legacy       bool   `json:"legacy,omitempty"`
}

// AuthProbeFunc probes whether an agent CLI is authenticated.
// Returns (authOK, needsLogin, errorDetail).
type AuthProbeFunc func(cliPath, agentType string) (bool, bool, string)

// WorkItemFailure is a summary of a failed dispatch.
type WorkItemFailure struct {
	ItemID   string    `json:"item_id"`
	Agent    string    `json:"agent"`
	Error    string    `json:"error"`
	FailedAt time.Time `json:"failed_at"`
}

// LaunchctlChecker abstracts launchctl probing for testability.
type LaunchctlChecker func(args ...string) error

// DefaultLaunchctlChecker shells the real launchctl. CollectNodeStatus and
// CollectLaunchAgents fall back to this when the caller passes nil — before
// this default existed every production caller (CLI node-status, doctor,
// dashboard, menubar) passed nil, so Loaded/DaemonLoaded were never probed at
// all and the fabric board reported loaded=false for daemons launchctl showed
// running with live PIDs (owner screenshot 2026-07-04).
func DefaultLaunchctlChecker(args ...string) error {
	return runLaunchctl(args...)
}

// claudeAuthProbeTimeout is how long DefaultAuthProbe waits for the Claude CLI to
// answer the auth ping. A cold Claude CLI (first invocation after boot, or after
// the model process was reaped) routinely takes well over the old 8s to print its
// first token, so an 8s ceiling turned a slow-but-authenticated CLI into a false
// "logged out". 30s is generous enough to clear a cold start while still bounding
// a genuinely hung probe. A timeout here is reported as an INCONCLUSIVE probe
// (needsLogin=false), never as a logout.
const claudeAuthProbeTimeout = 30 * time.Second

// authProbeTimeoutEnv lets a harness cap the Claude probe timeout (milliseconds).
// Production leaves it unset → the full 30s cold-start budget. Tests and CI that
// shell `doctor`/`node-status` (which run the real probe against an absent or
// slow `claude`) set it low so a package's test timeout is never consumed by a
// single 30s probe — a raised production budget must not slow the test suite.
const authProbeTimeoutEnv = "SIRSI_AUTH_PROBE_TIMEOUT_MS"

// claudeProbeTimeout returns the Claude auth-probe timeout, honoring an override
// from authProbeTimeoutEnv (ms) when it is a positive integer.
func claudeProbeTimeout() time.Duration {
	if v := os.Getenv(authProbeTimeoutEnv); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return claudeAuthProbeTimeout
}

// DefaultAuthProbe runs a minimal command to test whether an agent CLI is authenticated.
// For Claude: `claude --print "respond with OK"` prints a "not logged in" / /login
// signature if unauthenticated. For Codex: `codex --version` is sufficient (codex
// uses env-based API keys).
//
// Return contract (authOK, needsLogin, detail):
//   - (true,  false, "")     → authenticated.
//   - (false, true,  detail) → the output carried an unambiguous auth-failure
//     signature (isAuthError). Only THIS state means "re-auth required".
//   - (false, false, detail) → the probe could NOT conclude: a timeout on a cold
//     CLI start, a stripped-env credential-resolution problem, or any other
//     non-auth error. Callers treat this as DEGRADED / inconclusive — never as a
//     logout, and it must not count blocked items. Distinguishing these two
//     failure modes is the whole point (the 8s-timeout false-positive fix).
//
// Note: the Claude CLI requires USER and HOME to be present in the environment
// to locate its credential store. A probe that runs with a stripped env (e.g.
// inside a tightened sandbox) reports an auth failure even when valid credentials
// exist on disk; we demote that to the inconclusive state and name the missing
// env in the detail so operators can tell an env problem from a real logout.
func DefaultAuthProbe(cliPath, agentType string) (bool, bool, string) {
	timeout := claudeProbeTimeout()
	if agentType != "claude" {
		timeout = 8 * time.Second // `--version` is instant; no cold-start concern
	}
	return probeAuthWithTimeout(cliPath, agentType, timeout)
}

// probeAuthWithTimeout is the timeout-parameterized core of DefaultAuthProbe.
// Split out so tests can drive the real deadline-exceeded branch with a short,
// fast timeout instead of waiting 30s. Same return contract as DefaultAuthProbe.
func probeAuthWithTimeout(cliPath, agentType string, timeout time.Duration) (bool, bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	switch agentType {
	case "claude":
		cmd = exec.CommandContext(ctx, cliPath, "--print", "respond with OK")
	default:
		cmd = exec.CommandContext(ctx, cliPath, "--version")
	}

	out, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if err != nil {
		// A deadline-exceeded is inconclusive by definition — the CLI never got to
		// tell us whether it was authenticated. Report it as such, never a logout.
		if ctx.Err() == context.DeadlineExceeded {
			return false, false, fmt.Sprintf("auth probe timed out after %s (cold CLI start — inconclusive, not logged out)", timeout)
		}
		// Only an explicit auth-failure signature in the output means "needs login".
		// Any other error (non-zero exit without an auth signature, env problem, a
		// transient failure) is inconclusive — a degraded probe, never a logout.
		if isAuthError(outStr) {
			if agentType == "claude" {
				if missing := missingClaudeEnv(); missing != "" {
					// Env-resolution failure masquerades as an auth error. Demote to
					// inconclusive (needsLogin=false) and name the missing vars.
					return false, false, fmt.Sprintf("%s (missing env: %s — credentials cannot be located, probe inconclusive)", outStr, missing)
				}
			}
			return false, true, outStr
		}
		return false, false, outStr
	}
	return true, false, ""
}

// missingClaudeEnv returns a comma-joined list of env vars the Claude CLI
// requires for credential resolution that are absent in the current process.
// Returns "" when all required vars are present.
func missingClaudeEnv() string {
	var missing []string
	for _, key := range []string{"USER", "HOME"} {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	return strings.Join(missing, ",")
}

// isAuthError checks whether CLI output indicates an authentication failure.
func isAuthError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "not logged in") ||
		strings.Contains(lower, "/login") ||
		strings.Contains(lower, "please log in") ||
		strings.Contains(lower, "authentication required") ||
		strings.Contains(lower, "unauthorized")
}

// CollectNodeStatus gathers the Horus local-node view from all sources.
// Pass nil for authProbe to use DefaultAuthProbe.
func CollectNodeStatus(repoRoot string, launchctlCheck LaunchctlChecker, authProbe ...AuthProbeFunc) (*NodeStatus, error) {
	if launchctlCheck == nil {
		launchctlCheck = DefaultLaunchctlChecker
	}
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

	ns := &NodeStatus{
		SchemaVersion:   NodeStatusSchemaVersion,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		RouterHome:      routerRoot,
		RepoRoot:        repoRoot,
		PendingByAgent:  make(map[string][]string),
		WorkItemSummary: make(map[string]int),
	}

	// --- Registry ---
	reg, err := LoadRegistry(routerRoot)
	if err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}
	for id := range reg.Agents {
		ns.RegisteredAgents = append(ns.RegisteredAgents, id)
	}
	sort.Strings(ns.RegisteredAgents)
	ns.AgentCount = len(ns.RegisteredAgents)
	for _, id := range ns.RegisteredAgents {
		// node-status now surfaces the HONEST wake readiness — the same
		// ProbeWakeReadiness view the wake pass + doctor act on (PR #89 follow-up,
		// claude-home-acked). The old permissive view treated a bare Command array
		// as "cli-spawn ready" and an unwired mcp-notification as ready; the honest
		// view reports a legacy-command agent (no explicit wake.mechanism) and an
		// unwired mcp adapter as NOT ready, so the surface and the acted-on
		// readiness can never disagree.
		ns.WakeHealth = append(ns.WakeHealth, ProbeWakeReadiness(reg.Agents[id]))
	}

	// --- Router state ---
	r, err := New(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("open router: %w", err)
	}
	state, err := r.ReadState()
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	state.NormalizePending()

	ns.ActiveTopics = state.ActiveTopics
	ns.CompletedCount = len(state.CompletedTopics)
	ns.LastClaudeRead = state.LastClaudeRead
	ns.LastCodexRead = state.LastCodexRead

	if items, listErr := inboxUnion(routerRoot, ""); listErr == nil && len(items) > 0 {
		for _, item := range items {
			ns.PendingByAgent[item.To] = append(ns.PendingByAgent[item.To], item.ID)
			ns.TotalPending++
		}
	} else {
		// Legacy fallback for pre-ADR-024 routers that have not migrated to
		// items/*.md yet. The active CLI queue is items/; state.json pending is
		// no longer authoritative once item files exist.
		for agent, ids := range state.Pending {
			if len(ids) > 0 {
				ns.PendingByAgent[agent] = ids
				ns.TotalPending += len(ids)
			}
		}
	}

	// --- Work queue ---
	wq, err := LoadWorkQueue(routerRoot)
	if err == nil {
		for _, item := range wq.Items {
			ns.WorkItemSummary[string(item.Status)]++
		}
		// Collect recent terminal problems.
		for _, item := range wq.Items {
			if item.Status == StatusFailed || item.Status == StatusBlocked {
				failure := WorkItemFailure{
					ItemID:   item.ID,
					Agent:    item.TargetAgentID,
					Error:    item.LastError,
					FailedAt: item.CompletedAt,
				}
				if len(item.Attempts) > 0 {
					last := item.Attempts[len(item.Attempts)-1]
					failure.Error = last.Error
					failure.FailedAt = last.At
				}
				if failure.Error == "" {
					failure.Error = string(item.Status)
				}
				ns.RecentFailures = append(ns.RecentFailures, failure)
			}
		}
		// Sort failures newest first, limit to 5
		sort.Slice(ns.RecentFailures, func(i, j int) bool {
			return ns.RecentFailures[i].FailedAt.After(ns.RecentFailures[j].FailedAt)
		})
		if len(ns.RecentFailures) > 5 {
			ns.RecentFailures = ns.RecentFailures[:5]
		}
	}

	// --- Live thread registry ---
	// Reap dead/defunct-PID threads against OS truth before reading, so Horus
	// and the menubar never present a gone or zombie PID as a live agent (the
	// CTR false-active bug). Scoped to this machine by stable id; foreign tables
	// are unobservable (see ReapDeadThreads / SameMachine).
	_, _ = ReapDeadThreads(routerRoot)
	// ADR-024: sweep superseded strays too, so Horus/menubar node-status never
	// counts a surface's duplicate suspends/ghosts once a live watcher holds it.
	_, _ = ReapStrayThreads(routerRoot)
	if treg, loadErr := LoadThreadRegistry(routerRoot); loadErr == nil {
		now := time.Now().UTC()
		for _, thr := range treg.SortedThreads() {
			if thr.Status.IsTerminal() {
				continue
			}
			if thr.Status == ThreadStatusSuspended {
				continue
			}
			sum := ThreadSummary{
				ThreadID:      thr.ThreadID,
				AgentID:       thr.AgentID,
				Surface:       thr.Surface,
				Status:        thr.Status,
				Watches:       append([]string(nil), thr.Watches...),
				WakeMechanism: thr.WakeMechanism,
				CurrentItem:   thr.CurrentItem,
				LastError:     thr.LastError,
				StartedAt:     thr.StartedAt,
				LastSeenAt:    thr.LastSeenAt,
				AgeSeconds:    now.Sub(thr.StartedAt).Seconds(),
				IdleSeconds:   now.Sub(thr.LastSeenAt).Seconds(),
				Stale:         thr.IsStale(now, DefaultThreadStaleAfter),
				PID:           thr.PID,
				OSState:       PIDStateOf(thr.PID, thr.StartTime),
			}
			// Honest liveness, classified by watcher_type (codex-home SME verdict on #79):
			//  - loop-monitor (Claude) ONLY requires pgrep `thr-<id>` loop evidence —
			//    its /loop is thr-id-keyed AND its heartbeat is harness-gated, so it can
			//    be heartbeat-FRESH while the loop is DEAD (armed:false, loop_state:"dead"
			//    — the owner's "claims live but is idle"). NOT extended to surface-loop/
			//    pull-loop: their heartbeat is loop-driven (dead loop → stale → caught by
			//    !stale) and not contractually thr-id-pgrep-able, so requiring it would
			//    false-negative healthy workers.
			//  - app-heartbeat (Codex), native-runloop (resident UI), surface-loop,
			//    pull-loop: loop_state "na"; heartbeat freshness is the armed proof.
			sum.WatcherType = WatcherFor(thr.Surface, thr.AgentID, thr.ThreadID).Type
			switch {
			case requiresThreadIDLoop(sum.WatcherType):
				switch {
				case thr.ThreadID == "":
					sum.LoopState, sum.Armed, sum.ArmedReason = "unknown", false, "heartbeat-stale"
				case WatcherAlive(thr.ThreadID):
					sum.LoopState, sum.Armed, sum.ArmedReason = "alive", true, "loop-alive"
				default:
					sum.LoopState, sum.Armed, sum.ArmedReason = "dead", false, "loop-dead"
				}
			case sum.Stale:
				// Any non-loop-monitor surface gone stale is unarmed regardless of kind.
				sum.LoopState, sum.Armed, sum.ArmedReason = "na", false, "heartbeat-stale"
			case sum.WatcherType == watcherTypeAppHeartbeat:
				sum.LoopState, sum.Armed, sum.ArmedReason = "na", true, "app-heartbeat-fresh"
			case sum.WatcherType == watcherTypeNativeRunloop:
				sum.LoopState, sum.Armed, sum.ArmedReason = "na", true, "resident-runloop-fresh"
			default: // surface-loop / pull-loop / unknown — loop-driven heartbeat is the proof
				sum.LoopState, sum.Armed, sum.ArmedReason = "na", true, "heartbeat-fresh"
			}
			if sum.Stale {
				ns.StaleThreads = append(ns.StaleThreads, sum)
			} else {
				ns.LiveThreads = append(ns.LiveThreads, sum)
			}
		}
		ns.LiveThreadCount = len(ns.LiveThreads)
	}

	// --- LaunchAgent / helper health ---
	exe, err := os.Executable()
	if err == nil {
		exe, _ = ResolveStableBinary(repoRoot, exe)
	}
	if exe == "" {
		exe = "sirsi" // fallback for display
	}
	opts := DefaultServiceOptions(repoRoot, exe)
	ns.DaemonLabel = opts.Label
	ns.LaunchAgents = CollectLaunchAgents(repoRoot, launchctlCheck)

	if _, err := os.Stat(opts.PlistPath); err == nil {
		ns.DaemonInstalled = true
		if program, err := LaunchAgentProgram(opts.PlistPath); err == nil {
			ns.ConfiguredBinary = program
			if _, err := os.Stat(program); err == nil {
				ns.BinaryExists = true
			}
			ns.BinaryIsGoRun = IsGoRunBinary(program)
		}
	}

	// `launchctl list <label>` targets the caller's own session domain and
	// exits 0 iff the job is loaded (113 when it is not). The previous probe,
	// `launchctl print <label>`, requires a full domain target (gui/<uid>/…)
	// and without one ALWAYS exits 64 — so even an injected checker could
	// never report a loaded daemon.
	if err := launchctlCheck("list", ns.DaemonLabel); err == nil {
		ns.DaemonLoaded = true
	}

	// --- Agent CLI health ---
	probe := DefaultAuthProbe
	if len(authProbe) > 0 && authProbe[0] != nil {
		probe = authProbe[0]
	}

	for _, agentType := range []string{"claude", "codex"} {
		check := AgentHealthCheck{AgentType: agentType}
		path, err := exec.LookPath(agentType)
		if err != nil {
			check.AuthError = fmt.Sprintf("%s CLI not found in PATH", agentType)
		} else {
			check.CLIFound = true
			check.CLIPath = path
			authOK, needsLogin, detail := probe(path, agentType)
			check.AuthOK = authOK
			check.NeedsLogin = needsLogin
			// Degraded = the probe ran but could not conclude (timeout / transient /
			// env problem). NOT logged out — inconclusive. A surface must not alarm
			// on this and blocked-item counting must not count against it.
			check.Degraded = !authOK && !needsLogin
			if !authOK {
				if needsLogin {
					check.AuthError = fmt.Sprintf("not authenticated — run '%s' then /login", agentType)
				} else if detail != "" {
					check.AuthError = fmt.Sprintf("auth probe inconclusive: %s", detail)
				}
			}
		}

		// Count how many pending items are blocked by this agent type. Only a REAL
		// logout (NeedsLogin) blocks work — an inconclusive/degraded probe must not
		// mark otherwise-deliverable items as blocked (the 8s-timeout false-positive
		// used to strand every pending item behind a cold CLI start).
		if check.NeedsLogin {
			for agent, ids := range ns.PendingByAgent {
				if strings.Contains(agent, agentType) && len(ids) > 0 {
					check.BlockedItems += len(ids)
				}
			}
		}

		ns.AgentHealth = append(ns.AgentHealth, check)
	}

	// Strand visibility (PR #2): backlogged agents with no live consumer. A
	// consumer is an armed registry thread OR the agent's launchd wake job — the
	// latter is the durable post-cutover consumer and lives outside the registry,
	// so it must be credited here or every wake-loop agent false-strands (#298).
	strandAgents := make([]string, 0, len(ns.PendingByAgent))
	for a := range ns.PendingByAgent {
		strandAgents = append(strandAgents, a)
	}
	ns.StrandedInbox = computeStranded(routerRoot, ns.PendingByAgent, liveWakeAgents(strandAgents, launchctlCheck), noWakeAgents(reg))

	return ns, nil
}

// CollectLaunchAgents inventories the known macOS LaunchAgents Pantheon may
// install or inherit from older router automation.
//
// Single-backstop shape (backlog ruling 20260629-230327): the resident Horus
// supervisor is THE router automation — its duties (dispatch pump, sweep,
// registry police) run inside `sirsi horus supervise`. The three per-duty
// LaunchAgents that used to carry them are LEGACY: still inventoried so an
// operator can see stragglers `sirsi router install-daemons` will migrate
// away, but marked legacy so no surface alarms on their (expected) absence.
func CollectLaunchAgents(repoRoot string, launchctlCheck LaunchctlChecker) []LaunchAgentHealth {
	if launchctlCheck == nil {
		launchctlCheck = DefaultLaunchctlChecker
	}
	legacy := DefaultServiceOptions(repoRoot, "sirsi")
	home, _ := os.UserHomeDir()
	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	specs := []LaunchAgentHealth{
		{Label: "ai.sirsi.pantheon", Role: "menubar"},
		{Label: "ai.sirsi.horus.agent-router", Role: "router-supervisor"},
		{Label: "com.sirsi.idea-router", Role: "router-watchpaths", Legacy: true},
		{Label: "com.sirsi.idea-router-sweep", Role: "router-sweep", Legacy: true},
		{Label: "ai.sirsi.registry-police", Role: "registry-police", Legacy: true},
		{Label: legacy.Label, Role: "legacy-router-daemon", Legacy: true},
	}
	for i := range specs {
		if specs[i].Label == "" {
			continue
		}
		if specs[i].PlistPath == "" {
			specs[i].PlistPath = filepath.Join(agentDir, specs[i].Label+".plist")
		}
		if _, err := os.Stat(specs[i].PlistPath); err != nil {
			continue
		}
		specs[i].Installed = true
		if program, err := LaunchAgentProgram(specs[i].PlistPath); err == nil {
			specs[i].Program = program
			if _, statErr := os.Stat(program); statErr == nil {
				specs[i].ProgramFound = true
			}
		}
		// `list <label>` is the domain-correct loaded probe; `print <label>`
		// (no domain target) always exits 64 — see CollectNodeStatus.
		if err := launchctlCheck("list", specs[i].Label); err == nil {
			specs[i].Loaded = true
		}
	}
	return specs
}
