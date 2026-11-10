package main

// Ra charter M0, T4 slice: a dedicated read-only wake-latency report over
// opened -> lease_updated (first claim), the delivery+claim latency gap
// complementary to the M0-02 wake-loop-depth measurement in
// docs/router-service/M0_BASELINE.md.

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

var routerWakeLatencyCmd = &cobra.Command{
	Use:   "wake-latency",
	Short: "Read-only report: opened -> first-claim elapsed time, per item or aggregated",
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
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(samples)
		}
		if len(samples) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no claimed items with a distinct lease_updated timestamp")
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
