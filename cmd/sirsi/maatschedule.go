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
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/decision"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

// recordDecision appends to the live decision ledger (`sirsi maat decisions`).
// Logged best-effort: a decision that already happened (the reservation grant
// itself) must not fail because the ledger write did — the caller has already
// acted on it.
func recordDecision(kind, requester, resource, assessed, affected, determination, why, evidence string) {
	_ = decision.Append("", decision.New(kind, requester, resource, assessed, affected, determination, why, evidence))
}

// admissionRefusedExit mirrors maat-repro-lint's exit 97: a harness that calls
// `reserve` and is refused stops with this code instead of touching the cable.
const admissionRefusedExit = 97

func maatLedger() (*schedule.Ledger, error) {
	st, err := routerstore.Resolve()
	if err != nil {
		return nil, fmt.Errorf("resolve router store: %w", err)
	}
	return schedule.NewLedger(st), nil
}

func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

var (
	resHolder, resWork, resRegime, resStart, resEstEnd, resRepro string
	resPriority, resLeaseTTL                                     int
	resQueue, maatJSON                                           bool
	covHorizon                                                   int
	conflictMachine                                              string
)

var maatReserveCmd = &cobra.Command{
	Use:   "reserve <resource>",
	Short: "Reserve a machine or rail for a measurement window (enforced admission)",
	Long: `Reserve a resource (a machine id like m1/m5, or a rail like rail-a,
ci-runners@m5) for a time window. If a live foreign reservation overlaps, the
grant is REFUSED and the process exits ` + fmt.Sprint(admissionRefusedExit) + ` (unless --queue).
A harness calls this before touching a cable; it is the rails.lock replacement.`,
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
		}
		res, err := l.Reserve(req, resQueue)
		if err != nil {
			return err
		}
		assessed := fmt.Sprintf("window %s → %s, regime %s", req.Start, orNow(req.EstEnd), req.Regime)
		if res.Granted {
			recordDecision("reservation grant", req.Holder, req.Resource, assessed, req.Resource, "granted", req.Work, res.Reservation.ID)
		} else if res.Queued {
			recordDecision("reservation queue", req.Holder, req.Resource, assessed, req.Resource,
				"queued", fmt.Sprintf("held by %s until %s", res.Conflict.Holder, orNow(res.Conflict.EstEnd)), req.ID)
		} else {
			recordDecision("reservation refuse", req.Holder, req.Resource, assessed, req.Resource,
				"refused", fmt.Sprintf("held by %s until %s (work %q)", res.Conflict.Holder, orNow(res.Conflict.EstEnd), res.Conflict.Work), res.Conflict.ID)
		}
		if maatJSON {
			_ = emitJSON(res)
		} else if res.Granted {
			fmt.Printf("𓆄 reserved %s for %s until %s  (id %s)\n", req.Resource, req.Holder, orNow(req.EstEnd), res.Reservation.ID)
		} else if res.Queued {
			fmt.Printf("𓆄 QUEUED behind %s on %s (held until %s)\n", res.Conflict.Holder, req.Resource, orNow(res.Conflict.EstEnd))
		} else {
			fmt.Printf("𓆄 REFUSED — %s is held by %s until %s (work %q)\n", req.Resource, res.Conflict.Holder, orNow(res.Conflict.EstEnd), res.Conflict.Work)
		}
		if !res.Granted && !res.Queued {
			os.Exit(admissionRefusedExit)
		}
		return nil
	},
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
			fmt.Printf("𓆄 %s is free\n", args[0])
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
		if maatJSON {
			return emitJSON(r)
		}
		fmt.Printf("𓆄 extended %s until %s\n", r.ID, r.EstEnd)
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
	maatReserveCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")

	maatExtendCmd.Flags().StringVar(&resEstEnd, "est-end", "", "new estimated end RFC3339")
	maatCoverageCmd.Flags().IntVar(&covHorizon, "horizon", 24, "horizon hours")
	maatConflictCheckCmd.Flags().StringVar(&conflictMachine, "machine", "", "machine to probe (default: the resource)")

	for _, c := range []*cobra.Command{maatStatusCmd, maatWhoCmd, maatHeartbeatCmd, maatReleaseCmd, maatCoverageCmd, maatShouldDeferCmd, maatConflictCheckCmd, maatExtendCmd} {
		c.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	}

	maatCmd.AddCommand(maatReserveCmd, maatStatusCmd, maatWhoCmd, maatHeartbeatCmd, maatExtendCmd,
		maatReleaseCmd, maatCoverageCmd, maatShouldDeferCmd, maatConflictCheckCmd)
}
