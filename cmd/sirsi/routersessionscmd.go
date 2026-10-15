package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Service-side session table tools (task ra-session-table-gc). Like `router token`, these run against the SERVICE's own backend
// (a Cloud Run job passes --store), never over the wire.
var routerSessionsStore string

var routerSessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Inspect and garbage-collect the service's session table (run against the service backend)",
}

func openSessionStore() (*routerstore.SQLiteStore, error) {
	store, err := openServeStore(routerSessionsStore)
	if err != nil {
		return nil, err
	}
	s, ok := store.(*routerstore.SQLiteStore)
	if !ok {
		_ = store.Close()
		return nil, errors.New("sessions: this backend does not expose the session table")
	}
	return s, nil
}

var routerSessionsStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Row counts: total, live (seen < 1h), revoked, idle, and how many a GC would delete now",
	RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := openSessionStore()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		idle, _ := cmd.Flags().GetDuration("idle")
		st, err := s.SessionStats(idle)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "sessions total=%d live(1h)=%d revoked=%d idle(>%s)=%d collectable=%d\n", st.Total, st.Live, st.Revoked, idle, st.Idle, st.Collectable)
		for _, h := range st.ByHost {
			fmt.Fprintf(out, "  host %-28s %d\n", h.Host, h.Count)
		}
		return nil
	},
}

var routerSessionsGCCmd = &cobra.Command{
	Use:   "gc",
	Short: "Delete dead sessions in bounded batches (never one that owns a lease on an open item or unfinished task)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := openSessionStore()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		idle, _ := cmd.Flags().GetDuration("idle")
		limit, _ := cmd.Flags().GetInt("limit")
		batches, _ := cmd.Flags().GetInt("max-batches")
		dry, _ := cmd.Flags().GetBool("dry-run")
		out := cmd.OutOrStdout()
		before, err := s.SessionStats(idle)
		if err != nil {
			return err
		}
		if dry {
			fmt.Fprintf(out, "dry run: would delete %d of %d sessions (idle > %s); nothing changed\n", before.Collectable, before.Total, idle)
			return nil
		}
		var total int64
		for i := 0; i < batches; i++ {
			n, err := s.GCSessions(idle, limit)
			if err != nil {
				return err
			}
			total += n
			if n < int64(limit) {
				break
			}
		}
		after, err := s.SessionStats(idle)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "deleted %d sessions; total %d -> %d (collectable remaining %d)\n", total, before.Total, after.Total, after.Collectable)
		return nil
	},
}

func init() {
	routerSessionsCmd.PersistentFlags().StringVar(&routerSessionsStore, "store", os.Getenv("SIRSI_ROUTER_STORE"), "postgres:// DSN or SQLite path of the SERVICE's backend (required; default $SIRSI_ROUTER_STORE)")
	routerSessionsStatsCmd.Flags().Duration("idle", 24*time.Hour, "idle threshold")
	routerSessionsGCCmd.Flags().Duration("idle", 24*time.Hour, "delete sessions unseen (or revoked) for longer than this")
	routerSessionsGCCmd.Flags().Int("limit", 5000, "rows per batch")
	routerSessionsGCCmd.Flags().Int("max-batches", 40, "stop after this many batches")
	routerSessionsGCCmd.Flags().Bool("dry-run", false, "count what would be deleted; change nothing")
	routerSessionsCmd.AddCommand(routerSessionsStatsCmd, routerSessionsGCCmd)
	routerCmd.AddCommand(routerSessionsCmd)
}
