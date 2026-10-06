package main

// `sirsi router task-lease-audit` — the diagnostic router-task-lease-diagnostic-missing
// asked for: a read-only record of task-lease ownership decisions
// (checkTaskOwner verdicts, BindTaskSession outcomes) keyed by task id, from
// the service's own task_lease_log. audience_log (`sirsi router audience`)
// answers "was this thread registered"; this answers "who held this task's
// lease and what did the service decide" — a different gate, a different log.

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

var (
	taskLeaseAuditAgent string
	taskLeaseAuditSince time.Duration
	taskLeaseAuditJSON  bool
)

var routerTaskLeaseAuditCmd = &cobra.Command{
	Use:   "task-lease-audit [task-id]",
	Short: "Task-lease ownership audit: checkTaskOwner/BindTaskSession decisions for a task, from the service's task_lease_log",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := ""
		if len(args) == 1 {
			taskID = args[0]
		}
		store, err := routerstore.Resolve()
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		since := time.Now().UTC().Add(-taskLeaseAuditSince).Format(time.RFC3339Nano)
		events, err := store.TaskLeaseEventsSince(taskLeaseAuditAgent, taskID, since)
		if err != nil {
			return err
		}
		if taskLeaseAuditJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(events)
		}
		out := cmd.OutOrStdout()
		if len(events) == 0 {
			fmt.Fprintf(out, "no task-lease decisions recorded since %s for agent=%q task=%q — either nothing happened, or it predates this diagnostic\n", since, taskLeaseAuditAgent, taskID)
			return nil
		}
		fmt.Fprintf(out, "%d task-lease decisions since %s:\n", len(events), since)
		for _, e := range events {
			fmt.Fprintf(out, "  %s %-12s %-8s %s/%s session=%s thread=%q host=%q %s\n",
				e.TS, e.Op, e.Verdict, e.Agent, e.TaskID, shortID(e.SessionID), e.ThreadID, e.Host, e.Reason)
		}
		return nil
	},
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func init() {
	routerTaskLeaseAuditCmd.Flags().StringVar(&taskLeaseAuditAgent, "agent", "", "filter to one agent")
	routerTaskLeaseAuditCmd.Flags().DurationVar(&taskLeaseAuditSince, "since", 24*time.Hour, "window to audit")
	routerTaskLeaseAuditCmd.Flags().BoolVar(&taskLeaseAuditJSON, "json", false, "machine-readable report")
	routerCmd.AddCommand(routerTaskLeaseAuditCmd)
}
