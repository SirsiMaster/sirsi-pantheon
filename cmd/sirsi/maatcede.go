package main

// `sirsi maat cede request|grant|counter|decline|withdraw|list` — willing
// cessation (owner directive 2026-09-26). Ma'at never forces a lane off a
// machine or stops a run mid-run; a lane that needs time, cores, or a whole
// machine asks the current holder to cede, and the holder alone answers.
// A grant does not move anything — the holder still has to release/not-renew
// its own reservation, and the requester then reserves normally.

import (
	"fmt"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/spf13/cobra"
)

var (
	cedeHolder, cedeRequester, cedeAsk, cedeEarliest, cedeReason string
	cedeMinutes                                                  int
	cedeStart, cedeCounter                                       string
	cedeResourceFilter                                           string
	cedePendingOnly                                              bool
)

func cedeActor(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	return os.Getenv("SIRSI_AGENT_ID")
}

var maatCedeCmd = &cobra.Command{
	Use:   "cede",
	Short: "Ask a lane to willingly give up time, cores, or a machine — never forced",
}

var maatCedeRequestCmd = &cobra.Command{
	Use:   "request <resource>",
	Short: "File a cede request against a resource and its current holder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		req := schedule.Cede{
			Resource: args[0], Requester: cedeActor(cedeRequester), Holder: cedeHolder,
			Ask: cedeAsk, Minutes: cedeMinutes, EarliestStart: cedeEarliest, Reason: cedeReason,
		}
		c, err := l.RequestCede(req)
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(c)
		}
		fmt.Printf("𓆄 cede requested: %s asks %s to cede %s on %s for %d min  (id %s)\n",
			c.Requester, c.Holder, c.Ask, c.Resource, c.Minutes, c.ID)
		return nil
	},
}

func respondCedeCmd(use, short string, status schedule.CedeStatus) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := maatLedger()
			if err != nil {
				return err
			}
			by := cedeActor(cedeHolder)
			c, err := l.RespondCede(args[0], by, status, cedeReason, cedeStart, cedeCounter)
			if err != nil {
				return err
			}
			if maatJSON {
				return emitJSON(c)
			}
			fmt.Printf("𓆄 cede %s: %s (by %s)\n", c.ID, c.Status, by)
			return nil
		},
	}
}

var maatCedeGrantCmd = respondCedeCmd("grant", "Grant a cede request (a start time, may be \"after my current run\")", schedule.CedeStatusGranted)
var maatCedeCounterCmd = respondCedeCmd("counter", "Counter a cede request (fewer cores, shorter window, later start)", schedule.CedeStatusCountered)
var maatCedeDeclineCmd = respondCedeCmd("decline", "Decline a cede request", schedule.CedeStatusDeclined)

var maatCedeWithdrawCmd = &cobra.Command{
	Use:   "withdraw <id>",
	Short: "Withdraw your own still-pending cede request",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		by := cedeActor(cedeRequester)
		c, err := l.WithdrawCede(args[0], by)
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(c)
		}
		fmt.Printf("𓆄 cede %s withdrawn (by %s)\n", c.ID, by)
		return nil
	},
}

var maatCedeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cede requests",
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		rows, err := l.ListCedes(schedule.CedeFilter{Resource: cedeResourceFilter, PendingOnly: cedePendingOnly})
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(rows)
		}
		if len(rows) == 0 {
			fmt.Println("𓆄 no cede requests")
			return nil
		}
		for _, c := range rows {
			fmt.Printf("  %-8s %-14s %s→%s  %s (%d min)  %q\n", c.Status, c.Resource, c.Requester, c.Holder, c.Ask, c.Minutes, c.Reason)
		}
		return nil
	},
}

func init() {
	maatCedeRequestCmd.Flags().StringVar(&cedeRequester, "requester", "", "requester agent id (default $SIRSI_AGENT_ID)")
	maatCedeRequestCmd.Flags().StringVar(&cedeHolder, "holder", "", "current holder agent id (required)")
	maatCedeRequestCmd.Flags().StringVar(&cedeAsk, "ask", "", "\"machine\" or \"cores:N\" (required)")
	maatCedeRequestCmd.Flags().IntVar(&cedeMinutes, "minutes", 0, "duration in minutes, 1..60 (required)")
	maatCedeRequestCmd.Flags().StringVar(&cedeEarliest, "earliest", "", "earliest start RFC3339")
	maatCedeRequestCmd.Flags().StringVar(&cedeReason, "reason", "", "why (required)")

	for _, c := range []*cobra.Command{maatCedeGrantCmd, maatCedeCounterCmd, maatCedeDeclineCmd} {
		c.Flags().StringVar(&cedeHolder, "holder", "", "holder agent id (default $SIRSI_AGENT_ID) — must match the request's holder")
		c.Flags().StringVar(&cedeReason, "reason", "", "why")
	}
	maatCedeGrantCmd.Flags().StringVar(&cedeStart, "start", "", "grant start time, RFC3339 or \"after my current run\"")
	maatCedeCounterCmd.Flags().StringVar(&cedeCounter, "counter", "", "the counter-offer, e.g. \"cores:2 for 20m at 2026-09-26T11:00:00Z\"")

	maatCedeWithdrawCmd.Flags().StringVar(&cedeRequester, "requester", "", "requester agent id (default $SIRSI_AGENT_ID)")

	maatCedeListCmd.Flags().StringVar(&cedeResourceFilter, "resource", "", "filter by resource")
	maatCedeListCmd.Flags().BoolVar(&cedePendingOnly, "pending", false, "only pending cedes")

	for _, c := range []*cobra.Command{maatCedeRequestCmd, maatCedeGrantCmd, maatCedeCounterCmd, maatCedeDeclineCmd, maatCedeWithdrawCmd, maatCedeListCmd} {
		c.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	}

	maatCedeCmd.AddCommand(maatCedeRequestCmd, maatCedeGrantCmd, maatCedeCounterCmd, maatCedeDeclineCmd, maatCedeWithdrawCmd, maatCedeListCmd)
	maatCmd.AddCommand(maatCedeCmd)
}
