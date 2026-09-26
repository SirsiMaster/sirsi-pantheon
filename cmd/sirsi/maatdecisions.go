package main

// `sirsi maat decisions [show <id>]` — the live, drillable Ma'at decision view
// (owner ask via claude-io, 2026-09-26): every grant/refuse, conflict, guard
// verdict, window-gate block, and CI pause/resume Ma'at has made, with time,
// host, kind, requester, resource, what was assessed, who it affected, the
// determination, why, and an evidence link. Reads the same
// ~/.sirsi/maat/decisions.jsonl other hosts' writers (m5go, the
// maat-window-gate hook, maat-run-guard) already append to; `reserve` and
// `release` below write their own grant/refuse/release records natively,
// closing the gap named in the ask.

import (
	"fmt"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/decision"
	"github.com/spf13/cobra"
)

var (
	decKind, decHost, decSince string
	decLimit                   int
)

func loadDecisions() ([]decision.Line, error) {
	lines, err := decision.Read("")
	if err != nil {
		return nil, fmt.Errorf("read decisions ledger: %w", err)
	}
	since := time.Time{}
	if decSince != "" {
		d, err := time.ParseDuration(decSince)
		if err != nil {
			return nil, fmt.Errorf("--since: %w", err)
		}
		since = time.Now().Add(-d)
	}
	lines = decision.Filter(lines, decKind, decHost, since)
	decision.SortDesc(lines)
	if decLimit > 0 && len(lines) > decLimit {
		lines = lines[:decLimit]
	}
	return lines, nil
}

var maatDecisionsCmd = &cobra.Command{
	Use:   "decisions",
	Short: "Live, drillable Ma'at decision ledger (grants/refusals, conflicts, guards, gates)",
	RunE: func(cmd *cobra.Command, args []string) error {
		lines, err := loadDecisions()
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(lines)
		}
		if len(lines) == 0 {
			fmt.Println("𓆄 no decisions recorded")
			return nil
		}
		fmt.Printf("𓆄 Ma'at decisions (%d) — `sirsi maat decisions show <id>` to drill in\n", len(lines))
		for _, l := range lines {
			fmt.Printf("  %-8s %s  %-8s %-22s %-10s %s → %s\n",
				l.ID, short(l.Time()), l.Host(), l.Kind(), l.Determination(), l.Requester(), l.Resource())
		}
		return nil
	},
}

var maatDecisionsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show one decision's full record by its short ID from `sirsi maat decisions`",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		lines, err := decision.Read("")
		if err != nil {
			return fmt.Errorf("read decisions ledger: %w", err)
		}
		for _, l := range lines {
			if l.ID != args[0] {
				continue
			}
			if maatJSON {
				return emitJSON(l.Fields)
			}
			fmt.Printf("𓆄 decision %s\n", l.ID)
			for _, k := range []string{"time", "host", "kind", "requester", "resource", "assessed", "affected", "determination", "why", "evidence"} {
				if v, ok := l.Fields[k]; ok {
					fmt.Printf("  %-14s %v\n", k+":", v)
				}
			}
			for k, v := range l.Fields {
				switch k {
				case "time", "host", "kind", "requester", "resource", "assessed", "affected", "determination", "why", "evidence":
					continue
				}
				fmt.Printf("  %-14s %v\n", k+":", v)
			}
			return nil
		}
		return fmt.Errorf("no decision with id %q", args[0])
	},
}

func init() {
	maatDecisionsCmd.Flags().StringVar(&decKind, "kind", "", "filter by kind (exact, case-insensitive)")
	maatDecisionsCmd.Flags().StringVar(&decHost, "host", "", "filter by host (exact, case-insensitive)")
	maatDecisionsCmd.Flags().StringVar(&decSince, "since", "", "only decisions within this duration (e.g. 24h)")
	maatDecisionsCmd.Flags().IntVar(&decLimit, "limit", 50, "max rows (0 = unlimited)")
	maatDecisionsCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	maatDecisionsShowCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")

	maatDecisionsCmd.AddCommand(maatDecisionsShowCmd)
	maatCmd.AddCommand(maatDecisionsCmd)
}
