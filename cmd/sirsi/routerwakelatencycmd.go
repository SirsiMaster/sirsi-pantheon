package main

// Ra charter M0, T4 slice: a dedicated read-only wake-latency report over
// opened -> first_claimed_at (set once by the first claim, schema v27), the
// delivery+claim latency gap complementary to the M0-02 wake-loop-depth
// measurement in docs/router-service/M0_BASELINE.md.

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

var (
	wakeLatencyAgent string
	wakeLatencyLimit int
	wakeLatencyJSON  bool
)

// wakeLatencyScope is the A35 statement of exactly what this report measures.
const wakeLatencyScope = `Measured: first claim minus opened, per item, from the store's own timestamps
(items.first_claimed_at, written once by the first claim and never by a renew,
start-work or re-claim after requeue).
Excluded: items first claimed before schema v27 added the column (no backfill),
and never-claimed items - neither appears in the percentiles.
Percentiles are nearest-rank (p95 of n samples is the ceil(0.95*n)-th smallest).`

var routerWakeLatencyCmd = &cobra.Command{
	Use:   "wake-latency",
	Short: "Read-only report: opened -> first-claim elapsed time, per item or aggregated",
	Long:  "Read-only report of how long items waited between being opened and first being claimed.\n\n" + wakeLatencyScope,
	RunE: func(cmd *cobra.Command, _ []string) error {
		st, err := routerstore.Resolve()
		if err != nil {
			return err
		}
		defer st.Close()
		samples, err := st.WakeLatencySamples(wakeLatencyAgent, wakeLatencyLimit)
		if err != nil {
			return err
		}
		if wakeLatencyJSON {
			if samples == nil {
				samples = []routerstore.WakeLatencySample{}
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(samples)
		}
		fmt.Fprintln(cmd.OutOrStdout(), wakeLatencyScope)
		if len(samples) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no items with a first-claim timestamp")
			return nil
		}
		lat := make([]float64, len(samples))
		for i, smp := range samples {
			lat[i] = smp.LatencySeconds
		}
		sort.Float64s(lat)
		fmt.Fprintf(cmd.OutOrStdout(), "n=%d  p50=%.1fs  p95=%.1fs  max=%.1fs\n", len(lat), pct(lat, 50), pct(lat, 95), lat[len(lat)-1])
		return nil
	},
}

func init() {
	routerWakeLatencyCmd.Flags().StringVar(&wakeLatencyAgent, "agent", "", "limit to this agent (default: every agent)")
	routerWakeLatencyCmd.Flags().IntVar(&wakeLatencyLimit, "limit", 200, "max samples")
	routerWakeLatencyCmd.Flags().BoolVar(&wakeLatencyJSON, "json", false, "raw samples as JSON instead of the aggregated summary")
	routerCmd.AddCommand(routerWakeLatencyCmd)
}
