package main

// workboardcmd.go — `sirsi router workboard`: the Work Board (owner directive
// 2026-07-17). One glance answers: what are MY work packages, who are my
// peers and on what surfaces, and what pace is the fabric moving at.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

var workboardAgent string

var workboardCmd = &cobra.Command{
	Use:   "workboard",
	Short: "The Work Board — every agent's packages, peers, and the pace of the fabric",
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("locate repo root: %w", err)
		}
		board, err := router.ComputeWorkBoard(filepath.Join(repoRoot, ".agents", "idea-router"))
		if err != nil {
			return err
		}
		if JsonOutput {
			return json.NewEncoder(os.Stdout).Encode(board)
		}

		fmt.Printf("𓁢 Work Board — %d open package(s) · pace: %d closed today, %d this week",
			board.TotalOpen, board.ClosedToday, board.Closed7d)
		if board.AvgCloseHours > 0 {
			fmt.Printf(" · avg turnaround %s", humanHours(board.AvgCloseHours))
		}
		fmt.Println()
		fmt.Println()

		// Distinguish "nothing to do" from "couldn't read" — the board's job is
		// a positive claim ("nothing needs you right now"), so an empty result
		// after a successful read must say so explicitly. A failure surfaces as
		// an error above; reaching here with no agents IS the healthy-empty state.
		if len(board.Agents) == 0 {
			fmt.Println("    — none — fabric healthy")
			return nil
		}

		for _, a := range board.Agents {
			if workboardAgent != "" && a.AgentID != workboardAgent {
				continue
			}
			live := "○ offline"
			if a.Live {
				live = "● live (" + strings.Join(a.Surfaces, ", ") + ")"
			}
			pace := fmt.Sprintf("%d today / %d this week", a.ClosedToday, a.Closed7d)
			if a.AvgCloseHours > 0 {
				pace += " · avg " + humanHours(a.AvgCloseHours)
			}
			fmt.Printf("%s — %s · closed %s\n", a.AgentID, live, pace)
			if len(a.Open) == 0 {
				fmt.Println("    (no open packages)")
			}
			for _, p := range a.Open {
				fmt.Printf("    • [%s] %s  (from %s)\n", humanHours(p.AgeHours), p.Title, p.From)
			}
			fmt.Println()
		}
		return nil
	},
}

func humanHours(h float64) string {
	switch {
	case h < 1:
		return fmt.Sprintf("%.0fm", h*60)
	case h < 48:
		return fmt.Sprintf("%.1fh", h)
	default:
		return fmt.Sprintf("%.1fd", h/24)
	}
}

func init() {
	workboardCmd.Flags().StringVar(&workboardAgent, "agent", "", "Show only this agent's board")
	routerCmd.AddCommand(workboardCmd)
}
