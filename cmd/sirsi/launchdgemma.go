package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/spf13/cobra"
)

func init() {
	var thread, plan string
	c := &cobra.Command{Use: "gemma-worker <plan|plan140|apply|recover>", Short: "Exact joined native Gemma worker lifecycle; no broker or stop action", Args: cobra.ExactArgs(1)}
	c.Flags().StringVar(&thread, "thread", "", "Exact selected worker thread for read-only plan")
	c.Flags().StringVar(&plan, "plan", "", "Prior exact public plan JSON file for apply")
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if os.Getenv("SIRSI_AGENT_ID") != "sirsi-software-admin" {
			return fmt.Errorf("Gemma lifecycle: actual registered SSA required")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 25*time.Second)
		defer cancel()
		root, e := router.FindRepoRoot()
		if e != nil {
			return e
		}
		f, e := dispatch.Open(root)
		if e != nil {
			return e
		}
		defer f.Close()
		rs, ok := f.Store().(*routerstore.RemoteStore)
		if !ok {
			return fmt.Errorf("Gemma lifecycle: canonical remote store required")
		}
		var expected *setup.GemmaWorkerPlan
		if args[0] == "apply" {
			b, e := os.ReadFile(plan)
			if e != nil {
				return e
			}
			if len(b) > 32768 {
				return fmt.Errorf("Gemma lifecycle: oversized public plan")
			}
			expected = &setup.GemmaWorkerPlan{}
			if e = json.Unmarshal(b, expected); e != nil {
				return e
			}
		}
		p, e := setup.NativeGemmaWorkerLifecycle(ctx, args[0], thread, os.Getenv("SIRSI_THREAD_ID"), router.MachineID(), expected, authorizeLifecycle, rs.LaneClaimBarrier)
		if e != nil {
			return e
		}
		if args[0] == "plan" || args[0] == "plan140" {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(p)
		}
		_, e = fmt.Fprintln(cmd.OutOrStdout(), "Gemma worker lifecycle completed; no broker action")
		return e
	}
	launchdCmd.AddCommand(c)
}
