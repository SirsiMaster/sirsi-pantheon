package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/trustboundary"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	gateRoot, gateBase, gateHead                                string
	gatePrePush, gateAll, gateLintOnly, gateReport, gateInstall bool
)

// maatGateCmd is the portfolio Trust-Boundary gate (ADR-076). One line in any
// repo's .githooks/pre-push adopts it:
//
//	sirsi maat gate --pre-push < /dev/stdin || exit 1
var maatGateCmd = &cobra.Command{
	Use:   "gate",
	Short: "𓆄 Trust-Boundary gate: traceability, exemption growth, secrets, A–H lint (ADR-076)",
	Long: `𓆄 Ma'at Trust-Boundary gate (ADR-076)

Runs, with each verifier's OWN exit status as the verdict:
  1. scripts/verify-commit-traceability.sh --pull-request <base> <head>  (if the repo ships it)
  2. scripts/traceability-historical-exemptions.txt must not grow in the range (no self-exemption)
  3. gitleaks over the range (if installed; CI still gates)
  4. trust-boundary lint over the changed Go/TS/shell files — rules A–H, each finding
     names its letter; silence a reviewed false positive with
     "trust-boundary: <reason>" on the line or the line above.

  sirsi maat gate --pre-push            # inside .githooks/pre-push; reads git's ref lines on stdin
  sirsi maat gate --base X --head Y     # explicit range (CI, review)
  sirsi maat gate --all --lint-only     # lint the whole tree (survey existing code)
  sirsi maat gate --report              # print the A–H checklist skeleton for the PR/router report
  sirsi maat gate --install             # git config core.hooksPath .githooks for this repo (covers its worktrees)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if gateReport {
			fmt.Fprint(cmd.OutOrStdout(), trustboundary.Report())
			return nil
		}
		root := gateRoot
		if root == "" {
			// The gate is repo-generic: the enclosing git checkout (worktree-aware),
			// not Pantheon's router root.
			r, err := trustboundary.RepoRoot(".")
			if err != nil {
				return fmt.Errorf("not inside a git repo; pass --root: %w", err)
			}
			root = r
		}
		root, _ = filepath.Abs(root)
		if gateInstall {
			if err := trustboundary.ArmHooks(root); err != nil {
				return err
			}
			output.Success("Ma'at gate armed: core.hooksPath=.githooks in %s (shared by its worktrees)", root)
			return nil
		}
		base, head := gateBase, gateHead
		if gatePrePush {
			var ok bool
			base, head, ok = trustboundary.RangeFromPrePush(root, cmd.InOrStdin())
			if !ok {
				output.Info("𓆄 gate: no branch content in this push — nothing to check")
				return nil
			}
		}
		g := trustboundary.Gate{Root: root, Base: base, Head: head, All: gateAll, Lint: gateLintOnly, Out: cmd.ErrOrStderr()}
		res := g.Run()
		if JsonOutput {
			if err := emitJSON(res); err != nil {
				return err
			}
		} else {
			for _, s := range res.Steps {
				icon := map[string]string{"pass": "✅", "fail": "❌", "skip": "⏭️ "}[s.Status]
				fmt.Fprintf(cmd.OutOrStdout(), "  %s %-20s %s\n", icon, s.Name, s.Detail)
			}
			for _, f := range res.Findings {
				fmt.Fprintf(cmd.OutOrStdout(), "     %s\n", f)
			}
		}
		if !res.OK {
			// Exit non-zero through cobra so a hook's `|| exit 1` sees it. Never
			// print-and-exit-0: the status IS the gate.
			cmd.SilenceUsage = true
			return fmt.Errorf("𓆄 trust-boundary gate failed — do not --no-verify past this; fix or allowlist with a reason")
		}
		fmt.Fprintln(cmd.OutOrStdout(), "  𓆄 trust-boundary gate: the feather weighs true")
		return nil
	},
}

func init() {
	f := maatGateCmd.Flags()
	f.StringVar(&gateRoot, "root", "", "repo root (default: the enclosing git repo)")
	f.StringVar(&gateBase, "base", "", "range base commit (exclusive)")
	f.StringVar(&gateHead, "head", "", "range head commit (inclusive)")
	f.BoolVar(&gatePrePush, "pre-push", false, "read the pushed refs from stdin as git's pre-push hook provides them")
	f.BoolVar(&gateAll, "all", false, "lint the whole tree, not only the changed files")
	f.BoolVar(&gateLintOnly, "lint-only", false, "run only the trust-boundary lint")
	f.BoolVar(&gateReport, "report", false, "print the A–H checklist skeleton and exit")
	f.BoolVar(&gateInstall, "install", false, "arm .githooks for this repo (git config core.hooksPath .githooks)")
	maatCmd.AddCommand(maatGateCmd)
}
