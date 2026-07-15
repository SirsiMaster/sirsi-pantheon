package main

// sirsi activity — the trust ledger surface (TUI design proof gap V4).
//
// Every destructive operation (clean, purge) is appended to
// ~/Library/Logs/sirsi/operations.log by internal/oplog. That file is free
// text; internal/oplog.ReadLast is the ONE parser of it, and this command is
// the CLI contract every interactive surface (TUI Activity screen, menubar,
// dashboard) reads — so no surface ever parses log lines privately.

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/oplog"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seba"
)

var activityLimit int

// Injectable log path (Rule A16/A21) so tests read a fixture ledger, not the
// user's real one. Swapped under activityMu by tests — which must NOT use
// t.Parallel() (package-global swap; repo lessons #129/#131).
var (
	activityMu     sync.Mutex
	activityPathFn = oplog.Path
)

// activityReport is the `sirsi activity --json` contract: the last N
// operations, newest first, parsed once in Go (never free text on the wire).
type activityReport struct {
	Command string        `json:"command"`
	LogPath string        `json:"log_path"`
	Count   int           `json:"count"`
	Entries []oplog.Entry `json:"entries"`
}

var activityCmd = &cobra.Command{
	Use:   "activity",
	Short: "Recent operations — what sirsi actually changed (newest first)",
	Long: `Activity is the trust ledger: every destructive operation sirsi performs
(clean, purge) is recorded to ~/Library/Logs/sirsi/operations.log. This
command reads it back, newest first.

  sirsi activity              Last 20 operations
  sirsi activity --limit 5    Fewer (or more) entries
  sirsi activity --json       Structured output for tools and dashboards

Opt out of the ledger entirely with SIRSI_NO_OPLOG=1.`,
	RunE: runActivity,
}

func init() {
	activityCmd.Flags().IntVar(&activityLimit, "limit", 20, "Maximum entries to show (0 = all)")
}

func runActivity(cmd *cobra.Command, args []string) error {
	activityMu.Lock()
	pathFn := activityPathFn
	activityMu.Unlock()
	logPath := pathFn()

	entries, err := oplog.ReadLast(logPath, activityLimit)
	if err != nil {
		return fmt.Errorf("activity: reading operations log: %w", err)
	}

	if JsonOutput {
		report := activityReport{
			Command: "sirsi activity",
			LogPath: logPath,
			Count:   len(entries),
			Entries: entries,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	output.Header("Activity — Operations Ledger")

	if len(entries) == 0 {
		output.Info("No operations recorded yet. The ledger fills as sirsi cleans (sirsi clean --confirm) or purges.")
		output.Dim("Ledger: %s", output.ShortenPath(logPath))
		return nil
	}

	var rows [][]string
	for _, e := range entries {
		size := "—"
		if e.Bytes > 0 {
			size = seba.FormatBytes(e.Bytes)
		}
		rows = append(rows, []string{e.Time, e.Action, output.ShortenPath(e.Target), size})
	}
	output.Table([]string{"TIME", "ACTION", "TARGET", "SIZE"}, rows)

	output.Dim("%d operations (newest first) · ledger: %s", len(entries), output.ShortenPath(logPath))
	return nil
}
