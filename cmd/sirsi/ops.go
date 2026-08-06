package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

var opsCmd = &cobra.Command{
	Use:   "ops",
	Short: "Show the operator surface for agent work: router, review, memory, watch, reap, insight",
	Long: `Agent-Operations Parity means every routine operation an AI agent can
perform on this workstation has an operator-facing path in Sirsi. This command
is the map: deterministic command first, local-AI augmentation where available,
and the menubar surface that should expose the same work.

  sirsi ops          Human-readable capability map + live router summary
  sirsi ops --json   Machine-readable map for menus, tests, and docs`,
	RunE: runOps,
}

type opsCapability struct {
	ID                   string `json:"id"`
	Operation            string `json:"operation"`
	DeterministicCommand string `json:"deterministic_command"`
	LocalAICommand       string `json:"local_ai_command,omitempty"`
	MenubarSurface       string `json:"menubar_surface"`
	Status               string `json:"status"`
}

type opsReport struct {
	Capabilities []opsCapability `json:"capabilities"`
	Live         *opsLiveSummary `json:"live,omitempty"`
}

type opsLiveSummary struct {
	Repo          string `json:"repo"`
	PendingTotal  int    `json:"pending_total"`
	AgentsPending int    `json:"agents_pending"`
	LiveThreads   int    `json:"live_threads"`
	StaleThreads  int    `json:"stale_threads"`
	Stranded      int    `json:"stranded"`
}

func runOps(_ *cobra.Command, _ []string) error {
	report := opsReport{Capabilities: opsCapabilities()}

	if repoRoot, err := router.FindRepoRoot(); err == nil {
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		if live, liveErr := collectOpsLiveSummary(repoRoot, routerRoot); liveErr == nil {
			report.Live = live
		}
	}

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	renderOps(report)
	return nil
}

func opsCapabilities() []opsCapability {
	return []opsCapability{
		{
			ID:                   "wake",
			Operation:            "Check router, surface pending work, and wake available agents",
			DeterministicCommand: "sirsi ctr",
			LocalAICommand:       "sirsi ctr --reconcile",
			MenubarSurface:       "Horus - Ops",
			Status:               "ready",
		},
		{
			ID:                   "queue",
			Operation:            "Inspect queue depth, stale work, and open items",
			DeterministicCommand: "sirsi router status; sirsi router pull <agent>; sirsi router show <id>",
			LocalAICommand:       "sirsi router plan",
			MenubarSurface:       "Horus - Ops",
			Status:               "ready",
		},
		{
			ID:                   "respond",
			Operation:            "Send or close router work with an auditable result",
			DeterministicCommand: "sirsi router send ...; sirsi router close <id> --result @file",
			MenubarSurface:       "Horus - Ops",
			Status:               "ready",
		},
		{
			ID:                   "review",
			Operation:            "Review code or documents through bounded source context",
			DeterministicCommand: "sirsi horus outline <file>; sirsi horus context <symbol>",
			LocalAICommand:       "sirsi gemma --mode review <prompt>",
			MenubarSurface:       "Insight",
			Status:               "ready",
		},
		{
			ID:                   "ask",
			Operation:            "Ask the local Sirsi model for bounded advice without cloud calls",
			DeterministicCommand: "sirsi gemma status",
			LocalAICommand:       "sirsi gemma <prompt>",
			MenubarSurface:       "Ask Sirsi",
			Status:               "ready",
		},
		{
			ID:                   "memory",
			Operation:            "Read, sync, and preserve project memory",
			DeterministicCommand: "sirsi thoth status; sirsi thoth sync; sirsi continue",
			LocalAICommand:       "sirsi gemma --mode summarize <prompt>",
			MenubarSurface:       "Thoth - Memory",
			Status:               "ready",
		},
		{
			ID:                   "watch",
			Operation:            "Register threads, discover sessions, and keep inbox watchers visible",
			DeterministicCommand: "sirsi thread register ...; sirsi thread discover; sirsi horus supervise --once",
			MenubarSurface:       "Horus - Ops",
			Status:               "ready",
		},
		{
			ID:                   "reap",
			Operation:            "Retire dead or stale thread records using OS-truth liveness",
			DeterministicCommand: "sirsi thread list; sirsi thread prune",
			MenubarSurface:       "Horus - Ops",
			Status:               "ready",
		},
		{
			ID:                   "supervise",
			Operation:            "Audit agent wake health and router helper drift",
			DeterministicCommand: "sirsi router node-status; sirsi router doctor; sirsi horus supervise --once",
			MenubarSurface:       "Horus - Ops",
			Status:               "ready",
		},
		{
			ID:                   "insight",
			Operation:            "Diagnose host health, hardware pressure, activity, and consistency",
			DeterministicCommand: "sirsi diagnose; sirsi vitals; sirsi activity; sirsi insight",
			LocalAICommand:       "sirsi gemma --mode analyze <prompt>",
			MenubarSurface:       "Insight",
			Status:               "ready",
		},
	}
}

func collectOpsLiveSummary(repoRoot, routerRoot string) (*opsLiveSummary, error) {
	items, err := router.OpenItems(routerRoot, "")
	if err != nil {
		return nil, err
	}
	pending := make(map[string][]string)
	for _, item := range items {
		if strings.TrimSpace(item.To) == "" {
			continue
		}
		pending[item.To] = append(pending[item.To], item.ID)
	}

	threads, err := router.LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil, err
	}
	liveWatchers := map[string]bool{}
	liveThreads := 0
	staleThreads := 0
	now := time.Now().UTC()
	for _, thread := range threads.Threads {
		if thread == nil || thread.Status.IsTerminal() || thread.Status == router.ThreadStatusSuspended {
			continue
		}
		if thread.Status == router.ThreadStatusStale || now.Sub(thread.LastSeenAt) > router.DefaultThreadStaleAfter {
			staleThreads++
			continue
		}
		liveThreads++
		watches := thread.Watches
		if len(watches) == 0 {
			watches = []string{thread.AgentID}
		}
		for _, watch := range watches {
			if strings.TrimSpace(watch) != "" {
				liveWatchers[watch] = true
			}
		}
	}

	stranded := 0
	for agent, ids := range pending {
		if len(ids) > 0 && !liveWatchers[agent] {
			stranded++
		}
	}

	return &opsLiveSummary{
		Repo:          repoRoot,
		PendingTotal:  len(items),
		AgentsPending: len(nonEmptyPendingAgents(pending)),
		LiveThreads:   liveThreads,
		StaleThreads:  staleThreads,
		Stranded:      stranded,
	}, nil
}

func renderOps(report opsReport) {
	fmt.Println("𓇶 Agent Operations Parity")
	fmt.Println("  Every agent operation has an operator path: deterministic first, local AI optional.")
	if report.Live != nil {
		fmt.Printf("  Live: %d pending across %d agent(s), %d live thread(s), %d stale, %d stranded\n",
			report.Live.PendingTotal, report.Live.AgentsPending, report.Live.LiveThreads,
			report.Live.StaleThreads, report.Live.Stranded)
	}
	fmt.Println()

	for _, c := range report.Capabilities {
		fmt.Printf("  %s — %s\n", c.ID, c.Operation)
		fmt.Printf("      cli:     %s\n", c.DeterministicCommand)
		if c.LocalAICommand != "" {
			fmt.Printf("      local:   %s\n", c.LocalAICommand)
		}
		fmt.Printf("      menubar: %s\n", c.MenubarSurface)
	}
}

func nonEmptyPendingAgents(pending map[string][]string) []string {
	var agents []string
	for agent, ids := range pending {
		if len(ids) > 0 && strings.TrimSpace(agent) != "" {
			agents = append(agents, agent)
		}
	}
	return agents
}

func init() { rootCmd.AddCommand(opsCmd) }
