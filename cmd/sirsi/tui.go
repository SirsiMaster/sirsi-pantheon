package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/SirsiMaster/sirsi-pantheon/internal/tui"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the full-screen terminal app (the TUI surface)",
	Long: `Open Pantheon's full-screen terminal console: live Threads and Router
Inbox views, fully keyboard-driven.

  ↑/↓ move · enter inspect · tab next view · / filter · r refresh · q quit

Every surface is a face over the same engine — switch anytime with
'sirsi surface use <cli|tui|gui|ide>'.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		// Launching the TUI makes it the active surface.
		_ = setup.SaveActiveSurface(setup.SurfaceTUI)
		// Real data when we can resolve the router; fixtures otherwise.
		return tui.RunViews(liveViews())
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

// liveViews builds the console's views from real workstation state — the live
// thread registry and the router inbox. Returns nil when the router can't be
// resolved (e.g. run outside a repo), letting the TUI fall back to fixtures.
func liveViews() []tui.View {
	root, err := router.FindRepoRoot()
	if err != nil {
		return nil
	}
	routerRoot := filepath.Join(root, ".agents", "idea-router")

	var views []tui.View
	if v := threadsView(routerRoot); v != nil {
		views = append(views, v)
	}
	if v := inboxView(routerRoot); v != nil {
		views = append(views, v)
	}
	return views
}

// threadsView is the live "who is alive on this workstation" screen.
func threadsView(routerRoot string) tui.View {
	treg, err := router.LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil
	}
	rows := make([][]string, 0)
	for _, t := range treg.SortedThreads() {
		// Live console = live nodes: only genuinely active threads. Suspended
		// and terminal (reaped/closed) records dominate the registry but aren't
		// "alive right now".
		if string(t.Status) != "active" {
			continue
		}
		idle := "—"
		if !t.LastSeenAt.IsZero() {
			idle = time.Since(t.LastSeenAt).Round(time.Second).String()
		}
		rows = append(rows, []string{
			t.AgentID,
			t.Surface,
			string(t.Status),
			idle,
			fmt.Sprintf("%d", t.PID),
		})
	}
	return tui.NewView("Threads", tui.LayoutStream,
		[]tui.CommandID{tui.CmdMoveUp, tui.CmdRefresh, tui.CmdFilter, tui.CmdBack},
		tui.Table{
			Columns: []tui.Column{
				{Title: "AGENT", Width: 20, Align: tui.AlignLeft},
				{Title: "SURFACE", Width: 8, Align: tui.AlignLeft},
				{Title: "STATUS", Width: 10, Align: tui.AlignLeft},
				{Title: "IDLE", Width: 10, Align: tui.AlignRight},
				{Title: "PID", Width: 7, Align: tui.AlignRight},
			},
			Rows: rows,
		})
}

// inboxView is the live router inbox — every open work item on this
// workstation, read from items/ the same way `sirsi router pull` does.
func inboxView(routerRoot string) tui.View {
	items, err := work.ListAll(routerRoot)
	if err != nil {
		return nil
	}
	rows := make([][]string, 0)
	for _, it := range items {
		if it.Status != "open" {
			continue
		}
		typ := it.Type
		if typ == "" {
			typ = "—"
		}
		rows = append(rows, []string{
			it.From + " → " + it.To,
			it.Title,
			typ,
		})
	}
	return tui.NewView("Router Inbox", tui.LayoutInspect,
		[]tui.CommandID{tui.CmdMoveUp, tui.CmdInspect, tui.CmdRefresh, tui.CmdFilter},
		tui.Table{
			Columns: []tui.Column{
				{Title: "FROM → TO", Width: 26, Align: tui.AlignLeft},
				{Title: "TITLE", Width: 40, Align: tui.AlignLeft},
				{Title: "TYPE", Width: 9, Align: tui.AlignLeft},
			},
			Rows: rows,
		})
}
