package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

var (
	threadQuarThread     string
	threadQuarStale      bool
	threadQuarStaleAfter time.Duration
	threadQuarDryRun     bool
	threadQuarReason     string
	threadUnquarThread   string
)

var threadQuarantineCmd = &cobra.Command{
	Use:   "quarantine",
	Short: "Park stale thread records reversibly (never reaps, closes or deletes)",
	Long: `Parks stale threads in the "quarantined" status. The record is kept intact,
with who/when/why and its prior status, and its last_seen_at is preserved. A
quarantined thread is excluded from live/stale counts, attended-session
detection and wake targeting, and is listed separately by
"sirsi router doctor" and "sirsi thread list --quarantined".

A heartbeat or re-register from the same thread releases it automatically
("released from quarantine: proved alive"), so a live session is never cut off.
Threads whose heartbeat is fresh, or whose process is alive, are refused.
Only records owned by this host are changed. Undo with "sirsi thread unquarantine".`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if (threadQuarThread == "") == !threadQuarStale {
			return errors.New("choose exactly one scope: --thread <id> or --stale")
		}
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		root := filepath.Join(repoRoot, ".agents", "idea-router")
		changes, err := router.QuarantineThreads(root, threadQuarThread, threadQuarStaleAfter, quarantineActor(), threadQuarReason, threadQuarDryRun)
		if err != nil {
			return err
		}
		return renderQuarantine(cmd.OutOrStdout(), changes, threadQuarDryRun)
	},
}

var threadUnquarantineCmd = &cobra.Command{
	Use:   "unquarantine",
	Short: "Release a quarantined thread back to its prior status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if threadUnquarThread == "" {
			return errors.New("--thread is required")
		}
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		t, err := router.UnquarantineThread(filepath.Join(repoRoot, ".agents", "idea-router"), threadUnquarThread)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "released %s (%s) from quarantine, status=%s\n", t.ThreadID, t.AgentID, t.Status)
		return nil
	},
}

// quarantineActor names who ran the quarantine for the audit note.
func quarantineActor() string {
	who := strings.TrimSpace(os.Getenv("SIRSI_AGENT_ID"))
	if who == "" {
		who = "operator"
	}
	if id := strings.TrimSpace(os.Getenv("SIRSI_THREAD_ID")); id != "" {
		who += "/" + id
	}
	return who
}

func renderQuarantine(w io.Writer, changes []router.QuarantineChange, dryRun bool) error {
	if JsonOutput {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"dry_run": dryRun, "changes": changes})
	}
	if len(changes) == 0 {
		fmt.Fprintln(w, "No stale threads to quarantine.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ACTION\tTHREAD\tAGENT\tHOST\tSURFACE\tPRIOR\tIDLE")
	for _, c := range changes {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", c.Action, c.ThreadID, c.AgentID, displayUnknown(c.Host), c.Surface, c.PriorStatus,
			(time.Duration(c.IdleSeconds) * time.Second).String())
	}
	return tw.Flush()
}

func init() {
	threadQuarantineCmd.Flags().StringVar(&threadQuarThread, "thread", "", "Quarantine exactly this thread (refused unless stale)")
	threadQuarantineCmd.Flags().BoolVar(&threadQuarStale, "stale", false, "Quarantine every currently stale thread owned by this host")
	threadQuarantineCmd.Flags().DurationVar(&threadQuarStaleAfter, "stale-after", router.DefaultQuarantineStaleAfter, "Heartbeat must be quiet at least this long")
	threadQuarantineCmd.Flags().BoolVar(&threadQuarDryRun, "dry-run", false, "List what would change; write nothing")
	threadQuarantineCmd.Flags().StringVar(&threadQuarReason, "reason", "stale heartbeat", "Why (recorded on the thread)")
	threadUnquarantineCmd.Flags().StringVar(&threadUnquarThread, "thread", "", "Thread ID to release (required)")
	threadCmd.AddCommand(threadQuarantineCmd, threadUnquarantineCmd)
}
