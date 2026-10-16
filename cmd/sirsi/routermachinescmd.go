package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

var routerMachinesCmd = &cobra.Command{
	Use:   "machines",
	Short: "List the Macs admitted to the fabric (machine registry; Ra admits by merging machines.json)",
	Long: `Macs register with Ra like threads do. The record lives in .agents/idea-router/machines.json;
admission is a merge to main, and 'sirsi router registry sync' pins this host to it. Consumers such as
the Mercury run selector offer only machines listed here; discovery may find others, Ra admits.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := findRouterRoot()
		if err != nil {
			return err
		}
		ms, err := router.LoadMachines(root)
		if err != nil {
			return err
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{"machines": ms})
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tMODEL\tCHIP\tMEM\tTAILSCALE\tLAN\tPORT\tADMITTED")
		for _, m := range ms {
			fmt.Fprintf(w, "%s\t%s\t%s\t%dG\t%s\t%s\t%d\t%s by %s\n", m.ID, m.Model, m.Chip, m.MemoryGiB, m.Tailscale, m.LAN, m.MonitorPort, m.RegisteredAt[:10], m.AdmittedBy)
		}
		return w.Flush()
	},
}

func init() { routerCmd.AddCommand(routerMachinesCmd) }
