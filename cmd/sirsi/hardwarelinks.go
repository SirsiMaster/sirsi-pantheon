package main

// hardwarelinks.go — `sirsi hardware links`: read-only Thunderbolt link and
// RDMA-enablement telemetry. Notebook 4 §4.9 P0 ("removes all rate ambiguity")
// and the first field set of the Pantheon transport capability receipt
// (docs/HARDWARE_BASELINE_ALIGNMENT.md). Observes; never configures.

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/fabric"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

var hardwareLinksCmd = &cobra.Command{
	Use:   "links",
	Short: "Thunderbolt receptacles, attached peers, RDMA enablement — read-only, unknown stays unknown",
	RunE: func(cmd *cobra.Command, args []string) error {
		l := fabric.Collect(platform.Current())
		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(l)
		}
		fmt.Printf("Thunderbolt links — %s, macOS %s, collected %s\n", l.Host, l.MacOS, l.CollectedAt.Format("2006-01-02T15:04:05Z"))
		if !l.PortCountKnown {
			fmt.Println("  ports: UNKNOWN — system_profiler could not be read (see below)")
		}
		for _, p := range l.Ports {
			peer := ""
			if p.Peer != "" {
				peer = " → " + p.Peer
			}
			fmt.Printf("  receptacle %-2s %-22s advertised %-16s connected=%s%s  negotiated=%s\n",
				p.ReceptacleID, p.Bus, p.AdvertisedMax, p.Connected, peer, p.NegotiatedRate)
		}
		fmt.Printf("  interfaces: %v\n", l.Bridges)
		fmt.Printf("  rdma: family_loaded=%s nvram_enabled=%s\n", l.RDMA.FamilyLoaded, l.RDMA.NVRAMEnabled)
		if l.RDMA.NVRAMEnabled == fabric.No {
			fmt.Println("        RDMA over Thunderbolt is NOT enabled on this host (nvram rdma-enable absent; enable in Recovery per Apple TN3205)")
		}
		for _, u := range l.Unknown {
			fmt.Printf("  unknown: %s\n", u)
		}
		fmt.Println("  note: advertised is a ceiling, not a rate; negotiated rate needs a link-level probe (Notebook 4 Phase 0)")
		return nil
	},
}

func init() {
	hardwareCmd.AddCommand(hardwareLinksCmd)
}
