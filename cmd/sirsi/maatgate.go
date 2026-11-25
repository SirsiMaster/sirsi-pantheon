package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/trustboundary"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	gateRoot, gateBase, gateHead                   string
	gatePrePush, gateAll, gateLintOnly, gateReport bool
)

type gateRequestModes struct {
	root, base, head                         string
	rangeSet, prePush, all, lintOnly, report bool
}

// Check the entire request before report output, repository lookup, or a
// verifier. A mode must not accidentally fulfill a
// different operation's exit-status contract.
func (m gateRequestModes) validate() error {
	selectedRange := m.rangeSet || m.base != "" || m.head != ""
	if selectedRange && (strings.TrimSpace(m.base) == "" || strings.TrimSpace(m.head) == "") {
		return fmt.Errorf("--base and --head must both be nonempty; no survey fallback")
	}
	if selectedRange && m.lintOnly {
		return fmt.Errorf("--lint-only is informational compatibility mode and cannot be combined with an explicit range")
	}
	scanOptions := selectedRange || m.prePush || m.all || m.lintOnly
	if m.report && (scanOptions || m.root != "") {
		return fmt.Errorf("--report is informational and cannot be combined with another gate option")
	}
	if m.prePush && (selectedRange || m.all || m.lintOnly) {
		return fmt.Errorf("--pre-push requires the full check of the pushed refs; use scan options separately")
	}
	return nil
}

func (m gateRequestModes) enforcement() bool {
	return m.prePush || m.rangeSet || m.base != "" || m.head != ""
}

// Optional repository quality gate. The A–H rules are heuristic review aids;
// current Pantheon canon does not assign them policy authority. No hook is
// changed or installed by adding this command. Hook adoption is a separate,
// an explicit repository configuration decision outside this command:
//
//	sirsi maat gate --pre-push < /dev/stdin || exit 1
var maatGateCmd = &cobra.Command{
	Use:   "gate",
	Short: "𓆄 Ma'at repository gate: traceability, exemption growth, secrets, heuristic lint",
	Long: `𓆄 Ma'at repository gate

Runs, with each verifier's OWN exit status as the verdict:
  1. scripts/verify-commit-traceability.sh --pull-request <base> <head>  (if the repo ships it)
  2. scripts/traceability-historical-exemptions.txt must not grow in the range (no self-exemption)
  3. gitleaks over the range (if installed; CI still gates)
  4. optional A–H heuristic lint over changed Go/TS/shell files; each finding
     names a heuristic; silence a reviewed false positive with
     "trust-boundary: <reason>" on the line or the line above.

  sirsi maat gate --pre-push            # consume Git's pushed-ref records from stdin
  sirsi maat gate --base X --head Y     # explicit range (CI, review)
  sirsi maat gate --all --lint-only     # explicit lint-only compatibility mode
  sirsi maat gate --report              # print a read-only heuristic review worksheet`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		modes := gateRequestModes{
			root: gateRoot, base: gateBase, head: gateHead,
			rangeSet: cmd.Flags().Changed("base") || cmd.Flags().Changed("head"),
			prePush:  gatePrePush, all: gateAll, lintOnly: gateLintOnly, report: gateReport,
		}
		if err := modes.validate(); err != nil {
			return err
		}
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
		var absErr error
		root, absErr = filepath.Abs(root)
		if absErr != nil {
			return fmt.Errorf("resolve gate root: %w", absErr)
		}
		g := trustboundary.Gate{Root: root, All: gateAll, Lint: gateLintOnly, Out: cmd.ErrOrStderr(), Context: cmd.Context()}
		var res trustboundary.Result
		enforcement := modes.enforcement()
		if gatePrePush {
			ranges, err := trustboundary.RangesFromPrePush(root, cmd.InOrStdin())
			if err != nil {
				// Fail CLOSED: a branch ref without a verifiable base is refused,
				// never admitted as "nothing to check".
				cmd.SilenceUsage = true
				return fmt.Errorf("𓆄 trust-boundary gate refused the push: %v", err)
			}
			if len(ranges) == 0 {
				output.Info("𓆄 gate: no branch content in this push (tags / deletions only) — nothing to check")
				return nil
			}
			res = trustboundary.RunRanges(g, ranges)
		} else {
			g.Base, g.Head = gateBase, gateHead
			res = g.Run()
		}
		if JsonOutput {
			// JSON only on stdout; the exit status carries the verdict.
			if err := emitJSON(res); err != nil {
				return err
			}
			if !gateResultAccepted(res, enforcement) {
				cmd.SilenceUsage = true
				return gateResultError(res, enforcement, true)
			}
			return nil
		}
		for _, s := range res.Steps {
			icon := map[string]string{"pass": "✅", "fail": "❌", "skip": "⏭️ "}[s.Status]
			ref := ""
			if s.Ref != "" {
				ref = s.Ref + " "
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %s %s%-20s %s\n", icon, ref, s.Name, s.Detail)
		}
		for _, f := range res.Findings {
			fmt.Fprintf(cmd.OutOrStdout(), "     %s\n", f)
		}
		if !gateResultAccepted(res, enforcement) {
			// Exit non-zero through cobra so a hook's `|| exit 1` sees it. Never
			// print-and-exit-0 when a configured check failed or was skipped.
			cmd.SilenceUsage = true
			return gateResultError(res, enforcement, false)
		}
		if res.Scope == "lint-only" {
			fmt.Fprintln(cmd.OutOrStdout(), "  𓆄 informational lint-only compatibility result; this is not a full enforcement pass")
		} else if !res.Complete {
			fmt.Fprintf(cmd.OutOrStdout(), "  𓆄 informational survey only: incomplete checks %v; this is not an enforcement pass\n", res.Skipped)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "  𓆄 configured gate scope completed; lint is not A–H certification")
		}
		return nil
	},
}

func gateResultAccepted(res trustboundary.Result, enforcement bool) bool {
	if !res.OK {
		return false
	}
	return !enforcement || (res.Scope != "lint-only" && res.Complete && len(res.Skipped) == 0)
}

func gateResultError(res trustboundary.Result, enforcement, jsonOutput bool) error {
	if !res.OK {
		if jsonOutput {
			return fmt.Errorf("𓆄 configured Ma'at gate steps failed; see JSON result")
		}
		return fmt.Errorf("𓆄 trust-boundary gate failed — do not --no-verify past this; fix or allowlist with a reason")
	}
	if enforcement && (res.Scope == "lint-only" || !res.Complete || len(res.Skipped) != 0) {
		if jsonOutput {
			return fmt.Errorf("𓆄 enforcement gate was incomplete; see skipped checks in JSON result")
		}
		return fmt.Errorf("𓆄 enforcement gate was incomplete; inspect skipped checks before treating it as a pass")
	}
	return nil
}

func init() {
	f := maatGateCmd.Flags()
	f.StringVar(&gateRoot, "root", "", "repo root (default: the enclosing git repo)")
	f.StringVar(&gateBase, "base", "", "range base commit (exclusive)")
	f.StringVar(&gateHead, "head", "", "range head commit (inclusive)")
	f.BoolVar(&gatePrePush, "pre-push", false, "read the pushed refs from stdin as git's pre-push hook provides them")
	f.BoolVar(&gateAll, "all", false, "lint the whole tree, not only the changed files")
	f.BoolVar(&gateLintOnly, "lint-only", false, "run informational no-range trust-boundary lint compatibility mode")
	f.BoolVar(&gateReport, "report", false, "print the heuristic review worksheet without repository checks")
	maatCmd.AddCommand(maatGateCmd)
}
