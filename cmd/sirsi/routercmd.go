package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
	"github.com/spf13/cobra"
)

// workRoot resolves the router directory for the current repo without
// creating it. Read-only verbs (status, pull, show) use this so audits and
// sandboxed checks don't materialize an items/ directory as a side effect.
func workRoot() (string, error) {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return "", fmt.Errorf("no .agents/idea-router/ found: %w", err)
	}
	return filepath.Join(repoRoot, ".agents", "idea-router"), nil
}

// workRootEnsure is workRoot plus mkdir of items/. Writers (send, close) use it.
func workRootEnsure() (string, error) {
	root, err := workRoot()
	if err != nil {
		return "", err
	}
	if err := work.EnsureRoot(root); err != nil {
		return "", err
	}
	return root, nil
}

// loadOrLiteral returns the literal value, or the contents of the file if it
// starts with @. Lets callers pass --instructions "text" or --instructions @file.
func loadOrLiteral(v string) (string, error) {
	if strings.HasPrefix(v, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(v, "@"))
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return v, nil
}

var routerCmd = &cobra.Command{
	Use:   "router",
	Short: "Pull-model work queue between agent threads",
	Long: `Five verbs over a directory of markdown files: send, pull, show,
close, and status. send/pull/show/close are the workflow loop; status is a
read-only observer over items/. No daemon, no dispatch, no launch agents —
each agent session reads its own inbox on wake.

Thread registration is handled separately by sirsi thread register.`,
}

var statusStaleHours int

var routerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Summarize the work queue (read-only)",
	Long: `Read-only summary over items/. Does not create the directory if it is
missing — safe to run in sandboxed or audit-only contexts. Use --stale to
list open items older than N hours (default 24).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := workRoot()
		if err != nil {
			return err
		}
		all, err := work.ListAll(root)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		threshold := time.Duration(statusStaleHours) * time.Hour
		var open, closed int
		perAgent := map[string]int{}
		type openItem struct {
			it  work.Item
			age time.Duration
		}
		var opens []openItem
		for _, it := range all {
			if it.Status == "closed" {
				closed++
				continue
			}
			open++
			perAgent[it.To]++
			age := time.Duration(0)
			if t, perr := time.Parse(time.RFC3339, it.Opened); perr == nil {
				age = now.Sub(t)
			}
			opens = append(opens, openItem{it, age})
		}
		fmt.Printf("  Items: %d open, %d closed\n", open, closed)
		if open > 0 {
			fmt.Println("\n  Open by recipient:")
			recipients := make([]string, 0, len(perAgent))
			for a := range perAgent {
				recipients = append(recipients, a)
			}
			sort.Strings(recipients)
			for _, agent := range recipients {
				fmt.Printf("    %s: %d\n", agent, perAgent[agent])
			}
		}
		// Oldest open item is always useful — surfaces a stuck queue without flags.
		if len(opens) > 0 {
			sort.Slice(opens, func(i, j int) bool { return opens[i].age > opens[j].age })
			oldest := opens[0]
			fmt.Printf("\n  Oldest open: %s (%s, → %s)\n", humanAge(oldest.age), oldest.it.ID, oldest.it.To)
		}
		// --stale lists items past the threshold (default 24h).
		if statusStaleHours > 0 {
			var stale []openItem
			for _, o := range opens {
				if o.age >= threshold {
					stale = append(stale, o)
				}
			}
			if len(stale) > 0 {
				fmt.Printf("\n  Stale (>%dh):\n", statusStaleHours)
				for _, o := range stale {
					fmt.Printf("    • %s  age=%s  → %s\n", o.it.ID, humanAge(o.age), o.it.To)
				}
			}
		}
		return nil
	},
}

// humanAge renders a duration as a compact "5h12m" or "3d4h" string.
func humanAge(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	days := int(d / (24 * time.Hour))
	hours := int(d%(24*time.Hour)) / int(time.Hour)
	minutes := int(d%time.Hour) / int(time.Minute)
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

var (
	sendFrom         string
	sendTo           string
	sendTitle        string
	sendType         string
	sendInstructions string
)

var routerSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a work item from one agent to another",
	Long: `Writes a new open work item under .agents/idea-router/items/. The
recipient picks it up next time they run sirsi router pull <their-id>.

  sirsi router send --from claude-pantheon --to codex-pantheon \
    --title "review canon-sync" --instructions @proposal.md`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if sendFrom == "" || sendTo == "" {
			return fmt.Errorf("--from and --to are required")
		}
		if sendTitle == "" {
			return fmt.Errorf("--title is required")
		}
		instr, err := loadOrLiteral(sendInstructions)
		if err != nil {
			return fmt.Errorf("--instructions: %w", err)
		}
		root, err := workRootEnsure()
		if err != nil {
			return err
		}
		id, err := work.SendTyped(root, sendFrom, sendTo, sendTitle, sendType, instr)
		if err != nil {
			return err
		}
		fmt.Printf("  Sent %s → %s: %s\n", sendFrom, sendTo, id)
		return nil
	},
}

var routerPullCmd = &cobra.Command{
	Use:   "pull <agent>",
	Short: "Pull open work items addressed to an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := workRoot()
		if err != nil {
			return err
		}
		items, err := work.ListInbox(root, args[0])
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Printf("  No open items for %s.\n", args[0])
			return nil
		}
		fmt.Printf("  %d open items for %s:\n\n", len(items), args[0])
		for _, it := range items {
			fmt.Printf("  • %s\n      from: %s\n      title: %s\n      opened: %s\n\n", it.ID, it.From, it.Title, it.Opened)
		}
		fmt.Printf("  Read full: sirsi router show <id>\n")
		fmt.Printf("  Close when done: sirsi router close <id> --result @path/to/result.md\n")
		return nil
	},
}

var routerShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Print the full text of a work item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := workRoot()
		if err != nil {
			return err
		}
		path := filepath.Join(root, "items", args[0]+".md")
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read item: %w", err)
		}
		_, _ = os.Stdout.Write(data)
		return nil
	},
}

var routerAckCmd = &cobra.Command{
	Use:   "ack <agent> <id> [<id> ...]",
	Short: "Acknowledge legacy state.json pending entries",
	Long: `Removes one or more legacy pending stems from state.json for an agent.
This is a migration helper for dispatchers while item files are becoming the
canonical queue. It is idempotent and does not close items/*.md.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := workRoot()
		if err != nil {
			return err
		}
		if err := ackLegacyPending(root, args[0], args[1:]); err != nil {
			return err
		}
		fmt.Printf("  Acked %d legacy pending item(s) for %s\n", len(args)-1, args[0])
		return nil
	},
}

func ackLegacyPending(root, agent string, ids []string) error {
	statePath := filepath.Join(root, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return fmt.Errorf("read state.json: %w", err)
	}

	var state map[string]any
	if jerr := json.Unmarshal(data, &state); jerr != nil {
		return fmt.Errorf("parse state.json: %w", jerr)
	}

	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			idSet[id] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return nil
	}

	if pending, ok := state["pending"].(map[string]any); ok {
		pending[agent] = removeLegacyIDs(pending[agent], idSet)
	}
	for _, key := range []string{"pending_for_codex", "pending_for_claude"} {
		state[key] = removeLegacyIDs(state[key], idSet)
	}

	state[legacyReadKey(agent)] = time.Now().UTC().Format(time.RFC3339)

	updated, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state.json: %w", err)
	}
	updated = append(updated, '\n')
	return os.WriteFile(statePath, updated, 0o644)
}

func removeLegacyIDs(value any, ids map[string]struct{}) []any {
	list, ok := value.([]any)
	if !ok {
		return []any{}
	}
	out := make([]any, 0, len(list))
	for _, v := range list {
		s, ok := v.(string)
		if !ok {
			out = append(out, v)
			continue
		}
		if _, drop := ids[s]; !drop {
			out = append(out, s)
		}
	}
	return out
}

func legacyReadKey(agent string) string {
	family := agent
	if before, _, ok := strings.Cut(agent, "-"); ok {
		family = before
	}
	switch family {
	case "claude":
		return "last_claude_read"
	case "codex":
		return "last_codex_read"
	default:
		return "last_" + strings.ReplaceAll(agent, "-", "_") + "_read"
	}
}

var closeResult string

var routerCloseCmd = &cobra.Command{
	Use:   "close <id>",
	Short: "Mark a work item closed",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := loadOrLiteral(closeResult)
		if err != nil {
			return fmt.Errorf("--result: %w", err)
		}
		root, err := workRootEnsure()
		if err != nil {
			return err
		}
		if err := work.Close(root, args[0], result); err != nil {
			return err
		}
		fmt.Printf("  Closed %s\n", args[0])
		return nil
	},
}

// routerWakeInstallCmd installs the per-agent pull-loop LaunchAgent (PR#2,
// constraint 2). The loop polls the agent's inbox and heartbeats on a bounded
// interval — a pull-loop watcher armed by heartbeat freshness, not the
// loop-monitor pgrep gate. Idempotent: re-running reports "already installed".
var routerWakeInstallCmd = &cobra.Command{
	Use:   "wake-install <agent>",
	Short: "Install a worker/headless agent's pull-loop wake LaunchAgent (macOS)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := workRoot()
		if err != nil {
			return err
		}
		reg, err := router.LoadRegistry(root)
		if err != nil {
			return fmt.Errorf("load agents: %w", err)
		}
		cfg, err := reg.Lookup(args[0])
		if err != nil {
			return err
		}
		changed, path, err := router.InstallWakeLaunchAgent(*cfg, "")
		if err != nil {
			return err
		}
		if changed {
			fmt.Printf("✔ Installed wake LaunchAgent: %s\n", path)
			fmt.Printf("  Load it: launchctl load -w %s\n", path)
		} else {
			fmt.Printf("✓ Wake LaunchAgent already installed (no change): %s\n", path)
		}
		return nil
	},
}

// routerWakeLoopCmd runs a worker/headless agent's bounded pull-loop (A27). It is
// the long-lived foreground loop the wake LaunchAgent (`router wake-install`)
// invokes via KeepAlive — NOT a self-daemonizing verb (it blocks; launchd owns
// the lifecycle). Hidden because it is machine-run, not a human-facing command.
// It registers a concrete pull-loop thread, heartbeats each interval, and closes
// the thread on SIGINT/SIGTERM.
var routerWakeLoopCmd = &cobra.Command{
	Use:    "wake-loop <agent>",
	Short:  "Run a worker agent's bounded pull-loop (machine-run by the wake LaunchAgent)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := workRootEnsure()
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		return router.RunWakeLoop(ctx, root, args[0], router.DefaultWakeLoopInterval)
	},
}

// routerInstallDaemonsCmd idempotently (re)installs the router helper
// LaunchAgents node-status reports as missing (com.sirsi.idea-router,
// com.sirsi.idea-router-sweep, ai.sirsi.registry-police) for this clone. These
// relay work between agents while no session is open; when one is missing, work
// silently strands. Permissions/installs belong in setup — this is the same
// installer `sirsi setup` runs, exposed as a direct verb for repair. macOS only.
var routerInstallDaemonsCmd = &cobra.Command{
	Use:   "install-daemons",
	Short: "Install the router helper LaunchAgents (dispatch, sweep, registry-police) — macOS",
	Long: `Idempotently (re)installs the three router helper LaunchAgents this clone
relies on to relay work while no session is open:

  com.sirsi.idea-router        FSEvents dispatch on router state/items change
  com.sirsi.idea-router-sweep  hourly queue reconciliation sweep
  ai.sirsi.registry-police     periodic OS-dead thread-registry reap

An already-correct daemon is left untouched; a missing or drifted one is
rewritten (pointing at this clone's script paths) and loaded. A daemon whose
backing script is absent is skipped, never installed broken. macOS only.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		res := setup.InstallRouterDaemons()
		switch res.Status {
		case setup.StatusOK:
			fmt.Printf("✔ %s\n", res.Message)
		case setup.StatusSkipped:
			fmt.Printf("• Skipped: %s\n", res.Message)
		default:
			return fmt.Errorf("%s", res.Message)
		}
		return nil
	},
}

// routerBoardCmd prints the owner-actionable router board the conduit regenerates
// at ~/.sirsi/router-board.md each cycle (blockers, stranded inboxes, live
// threads). Read-only convenience mirror of what the menubar Router view renders.
var routerBoardCmd = &cobra.Command{
	Use:   "board",
	Short: "Print the owner-actionable router board (~/.sirsi/router-board.md)",
	Long: `Prints ~/.sirsi/router-board.md — the lean, owner-actionable board the
router conduit regenerates each cycle (blockers, stranded inboxes, live threads).

This is a read-only convenience mirror of the menubar's Router view. If the board
file is absent (no conduit has run), it points you at 'sirsi router node-status'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home: %w", err)
		}
		boardPath := filepath.Join(home, ".sirsi", "router-board.md")
		data, err := os.ReadFile(boardPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No router board yet (no conduit run recorded).")
				fmt.Println("Run `sirsi router node-status` for the live fabric view.")
				return nil
			}
			return fmt.Errorf("read board: %w", err)
		}
		fmt.Print(string(data))
		if !strings.HasSuffix(string(data), "\n") {
			fmt.Println()
		}
		return nil
	},
}

func init() {
	routerSendCmd.Flags().StringVar(&sendFrom, "from", "", "Sender agent id (e.g., claude-pantheon)")
	routerSendCmd.Flags().StringVar(&sendTo, "to", "", "Recipient agent id (e.g., codex-pantheon)")
	routerSendCmd.Flags().StringVar(&sendTitle, "title", "", "Short title for the work item")
	routerSendCmd.Flags().StringVar(&sendType, "type", "", "Message type: proposal|review|decision (ADR-024 §5 — one inbox, no reviews/ or decisions/ dirs)")
	routerSendCmd.Flags().StringVar(&sendInstructions, "instructions", "", "Instructions body (literal text, or @file)")
	routerCloseCmd.Flags().StringVar(&closeResult, "result", "", "Result body (literal text, or @file)")
	routerStatusCmd.Flags().IntVar(&statusStaleHours, "stale", 24, "Hours after which an open item is flagged as stale (0 disables)")
	routerDoctorCmd.Flags().BoolVar(&routerDoctorFix, "fix", false, "run the safe repair: reap OS-dead thread records (non-destructive)")
	routerCmd.AddCommand(routerStatusCmd, routerSendCmd, routerPullCmd, routerShowCmd, routerCloseCmd, routerAckCmd, routerDoctorCmd, routerWakeInstallCmd, routerWakeLoopCmd, routerInstallDaemonsCmd, routerBoardCmd)
}
