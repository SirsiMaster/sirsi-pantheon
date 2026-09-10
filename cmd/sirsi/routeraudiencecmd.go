package main

// `sirsi router audience` — the Rule of Ra audit (ADR-062 20b.3): who mutated
// the ledger without being bound to a registered thread, from the service's own
// audience log, plus the live sessions in the window that carry no thread.

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

var audienceSince time.Duration
var audienceJSON bool

var routerAudienceCmd = &cobra.Command{
	Use:   "audience",
	Short: "Rule of Ra audit: gated calls whose session was not bound to its own registered thread (from the service's audience log)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		store, err := routerstore.Resolve()
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		since := time.Now().UTC().Add(-audienceSince).Format(time.RFC3339Nano)
		rep, err := store.AudienceSince(since)
		if err != nil {
			return err
		}
		if audienceJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(rep)
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "audience since %s (gate mode %q): %d gated calls, %d allowed, %d without audience\n", since, rep.Mode, rep.Gated, rep.Allowed, len(rep.Failures))
		switch {
		case rep.Mode == "off":
			fmt.Fprintln(out, "  the gate is OFF: nothing is recorded; this is not compliance")
		case !rep.Recorded:
			fmt.Fprintln(out, "  no gated calls recorded in the window: no history, not compliance")
		case len(rep.Failures) == 0:
			fmt.Fprintln(out, "  every recorded mutation in the window came from a session bound to its own active registered thread")
		}
		for agent, n := range rep.ByAgent {
			fmt.Fprintf(out, "  %-24s %d\n", agent, n)
		}
		for i, f := range rep.Failures {
			if i == 20 {
				fmt.Fprintf(out, "  … %d more\n", len(rep.Failures)-20)
				break
			}
			fmt.Fprintf(out, "  %s %-8s %-18s %s@%s thread=%q %s\n", f.TS[:19], f.Verdict, f.Method, f.Agent, f.Host, f.ThreadID, f.Reason)
		}
		if len(rep.Unbound) > 0 {
			fmt.Fprintf(out, "live sessions without a thread (%d):\n", len(rep.Unbound))
			for _, u := range rep.Unbound {
				fmt.Fprintf(out, "  %s\n", u)
			}
		}
		return nil
	},
}

func init() {
	routerAudienceCmd.Flags().DurationVar(&audienceSince, "since", 24*time.Hour, "window to audit")
	routerAudienceCmd.Flags().BoolVar(&audienceJSON, "json", false, "machine-readable report")
	routerCmd.AddCommand(routerAudienceCmd)
}
