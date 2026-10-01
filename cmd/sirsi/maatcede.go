package main

// `sirsi maat cede request|grant|counter|decline|withdraw|list` — willing
// cessation (owner directive 2026-09-26). Ma'at never forces a lane off a
// machine or stops a run mid-run; a lane that needs time, cores, or a whole
// machine asks the current holder to cede, and the holder alone answers.
// A grant does not move anything — the holder still has to release/not-renew
// its own reservation, and the requester then reserves normally.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// cedeDecisionRecord is one JSON line in the Ma'at decision ledger — a
// read-only operator projection (codex-pantheon, PR #793 follow-up). Field
// names/order match the existing writer ~/.local/bin/maat-decision exactly.
// "maat:cedes" (cede.go) stays the scheduler's actual authority; this is only
// a drillable record of what was assessed, who was affected, and why.
type cedeDecisionRecord struct {
	TS            string `json:"ts"`
	Host          string `json:"host"`
	Kind          string `json:"kind"`
	Determination string `json:"determination"`
	Requester     string `json:"requester"`
	Resource      string `json:"resource"`
	Assessed      string `json:"assessed"`
	Affected      string `json:"affected"`
	Why           string `json:"why"`
	Evidence      string `json:"evidence"`
}

// decisionsPath resolves the decision ledger path: $MAAT_DECISIONS if set,
// else ~/.sirsi/maat/decisions.jsonl.
func decisionsPath() string {
	if p := os.Getenv("MAAT_DECISIONS"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".sirsi", "maat", "decisions.jsonl")
}

// shortHostname mirrors `hostname -s`: the hostname up to the first dot.
func shortHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	if i := strings.Index(h, "."); i >= 0 {
		h = h[:i]
	}
	return h
}

// logCedeDecision appends one JSON line to the decision ledger and returns
// the append error (mkdir/open/write) to the caller instead of swallowing
// it. The scheduler transition this projects has already committed by the
// time this runs, so a failure here is reported, never silently dropped —
// the CLI must not claim a durable cede success when the ledger record
// never landed.
func logCedeDecision(kind, determination, requester, resource, assessed, affected, why, evidence string) error {
	return appendCedeDecision(cedeDecisionRecord{
		TS: time.Now().Format(time.RFC3339), Host: shortHostname(),
		Kind: kind, Determination: determination, Requester: requester, Resource: resource,
		Assessed: assessed, Affected: affected, Why: why, Evidence: evidence,
	})
}

// appendCedeDecision does the actual marshal/mkdir/open/write; injected as a
// variable per A16 so tests can force each failure mode deterministically.
var appendCedeDecision = func(rec cedeDecisionRecord) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("decision ledger marshal failed: %w", err)
	}
	path := decisionsPath()
	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o755); mkdirErr != nil {
		return fmt.Errorf("decision ledger mkdir failed: %w", mkdirErr)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("decision ledger open failed: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("decision ledger write failed: %w", err)
	}
	return nil
}

// cedeProjectionErr wraps a logCedeDecision failure with the fact that the
// scheduler transition already committed under id — so the CLI reports a
// clear failure instead of a false success, and a caller must not retry the
// same request (it would double-apply an already-committed transition).
func cedeProjectionErr(op, id string, err error) error {
	return fmt.Errorf("cede %s committed (id %s) but decision ledger projection failed — do not retry, the transition already applied: %w", op, id, err)
}

// cedeAssessed renders the "assessed" field, e.g. "cede machine 25 min on m1
// from 2026-09-26T11:00:00Z".
func cedeAssessed(c *schedule.Cede) string {
	s := fmt.Sprintf("cede %s %d min on %s", c.Ask, c.Minutes, c.Resource)
	if c.EarliestStart != "" {
		s += " from " + c.EarliestStart
	}
	return s
}

// cedeKindByStatus maps a response status to the decision ledger's kind and
// determination strings.
var cedeKindByStatus = map[schedule.CedeStatus]struct{ Kind, Determination string }{
	schedule.CedeStatusGranted:   {"cede-grant", "grant"},
	schedule.CedeStatusCountered: {"cede-counter", "counter"},
	schedule.CedeStatusDeclined:  {"cede-decline", "decline"},
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
		if err := logCedeDecision("cede-request", "pending", c.Requester, c.Resource, cedeAssessed(c), c.Holder, c.Reason, c.ID); err != nil {
			return cedeProjectionErr("request", c.ID, err)
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
			info := cedeKindByStatus[status]
			why := ""
			if c.Decision != nil {
				why = c.Decision.Reason
				if status == schedule.CedeStatusCountered && c.Decision.Counter != "" {
					why = fmt.Sprintf("%s — counter: %s", why, c.Decision.Counter)
				}
			}
			if err := logCedeDecision(info.Kind, info.Determination, by, c.Resource, cedeAssessed(c), c.Requester, why, c.ID); err != nil {
				return cedeProjectionErr(info.Determination, c.ID, err)
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
		if err := logCedeDecision("cede-withdraw", "withdraw", by, c.Resource, cedeAssessed(c), c.Holder, c.Reason, c.ID); err != nil {
			return cedeProjectionErr("withdraw", c.ID, err)
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
