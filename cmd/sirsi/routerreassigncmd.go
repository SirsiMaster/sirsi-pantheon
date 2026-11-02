package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

var (
	drainAgent  string
	drainDryRun bool
)

// openFacadeAs opens the dispatch facade and resolves the acting agent.
func openFacadeAs(override string) (*dispatch.Facade, string, error) {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return nil, "", fmt.Errorf("no .agents/idea-router/ found: %w", err)
	}
	f, err := dispatch.Open(repoRoot)
	if err != nil {
		return nil, "", err
	}
	actor, reason := resolveCurrentAgent(filepath.Join(repoRoot, ".agents", "idea-router"), override)
	if actor == "" {
		_ = f.Close()
		return nil, "", fmt.Errorf("resolve acting agent: %s", reason)
	}
	return f, actor, nil
}

type routerReassignJSON struct {
	ID     string `json:"id"`
	To     string `json:"to"`
	Status string `json:"status"`
}

type routerReassignOperation func(id, to, agent string) error

func newRouterReassignCmd(reassign routerReassignOperation) *cobra.Command {
	var to, agent string
	cmd := &cobra.Command{
		Use:   "reassign <id> --to <agent>",
		Short: "Hand an open item to another declared agent, keeping its id and history",
		Long: `Moves an open, unclaimed item to another declared agent. The id, sender and
body are kept, so replies still thread; a note records who moved it and when.

Allowed only for the item's current recipient (a hand-off), or when the item
sits in a retired alias's mailbox and --to is that alias's declared successor
(see 'router drain-aliases'). ADR-072 C5.

  sirsi router reassign 20260930-150958-codex-finalwishes-claude-finalwishes-helper-... --to claude-finalwishes-m5`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if to == "" {
				return fmt.Errorf("--to is required")
			}
			if err := reassign(args[0], to, agent); err != nil {
				return err
			}
			if JsonOutput {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(routerReassignJSON{ID: args[0], To: to, Status: "open"}); err != nil {
					return fmt.Errorf("router reassign committed item %s but JSON response write failed; reconcile by ID and do not retry: %w", args[0], err)
				}
				return nil
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "  Reassigned %s → %s\n", args[0], to)
			return err
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "Declared agent to receive the item")
	cmd.Flags().StringVar(&agent, "agent", "", "Acting agent id (otherwise resolved from the current session)")
	return cmd
}

var routerReassignCmd = newRouterReassignCmd(func(id, to, override string) error {
	f, actor, err := openFacadeAs(override)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Reassign(actor, id, to)
})

var routerDrainAliasesCmd = &cobra.Command{
	Use:   "drain-aliases",
	Short: "Move every open item in a retired alias's mailbox to its declared successor",
	Long: `Reads the "aliases" map in agents.json (retired name → successor) and moves
each alias's open, unclaimed items to the successor, keeping ids. New sends to
an alias already deliver to the successor; this empties what arrived before.
Claimed items are reported and left alone. ADR-072 C5.

  sirsi router drain-aliases --dry-run
  sirsi router drain-aliases`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f, actor, err := openFacadeAs(drainAgent)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		res, err := f.DrainAliases(actor, drainDryRun)
		sort.Slice(res, func(i, j int) bool { return res[i].ID < res[j].ID })
		moved, failed := 0, 0
		for _, r := range res {
			switch {
			case r.Err != nil:
				failed++
				fmt.Printf("  ✗ %s  %s → %s: %v\n", r.ID, r.From, r.To, r.Err)
			case drainDryRun:
				fmt.Printf("  would move %s  %s → %s\n", r.ID, r.From, r.To)
			default:
				moved++
				fmt.Printf("  moved %s  %s → %s\n", r.ID, r.From, r.To)
			}
		}
		if drainDryRun {
			fmt.Printf("  %d item(s) would move (dry run)\n", len(res))
		} else {
			fmt.Printf("  %d moved, %d failed\n", moved, failed)
		}
		if err != nil {
			return err
		}
		if failed > 0 {
			return fmt.Errorf("%d item(s) could not be moved", failed)
		}
		return nil
	},
}

func init() {
	routerDrainAliasesCmd.Flags().StringVar(&drainAgent, "agent", "", "Acting agent id (otherwise resolved from the current session)")
	routerDrainAliasesCmd.Flags().BoolVar(&drainDryRun, "dry-run", false, "Report what would move without moving it")
	routerCmd.AddCommand(routerReassignCmd, routerDrainAliasesCmd)
}
