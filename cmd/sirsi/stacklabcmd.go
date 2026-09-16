package main

// `sirsi stacklab doctor` — ADR-066 / PANTHEON_RULES A37 enforcement: every
// peer wing the router wing declares in allowed_peer_wings must be built,
// pushed, pinned, declared and schema-valid, or it is not canonical (origin
// is truth — ADR-066 §1). Modeled on `sirsi router doctor`
// (cmd/sirsi/routerdoctor.go): report-only table by default, --json for
// machine-readable output, non-zero exit on any finding.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/stacklab"
	"github.com/spf13/cobra"
)

// routerWingPath is this repo's own router wing record — the roster of
// declared peers lives in its handoffs.allowed_peer_wings (ADR-066 §3).
const routerWingPath = "docs/router-service/stacklab/router-wing-ra-v1.json"

var stacklabCmd = &cobra.Command{
	Use:   "stacklab",
	Short: "Stack Lab wing authority checks (ADR-066)",
}

var stacklabDoctorJSON bool

var stacklabDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Enforce ADR-066: every declared peer wing must be built, pushed, pinned, declared and schema-valid",
	Long: `Reads the router wing's allowed_peer_wings roster and, for every declared
peer, checks origin/main of its owning repo (never a working tree or mirror —
A35) for a schema-valid wing record, and the sirsi-stacklab registry for a
matching SHA-256 pin. Findings: stranded/unbuilt, unpushed/stranded,
unpinned, undeclared, invalid. Clean only when every peer clears all five.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("locate repo root: %w", err)
		}
		raw, err := os.ReadFile(filepath.Join(repoRoot, routerWingPath))
		if err != nil {
			return fmt.Errorf("read router wing %s: %w", routerWingPath, err)
		}
		routerWing, err := stacklab.ValidateWing(raw)
		if err != nil {
			return fmt.Errorf("router wing %s is itself schema-invalid: %w", routerWingPath, err)
		}
		roster := routerWing.Handoffs.AllowedPeerWings

		rep := stacklab.Run(stacklab.GHRemoteReader{}, roster, stacklab.LaneRepoMap)

		out := cmd.OutOrStdout()
		if stacklabDoctorJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			if err := enc.Encode(rep); err != nil {
				return err
			}
		} else {
			printStacklabReport(out, rep)
		}

		if !rep.Clean() {
			os.Exit(1)
		}
		return nil
	},
}

func printStacklabReport(out interface{ Write([]byte) (int, error) }, rep stacklab.Report) {
	fmt.Fprintf(out, "𓋹 Stack Lab Doctor (ADR-066) — %d declared peer(s)\n\n", len(rep.Roster))

	if len(rep.Findings) == 0 && len(rep.Unknown) == 0 {
		fmt.Fprintln(out, "✅ every declared peer is built, pushed, pinned, declared and valid.")
		return
	}

	byKind := map[string][]stacklab.Finding{}
	for _, f := range rep.Findings {
		byKind[f.Kind] = append(byKind[f.Kind], f)
	}
	kinds := []string{stacklab.KindStrandedUnbuilt, stacklab.KindUnpushedStranded, stacklab.KindUnpinned, stacklab.KindUndeclared, stacklab.KindInvalid}
	for _, k := range kinds {
		fs := byKind[k]
		if len(fs) == 0 {
			continue
		}
		fmt.Fprintf(out, "⚠ %s (%d):\n", k, len(fs))
		for _, f := range fs {
			fmt.Fprintf(out, "    %-42s %s\n", f.WingID, f.Detail)
		}
		fmt.Fprintln(out)
	}
	if len(rep.Unknown) > 0 {
		sort.Strings(rep.Unknown)
		fmt.Fprintf(out, "⚠ unknown — could not read %d origin/registry record(s), not counted as clean:\n", len(rep.Unknown))
		for _, u := range rep.Unknown {
			fmt.Fprintf(out, "    %s\n", u)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "Found %d finding(s). Exit non-zero — see PANTHEON_RULES A37.\n", len(rep.Findings)+boolToInt(len(rep.Unknown) > 0))
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func init() {
	stacklabDoctorCmd.Flags().BoolVar(&stacklabDoctorJSON, "json", false, "machine-readable findings array")
	stacklabCmd.AddCommand(stacklabDoctorCmd)
	rootCmd.AddCommand(stacklabCmd)
}
