package main

// `sirsi maat delegate` — ADR-120 step 1: create, re-issue, or revoke a
// pod-local Ma'at booking delegation. Design/schema stage: no pod authority
// process (step 2) exists yet to consume a delegation; this command only
// writes the canonical fence record ReserveAsAuthority/Reserve now enforce.

import (
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/spf13/cobra"
)

var (
	delegatePod, delegateAuthority, delegateResources, delegateAddrs string
	delegateTTL                                                      time.Duration
	delegateRevoke                                                   bool
)

var maatDelegateCmd = &cobra.Command{
	Use:   "delegate",
	Short: "Create, re-issue, or revoke a pod-local Ma'at booking delegation (ADR-120)",
	Long: `Delegates exactly the named resources to a pod authority for a finite
window (max 12h, ADR-120 §4). Re-running for the same --pod re-issues (bumps
the epoch, fencing out the old authority instance); a resource already
delegated to a DIFFERENT live pod is refused. --revoke ends a pod's
delegation immediately. While a delegation is live, the canonical ledger
refuses direct "sirsi maat reserve" calls on its resources (ADR-120 §2).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		if delegateRevoke {
			if delegatePod == "" {
				return fmt.Errorf("--pod is required")
			}
			d, err := l.Revoke(delegatePod)
			if err != nil {
				return err
			}
			if maatJSON {
				return emitJSON(d)
			}
			fmt.Printf("𓆄 revoked delegation for pod %q (was epoch %d)\n", d.Pod, d.Epoch)
			return nil
		}
		if delegatePod == "" || delegateAuthority == "" || delegateResources == "" {
			return fmt.Errorf("--pod, --authority, and --resources are required")
		}
		ttl := delegateTTL
		if ttl <= 0 || ttl > 12*time.Hour {
			ttl = 12 * time.Hour
		}
		now := time.Now()
		req := schedule.Delegation{
			Pod:              delegatePod,
			AuthorityMachine: delegateAuthority,
			Resources:        splitCSV(delegateResources),
			IssuedAt:         now.Format(time.RFC3339),
			NotAfter:         now.Add(ttl).Format(time.RFC3339),
		}
		if delegateAddrs != "" {
			req.AuthorityAddrs = splitCSV(delegateAddrs)
		}
		d, err := l.Delegate(req)
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(d)
		}
		fmt.Printf("𓆄 delegated %s to pod %q (authority %s, epoch %d) until %s\n",
			strings.Join(d.Resources, ","), d.Pod, d.AuthorityMachine, d.Epoch, d.NotAfter)
		return nil
	},
}

// splitCSV trims and drops empty entries from a comma-separated flag value.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func init() {
	maatDelegateCmd.Flags().StringVar(&delegatePod, "pod", "", "pod id, e.g. studio-m5")
	maatDelegateCmd.Flags().StringVar(&delegateAuthority, "authority", "", "machine-registry id hosting the pod authority")
	maatDelegateCmd.Flags().StringVar(&delegateResources, "resources", "", "comma-separated resource ids to delegate")
	maatDelegateCmd.Flags().StringVar(&delegateAddrs, "addrs", "", "comma-separated addresses the authority serves on")
	maatDelegateCmd.Flags().DurationVar(&delegateTTL, "ttl", 12*time.Hour, "delegation window (max 12h)")
	maatDelegateCmd.Flags().BoolVar(&delegateRevoke, "revoke", false, "revoke pod's delegation")
	maatDelegateCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	maatCmd.AddCommand(maatDelegateCmd)
}
