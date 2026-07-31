package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// `sirsi gemma reap-orphans` — the lever for the "Duplicate Model Brokers"
// finding (ADR-033: an alarming check ships something that acts on what it
// alarmed about).
//
// A second broker serves nothing: nothing routes to it, so its pages get pushed
// into swap and sit there. Observed live 2026-07-30 — an orphan holding 14.9 GB
// of footprint against 0.1 GB RSS while swap sat at 93% used. Stopping it
// released ~15 GB and macOS shrank the swap file from 18,432 MB to 3,072 MB.
//
// Safety, in the order that matters (PANTHEON_RULES.md A32 — do no harm to the
// running host): the CANONICAL broker is the one named by ~/.sirsi/gemma-server.pid,
// and it is never a candidate. Everything else is SIGTERMed with a real grace
// window before escalating. Dry-run by default.
var gemmaReapOrphansApply bool

var gemmaReapOrphansCmd = &cobra.Command{
	Use:   "reap-orphans",
	Short: "Stop duplicate model brokers — the canonical one (per pidfile) is never touched (dry-run by default)",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		brokers, err := guard.FindModelBrokers(platform.Current())
		if err != nil {
			return fmt.Errorf("discover brokers: %w", err)
		}
		canonical := canonicalBrokerPID(home)
		me := os.Getpid()

		var orphans []guard.BrokerProc
		for _, b := range brokers {
			if b.PID == canonical || b.PID == me {
				continue
			}
			orphans = append(orphans, b)
		}

		if canonical == 0 {
			// No pidfile means no way to tell which broker is load-bearing. Refuse
			// rather than guess — killing the only live broker would take the
			// machine's local intelligence offline to fix a duplication that may
			// not exist.
			fmt.Println("𓁟 no canonical broker in ~/.sirsi/gemma-server.pid — refusing to guess which broker is load-bearing")
			return nil
		}
		if len(orphans) == 0 {
			fmt.Printf("𓁟 one broker running (pid %d, canonical) — nothing to reap\n", canonical)
			return nil
		}

		verb := "WOULD-STOP"
		if gemmaReapOrphansApply {
			verb = "STOPPING"
		}
		stopped := 0
		for _, o := range orphans {
			fmt.Printf("%s pid=%d %.1f GB%s (canonical pid %d is untouched)\n",
				verb, o.PID, float64(o.Worst)/(1<<30), o.Note, canonical)
			if !gemmaReapOrphansApply {
				continue
			}
			_ = syscall.Kill(o.PID, syscall.SIGTERM)
			// A real grace window, not a decorative one: re-check after waiting.
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				if syscall.Kill(o.PID, 0) != nil {
					break
				}
				time.Sleep(time.Second)
			}
			if syscall.Kill(o.PID, 0) == nil {
				_ = syscall.Kill(o.PID, syscall.SIGKILL)
				time.Sleep(time.Second)
			}
			if syscall.Kill(o.PID, 0) != nil {
				stopped++
			} else {
				fmt.Printf("  ✘ pid %d survived SIGTERM+SIGKILL\n", o.PID)
			}
		}
		fmt.Printf("reaped %d/%d orphan broker(s); apply=%v\n", stopped, len(orphans), gemmaReapOrphansApply)
		return nil
	},
}

// canonicalBrokerPID reads the pidfile and returns the pid only if it is alive.
func canonicalBrokerPID(home string) int {
	raw, err := os.ReadFile(filepath.Join(home, ".sirsi", "gemma-server.pid")) //nolint:gosec // fixed path
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return 0
	}
	if syscall.Kill(pid, 0) != nil {
		return 0 // stale pidfile — do not treat a dead pid as canonical
	}
	return pid
}

func init() {
	gemmaReapOrphansCmd.Flags().BoolVar(&gemmaReapOrphansApply, "apply", false, "actually stop the orphan brokers")
	gemmaCmd.AddCommand(gemmaReapOrphansCmd)
}
