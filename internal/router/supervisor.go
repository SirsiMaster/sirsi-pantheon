package router

import (
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
	PendingItems  []string         `json:"pending_items,omitempty"`
	PendingCount  int              `json:"pending_count"`
	LiveThreads   []string         `json:"live_threads,omitempty"`
	StaleThreads  []string         `json:"stale_threads,omitempty"`
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
		inbox, ierr := work.ListInbox(routerRoot, id)
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
			status.PendingItems = append(status.PendingItems, item.ID)
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
		case !ready && status.PendingCount > 0:
			status.Status = SupervisorStatusBlocked
		case len(status.StaleThreads) > 0 && len(status.LiveThreads) == 0:
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

	return &SuperviseReport{
		ThreadID:         thread.ThreadID,
		RouterRoot:       routerRoot,
		RepoRoot:         repoRoot,
		Status:           thread.Status,
		PendingTotal:     pendingTotal,
		LiveThreadCount:  liveCount,
		StaleThreadCount: staleCount,
		Agents:           agents,
		Duties:           duties,
	}, nil
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
	default:
		if strings.TrimSpace(cfg.WakeMechanism()) == "" {
			return false, "no wake mechanism configured"
		}
		return false, fmt.Sprintf("unsupported wake mechanism %q", cfg.WakeMechanism())
	}
}
