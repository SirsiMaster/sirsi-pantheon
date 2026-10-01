package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

var (
	pingAll  bool
	pingJSON bool
)

// pingLane resolves a retired name to its declared successor (ADR-072 C5), then
// computes the lane's honest availability. It reads state only.
func pingLane(f *dispatch.Facade, routerRoot, name string, reg *router.Registry, threads *router.ThreadRegistry) (router.PingResult, string) {
	target := name
	if aliases, err := f.Aliases(); err == nil {
		if succ, ok := aliases[name]; ok {
			target = succ
		}
	}
	cfg, declared := reg.Agents[target]
	if !declared {
		return router.PingResult{Agent: target, Verdict: router.VerdictUnstaffed, Detail: "not a declared agent"}, target
	}
	return router.PingLane(threads, cfg, target, time.Now().UTC()), target
}

var routerPingCmd = &cobra.Command{
	Use:   "ping <lane> | --all",
	Short: "Can this lane actually work right now? (reads published lane state; no model call)",
	Long: `Reports whether a recipient can be expected to work an item you send it, from the
honest state each worker publishes on its heartbeat — not from "a launch job is
installed". Verdicts:

  LIVE           an attended session is consuming the inbox now
  WAKEABLE       a worker loop with a working consumer will start a session
  HELD           would work but is gated now (measurement window, load, back-off,
                 relay unreachable, quarantine); the detail says until when
  AUTH_REQUIRED  the consumer cannot log in
  WATCH_ONLY     a wake loop exists but declares no consumer
  UNSTAFFED      no worker path at all
  UNREACHABLE    its worker stopped reporting

A retired name resolves to its declared successor.

  sirsi router ping codex-finalwishes
  sirsi router ping --all`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !pingAll && len(args) != 1 {
			return fmt.Errorf("give a lane, or --all")
		}
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no .agents/idea-router/ found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		f, err := dispatch.Open(repoRoot)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		reg, err := router.LoadRegistry(routerRoot)
		if err != nil {
			return err
		}
		threads, err := router.LoadThreadRegistry(routerRoot)
		if err != nil {
			return err
		}
		var out []router.PingResult
		if pingAll {
			for id, cfg := range reg.Agents {
				if cfg.Type == "human" {
					continue
				}
				out = append(out, router.PingLane(threads, cfg, id, time.Now().UTC()))
			}
			rank := map[string]int{router.VerdictUnreachable: 0, router.VerdictAuthRequired: 1, router.VerdictWatchOnly: 2, router.VerdictUnstaffed: 3, router.VerdictHeld: 4, router.VerdictWakeable: 5, router.VerdictLive: 6}
			sort.Slice(out, func(i, j int) bool {
				if rank[out[i].Verdict] != rank[out[j].Verdict] {
					return rank[out[i].Verdict] < rank[out[j].Verdict]
				}
				return out[i].Agent < out[j].Agent
			})
		} else {
			r, _ := pingLane(f, routerRoot, args[0], reg, threads)
			out = []router.PingResult{r}
		}
		if pingJSON {
			b, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(b))
			return nil
		}
		for _, r := range out {
			fmt.Printf("  %-13s %-26s %s\n", r.Verdict, r.Agent, r.Detail)
		}
		return nil
	},
}

// printSendVerdict tells the sender, right after a send, whether the recipient
// can be expected to work it. It never fails the send.
func printSendVerdict(f *dispatch.Facade, repoRoot, to string) {
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	reg, err := router.LoadRegistry(routerRoot)
	if err != nil {
		return
	}
	threads, err := router.LoadThreadRegistry(routerRoot)
	if err != nil {
		return
	}
	r, target := pingLane(f, routerRoot, to, reg, threads)
	note := ""
	if target != to {
		note = fmt.Sprintf(" (%s → %s)", to, target)
	}
	// stderr, never stdout: callers parse `send`'s stdout for the item id, and a
	// second line there broke them (TestRouterRespondStoreOnlyItem).
	fmt.Fprintf(os.Stderr, "    recipient %s%s: %s — %s\n", target, note, r.Verdict, r.Detail)
}

func init() {
	routerPingCmd.Flags().BoolVar(&pingAll, "all", false, "Ping every declared non-human lane, worst first")
	routerPingCmd.Flags().BoolVar(&pingJSON, "json", false, "Machine-readable output")
	routerCmd.AddCommand(routerPingCmd)
}
