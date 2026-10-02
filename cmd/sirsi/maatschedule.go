package main

// `sirsi maat reserve|status|who-is-on|heartbeat|extend|release|coverage|
// should-defer|conflict-check` — the Ma'at metal reservation scheduler
// (stacklab.wing.maat, MAAT-WING-001.G1). Enforced admission and de-confliction
// of shared machine + rail usage, so lanes cannot contaminate each other's
// measurement windows. Replaces the advisory rails.lock.
//
// The ledger lives in the shared router store (cross-host: an M1 reservation is
// visible on the M5 and vice-versa, and to any future Mac). Resources are
// free-form (a machine id, a rail name) — a new Mac participates with no code
// change.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/decision"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

// admissionRefusedExit mirrors maat-repro-lint's exit 97: a harness that calls
// `reserve` and is refused stops with this code instead of touching the cable.
const admissionRefusedExit = 97

// recordDecision appends the human-readable outcome to Ma'at's host-local
// decision ledger. The reservation ledger remains the scheduling authority;
// this is an explanatory projection for `sirsi maat decisions`.
func recordDecision(kind, requester, resource, assessed, affected, determination, why, evidence string) {
	_ = decision.Append("", decision.New(kind, requester, resource, assessed, affected, determination, why, evidence))
}

func maatLedger() (*schedule.Ledger, error) {
	st, err := routerstore.Resolve()
	if err != nil {
		return nil, fmt.Errorf("resolve router store: %w", err)
	}
	return schedule.NewLedger(st), nil
}

var newMaatDecisionJournal = func() (maat.DecisionJournal, error) {
	return maat.NewDefaultDecisionJournal()
}

// recordReservationDecision gives every native reserve outcome the same
// explanatory shape that the dashboard, CLI, and future Ra projection read.
// The reservation store remains the scheduler authority; this is an append-only
// explanation of the decision, never a second reservation registry.
func recordReservationDecision(req schedule.Reservation, result schedule.ReserveResult) error {
	journal, err := newMaatDecisionJournal()
	if err != nil {
		return err
	}
	decision := maat.Decision{
		Kind:          "reservation grant",
		Requester:     req.Holder,
		Resource:      req.Resource,
		Assessed:      "no conflicting active reservation",
		Determination: "grant",
		Why:           "requested window is available",
	}
	switch {
	case result.Granted:
		if result.Reservation != nil {
			decision.Evidence = "reservation:" + result.Reservation.ID
		}
	case result.Queued:
		decision.Kind = "reservation queue"
		decision.Assessed = "conflicting active reservation"
		decision.Determination = "queue"
		decision.Why = "request queued behind current holder"
		if result.Conflict != nil {
			decision.Affected = result.Conflict.Holder
			decision.Evidence = "reservation:" + result.Conflict.ID
		}
	default:
		decision.Kind = "reservation refusal"
		decision.Assessed = "conflicting active reservation"
		decision.Determination = "refuse"
		decision.Why = "requested window overlaps a live foreign reservation"
		if result.Conflict != nil {
			decision.Affected = result.Conflict.Holder
			decision.Evidence = "reservation:" + result.Conflict.ID
		}
	}
	if err := journal.Append(decision); err != nil {
		return fmt.Errorf("append Ma'at reservation decision: %w", err)
	}
	return nil
}

func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

var (
	resHolder, resWork, resRegime, resStart, resEstEnd, resRepro string
	resPriority, resLeaseTTL, resExemptPID                       int
	resQueue, maatJSON                                           bool
	covHorizon                                                   int
	conflictMachine                                              string
)

var maatReserveCmd = &cobra.Command{
	Use:   "reserve <resource>",
	Short: "Reserve a machine or rail for a measurement window (enforced admission, never a lockout)",
	Long: `Reserve a resource (a machine id like m1/m5, or a rail like rail-a,
ci-runners@m5) for a time window. Ma'at never locks a lane out (owner
directive 2026-09-26): a live foreign reservation, or an unanswered cede
request addressed to you, grants a bounded FLOOR share instead of refusing —
only --queue waits for the full window. A harness calls this before touching
a cable; it is the rails.lock replacement.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		holder := resHolder
		if holder == "" {
			holder = os.Getenv("SIRSI_AGENT_ID")
		}
		start := resStart
		if start == "" {
			start = time.Now().Format(time.RFC3339)
		}
		req := schedule.Reservation{
			Resource: args[0], Holder: holder, Work: resWork,
			Regime: schedule.Regime(resRegime), Priority: resPriority,
			Repro: resRepro, Start: start, EstEnd: resEstEnd, LeaseTTLSec: resLeaseTTL,
			ExemptPID: resExemptPID,
		}
		res, err := l.Reserve(req, resQueue)
		if err != nil {
			return err
		}
		if res.Granted && res.Reservation.Share == schedule.ShareFloor {
			conflictHolder := ""
			if res.Conflict != nil {
				conflictHolder = res.Conflict.Holder
			}
			if err := logFloorGrant(l, req.Holder, req.Resource, res.Reservation, conflictHolder); err != nil {
				return err
			}
		}
		if err := recordReservationDecision(req, res); err != nil {
			// The scheduler state is already durable. Return the append failure
			// explicitly so a caller never mistakes an unprojected decision for a
			// complete, drillable outcome.
			return err
		}
		switch {
		case maatJSON:
			_ = emitJSON(res)
		case res.Queued:
			fmt.Printf("𓆄 QUEUED behind %s on %s (held until %s)\n", res.Conflict.Holder, req.Resource, orNow(res.Conflict.EstEnd))
		case res.Reservation.Share == schedule.ShareFloor:
			printFloorGrant(res.Reservation)
		default:
			fmt.Printf("𓆄 reserved %s for %s until %s  (id %s)\n", req.Resource, req.Holder, orNow(req.EstEnd), res.Reservation.ID)
		}
		return nil
	},
}

// printFloorGrant prints a bounded floor-share grant: a grant, not a
// refusal, so it exits 0 (owner directive 2026-09-26: never a lockout).
func printFloorGrant(r *schedule.Reservation) {
	fmt.Printf("𓆄 granted FLOOR: %d cores on %s (%s)\n", r.Cores, r.Resource, r.Reason)
	if len(r.PendingCedes) > 0 {
		fmt.Println("    open cede requests — answer to get more than the floor:")
		for _, id := range r.PendingCedes {
			fmt.Printf("      sirsi maat cede grant|counter|decline %s --reason \"...\"\n", id)
		}
	}
}

// logFloorGrant appends the decision-ledger line for a floor-share grant via
// logCedeDecision and returns its append error. affected is the conflicting
// holder's name when the floor came from a live conflict, and/or the
// requesters of any open cede that also capped the grant.
func logFloorGrant(l *schedule.Ledger, holder, resource string, r *schedule.Reservation, conflictHolder string) error {
	affected := conflictHolder
	evidence := r.ID
	if len(r.PendingCedes) > 0 {
		cedes, _ := l.ListCedes(schedule.CedeFilter{Holder: holder, PendingOnly: true})
		requesters := make([]string, 0, len(cedes))
		for _, c := range cedes {
			requesters = append(requesters, c.Requester)
		}
		if affected != "" {
			affected += ","
		}
		affected += strings.Join(requesters, ",")
		evidence += "," + strings.Join(r.PendingCedes, ",")
	}
	assessed := fmt.Sprintf("reserve %s (floor %d cores)", resource, r.Cores)
	if err := logCedeDecision("reserve", "grant-floor", holder, resource, assessed, affected, r.Reason, evidence); err != nil {
		return cedeProjectionErr("grant-floor", r.ID, err)
	}
	return nil
}

var maatStatusCmd = &cobra.Command{
	Use:   "status [resource]",
	Short: "List reservations (all, or for one resource)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		resource := ""
		if len(args) == 1 {
			resource = args[0]
		}
		rows, err := l.Status(resource)
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(rows)
		}
		if len(rows) == 0 {
			fmt.Println("𓆄 no reservations")
			return nil
		}
		fmt.Printf("𓆄 Ma'at reservations (%d)\n", len(rows))
		for _, r := range rows {
			fmt.Printf("  %-14s %-9s %-7s %-12s %s → %s  %s\n",
				r.Resource, r.Status, r.Regime, r.Holder, short(r.Start), short(r.EstEnd), r.Work)
		}
		if cedes, err := l.ListCedes(schedule.CedeFilter{Resource: resource, PendingOnly: true}); err == nil && len(cedes) > 0 {
			fmt.Printf("𓆄 pending cede requests (%d) — never auto-granted\n", len(cedes))
			for _, c := range cedes {
				fmt.Printf("  %-8s %-14s %s→%s  %s (%d min)  %q\n", c.Status, c.Resource, c.Requester, c.Holder, c.Ask, c.Minutes, c.Reason)
			}
		}
		return nil
	},
}

var maatWhoCmd = &cobra.Command{
	Use:   "who-is-on <resource>",
	Short: "Show the current live holder of a resource",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		who, err := l.WhoIsOn(args[0])
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(who)
		}
		if who == nil {
			fmt.Printf("𓆄 %s has no reservation\n", args[0])
			// A35: the ledger only knows who asked. Say what is actually running.
			if actors, aerr := schedule.LiveActivity(args[0]); aerr == nil && len(actors) > 0 {
				fmt.Printf("⚠ but %d live actor(s) on this host — not free:\n", len(actors))
				for i, a := range actors {
					if i == 5 {
						fmt.Printf("    … %d more\n", len(actors)-5)
						break
					}
					fmt.Printf("    %s: %s\n", a.Kind, a.Detail)
				}
			}
			return nil
		}
		fmt.Printf("𓆄 %s: %s (%s, %s) until %s\n", args[0], who.Holder, who.Regime, who.Work, orNow(who.EstEnd))
		return nil
	},
}

var maatHeartbeatCmd = &cobra.Command{
	Use: "heartbeat <id>", Short: "Refresh a reservation's lease so it does not expire", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		r, err := l.Heartbeat(args[0])
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(r)
		}
		fmt.Printf("𓆄 heartbeat %s (lease refreshed)\n", r.ID)
		return nil
	},
}

var maatExtendCmd = &cobra.Command{
	Use: "extend <id> --est-end <rfc3339>", Short: "Push a reservation's end out", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if resEstEnd == "" {
			return fmt.Errorf("--est-end is required")
		}
		l, err := maatLedger()
		if err != nil {
			return err
		}
		r, err := l.Extend(args[0], resEstEnd)
		if err != nil {
			return err
		}
		if r.Share == schedule.ShareFloor && len(r.PendingCedes) > 0 {
			if err := logFloorGrant(l, r.Holder, r.Resource, r, ""); err != nil {
				return err
			}
		}
		if maatJSON {
			return emitJSON(r)
		}
		if r.Share == schedule.ShareFloor {
			fmt.Printf("𓆄 extended %s until %s — kept at FLOOR (%d cores): %s\n", r.ID, r.EstEnd, r.Cores, r.Reason)
		} else {
			fmt.Printf("𓆄 extended %s until %s\n", r.ID, r.EstEnd)
		}
		return nil
	},
}

var maatReleaseCmd = &cobra.Command{
	Use: "release <id>", Short: "End a reservation, freeing the resource now", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		r, err := l.Release(args[0])
		if err != nil {
			return err
		}
		recordDecision("reservation release", r.Holder, r.Resource, fmt.Sprintf("held %s → %s", r.Start, orNow(r.EstEnd)), r.Resource, "released", r.Work, r.ID)
		if maatJSON {
			return emitJSON(r)
		}
		fmt.Printf("𓆄 released %s (%s is free)\n", r.ID, r.Resource)
		return nil
	},
}

var maatCoverageCmd = &cobra.Command{
	Use: "coverage <resource>", Short: "Usage plan / coverage read-model for a resource (dashboard data)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		cov, err := l.Coverage(args[0], covHorizon)
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(cov)
		}
		fmt.Printf("𓆄 %s — next %dh: %d%% reserved (%d min), %d conflicts/24h\n",
			cov.Resource, cov.HorizonHours, cov.UtilizationPct, cov.ReservedMinutes, cov.ConflictsCaught)
		if cov.Current != nil {
			fmt.Printf("  now: %s (%s) until %s\n", cov.Current.Holder, cov.Current.Regime, orNow(cov.Current.EstEnd))
		} else {
			fmt.Println("  now: free")
		}
		for _, u := range cov.Upcoming {
			fmt.Printf("  next: %s %s → %s (%s)\n", u.Holder, short(u.Start), short(u.EstEnd), u.Work)
		}
		return nil
	},
}

var maatShouldDeferCmd = &cobra.Command{
	Use: "should-defer <machine>", Short: "Exit 0 if a runner must defer (a quiet/loaded reservation covers the machine)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		defer1, cur, err := l.ShouldDefer(args[0])
		if err != nil {
			return err
		}
		if maatJSON {
			_ = emitJSON(map[string]any{"defer": defer1, "current": cur})
		} else if defer1 {
			fmt.Printf("𓆄 DEFER — %s reserved by %s (%s) until %s\n", args[0], cur.Holder, cur.Regime, orNow(cur.EstEnd))
		} else {
			fmt.Printf("𓆄 clear — %s\n", args[0])
		}
		if defer1 {
			os.Exit(admissionRefusedExit) // same convention: a deferring runner stops
		}
		return nil
	},
}

var maatConflictCheckCmd = &cobra.Command{
	Use: "conflict-check <resource>", Short: "Detect foreign load during a reserved window; invalidate the block and notify the intruder",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		machine := conflictMachine
		if machine == "" {
			machine = args[0]
		}
		rep, err := l.CheckConflicts(args[0], machine)
		if err != nil {
			return err
		}
		if !rep.Clean && rep.Reservation != "" {
			if _, err := l.Invalidate(rep.Reservation, rep.Summary); err != nil {
				return err
			}
			notifyIntruders(rep)
		}
		determination := "clean"
		if !rep.Clean {
			determination = "conflict"
		}
		recordDecision("conflict-check", rep.Holder, rep.Resource, fmt.Sprintf("live activity on %s", machine), rep.Holder, determination, rep.Summary, rep.Reservation)
		if maatJSON {
			_ = emitJSON(rep)
		} else if rep.Clean {
			fmt.Printf("𓆄 clean — %s\n", rep.Summary)
			for _, s := range rep.Shared {
				fmt.Printf("    %s\n", s)
			}
		} else {
			fmt.Printf("𓆄 CONFLICT — %s\n  block %s INVALIDATED\n", rep.Summary, rep.Reservation)
		}
		if !rep.Clean {
			os.Exit(admissionRefusedExit)
		}
		return nil
	},
}

// notifyIntruders sends a router item to any intruder that could be attributed
// to an agent id. Unattributed intruders are named in the report only.
func notifyIntruders(rep schedule.ConflictReport) {
	st, err := routerstore.Resolve()
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for _, a := range rep.Intruders {
		if a.Owner == "" || seen[a.Owner] {
			continue
		}
		seen[a.Owner] = true
		body := fmt.Sprintf("Your %s (%s) ran on %s during %s's reserved measurement window (reservation %s). That block is now INVALID. Reserve the resource with `sirsi maat reserve` before running, or check `sirsi maat who-is-on %s`.",
			a.Kind, a.Detail, rep.Resource, rep.Holder, rep.Reservation, rep.Resource)
		_, _ = st.Send("maat", a.Owner, "Ma'at: your run contaminated a reserved window on "+rep.Resource, "decision", body)
	}
}

func orNow(s string) string {
	if s == "" {
		return "open"
	}
	return short(s)
}

func short(rfc string) string {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return rfc
	}
	return t.Local().Format("15:04")
}

func init() {
	maatReserveCmd.Flags().StringVar(&resHolder, "holder", "", "holder agent id (default $SIRSI_AGENT_ID)")
	maatReserveCmd.Flags().StringVar(&resWork, "work", "", "work label (series/arm)")
	maatReserveCmd.Flags().StringVar(&resRegime, "regime", "quiet", "quiet|loaded|build")
	maatReserveCmd.Flags().StringVar(&resStart, "start", "", "start RFC3339 (default now)")
	maatReserveCmd.Flags().StringVar(&resEstEnd, "est-end", "", "estimated end RFC3339")
	maatReserveCmd.Flags().StringVar(&resRepro, "repro", "", "repro path")
	maatReserveCmd.Flags().IntVar(&resPriority, "priority", 0, "priority (higher wins the queue)")
	maatReserveCmd.Flags().IntVar(&resLeaseTTL, "lease-ttl", 120, "lease TTL seconds (expires without heartbeat)")
	maatReserveCmd.Flags().BoolVar(&resQueue, "queue", false, "queue behind the holder instead of refusing")
	maatReserveCmd.Flags().IntVar(&resExemptPID, "exempt-pid", 0, "PID of this reservation's own guarded-run process tree (e.g. the run-guard script); conflict-check never treats its ancestors or descendants as intruders")
	maatReserveCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")

	maatExtendCmd.Flags().StringVar(&resEstEnd, "est-end", "", "new estimated end RFC3339")
	maatCoverageCmd.Flags().IntVar(&covHorizon, "horizon", 24, "horizon hours")
	maatConflictCheckCmd.Flags().StringVar(&conflictMachine, "machine", "", "machine to probe (default: the resource)")

	for _, c := range []*cobra.Command{maatStatusCmd, maatDecisionsCmd, maatWhoCmd, maatHeartbeatCmd, maatReleaseCmd, maatCoverageCmd, maatShouldDeferCmd, maatConflictCheckCmd, maatExtendCmd} {
		c.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	}

	maatCmd.AddCommand(maatReserveCmd, maatStatusCmd, maatDecisionsCmd, maatWhoCmd, maatHeartbeatCmd, maatExtendCmd,
		maatReleaseCmd, maatCoverageCmd, maatShouldDeferCmd, maatConflictCheckCmd)
}
