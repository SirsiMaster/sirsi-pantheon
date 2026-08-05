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

var routerTaskAddCmd = &cobra.Command{
	Use: "add <agent> <task-id>", Args: cobra.ExactArgs(2), Short: "Add a task",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(taskAddSubject) == "" {
			return fmt.Errorf("--subject is required")
		}
		f, err := openTaskFacade()
		if err != nil {
			return err
		}
		defer f.Close()
		err = f.Store().AddTask(routerstore.Task{Agent: args[0], TaskID: args[1], Subject: taskAddSubject, Status: taskAddStatus, Phase: taskAddPhase, ResponsibleParty: taskAddResponsible, BlockedBy: taskAddBlockedBy})
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
		t, err := f.Store().UpdateTask(args[0], args[1], u)
		if err == nil {
			fmt.Printf("  Updated task %s/%s → %s\n", t.Agent, t.TaskID, t.Status)
		}
		return err
	},
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
	routerTaskListCmd.Flags().BoolVar(&ledgerJSON, "json", false, "Emit JSON")
	routerTaskCmd.AddCommand(routerTaskAddCmd, routerTaskUpdateCmd, routerTaskListCmd)
	routerCmd.AddCommand(routerLedgerCmd, routerTaskCmd, routerDependCmd)
}
