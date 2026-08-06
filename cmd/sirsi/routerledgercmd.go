package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

var (
	ledgerJSON                                                                                       bool
	ledgerStaleAfter                                                                                 time.Duration
	taskAddSubject, taskAddStatus, taskAddPhase, taskAddResponsible, taskAddBlockedBy                string
	taskUpdateSubject, taskUpdateStatus, taskUpdatePhase, taskUpdateResponsible, taskUpdateBlockedBy string
	taskUpdateCharter, taskUpdateOutline, taskUpdateTimeline, taskUpdateTestState, taskUpdateStage   string
	taskUpdateLinks                                                                                  []string
	taskUpdateAddTokens, taskUpdateAddSeconds                                                        int64
	taskLeaseWorker, taskLeaseThread, taskLeaseToken, taskLeaseResult, taskLeaseReason               string
	taskLeaseTTL                                                                                     time.Duration
)

var routerLedgerCmd = &cobra.Command{
	Use:   "ledger [agent]",
	Short: "Show open work, age, staleness, dependencies, and task registry",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scope := ""
		if len(args) == 1 {
			scope = args[0]
		}
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return err
		}
		s, err := ledger.Build(repoRoot, scope, time.Now().UTC(), ledgerStaleAfter)
		if err != nil {
			return err
		}
		if ledgerJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(s)
		}
		renderLedger(s)
		return nil
	},
}

func renderLedger(s ledger.Snapshot) {
	if len(s.Agents) == 0 {
		fmt.Println("  Ledger clear — no open items or registered tasks.")
		return
	}
	for _, a := range s.Agents {
		stale := ""
		if a.Stale {
			stale = " STALE"
		}
		fmt.Printf("\n%s — %d open · oldest %s · blocked %d · unblocked/unpicked %d%s\n",
			a.AgentID, len(a.Items), ledger.FormatAge(a.OldestAgeSeconds), a.BlockedCount, a.UnblockedUnpicked, stale)
		for _, it := range a.Items {
			flags := make([]string, 0, 3)
			if it.Stale {
				flags = append(flags, "stale")
			}
			if it.Picked {
				flags = append(flags, "picked")
			}
			if it.Blocked {
				flags = append(flags, "blocked by "+strings.Join(it.DependencyChain, " → "))
			}
			suffix := ""
			if len(flags) > 0 {
				suffix = " [" + strings.Join(flags, ", ") + "]"
			}
			kind := it.Type
			if kind == "" {
				kind = "item"
			}
			fmt.Printf("  • %s  age=%s  from=%s  type=%s%s\n    %s\n", it.ID, ledger.FormatAge(it.AgeSeconds), it.From, kind, suffix, it.Title)
		}
		if len(a.Tasks) > 0 {
			fmt.Println("  TASK REGISTRY")
			for _, t := range a.Tasks {
				meta := ""
				if t.Phase != "" {
					meta += " phase=" + t.Phase
				}
				if t.BlockedBy != "" {
					meta += " blocked_by=" + t.BlockedBy
				}
				fmt.Printf("    • %s  %s  responsible=%s%s  %s\n", t.TaskID, t.Status, t.ResponsibleParty, meta, t.Subject)
			}
		}
	}
}

var routerTaskCmd = &cobra.Command{Use: "task", Short: "Manage the durable per-agent task registry"}

var routerTaskClaimCmd = &cobra.Command{
	Use: "claim <agent>", Args: cobra.ExactArgs(1), Short: "Atomically lease the next actionable task",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(taskLeaseWorker) == "" || strings.TrimSpace(taskLeaseThread) == "" {
			return fmt.Errorf("--worker and --thread are required")
		}
		f, err := openTaskFacade()
		if err != nil {
			return err
		}
		defer f.Close()
		lease, err := f.Store().ClaimNextTask(args[0], taskLeaseWorker, taskLeaseThread, taskLeaseTTL)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(lease)
	},
}

var routerTaskRenewCmd = &cobra.Command{
	Use: "renew <agent> <task-id>", Args: cobra.ExactArgs(2), Short: "Renew a fenced task lease",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := openTaskFacade()
		if err != nil {
			return err
		}
		defer f.Close()
		return f.Store().RenewTaskLease(args[0], args[1], taskLeaseToken, taskLeaseTTL)
	},
}

var routerTaskCompleteCmd = &cobra.Command{
	Use: "complete <agent> <task-id>", Args: cobra.ExactArgs(2), Short: "Complete a fenced task with evidence",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := openTaskFacade()
		if err != nil {
			return err
		}
		defer f.Close()
		return f.Store().CompleteTaskLease(args[0], args[1], taskLeaseToken, taskLeaseResult)
	},
}

var routerTaskReleaseCmd = &cobra.Command{
	Use: "release <agent> <task-id>", Args: cobra.ExactArgs(2), Short: "Release a fenced task after a recoverable failure",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := openTaskFacade()
		if err != nil {
			return err
		}
		defer f.Close()
		return f.Store().ReleaseTaskLease(args[0], args[1], taskLeaseToken, taskLeaseReason)
	},
}

var routerTaskAddCmd = &cobra.Command{
	Use: "add <agent> <task-id>", Args: cobra.ExactArgs(2), Short: "Add a task",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(taskAddSubject) == "" {
			return fmt.Errorf("--subject is required")
		}
		if taskAddStatus == "in-progress" || taskAddStatus == "done" {
			return fmt.Errorf("new executable tasks must start pending or blocked; use task claim and task complete for fenced execution")
		}
		f, err := openTaskFacade()
		if err != nil {
			return err
		}
		defer f.Close()
		err = f.AddTask(routerstore.Task{Agent: args[0], TaskID: args[1], Subject: taskAddSubject, Status: taskAddStatus, Phase: taskAddPhase, ResponsibleParty: taskAddResponsible, BlockedBy: taskAddBlockedBy})
		if err == nil {
			fmt.Printf("  Added task %s/%s\n", args[0], args[1])
		}
		return err
	},
}

var routerTaskUpdateCmd = &cobra.Command{
	Use: "update <agent> <task-id>", Args: cobra.ExactArgs(2), Short: "Update a task",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := openTaskFacade()
		if err != nil {
			return err
		}
		defer f.Close()
		u := routerstore.TaskUpdate{Subject: taskUpdateSubject, Status: taskUpdateStatus, Phase: taskUpdatePhase, ResponsibleParty: taskUpdateResponsible}
		u.BlockedBySet = cmd.Flags().Changed("blocked-by")
		u.BlockedBy = taskUpdateBlockedBy
		u.CharterSet = cmd.Flags().Changed("charter")
		u.Charter = taskUpdateCharter
		u.OutlineSet = cmd.Flags().Changed("outline")
		if u.OutlineSet {
			u.Outline, err = readTaskFile(taskUpdateOutline)
			if err != nil {
				return err
			}
		}
		u.TimelineSet = cmd.Flags().Changed("timeline")
		if u.TimelineSet {
			body, readErr := readTaskFile(taskUpdateTimeline)
			if readErr != nil {
				return readErr
			}
			if err = json.Unmarshal([]byte(body), &u.Timeline); err != nil {
				return fmt.Errorf("--timeline: %w", err)
			}
		}
		for _, raw := range taskUpdateLinks {
			parts := strings.SplitN(raw, ":", 3)
			if len(parts) != 3 {
				return fmt.Errorf("--link must be kind:label:url")
			}
			u.Links = append(u.Links, routerstore.TaskLink{Kind: parts[0], Label: parts[1], URL: parts[2]})
		}
		u.TestState, u.Stage = taskUpdateTestState, taskUpdateStage
		u.AddTokens, u.AddSeconds = taskUpdateAddTokens, taskUpdateAddSeconds
		t, err := f.UpdateTask(args[0], args[1], u)
		if err == nil {
			fmt.Printf("  Updated task %s/%s → %s\n", t.Agent, t.TaskID, t.Status)
		}
		return err
	},
}

func readTaskFile(value string) (string, error) {
	if !strings.HasPrefix(value, "@") || len(value) == 1 {
		return "", fmt.Errorf("value must use @file syntax")
	}
	body, err := os.ReadFile(strings.TrimPrefix(value, "@"))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

var routerTaskListCmd = &cobra.Command{
	Use: "list [agent]", Args: cobra.MaximumNArgs(1), Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		agent := ""
		if len(args) == 1 {
			agent = args[0]
		}
		f, err := openTaskFacade()
		if err != nil {
			return err
		}
		defer f.Close()
		tasks, err := f.Store().ListTasks(agent)
		if err != nil {
			return err
		}
		if ledgerJSON {
			return json.NewEncoder(os.Stdout).Encode(tasks)
		}
		for _, t := range tasks {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", t.Agent, t.TaskID, t.Subject, t.Status, t.Phase, t.ResponsibleParty, t.BlockedBy, t.Created, t.Updated)
		}
		return nil
	},
}

var routerDependCmd = &cobra.Command{
	Use: "depend <item-id> <blocked-by-item-id|->", Args: cobra.ExactArgs(2),
	Short: "Set or clear an item's dependency edge",
	RunE: func(cmd *cobra.Command, args []string) error {
		dep := args[1]
		if dep == "-" {
			dep = ""
		}
		f, err := openTaskFacade()
		if err != nil {
			return err
		}
		defer f.Close()
		if err := f.SetBlockedBy(args[0], dep); err != nil {
			return err
		}
		fmt.Printf("  Updated dependency for %s\n", args[0])
		return nil
	},
}

func openTaskFacade() (*dispatch.Facade, error) {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return nil, err
	}
	return dispatch.Open(repoRoot)
}

func init() {
	routerLedgerCmd.Flags().BoolVar(&ledgerJSON, "json", false, "Emit JSON")
	routerLedgerCmd.Flags().DurationVar(&ledgerStaleAfter, "stale-after", ledger.DefaultStaleAfter, "Heartbeat age that marks open work stale")
	routerTaskAddCmd.Flags().StringVar(&taskAddSubject, "subject", "", "Task subject (required)")
	routerTaskAddCmd.Flags().StringVar(&taskAddStatus, "status", "pending", "pending|in-progress|blocked|done")
	routerTaskAddCmd.Flags().StringVar(&taskAddPhase, "phase", "", "Plain-English phase group for Ledger Board (e.g. Infrastructure)")
	routerTaskAddCmd.Flags().StringVar(&taskAddResponsible, "responsible-party", "self", "self|codex|owner|agent id")
	routerTaskAddCmd.Flags().StringVar(&taskAddBlockedBy, "blocked-by", "", "Dependency task id")
	routerTaskUpdateCmd.Flags().StringVar(&taskUpdateSubject, "subject", "", "Replacement subject")
	routerTaskUpdateCmd.Flags().StringVar(&taskUpdateStatus, "status", "", "pending|in-progress|blocked|done")
	routerTaskUpdateCmd.Flags().StringVar(&taskUpdatePhase, "phase", "", "Plain-English phase group for Ledger Board")
	routerTaskUpdateCmd.Flags().StringVar(&taskUpdateResponsible, "responsible-party", "", "self|codex|owner|agent id")
	routerTaskUpdateCmd.Flags().StringVar(&taskUpdateBlockedBy, "blocked-by", "", "Dependency task id; empty clears")
	routerTaskUpdateCmd.Flags().StringVar(&taskUpdateCharter, "charter", "", "Task charter")
	routerTaskUpdateCmd.Flags().StringVar(&taskUpdateOutline, "outline", "", "Formal outline as @file")
	routerTaskUpdateCmd.Flags().StringVar(&taskUpdateTimeline, "timeline", "", "Timeline JSON array as @file")
	routerTaskUpdateCmd.Flags().StringArrayVar(&taskUpdateLinks, "link", nil, "Append kind:label:url (repeatable)")
	routerTaskUpdateCmd.Flags().StringVar(&taskUpdateTestState, "test-state", "", "untested|tested|passed|failed")
	routerTaskUpdateCmd.Flags().StringVar(&taskUpdateStage, "stage", "", "spec|build|review|verify|shipped")
	routerTaskUpdateCmd.Flags().Int64Var(&taskUpdateAddTokens, "add-tokens", 0, "Add output tokens to the task cost center")
	routerTaskUpdateCmd.Flags().Int64Var(&taskUpdateAddSeconds, "add-seconds", 0, "Add active-work seconds to the task cost center")
	routerTaskListCmd.Flags().BoolVar(&ledgerJSON, "json", false, "Emit JSON")
	routerTaskClaimCmd.Flags().StringVar(&taskLeaseWorker, "worker", "", "Concrete worker identity (required)")
	routerTaskClaimCmd.Flags().StringVar(&taskLeaseThread, "thread", "", "Durable thread/task identity (required)")
	routerTaskClaimCmd.Flags().DurationVar(&taskLeaseTTL, "ttl", 10*time.Minute, "Lease duration")
	for _, c := range []*cobra.Command{routerTaskRenewCmd, routerTaskCompleteCmd, routerTaskReleaseCmd} {
		c.Flags().StringVar(&taskLeaseToken, "lease", "", "Fenced task lease token (required)")
	}
	routerTaskRenewCmd.Flags().DurationVar(&taskLeaseTTL, "ttl", 10*time.Minute, "Lease duration")
	routerTaskCompleteCmd.Flags().StringVar(&taskLeaseResult, "result-ref", "", "Evidence/proof reference (required)")
	routerTaskReleaseCmd.Flags().StringVar(&taskLeaseReason, "reason", "", "Recoverable failure reason")
	routerTaskCmd.AddCommand(routerTaskAddCmd, routerTaskUpdateCmd, routerTaskListCmd, routerTaskClaimCmd, routerTaskRenewCmd, routerTaskCompleteCmd, routerTaskReleaseCmd)
	routerCmd.AddCommand(routerLedgerCmd, routerTaskCmd, routerDependCmd)
}
