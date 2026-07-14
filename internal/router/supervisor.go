package router

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

const (
	SupervisorAgentID    = "horus-supervisor"
	SupervisorSurface    = "worker"
	SupervisorWorkstream = "ra-horus-router-hypervisor-canon"
)

// SupervisorSchemaVersion is the frozen contract version for the SuperviseReport
// ("board") JSON shape. Every Fabric renderer — CLI, menubar, TUI, Swift app,
// and the local dashboard HTML — decodes tolerantly against this field so one
// producer feeds every viewport (ADR-038 P4). Additive fields do NOT bump it;
// a rename or type change does.
//
//	1.0.0 — pending_items was a flat []string of item ids
//	1.1.0 — pending_items is []PendingItem (drillable record: title, from,
//	        type, age, summary) + per-agent oldest_pending_age_seconds and the
//	        top-level schema_version / generated_at stamps for column sorting
//	        and drill-down without re-reading item files.
//
// The 1.0.0→1.1.0 pending_items type change is a breaking shape change taken as
// a MINOR bump by conscious decision: this is a pre-1.0 internal contract, every
// renderer is in-repo and updated in lockstep, they all decode tolerantly, and
// there is no external --json consumer. Renderers gate on this field, so the
// bump is what a reader keys on regardless of the SemVer class.
const SupervisorSchemaVersion = "1.1.0"

// BoardFileName is the canonical on-disk board the supervisor writes on every
// pass, under the router root. Thin renderers read this one file instead of
// re-aggregating or shelling out to the CLI — the "one producer, N read-only
// projections" contract (ADR-026, ADR-038 P4). It is a runtime file
// (gitignored), never committed.
const BoardFileName = "board.json"

type SupervisorStatus string

const (
	SupervisorStatusWakeable SupervisorStatus = "wakeable"
	SupervisorStatusStale    SupervisorStatus = "stale"
	SupervisorStatusBlocked  SupervisorStatus = "blocked"
	SupervisorStatusManual   SupervisorStatus = "manual"
)

type SuperviseOptions struct {
	RepoRoot string
	AgentID  string
	ThreadID string
	PID      int
	Now      time.Time
}

type SuperviseReport struct {
	// Contract stamps (schema 1.1.0). Renderers gate on SchemaVersion.
	SchemaVersion string `json:"schema_version"`
	GeneratedAt   string `json:"generated_at"` // RFC3339 UTC of this pass

	ThreadID         string               `json:"thread_id"`
	RouterRoot       string               `json:"router_root"`
	RepoRoot         string               `json:"repo_root"`
	Status           ThreadStatus         `json:"status"`
	PendingTotal     int                  `json:"pending_total"`
	LiveThreadCount  int                  `json:"live_thread_count"`
	StaleThreadCount int                  `json:"stale_thread_count"`
	Agents           []AgentSurfaceStatus `json:"agents"`
	// Duties are the folded single-backstop passes (dispatch pump, sweep,
	// registry police) this tick ran, skipped, or failed — see supervisorduties.go.
	Duties []DutyResult `json:"duties,omitempty"`
}

type AgentSurfaceStatus struct {
	AgentID       string           `json:"agent_id"`
	Cwd           string           `json:"cwd,omitempty"`
	WakeMechanism string           `json:"wake_mechanism,omitempty"`
	Status        SupervisorStatus `json:"status"`
	Detail        string           `json:"detail,omitempty"`
	PendingItems  []PendingItem    `json:"pending_items,omitempty"`
	PendingCount  int              `json:"pending_count"`
	// OldestPendingAgeSeconds is the age of the oldest open inbox item for this
	// agent (0 when none) — a scalar the renderer sorts a column on without
	// walking pending_items.
	OldestPendingAgeSeconds float64  `json:"oldest_pending_age_seconds"`
	LiveThreads             []string `json:"live_threads,omitempty"`
	StaleThreads            []string `json:"stale_threads,omitempty"`
}

// PendingItem is the drillable projection of one open inbox item: everything a
// Fabric renderer needs to render a row, sort it, and expand its detail —
// without re-reading the item markdown. Populated from work.Item (ADR-024), so
// it carries no field the inbox does not already own.
type PendingItem struct {
	ID         string  `json:"id"`
	From       string  `json:"from,omitempty"`
	Title      string  `json:"title,omitempty"`
	Type       string  `json:"type,omitempty"`    // proposal | review | decision | ""
	Opened     string  `json:"opened,omitempty"`  // RFC3339
	AgeSeconds float64 `json:"age_seconds"`       // now − opened (0 if unparseable)
	Summary    string  `json:"summary,omitempty"` // first non-empty instruction line, trimmed
	// Wake-delivery truth (PR#2 wake-or-declare-unavailable): the drill-down that
	// tells an operator whether a pending item is actually reachable or stranded.
	WakeStatus  string `json:"wake_status,omitempty"`  // pending|wake-attempted|wake-unavailable|armed
	WakeAdapter string `json:"wake_adapter,omitempty"` // adapter that fired, when one did
	WakeError   string `json:"wake_error,omitempty"`   // why it is wake-unavailable, when it is
}

func SuperviseOnce(opts SuperviseOptions) (*SuperviseReport, error) {
	repoRoot := opts.RepoRoot
	if repoRoot == "" {
		found, err := FindRepoRoot()
		if err != nil {
			return nil, fmt.Errorf("find repo root: %w", err)
		}
		repoRoot = found
	}
	if abs, err := filepath.Abs(repoRoot); err == nil {
		repoRoot = abs
	}
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

	reg, err := LoadRegistry(routerRoot)
	if err != nil {
		return nil, fmt.Errorf("load agents: %w", err)
	}

	agentIDs := make([]string, 0, len(reg.Agents))
	for id := range reg.Agents {
		agentIDs = append(agentIDs, id)
	}
	sort.Strings(agentIDs)

	threadReg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil, fmt.Errorf("load threads: %w", err)
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	agents := make([]AgentSurfaceStatus, 0, len(agentIDs))
	pendingTotal := 0
	for _, id := range agentIDs {
		cfg := reg.Agents[id]
		inbox, ierr := inboxUnion(routerRoot, id)
		if ierr != nil {
			return nil, fmt.Errorf("list inbox for %s: %w", id, ierr)
		}

		status := AgentSurfaceStatus{
			AgentID:       id,
			Cwd:           cfg.Cwd,
			WakeMechanism: cfg.WakeMechanism(),
			Status:        SupervisorStatusWakeable,
		}
		for _, item := range inbox {
			pi := toPendingItem(item, now)
			status.PendingItems = append(status.PendingItems, pi)
			if pi.AgeSeconds > status.OldestPendingAgeSeconds {
				status.OldestPendingAgeSeconds = pi.AgeSeconds
			}
		}
		status.PendingCount = len(status.PendingItems)
		pendingTotal += status.PendingCount

		for _, thread := range threadReg.SortedThreads() {
			if thread.AgentID != id || thread.Status.IsTerminal() {
				continue
			}
			if thread.IsStale(now, DefaultThreadStaleAfter) {
				status.StaleThreads = append(status.StaleThreads, thread.ThreadID)
				continue
			}
			status.LiveThreads = append(status.LiveThreads, thread.ThreadID)
		}

		ready, detail := agentWakeReady(cfg)
		status.Detail = detail
		switch {
		case len(status.LiveThreads) > 0:
			// A live session/thread is already consuming this agent's inbox, so it
			// is NOT blocked even without an armed background wake path (the
			// leak-guard case: we deliberately do not install a wake LaunchAgent on
			// top of a live session). Healthy.
			status.Status = SupervisorStatusWakeable
		case !ready && status.PendingCount > 0:
			status.Status = SupervisorStatusBlocked
		case len(status.StaleThreads) > 0:
			status.Status = SupervisorStatusStale
		case ready:
			status.Status = SupervisorStatusWakeable
		default:
			status.Status = SupervisorStatusManual
		}
		agents = append(agents, status)
	}

	supervisorID := opts.AgentID
	if supervisorID == "" {
		supervisorID = SupervisorAgentID
	}
	pid := opts.PID
	if pid <= 0 {
		pid = os.Getpid()
	}
	host, _ := os.Hostname()
	thread, err := RegisterThread(routerRoot, &Thread{
		ThreadID:      opts.ThreadID,
		AgentID:       supervisorID,
		Surface:       SupervisorSurface,
		Repo:          repoRoot,
		Workstream:    SupervisorWorkstream,
		Status:        ThreadStatusActive,
		Watches:       agentIDs,
		WakeMechanism: "resident-loop",
		PID:           pid,
		Host:          host,
	})
	if err != nil {
		return nil, fmt.Errorf("register supervisor thread: %w", err)
	}
	status := ThreadStatusIdle
	if pendingTotal > 0 {
		status = ThreadStatusActive
	}
	thread, err = Heartbeat(routerRoot, thread.ThreadID, HeartbeatUpdate{Status: status})
	if err != nil {
		return nil, fmt.Errorf("heartbeat supervisor thread: %w", err)
	}

	// Single-backstop duty passes (backlog ruling 20260629-230327): the work the
	// three legacy router LaunchAgents used to do runs HERE, cadence-gated,
	// bounded, and error-isolated — a failing duty is reported, never fatal.
	duties := runSupervisorDuties(routerRoot, repoRoot, now)

	liveCount, staleCount := 0, 0
	if refreshed, loadErr := LoadThreadRegistry(routerRoot); loadErr == nil {
		for _, thread := range refreshed.SortedThreads() {
			if thread.Status.IsTerminal() {
				continue
			}
			if thread.IsStale(now, DefaultThreadStaleAfter) {
				staleCount++
			} else {
				liveCount++
			}
		}
	}

	report := &SuperviseReport{
		SchemaVersion:    SupervisorSchemaVersion,
		GeneratedAt:      now.UTC().Format(time.RFC3339),
		ThreadID:         thread.ThreadID,
		RouterRoot:       routerRoot,
		RepoRoot:         repoRoot,
		Status:           thread.Status,
		PendingTotal:     pendingTotal,
		LiveThreadCount:  liveCount,
		StaleThreadCount: staleCount,
		Agents:           agents,
		Duties:           duties,
	}

	// Persist the board for thin renderers (menubar, TUI, Swift app, dashboard)
	// to read one file instead of re-aggregating. Best-effort: a write failure
	// never fails the pass — the returned report is still authoritative — but it
	// is surfaced to stderr so a persistently unwritable board (e.g. permissions)
	// is never silently invisible to an operator watching the supervisor.
	if err := WriteBoard(routerRoot, report); err != nil {
		fmt.Fprintf(os.Stderr, "supervise: board not persisted: %v\n", err)
	}

	return report, nil
}

// toPendingItem projects an open inbox item into its drillable board record.
// Age is now − opened; an unparseable/empty Opened yields age 0 rather than a
// negative or garbage value, so a renderer sorting by age never sees noise.
func toPendingItem(item work.Item, now time.Time) PendingItem {
	pi := PendingItem{
		ID:          item.ID,
		From:        item.From,
		Title:       item.Title,
		Type:        item.Type,
		Opened:      item.Opened,
		Summary:     firstLine(item.Instructions),
		WakeStatus:  item.WakeStatus,
		WakeAdapter: item.WakeAdapter,
		WakeError:   item.WakeError,
	}
	if item.Opened != "" {
		if opened, err := time.Parse(time.RFC3339, item.Opened); err == nil {
			if age := now.Sub(opened).Seconds(); age > 0 {
				pi.AgeSeconds = age
			}
		}
	}
	return pi
}

// firstLine returns the first non-empty, trimmed line of s — the item's summary
// for a collapsed board row. Empty when s has no printable content.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// WriteBoard atomically writes the report as board.json under the router root.
// Atomic (temp + rename) so a renderer never reads a half-written file. The
// board is a gitignored runtime projection, regenerated every supervisor pass.
func WriteBoard(routerRoot string, report *SuperviseReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal board: %w", err)
	}
	dst := filepath.Join(routerRoot, BoardFileName)
	tmp, err := os.CreateTemp(routerRoot, ".board-*.json")
	if err != nil {
		return fmt.Errorf("create board temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write board temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close board temp: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("commit board: %w", err)
	}
	return nil
}

func agentWakeReady(cfg AgentConfig) (bool, string) {
	if cfg.Cwd != "" {
		if _, err := os.Stat(cfg.Cwd); err != nil {
			return false, fmt.Sprintf("cwd unavailable: %s", cfg.Cwd)
		}
	}
	if err := cfg.Validate(); err != nil {
		return false, err.Error()
	}
	switch cfg.WakeMechanism() {
	case WakeCLISpawn:
		if len(cfg.Command) == 0 {
			return false, "missing command"
		}
		path, err := exec.LookPath(cfg.Command[0])
		if err != nil {
			return false, fmt.Sprintf("%s not found in PATH", cfg.Command[0])
		}
		return true, path
	case WakeAPICall:
		return true, cfg.Wake.Endpoint
	case WakeMCPNotification:
		return true, cfg.Wake.MCPServer
	case WakeLaunchAgent:
		// The per-agent pull-loop LaunchAgent (installed by `sirsi router
		// wake-install`). Recognized as a real wake mechanism — no longer the
		// catch-all "unsupported" — but READY only when the plist is actually
		// installed, exactly matching the canonical ProbeWakeReadiness. If it is
		// configured-but-not-installed, the agent genuinely cannot be woken yet,
		// so report NOT ready with the actionable fix: an agent sitting on pending
		// items then surfaces as Blocked (current + fixable), and the board never
		// disagrees with the wake pass (which files the same item as unavailable).
		label := WakeLaunchAgentLabel(cfg.ID)
		if WakeLaunchAgentInstalled(cfg.ID) {
			return true, label
		}
		return false, label + " not installed — run `sirsi router wake-install`"
	default:
		if strings.TrimSpace(cfg.WakeMechanism()) == "" {
			return false, "no wake mechanism configured"
		}
		return false, fmt.Sprintf("unsupported wake mechanism %q", cfg.WakeMechanism())
	}
}
