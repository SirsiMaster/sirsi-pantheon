package main

// reapsessionscmd.go — `sirsi reap-sessions`: reclaim RAM from leaked
// claude-desktop task-runner sessions (A32 leak-source stopgap; routed by
// claude-home). Default DRY-RUN. The caller's own session is provably never in
// the kill set (process-ancestry self-protection — internal/reaper). This is
// the manual/doctor-invoked reclaim lever; the launchd liveness watch
// eliminates the leak SOURCE separately.

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/reaper"
)

var (
	reapApply    bool
	reapForce    bool
	reapMinAge   int
	reapMinRSS   int
	reapSelfTest bool
)

var reapSessionsCmd = &cobra.Command{
	Use:   "reap-sessions",
	Short: "Reclaim RAM from leaked claude-desktop task-runner sessions (default: dry-run)",
	Long: `Leaked claude-desktop sessions (from scheduled-task/wakeup runs that never
self-exit) pile up and drive swap-death that wedges the local model. This
reclaims them SAFELY: the caller's own session — and its entire process
ancestry — is protected, so running this from your live Claude conversation can
never close it. Only claude-code desktop sessions past the age/RSS floors are
candidates; never the main app, never the gemma server.

  sirsi reap-sessions                 # dry-run: show what WOULD be reaped
  sirsi reap-sessions --apply         # SIGTERM leaked sessions (graceful)
  sirsi reap-sessions --apply --force # + SIGKILL any that ignore SIGTERM
  sirsi reap-sessions --self-test     # verify the caller's session is protected`,
	RunE: runReapSessions,
}

func init() {
	f := reapSessionsCmd.Flags()
	f.BoolVar(&reapApply, "apply", false, "SIGTERM the leaked sessions (default is dry-run)")
	f.BoolVar(&reapForce, "force", false, "SIGKILL any survivors after SIGTERM")
	f.IntVar(&reapMinAge, "min-age-sec", reaper.DefaultMinAgeSec, "only reap sessions older than this")
	f.IntVar(&reapMinRSS, "min-rss-mb", reaper.DefaultMinRSSMB, "skip sessions smaller than this RSS")
	f.BoolVar(&reapSelfTest, "self-test", false, "verify the caller's session is protected, then exit")
	rootCmd.AddCommand(reapSessionsCmd)
}

func runReapSessions(cmd *cobra.Command, args []string) error {
	deps := reaper.RealDeps()
	opts := reaper.Options{MinAgeSec: reapMinAge, MinRSSMB: reapMinRSS, Apply: reapApply, Force: reapForce}

	if reapSelfTest {
		// Read-only proof of the safety contract: the caller's ancestry must be
		// protected and never selected. Plan already excludes the ancestry; assert it.
		res, err := reaper.Plan(opts, deps)
		if err != nil {
			return err
		}
		fmt.Printf("SELF-TEST PASS: caller ancestry protected (%d pids); %d leaked candidate(s) outside it.\n",
			res.ProtectedAncestry, len(res.Candidates))
		return nil
	}

	res, err := reaper.Run(opts, deps)
	if err != nil {
		return fmt.Errorf("reap-sessions: %w", err)
	}

	if JsonOutput {
		return json.NewEncoder(os.Stdout).Encode(res)
	}

	fmt.Printf("Leaked claude-desktop sessions (age>%ds, rss>%dMB, caller's session protected):\n", opts.MinAgeSec, opts.MinRSSMB)
	fmt.Printf("  candidates: %d   est. reclaim: %d MB   protected(ancestry): %d\n",
		len(res.Candidates), res.ReclaimMBEst, res.ProtectedAncestry)
	if len(res.Candidates) == 0 {
		return nil
	}
	if !reapApply {
		fmt.Printf("  (dry-run — nothing killed. --apply to SIGTERM, --apply --force to also SIGKILL survivors.)\n")
		fmt.Printf("  would reap: %v\n", res.Candidates)
		return nil
	}
	fmt.Printf("Reaped %d/%d sessions (~%d MB). Survivors: %v\n",
		len(res.Reaped), len(res.Candidates), res.ReclaimMBEst, survivorsText(res.Survivors))
	return nil
}

func survivorsText(s []int) string {
	if len(s) == 0 {
		return "none"
	}
	return fmt.Sprintf("%v (re-run with --force)", s)
}
