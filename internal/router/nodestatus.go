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
}

// AgentHealthCheck reports whether a local agent CLI is available and authenticated.
type AgentHealthCheck struct {
	AgentType    string `json:"agent_type"` // "claude", "codex"
	CLIFound     bool   `json:"cli_found"`
	CLIPath      string `json:"cli_path,omitempty"`
	AuthOK       bool   `json:"auth_ok"`
	AuthError    string `json:"auth_error,omitempty"`
	NeedsLogin   bool   `json:"needs_login,omitempty"`
	BlockedItems int    `json:"blocked_items,omitempty"`
}

// AgentWakeHealth reports the registered wake mechanism readiness per agent.
type AgentWakeHealth struct {
	AgentID   string `json:"agent_id"`
	Mechanism string `json:"mechanism"`
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

// DefaultAuthProbe runs a minimal command to test whether an agent CLI is authenticated.
// For Claude: `claude --print "ping"` fails with "Not logged in" if unauthenticated.
// For Codex: `codex --version` is sufficient (codex uses env-based API keys).
//
// Note: the Claude CLI requires USER and HOME to be present in the environment
// to locate its credential store. A probe that runs with a stripped env (e.g.
// inside a tightened sandbox) will report "Not logged in" even when valid
// credentials exist on disk. We surface that distinction in the returned
// detail so operators can tell an unauthenticated CLI from an env-propagation
// problem.
func DefaultAuthProbe(cliPath, agentType string) (bool, bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
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
		needsLogin := isAuthError(outStr)
		if needsLogin && agentType == "claude" {
			if missing := missingClaudeEnv(); missing != "" {
				return false, false, fmt.Sprintf("%s (missing env: %s — credentials cannot be located)", outStr, missing)
			}
		}
		return false, needsLogin, outStr
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
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

	ns := &NodeStatus{
		SchemaVersion:   NodeStatusSchemaVersion,
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
		cfg := reg.Agents[id]
		check := AgentWakeHealth{AgentID: id, Mechanism: cfg.WakeMechanism(), Ready: true}
		if vErr := cfg.Validate(); vErr != nil {
			check.Ready = false
			check.Detail = vErr.Error()
		} else {
			switch cfg.WakeMechanism() {
			case WakeCLISpawn:
				if len(cfg.Command) > 0 {
					if path, lookErr := exec.LookPath(cfg.Command[0]); lookErr == nil {
						check.Detail = path
					} else {
						check.Ready = false
						check.Detail = fmt.Sprintf("%s not found in PATH", cfg.Command[0])
					}
				}
			case WakeAPICall:
				check.Detail = cfg.Wake.Endpoint
			case WakeMCPNotification:
				check.Detail = cfg.Wake.MCPServer
			}
		}
		ns.WakeHealth = append(ns.WakeHealth, check)
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

	for agent, ids := range state.Pending {
		if len(ids) > 0 {
			ns.PendingByAgent[agent] = ids
			ns.TotalPending += len(ids)
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
	// CTR false-active bug). Scoped to this host; remote tables are unobservable.
	host, _ := os.Hostname()
	_, _ = ReapDeadThreads(routerRoot, host)
	if treg, loadErr := LoadThreadRegistry(routerRoot); loadErr == nil {
		now := time.Now().UTC()
		for _, thr := range treg.SortedThreads() {
			if thr.Status.IsTerminal() {
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

	if launchctlCheck != nil {
		userDomain := "" // caller provides the full argument
		// We check by calling print on the label
		if err := launchctlCheck("print", userDomain+ns.DaemonLabel); err == nil {
			ns.DaemonLoaded = true
		}
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
			if !authOK {
				if needsLogin {
					check.AuthError = fmt.Sprintf("not authenticated — run '%s' then /login", agentType)
				} else if detail != "" {
					check.AuthError = fmt.Sprintf("CLI check failed: %s", detail)
				}
			}
		}

		// Count how many pending items are blocked by this agent type
		for agent, ids := range ns.PendingByAgent {
			if strings.Contains(agent, agentType) && len(ids) > 0 && !check.AuthOK {
				check.BlockedItems += len(ids)
			}
		}

		ns.AgentHealth = append(ns.AgentHealth, check)
	}

	return ns, nil
}

// CollectLaunchAgents inventories the known macOS LaunchAgents Pantheon may
// install or inherit from older router automation.
func CollectLaunchAgents(repoRoot string, launchctlCheck LaunchctlChecker) []LaunchAgentHealth {
	legacy := DefaultServiceOptions(repoRoot, "sirsi")
	home, _ := os.UserHomeDir()
	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	specs := []LaunchAgentHealth{
		{Label: "ai.sirsi.pantheon", Role: "menubar"},
		{Label: "com.sirsi.idea-router", Role: "router-watchpaths"},
		{Label: "com.sirsi.idea-router-sweep", Role: "router-sweep"},
		{Label: "ai.sirsi.registry-police", Role: "registry-police"},
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
		if launchctlCheck != nil {
			if err := launchctlCheck("print", specs[i].Label); err == nil {
				specs[i].Loaded = true
			}
		}
	}
	return specs
}
