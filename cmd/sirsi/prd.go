package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/prd"
)

var (
	prdSyncAgent string
	prdSyncJSON  bool
)

var prdCmd = &cobra.Command{
	Use:   "prd",
	Short: "Inspect and reconcile agent PRDs (.agents/prd/<agent>.json)",
	Long: `Each thread carries a PRD derived from repo canon (RULES + ARCHITECTURE +
ADR-INDEX + ROADMAP). 'sirsi prd sync' keeps it honest — well-formed, valid
statuses, canon references present — and surfaces the open items + drift so the
PRD reflects reality rather than rotting.`,
}

var prdSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Reconcile each PRD against canon: validate + surface open items and drift",
	Long: `Load every .agents/prd/<agent>.json (or one with --agent), then:
  • validate it (well-formed, statuses in open|done|owner-gated|blocked, no dup IDs)
  • check its canon_refs resolve to files in the repo (a PRD pointing at missing
    canon is stale)
  • summarize task status and list what's still open

Read-only. Exits non-zero if any PRD has a problem, so it can gate CI/hooks.

(Next iteration: semantic derivation of tasks from the ROADMAP text — pending a
canon↔task linkage convention; auto-guessing status would silently mis-state work.)`,
	RunE: runPRDSync,
}

type prdReport struct {
	Agent    string      `json:"agent"`
	File     string      `json:"file"`
	Summary  prd.Summary `json:"summary"`
	Problems []string    `json:"problems"`
}

func runPRDSync(_ *cobra.Command, _ []string) error {
	root, prdDir, err := findPRDDir()
	if err != nil {
		return err
	}

	var files []string
	if prdSyncAgent != "" {
		files = []string{filepath.Join(prdDir, prdSyncAgent+".json")}
	} else {
		files, _ = filepath.Glob(filepath.Join(prdDir, "*.json"))
		sort.Strings(files)
	}
	if len(files) == 0 {
		return fmt.Errorf("no PRDs found in %s", prdDir)
	}

	var reports []prdReport
	problemsTotal := 0
	for _, f := range files {
		p, err := prd.Load(f)
		if err != nil {
			reports = append(reports, prdReport{File: filepath.Base(f), Problems: []string{err.Error()}})
			problemsTotal++
			continue
		}
		probs := prd.Validate(p, root)
		problemsTotal += len(probs)
		reports = append(reports, prdReport{Agent: p.Agent, File: filepath.Base(f), Summary: prd.Summarize(p), Problems: probs})
	}

	if prdSyncJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(reports)
	}

	for _, r := range reports {
		name := r.Agent
		if name == "" {
			name = r.File
		}
		state := "✓ reconciled"
		if len(r.Problems) > 0 {
			state = fmt.Sprintf("⚠ %d problem(s)", len(r.Problems))
		} else if r.Summary.Complete {
			state = "✓ complete"
		}
		fmt.Printf("\n  𓁢 %s — %s\n", name, state)
		if r.Summary.Total > 0 {
			fmt.Printf("     %d task(s): %s\n", r.Summary.Total, statusLine(r.Summary.ByStatus))
			if len(r.Summary.OpenIDs) > 0 {
				fmt.Printf("     open: %s\n", strings.Join(r.Summary.OpenIDs, ", "))
			}
		}
		for _, pb := range r.Problems {
			fmt.Printf("       ✗ %s\n", pb)
		}
	}
	fmt.Println()

	if problemsTotal > 0 {
		return fmt.Errorf("%d PRD problem(s) found", problemsTotal)
	}
	return nil
}

// statusLine renders the status counts in a stable order.
func statusLine(by map[string]int) string {
	var parts []string
	for _, st := range []string{"open", "blocked", "owner-gated", "done"} {
		if n := by[st]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, st))
		}
	}
	return strings.Join(parts, " · ")
}

// findPRDDir walks up from the working directory to the repo containing
// .agents/prd, returning the repo root and that directory.
func findPRDDir() (root, prdDir string, err error) {
	dir, e := os.Getwd()
	if e != nil {
		return "", "", e
	}
	start := dir
	for {
		cand := filepath.Join(dir, ".agents", "prd")
		if fi, e := os.Stat(cand); e == nil && fi.IsDir() {
			return dir, cand, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf(".agents/prd not found from %s upward", start)
		}
		dir = parent
	}
}

func init() {
	prdSyncCmd.Flags().StringVar(&prdSyncAgent, "agent", "", "reconcile only this agent's PRD (default: all)")
	prdSyncCmd.Flags().BoolVar(&prdSyncJSON, "json", false, "machine-readable output")
	prdCmd.AddCommand(prdSyncCmd)
	rootCmd.AddCommand(prdCmd)
}
