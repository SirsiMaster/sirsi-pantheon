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

	"github.com/SirsiMaster/sirsi-pantheon/internal/stacklab"
	"github.com/spf13/cobra"
)

// routerWingPath is this repo's own router wing record — the roster of
// declared peers lives in its handoffs.allowed_peer_wings (ADR-066 §3).
const routerWingPath = "contracts/stacklab/ra-horus-fabric-wing-v1.json"

// pantheonRepo owns the router wing (the roster source). The doctor reads it from
// origin/main, never the working tree — a partial/stale local checkout must never
// block or skew the roster (A35/A37; 2026-09-28: a lane's stale checkout missing
// this path hard-exited the doctor before it evaluated any peer wing).
const pantheonRepo = "SirsiMaster/sirsi-pantheon"

var stacklabCmd = &cobra.Command{
	Use:   "stacklab",
	Short: "Stack Lab wing authority checks (ADR-066)",
}

var stacklabDoctorJSON bool
var stacklabCatalogJSON bool

var stacklabCatalogGetwd = os.Getwd

var stacklabDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Enforce ADR-066: every declared peer wing must be built, pushed, pinned, declared and schema-valid",
	Long: `Reads the router wing's allowed_peer_wings roster and, for every declared
peer, checks origin/main of its owning repo (never a working tree or mirror —
A35) for a schema-valid wing record, and the sirsi-stacklab registry for a
matching SHA-256 pin. Findings: stranded/unbuilt, unpushed/stranded,
unpinned, undeclared, invalid. Clean only when every peer clears all five.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := stacklab.NewGHRemoteReader()
		// Read the router wing (roster source) from origin/main, not the working
		// tree — origin is truth (A35/A37), same as every peer record below.
		raw, exists, err := reader.ReadFile(pantheonRepo, routerWingPath, "main")
		if err != nil {
			return fmt.Errorf("read router wing %s from origin/main of %s: %w", routerWingPath, pantheonRepo, err)
		}
		if !exists {
			return fmt.Errorf("router wing %s is not on origin/main of %s — the roster source must exist on origin (A37)", routerWingPath, pantheonRepo)
		}
		routerWing, err := stacklab.ValidateWing(raw)
		if err != nil {
			return fmt.Errorf("router wing %s is schema-invalid on origin/main: %w", routerWingPath, err)
		}
		roster := routerWing.Handoffs.AllowedPeerWings

		rep := stacklab.Run(reader, roster, stacklab.LaneRepoMap, routerWing.ID)

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

var stacklabCatalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Project local Stack Lab recipes and wing contracts",
	Long: `Reads the selected checkout's direct versioned Stack Lab recipes and wings.
This is a local source catalog, not a claim that a wing is canonical, released,
or remotely pinned. Unreadable or malformed contract records are emitted in
unknown and never silently omitted.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Catalog is a source-worktree operation, unlike `stacklab doctor`,
		// which deliberately reads Ra's shared router authority. Using
		// FindRepoRoot here redirected a user in a linked worktree to the main
		// checkout and silently projected the wrong recipes. Resolve the nearest
		// checkout carrying this command's own contract directory instead.
		repoRoot, err := findStacklabCatalogRoot()
		if err != nil {
			return fmt.Errorf("locate selected Stack Lab checkout: %w", err)
		}
		catalog, err := stacklab.LoadLocalCatalog(repoRoot)
		if err != nil {
			return err
		}
		if stacklabCatalogJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			if err := enc.Encode(catalog); err != nil {
				return err
			}
		} else {
			printStacklabCatalog(cmd.OutOrStdout(), catalog)
		}
		if !catalog.Complete() {
			return fmt.Errorf("Stack Lab catalog is incomplete")
		}
		return nil
	},
}

// findStacklabCatalogRoot resolves the nearest source checkout containing both
// the repository module marker and the catalog's contract directory. It never
// consults the shared router root: the catalog must describe the checkout the
// operator selected, including an isolated worktree under review.
func findStacklabCatalogRoot() (string, error) {
	dir, err := stacklabCatalogGetwd()
	if err != nil {
		return "", err
	}
	if resolved, evalErr := filepath.EvalSymlinks(dir); evalErr == nil {
		dir = resolved
	}
	for {
		module, moduleErr := os.Stat(filepath.Join(dir, "go.mod"))
		contracts, contractsErr := os.Stat(filepath.Join(dir, "contracts", "stacklab"))
		if moduleErr == nil && !module.IsDir() && contractsErr == nil && contracts.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no checkout with go.mod and contracts/stacklab found from current directory")
		}
		dir = parent
	}
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

func printStacklabCatalog(out interface{ Write([]byte) (int, error) }, catalog stacklab.Catalog) {
	fmt.Fprintf(out, "𓋹 Stack Lab Local Catalog — %d contract(s)\n\n", len(catalog.Entries))
	for _, entry := range catalog.Entries {
		fmt.Fprintf(out, "%s · %s\n  %s\n", entry.Kind, entry.ID, entry.SourcePath)
		if entry.Purpose != "" {
			fmt.Fprintf(out, "  %s\n", entry.Purpose)
		}
		for _, component := range entry.Components {
			fmt.Fprintf(out, "  component %s\n", component.ID)
		}
	}
	if len(catalog.Unknown) > 0 {
		fmt.Fprintln(out, "\nUnreadable or malformed contract(s) — not complete:")
		for _, unknown := range catalog.Unknown {
			fmt.Fprintf(out, "  %s\n", unknown)
		}
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func init() {
	stacklabDoctorCmd.Flags().BoolVar(&stacklabDoctorJSON, "json", false, "machine-readable findings array")
	stacklabCatalogCmd.Flags().BoolVar(&stacklabCatalogJSON, "json", false, "machine-readable local recipe and wing catalog")
	stacklabCmd.AddCommand(stacklabDoctorCmd)
	stacklabCmd.AddCommand(stacklabCatalogCmd)
	rootCmd.AddCommand(stacklabCmd)
}
