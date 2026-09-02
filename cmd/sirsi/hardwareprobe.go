package main

// hardwareprobe.go — `sirsi hardware probe`: the Notebook 4 Phase 0 two-host
// link probe. `serve` on one Mac, `probe <host:port>` on the other, over the
// Thunderbolt Bridge address. TCP only; the receipt says so.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/fabric"
)

var (
	probeListen string
	probePings  int
	probeBulkMB int64
)

var hardwareProbeCmd = &cobra.Command{
	Use:   "probe <host:port>",
	Short: "Phase 0 link probe: RTT at 64 B and 14 KB, bulk TCP goodput — observed only",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := fabric.Probe(args[0], fabric.ProbeOptions{Pings: probePings, BulkBytes: probeBulkMB << 20})
		if err != nil {
			return err
		}
		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(r)
		}
		fmt.Printf("Link probe %s → %s  transport=%s  collected %s\n", r.LocalAddr, r.RemoteAddr, r.Transport, r.CollectedAt.Format(time.RFC3339))
		for _, x := range r.RTT {
			fmt.Printf("  rtt %5d B  n=%d  min %.0f  p50 %.0f  p95 %.0f  p99 %.0f µs\n", x.PayloadBytes, x.Samples, x.MinUs, x.P50us, x.P95us, x.P99us)
		}
		fmt.Printf("  bulk %d MiB in %.2fs → %.0f MB/s (%.2f Gb/s) one direction, application bytes\n", r.BulkBytes>>20, r.BulkSeconds, r.GoodputMBps, r.GoodputGbps)
		fmt.Printf("  rdma: %s\n", r.RDMA)
		for _, c := range r.Caveats {
			fmt.Printf("  caveat: %s\n", c)
		}
		return nil
	},
}

var hardwareProbeServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Answer link probes until interrupted",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		addr, err := fabric.Serve(ctx, probeListen)
		if err != nil {
			return err
		}
		port := 0
		if ta, ok := addr.(*net.TCPAddr); ok {
			port = ta.Port
		}
		fmt.Fprintf(os.Stderr, "probe server listening on %s — on the peer: sirsi hardware probe <this-host-bridge-ip>:%d\n", addr, port)
		<-ctx.Done()
		return nil
	},
}

func init() {
	hardwareProbeCmd.Flags().IntVar(&probePings, "pings", 200, "round trips per payload size")
	hardwareProbeCmd.Flags().Int64Var(&probeBulkMB, "bulk-mib", 256, "bytes sent for goodput, in MiB")
	hardwareProbeServeCmd.Flags().StringVar(&probeListen, "listen", ":7477", "address to listen on")
	hardwareProbeCmd.AddCommand(hardwareProbeServeCmd)
	hardwareCmd.AddCommand(hardwareProbeCmd)
}
