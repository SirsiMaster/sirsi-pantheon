package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

var convertAgent, convertTask string

var routerConvertCmd = &cobra.Command{
	Use:   "convert <item-id>",
	Short: "Close an open router item and turn it into a ledger task, atomically",
	Long: `Closes the item and adds a pending task in ONE transaction: the task's subject is the
item title, its outline names the item and its sender, and the item's close result reads
"converted to task <agent>/<task-id> by <actor>". Either both happen or neither does.
Re-running for the same item returns the same task. An item closed any other way is refused.

--agent is the ledger the task lands on (default: you). --task is optional; it defaults to
the item id, which is already unique. Converting an item addressed to someone else needs
close:any, like 'router close'.

  sirsi router convert 20261102-115454-ra-codex-pantheon-review --task review-1310`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no .agents/idea-router/ found: %w", err)
		}
		f, err := dispatch.Open(repoRoot)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		actor, reason := resolveCurrentAgent(filepath.Join(repoRoot, ".agents", "idea-router"), "")
		if actor == "" {
			return fmt.Errorf("resolve acting agent: %s", reason)
		}
		agent := convertAgent
		if agent == "" {
			agent = actor
		}
		t, err := f.ConvertItemToTask(actor, args[0], agent, convertTask)
		if err != nil {
			return err
		}
		fmt.Printf("  Converted %s → task %s/%s (%s)\n", args[0], t.Agent, t.TaskID, t.Status)
		return nil
	},
}

func init() {
	routerConvertCmd.Flags().StringVar(&convertAgent, "agent", "", "Ledger the task lands on (default: the acting agent)")
	routerConvertCmd.Flags().StringVar(&convertTask, "task", "", "Task id (default: the item id)")
	routerCmd.AddCommand(routerConvertCmd)
}
