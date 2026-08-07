package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
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

// inlineBodyLimit is the length above which a body MUST come from a file.
//
// Router bodies are composed in a shell. A backtick or $(...) inside a
// double-quoted argument is EVALUATED BY THE SHELL before this binary ever
// runs, so the text that arrives is already mangled — silently, with the
// evaluated span replaced by command output or by nothing. It has corrupted a
// stored record (an owner-gate row rewritten by an evaluated backtick) and, in
// one session, blanked command names out of three separate item bodies written
// by an author who knew about the hazard and had a memory note describing it.
//
// Discipline demonstrably does not fix this, so the safe path is made the only
// path for bodies long enough to contain prose. Short bodies stay inline
// because "ack" and "merged as abc123" are not worth a temp file.
const inlineBodyLimit = 280

// loadOrLiteral returns the literal value, or the contents of the file if it
// starts with @.
//
// Refuses a long inline body rather than storing text the shell may already
// have rewritten. The refusal is the feature: a truncated record looks
// plausible, which is precisely what makes it dangerous.
func loadOrLiteral(v string) (string, error) {
	if strings.HasPrefix(v, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(v, "@"))
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if len(v) > inlineBodyLimit {
		return "", fmt.Errorf(
			"body is %d chars; anything over %d must be passed as @file.\n"+
				"  Shells evaluate backticks and $(...) inside a quoted argument BEFORE sirsi sees it,\n"+
				"  so a long inline body can arrive silently rewritten and be stored that way.\n"+
				"  Write it to a file and pass @that-file.",
			len(v), inlineBodyLimit)
	}
	return v, nil
}

var routerCmd = &cobra.Command{
	Use:   "router",
	Short: "Pull-model work queue between agent threads",
	Long: `Ra's durable work router: send, pull, show, close, status, ledger,
and task-registry verbs. The message loop and task commitments share one
offline-first store; ledger joins them with thread heartbeat/current-item truth.

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
		// Summarize through the facade (files ∪ store) so the counts are accurate
		// after the cutover, when open items live only as store rows.
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no .agents/idea-router/ found: %w", err)
		}
		f, err := dispatch.Open(repoRoot)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		all, err := f.ListAll()
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
	Long: `Commits a new open work item to the durable router store (~/.sirsi/router.db).
The recipient picks it up on sirsi router pull <their-id>, or wakes on it
immediately if they are blocked in sirsi router wait <their-id>.

Before the store cutover this also wrote an items/<id>.md audit view; with the
cutover live the store row IS the record.

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
		// Router v2 Phase 3: THE send facade (store-first guards — idempotency,
		// quotas, breakers — then the items/*.md audit view; §2b axiom 8).
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no .agents/idea-router/ found: %w", err)
		}
		f, err := dispatch.Open(repoRoot)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		res, err := f.Send(sendFrom, sendTo, sendTitle, sendType, instr)
		if err != nil {
			return err
		}
		if res.Deduped {
			fmt.Printf("  Deduped %s → %s: %s (same logical send this window — nothing appended)\n", sendFrom, sendTo, res.ID)
		} else {
			fmt.Printf("  Sent %s → %s: %s\n", sendFrom, sendTo, res.ID)
		}
		return nil
	},
}

var pullBuildFilter bool

var routerPullCmd = &cobra.Command{
	Use:   "pull <agent>",
	Short: "Pull open work items addressed to an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Read through the facade (store rows ∪ file items) so store-only items
		// — the post-cutover steady state — are visible to pull, not just wait.
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no .agents/idea-router/ found: %w", err)
		}
		f, err := dispatch.Open(repoRoot)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		items, err := f.Inbox(args[0])
		if err != nil {
			return err
		}

		if pullBuildFilter {
			// Guard: headless build workers cannot execute decision/review/proposal
			// items (no build artifact exists). Split into buildable vs. deferred and
			// warn about deferred ones so they don't silently spin for the full timeout.
			var buildable, nonBuildable []work.Item
			for _, it := range items {
				if it.IsBuildable() {
					buildable = append(buildable, it)
				} else {
					nonBuildable = append(nonBuildable, it)
				}
			}
			if len(nonBuildable) > 0 {
				fmt.Printf("  ⚠ %d item(s) skipped — not build-shaped (type: decision/review/proposal):\n", len(nonBuildable))
				for _, it := range nonBuildable {
					fmt.Printf("    • %s  type:%s  from:%s\n      → route to human/agent lane: sirsi router respond %s --result \"<answer>\"\n\n", it.ID, it.Type, it.From, it.ID)
				}
			}
			items = buildable
		}

		if len(items) == 0 {
			fmt.Printf("  No open items for %s.\n", args[0])
			return nil
		}
		fmt.Printf("  %d open items for %s:\n\n", len(items), args[0])
		for _, it := range items {
			typeHint := ""
			if it.Type != "" {
				typeHint = fmt.Sprintf("  type: %s\n    ", it.Type)
			}
			fmt.Printf("  • %s\n    %sfrom: %s\n      title: %s\n      opened: %s\n\n", it.ID, typeHint, it.From, it.Title, it.Opened)
		}
		fmt.Printf("  Read full: sirsi router show <id>\n")
		fmt.Printf("  Close when done: sirsi router close <id> --result @path/to/result.md\n")
		return nil
	},
}

var routerWaitTimeout int

var routerWaitCmd = &cobra.Command{
	Use:   "wait <agent>",
	Short: "Block until open work is addressed to an agent (store event-wake, <250ms)",
	Long: `Block until the agent has open work, print the inbox, and return.

This is the store-backed counterpart to 'pull'. Instead of watching the items/
directory on a timer, it parks on the dispatch store's per-agent wake FIFO and
returns within ~250ms of a matching send (PRD /goal #1). A '/loop' watcher can
call this in place of watching items/, so agent wake rides the store — and
therefore survives the Router v2 file-write cutover (ADR-036), where a send is
a store row with no items/<id>.md to watch. The work-check is the dual-read
union (store rows ∪ legacy file items), and a bounded re-check still catches
legacy file-only sends. Returns after --timeout seconds even with no work
(exit 0), so a shell loop calls it repeatedly.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no .agents/idea-router/ found: %w", err)
		}
		f, err := dispatch.Open(repoRoot)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		// Honor Ctrl-C / SIGTERM so a supervised loop can stop the wait cleanly.
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		items, err := f.Wait(ctx, args[0], time.Duration(routerWaitTimeout)*time.Second)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Printf("  No open items for %s after %ds.\n", args[0], routerWaitTimeout)
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
		// Read through the facade so `show` renders a store-only item (no file)
		// after the cutover, falling back to the file when one exists.
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no .agents/idea-router/ found: %w", err)
		}
		f, err := dispatch.Open(repoRoot)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		md, err := f.Show(args[0])
		if err != nil {
			return err
		}
		fmt.Print(md)
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

var (
	closeResult  string
	closeProof   string
	closeBlocked bool
	closeAck     bool
	closeAgent   string
)

var (
	respondResult string
	respondTitle  string
	respondAgent  string
)

// routerRespondCmd is the atomic request→response primitive (owner rule
// 2026-06-15: a request ALWAYS requires a response). A close-with-Result is
// audit-only — the sender is not notified; only a fresh inbound wakes them.
// This verb does both: close the item with the Result, then route a
// type:decision inbound back to the requester carrying the same Result. It
// goes through the facade, so it works for file items AND store-only items
// (the post-cutover steady state, where the file-based sirsi-respond.sh
// wrapper found nothing to read).
var routerRespondCmd = &cobra.Command{
	Use:   "respond <id>",
	Short: "Close a request with a Result AND route the response back to its sender (atomic)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := loadOrLiteral(respondResult)
		if err != nil {
			return fmt.Errorf("--result: %w", err)
		}
		if strings.TrimSpace(result) == "" {
			return fmt.Errorf("--result is required: a response with no body answers nothing")
		}
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no .agents/idea-router/ found: %w", err)
		}
		f, err := dispatch.Open(repoRoot)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		item, err := f.Get(args[0])
		if err != nil {
			return err
		}
		if item.From == "" {
			return fmt.Errorf("item %s has no from: — cannot notify the requester", args[0])
		}
		me, reason := resolveCurrentAgent(filepath.Join(repoRoot, ".agents", "idea-router"), respondAgent)
		if me == "" {
			return fmt.Errorf("resolve acting agent: %s", reason)
		}
		if actorErr := f.ValidateAgent("acting agent", me); actorErr != nil {
			return actorErr
		}
		// NOTIFY FIRST, then close. There is no cross-row transaction here (the
		// file era has no transaction at all), so one of the two orders has to
		// be the survivable one — and only this order is. Closing first can
		// strand the requester: the request is gone from their queue and the
		// notification never arrives, with nothing left open to retry from.
		// Notifying first fails safe — the request stays OPEN until it is
		// answered, so a retry is always available, and the retry is harmless
		// because the store's idem_key dedupes an identical resend
		// (SendGuarded → deduped=true) instead of double-notifying.
		title := respondTitle
		if title == "" {
			t := item.Title
			if len(t) > 80 {
				t = t[:80]
			}
			title = "RESPONSE: " + t
		}
		body := fmt.Sprintf("RESPONSE to your request %q (your item %s, closed with this as the Result).\n\n%s",
			item.Title, args[0], result)
		res, err := f.Send(me, item.From, title, "decision", body)
		if err != nil {
			return fmt.Errorf("notifying %s FAILED — %s left OPEN, nothing lost, rerun respond: %w",
				item.From, args[0], err)
		}
		if res.Deduped {
			fmt.Printf("  Notified %s (response %s already sent this window — deduped, not resent)\n", item.From, res.ID)
		} else {
			fmt.Printf("  Notified %s (fresh inbound %s)\n", item.From, res.ID)
		}

		// Close with the Result (audit trail). A respond close is by definition
		// an acknowledgement — the notification above IS the response — so it
		// carries --ack semantics past the ADR-037 proof gate.
		if cerr := f.CloseItem(me, args[0], result); cerr != nil {
			return fmt.Errorf("%s notified via %s but closing %s FAILED — rerun respond, the resend dedupes: %w",
				item.From, res.ID, args[0], cerr)
		}
		fmt.Printf("  Closed %s (Result recorded)\n", args[0])
		return nil
	},
}

var routerCloseCmd = &cobra.Command{
	Use:   "close <id>",
	Short: "Mark a work item closed",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := loadOrLiteral(closeResult)
		if err != nil {
			return fmt.Errorf("--result: %w", err)
		}
		// Phase 3: the facade closes the canonical file AND mirrors the close
		// into the store, so the durable index never lies about liveness.
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no .agents/idea-router/ found: %w", err)
		}
		// ADR-037 completion-proof gate (restored after the Router v2 facade
		// rewrite dropped it — codex-home closure audit 2026-07-22). A close in
		// a repo with .agents/completion.contract.json must carry --proof,
		// --blocked, or --ack; a bare close is a done-claim without evidence.
		if gateErr := enforceCompletionProof(repoRoot, args[0], closeProof, closeBlocked, closeAck, result); gateErr != nil {
			return gateErr
		}
		f, err := dispatch.Open(repoRoot)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		actor, reason := resolveCurrentAgent(filepath.Join(repoRoot, ".agents", "idea-router"), closeAgent)
		if actor == "" {
			return fmt.Errorf("resolve acting agent: %s", reason)
		}
		if err := f.CloseItem(actor, args[0], result); err != nil {
			return err
		}
		fmt.Printf("  Closed %s\n", args[0])
		return nil
	},
}

// enforceCompletionProof is the ADR-037 close gate. In a repo carrying
// .agents/completion.contract.json every close must be one of: a validated
// --proof, an explicit --blocked (with the blocker in --result), or an
// explicit --ack (coordination-only close, with --result). Repos without a
// contract are ungated. Restored from the pre-facade close path; the target
// repo is the router's own repo — Router v2 items no longer carry a Repo field.
func enforceCompletionProof(repoRoot, itemID, proof string, blocked, ack bool, result string) error {
	contractPath := filepath.Join(repoRoot, ".agents", "completion.contract.json")
	_, contractErr := os.Stat(contractPath)
	hasContract := contractErr == nil
	if contractErr != nil && !os.IsNotExist(contractErr) {
		return fmt.Errorf("check completion contract: %w", contractErr)
	}

	if blocked {
		if strings.TrimSpace(result) == "" {
			return fmt.Errorf("--blocked requires --result explaining the blocker")
		}
		return nil
	}
	if ack {
		if strings.TrimSpace(result) == "" {
			return fmt.Errorf("--ack requires --result explaining what was acknowledged")
		}
		return nil
	}
	if proof == "" {
		if hasContract {
			return fmt.Errorf("completion proof required for %s: pass --proof .agents/proofs/%s.json, or use --blocked/--ack with --result", repoRoot, itemID)
		}
		return nil
	}
	if !hasContract {
		return fmt.Errorf("--proof supplied but no completion contract exists at %s", contractPath)
	}
	return validateCompletionProof(repoRoot, proof)
}

// validateCompletionProof shells out to the portfolio gate validator
// (tools/agent_completion_gate.py beside the repo, or
// SIRSI_COMPLETION_GATE_SCRIPT). The proof schema and validation rules live
// with the portfolio law, not in this binary.
func validateCompletionProof(repoRoot, proof string) error {
	script := os.Getenv("SIRSI_COMPLETION_GATE_SCRIPT")
	if script == "" {
		devRoot := filepath.Dir(repoRoot)
		script = filepath.Join(devRoot, "tools", "agent_completion_gate.py")
	}
	proofPath := proof
	if !filepath.IsAbs(proofPath) {
		proofPath = filepath.Join(repoRoot, proofPath)
	}
	out, err := exec.Command("python3", script, "validate", "--repo", repoRoot, "--proof", proofPath).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("completion proof validation failed: %s", msg)
	}
	return nil
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
		// LEAK GUARD (owner finding 2026-07-10): refuse only when the agent
		// already has an ARMED watcher. A merely live-but-loop-dead interactive
		// session is precisely the state wake-install repairs; blocking on any
		// live thread strands work until the owner manually types /loop.
		force, _ := cmd.Flags().GetBool("force")
		if wakeInstallBlocked(root, cfg.ID, force) {
			fmt.Printf("⚠️  %s already has an armed watcher — not arming a duplicate background wake channel.\n", cfg.ID)
			fmt.Printf("   A second pull-loop on top of an armed watcher spawns duplicate processes\n")
			fmt.Printf("   each tick (the wake-loop leak). Use --force only for deliberate repair.\n")
			return nil
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

func wakeInstallBlocked(root, agentID string, force bool) bool {
	return !force && router.AgentArmed(root, agentID)
}

// routerCutoverCmd manages the ADR-036/037 store-authority cutover as a
// deterministic, doctor-visible lever — NOT a manual env-var. `enable` drops the
// persistent marker (routercfg.MarkerPath) so every new process on the host
// treats the store as the sole authority, and (with --rearm) reinstalls the
// headless wake LaunchAgents so they restart in store-wake mode. This is the
// "owner-visible deploy step" ADR-036 named, now a shipped verb + a doctor check.
var routerCutoverCmd = &cobra.Command{
	Use:   "cutover",
	Short: "Store-authority cutover (ADR-036/037): show/enable/disable store-only dispatch",
	RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
}

// cutoverModeLine reports the effective mode and where the setting came from.
func cutoverModeLine() string {
	on := routercfg.StoreWake()
	src := "default (legacy dual-write + items/ watch)"
	if v, ok := os.LookupEnv(routercfg.StoreWakeEnv); ok {
		src = fmt.Sprintf("env %s=%q", routercfg.StoreWakeEnv, v)
	} else if p := routercfg.MarkerPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			src = "marker " + p
		}
	}
	if on {
		return "STORE-ONLY (cutover active) — source: " + src
	}
	return "LEGACY (files + store) — source: " + src
}

var routerCutoverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current cutover mode and its source",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("  Cutover mode: %s\n", cutoverModeLine())
		if root, err := workRoot(); err == nil {
			if reg, rErr := router.LoadRegistry(root); rErr == nil {
				fmt.Printf("  Registered agents: %d (durable watchers re-arm on restart)\n", len(reg.Agents))
			}
		}
		return nil
	},
}

var routerCutoverEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Flip the fabric to store-only dispatch (drops the persistent marker) + re-arm watchers",
	Long: `Enable the ADR-036/037 store-authority cutover on this host.

Writes the persistent marker so every NEW process (launchd watchers, CLI,
sessions) reads the store as the sole dispatch + wake authority: sends stop
writing items/<id>.md, and wake rides 'sirsi router wait'. Already-running
durable watchers pick up the new mode when they restart; --rearm reinstalls the
headless wake LaunchAgents now. Live interactive sessions re-arm on their next
loop tick (the arm instruction is regenerated store-aware). Reversible with
'sirsi router cutover disable'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		p := routercfg.MarkerPath()
		if p == "" {
			return fmt.Errorf("cannot resolve home directory for the cutover marker")
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("create marker dir: %w", err)
		}
		if err := os.WriteFile(p, []byte("ADR-036/037 store-authority cutover active\n"), 0o644); err != nil {
			return fmt.Errorf("write marker: %w", err)
		}
		if !routercfg.StoreWake() {
			// An explicit env override can still force it off — say so plainly
			// rather than claiming a flip that isn't in effect.
			fmt.Printf("⚠️  Marker written (%s) but %s forces it OFF for this process — unset it to take effect.\n", p, routercfg.StoreWakeEnv)
			return nil
		}
		fmt.Printf("✔ Cutover ENABLED — %s\n", cutoverModeLine())

		rearm, _ := cmd.Flags().GetBool("rearm")
		if rearm {
			root, err := workRoot()
			if err != nil {
				return err
			}
			reg, err := router.LoadRegistry(root)
			if err != nil {
				return fmt.Errorf("load agents: %w", err)
			}
			rearmed, skipped := 0, 0
			for _, a := range reg.Agents {
				// Same leak guard as `router wake-install`: skip agents that already
				// have an armed watcher, but allow loop-dead live sessions to be
				// repaired by installing the pull-loop.
				if wakeInstallBlocked(root, a.ID, false) {
					skipped++
					continue
				}
				if changed, _, iErr := router.InstallWakeLaunchAgent(a, ""); iErr == nil && changed {
					rearmed++
				}
			}
			fmt.Printf("  Re-armed %d headless wake LaunchAgent(s) into store-wake mode", rearmed)
			if skipped > 0 {
				fmt.Printf("; skipped %d with an armed watcher", skipped)
			}
			fmt.Println(".")
		}
		fmt.Printf("  Live sessions re-arm on their next loop tick; run `sirsi router cutover status` to verify.\n")
		return nil
	},
}

var routerCutoverDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Revert to legacy files + store dual-write (removes the marker)",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := routercfg.MarkerPath()
		if p == "" {
			return fmt.Errorf("cannot resolve home directory for the cutover marker")
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove marker: %w", err)
		}
		fmt.Printf("✔ Cutover DISABLED — %s\n", cutoverModeLine())
		if _, ok := os.LookupEnv(routercfg.StoreWakeEnv); ok {
			fmt.Printf("  Note: %s is still set in this environment and overrides the marker.\n", routercfg.StoreWakeEnv)
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
		// launchd runs this without -v, and the default level (Warn) drops every
		// log.Printf in the loop — the wake-*.log files sat at 0 bytes while the
		// loops ran. This loop's stderr IS its dedicated log file, so Info is the
		// correct floor for it. Honors --quiet.
		logging.EnableDaemonLogging()
		root, err := workRootEnsure()
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		return router.RunWakeLoop(ctx, root, args[0], router.DefaultWakeLoopInterval)
	},
}

// routerInstallDaemonsCmd ensures the single-backstop router automation
// (backlog ruling 20260629-230327): the ONE resident supervisor LaunchAgent
// installed and loaded, and the three legacy per-duty LaunchAgents migrated
// away when present. The duties those agents carried (dispatch pump, hourly
// sweep, registry police) now run inside `sirsi horus supervise` — see
// internal/router/supervisorduties.go. Idempotent; macOS only.
var routerInstallDaemonsCmd = &cobra.Command{
	Use:   "install-daemons",
	Short: "Ensure the single router supervisor and migrate away legacy per-duty agents — macOS",
	Long: `Ensures the single-backstop router automation:

  1. Installs (or confirms) the ONE resident supervisor LaunchAgent
     (ai.sirsi.horus.agent-router). Its loop now carries the dispatch pump,
     the hourly queue sweep, and the registry-police pass — cadence-gated
     and error-isolated.
  2. Migrates away the three legacy per-duty LaunchAgents when present
     (com.sirsi.idea-router, com.sirsi.idea-router-sweep,
     ai.sirsi.registry-police): each is unloaded and its plist removed,
     and the migration is reported.

Idempotent — a second run confirms the supervisor and finds nothing to
migrate. macOS only.`,
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

// routerQuarantineWorkerCmd is 𓁵 Sekhmet's kill switch for the runaway-executor
// class (ADR-035; the 2026-07-03/04 incident: 19,195 sessions spawned, 0 closed,
// 1.3 TB of orphaned build trees). It stops every claude build-worker LaunchAgent
// now (bootout) and keeps it stopped across logins (plist → .quarantined rename).
// It is the remediation behind the doctor's "Runaway Executor" finding, and the
// OFF state the Dispatch Contract requires until the Phase-2 acceptance bar
// passes (PRD ROUTER_V2_DURABLE_DISPATCH §2b). Wake-loop watchers and the router
// supervisor are NEVER touched — the incident's watchers were healthy.
var quarantineWorkerDryRun bool

var routerQuarantineWorkerCmd = &cobra.Command{
	Use:   "quarantine-worker",
	Short: "𓁵 Stop every claude build-worker LaunchAgent — bootout now, quarantine its plist (macOS)",
	Long: `Stops the claude build-worker executor tier, durably:

  1. Boots every loaded ai.sirsi.claude-worker.* job out of launchd (stops it now).
  2. Renames its plist to *.plist.quarantined so login/RunAtLoad cannot bring it
     back. Rename it back by hand to re-arm — but per the Dispatch Contract
     (docs/prd/ROUTER_V2_DURABLE_DISPATCH.md §2b) the worker stays OFF until the
     Phase-2 acceptance bar passes.

Wake-loop watchers (ai.sirsi.router.wake.*) and the router supervisor are never touched.
Idempotent; --dry-run reports the full plan without changing anything.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		res, err := router.QuarantineWorkers(quarantineWorkerDryRun, nil)
		if err != nil {
			return err
		}
		if len(res.BootedOut) == 0 && len(res.Quarantined) == 0 {
			fmt.Println("✓ No claude build-worker LaunchAgents found — nothing to quarantine.")
			return nil
		}
		verb := ""
		if res.DryRun {
			verb = "would be "
		}
		for _, label := range res.BootedOut {
			fmt.Printf("✔ %sbooted out: %s\n", verb, label)
		}
		for _, p := range res.Quarantined {
			fmt.Printf("✔ %squarantined: %s → %s.quarantined\n", verb, p, filepath.Base(p))
		}
		fmt.Println("  Wake-loops and the supervisor were not touched.")
		return nil
	},
}

// routerQuarantineCmd is the fabric-wide operator off-switch (R7/G6),
// generalized from `sirsi gemma quarantine` (gemma_quarantine.go). Unlike
// quarantine-worker (one-shot bootout + plist rename, claude-worker labels
// only), this writes a durable marker every dispatcher in the fabric checks
// BEFORE acting — the wake-loop's own consumer dispatch (wake.go) and the
// dead-label kickstart duty (launchdkickstart.go) — so it holds against
// exactly the three revival paths that defeated `bootout`/`disable` on
// 2026-08-06 (liveness-watch re-bootstrap in 40s, print-disabled silently
// cleared, horus.agent-router KeepAlive reinstalling all 24 lanes).
var routerQuarantineCmd = &cobra.Command{
	Use:   "quarantine",
	Short: "Stand the whole fabric down — no dispatcher may start a new lane or consumer",
	Long: `Writes the fabric quarantine marker every dispatcher checks BEFORE
reviving a dead launchd label or spawning a new inbox consumer.

  sirsi router quarantine
  sirsi router unquarantine

Does not stop anything already running — pair with quarantine-worker for that.
This is the durable OFF switch: it holds across a supervisor restart, unlike
launchctl bootout/disable.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("router quarantine: %w", err)
		}
		path := router.FabricQuarantineMarkerPath(home)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("router quarantine: %w", err)
		}
		if err := os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o600); err != nil {
			return fmt.Errorf("router quarantine: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Fabric quarantined. No dispatcher will start a new lane or consumer until `sirsi router unquarantine`.\n")
		return nil
	},
}

var routerUnquarantineCmd = &cobra.Command{
	Use:   "unquarantine",
	Short: "Clear the fabric quarantine marker so dispatch resumes",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("router unquarantine: %w", err)
		}
		path := router.FabricQuarantineMarkerPath(home)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("router unquarantine: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Fabric quarantine cleared. Dispatch resumes on the next tick.\n")
		return nil
	},
}

// routerMigrateCmd is the Phase-4 one-shot importer (PRD /goal #4): every
// canonical items/*.md lands in the durable store with verification evidence
// (count-in == count-out + spot-checked bodies). Idempotent — safe to re-run
// any time; rows upsert by id. File writes are NOT stopped here: the cutover
// (stop tracking runtime items in git) is a separate, owner-visible step at
// the end of the deprecation window.
var routerMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Import every items/*.md into the durable router store (idempotent, verified)",
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no .agents/idea-router/ found: %w", err)
		}
		f, err := dispatch.Open(repoRoot)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		rep, err := f.Migrate()
		if err != nil {
			return err
		}
		fmt.Printf("  Files seen:   %d\n", rep.FilesSeen)
		fmt.Printf("  Imported:     %d new, %d refreshed (count-out %d)\n", rep.Inserted, rep.Updated, rep.CountOut())
		fmt.Printf("  Spot-checked: %d item(s) byte-compared file↔store\n", rep.SpotChecked)
		if len(rep.Errors) > 0 {
			for _, e := range rep.Errors {
				fmt.Printf("  ✗ %s\n", e)
			}
			return fmt.Errorf("migration completed with %d error(s) — zero-loss NOT proven", len(rep.Errors))
		}
		if rep.CountOut() != rep.FilesSeen {
			return fmt.Errorf("count-in %d != count-out %d — zero-loss NOT proven", rep.FilesSeen, rep.CountOut())
		}
		fmt.Println("  ✔ Zero-loss verified: count-in == count-out, spot-checks match.")
		return nil
	},
}

// routerBoardCmd prints the owner-actionable router board. It renders live
// NodeStatus first; the historical ~/.sirsi/router-board.md file is only a
// visibly-stale fallback when the live read-model is unavailable.
var routerBoardCmd = &cobra.Command{
	Use:   "board",
	Short: "Print the live owner-actionable router board",
	Long: `Prints the live owner-actionable router board from the router's
authoritative read-model (queue, stranded inboxes, live threads, helpers).

The old ~/.sirsi/router-board.md conduit artifact is used only as a marked
fallback when the live read-model cannot be collected.

With --json the verb emits an explicit source envelope. source=live-node-status
is current truth; source=cached-markdown is a stale fallback and includes the
cached file's modified_at.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home: %w", err)
		}
		boardPath := filepath.Join(home, ".sirsi", "router-board.md")
		data, err := os.ReadFile(boardPath)
		exists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("read board: %w", err)
		}
		var modifiedAt string
		if info, serr := os.Stat(boardPath); serr == nil {
			modifiedAt = info.ModTime().UTC().Format(time.RFC3339)
		}

		live, liveErr := collectLiveRouterBoard()
		// The root --json flag must yield machine output here too — before this
		// branch existed it was silently swallowed (markdown printed, exit 0),
		// which is a contract violation for scripted callers (#147 review, minor 5).
		if JsonOutput {
			envelope := routerBoardEnvelope(boardPath, data, exists, modifiedAt, live, liveErr)
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(envelope)
		}
		if liveErr == nil {
			renderNodeStatus(live)
			return nil
		}
		if exists {
			fmt.Printf("⚠ live router board unavailable (%v); showing stale cached markdown", liveErr)
			if modifiedAt != "" {
				fmt.Printf(" from %s", modifiedAt)
			}
			fmt.Println(".")
			fmt.Println()
			fmt.Print(string(data))
			if !strings.HasSuffix(string(data), "\n") {
				fmt.Println()
			}
			return nil
		}
		return fmt.Errorf("collect live router board: %w", liveErr)
	},
}

func collectLiveRouterBoard() (*router.NodeStatus, error) {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("locate repo root: %w", err)
	}
	ns, err := router.CollectNodeStatus(repoRoot, nil)
	if err != nil {
		return nil, fmt.Errorf("collect node-status: %w", err)
	}
	return ns, nil
}

func routerBoardEnvelope(boardPath string, cached []byte, exists bool, modifiedAt string, live *router.NodeStatus, liveErr error) map[string]any {
	if liveErr == nil && live != nil {
		return map[string]any{
			"source":      "live-node-status",
			"stale":       false,
			"node_status": live,
		}
	}
	envelope := map[string]any{
		"source":     "cached-markdown",
		"stale":      true,
		"path":       boardPath,
		"exists":     exists,
		"content":    string(cached),
		"live_error": fmt.Sprint(liveErr),
	}
	if modifiedAt != "" {
		envelope["modified_at"] = modifiedAt
	}
	if !exists {
		envelope["hint"] = "no cached conduit board exists and live node-status collection failed"
	}
	return envelope
}

var (
	pruneDays      int
	pruneDryRun    bool
	pruneItemsOnly bool
	pruneLogsOnly  bool
	pruneNoHome    bool
)

// routerPruneCmd applies the router fabric's retention policy: closed item
// payloads, dated incident dumps, append-only logs, and terminal work records
// past the retention window are compacted/removed (age cap), and
// oversized-but-recent logs are tail-capped (size cap). Owner directive
// 2026-07-10: at most a 90-day log period; logging beyond that is wasteful.
// Dry-run first (Rule A1).
var routerPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Apply the router retention policy (default: 90-day cap; --dry-run to preview)",
	Long: `Reclaim router byproduct storage under the retention window (default 90 days).

Compacted or removed when older than the window:
  • closed item payloads (item ids become tombstones; open items are NEVER touched)
  • dated quarantine/incident dumps (quarantine-YYYYMMDD-*)
  • stale logs, and terminal (completed/failed/blocked) work-queue records

Capped regardless of age (size policy):
  • active append-only logs tail-capped to their most recent 4 MiB
  • the regenerated process snapshot removed when it exceeds 8 MiB

Also sweeps ~/.sirsi runtime logs unless --no-home is set. Always run with
--dry-run first: it reports the exact bytes it would reclaim and mutates nothing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := workRoot()
		if err != nil {
			return err
		}
		if pruneDays < 0 {
			return fmt.Errorf("--days must be >= 0")
		}
		cutoff := time.Now().Add(-time.Duration(pruneDays) * 24 * time.Hour)

		var reports []router.PruneReport
		switch {
		case pruneItemsOnly:
			items, perr := work.PruneItems(root, cutoff, pruneDryRun)
			if perr != nil {
				return perr
			}
			rep := router.PruneReport{Cutoff: cutoff, DryRun: pruneDryRun}
			for _, it := range items {
				rep.Actions = append(rep.Actions, router.PruneAction{Path: "items/" + it.ID + ".md", Kind: "item", Before: it.Bytes, After: it.After})
			}
			reports = append(reports, rep)
		case pruneLogsOnly:
			logDir := filepath.Join(root, "logs")
			rep := router.PruneReport{Cutoff: cutoff, DryRun: pruneDryRun}
			if lerr := router.PruneLogDirExported(logDir, cutoff, pruneDryRun, &rep); lerr != nil {
				return lerr
			}
			reports = append(reports, rep)
		default:
			rep, perr := router.PruneArtifacts(root, cutoff, pruneDryRun)
			if perr != nil {
				return perr
			}
			reports = append(reports, rep)
		}

		if !pruneItemsOnly && !pruneNoHome {
			if home, herr := os.UserHomeDir(); herr == nil {
				hrep, herr := router.PruneHomeLogs(filepath.Join(home, ".sirsi"), cutoff, pruneDryRun)
				if herr == nil {
					reports = append(reports, hrep)
				}
			}
		}

		if routerJSON, _ := cmd.Flags().GetBool("json"); routerJSON {
			return json.NewEncoder(os.Stdout).Encode(reports)
		}
		printPruneReports(reports, pruneDays, pruneDryRun)
		return nil
	},
}

// printPruneReports renders the Ma'at-styled human summary.
func printPruneReports(reports []router.PruneReport, days int, dryRun bool) {
	var total int64
	var actions int
	byKind := map[string]int64{}
	for _, r := range reports {
		for _, a := range r.Actions {
			total += a.Reclaimed()
			actions++
			byKind[a.Kind] += a.Reclaimed()
		}
	}
	verb := "Reclaimed"
	if dryRun {
		verb = "Would reclaim"
	}
	fmt.Printf("𓆄 Ma'at router retention — %d-day window\n", days)
	if actions == 0 {
		fmt.Printf("  Nothing to prune. Fabric is within retention.\n")
		return
	}
	for _, kind := range []string{"item", "quarantine", "log-deleted", "log-capped", "workqueue", "snapshot"} {
		if b, ok := byKind[kind]; ok {
			fmt.Printf("  %-12s %s\n", kind, humanBytes(b))
		}
	}
	fmt.Printf("  %s %s across %d artifact(s)%s\n", verb, humanBytes(total), actions, dryRunNote(dryRun))
}

func dryRunNote(dryRun bool) string {
	if dryRun {
		return "  (dry-run — nothing deleted; re-run without --dry-run to apply)"
	}
	return ""
}

// humanBytes formats a byte count as a compact human string.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// dumpRecord projects a router item onto the fixed H4-boundary export contract:
// exactly the nine agreed fields, keys stable, no store schema or wake state.
// Extracted so the contract is unit-testable without opening a real store.
func dumpRecord(it work.Item) map[string]string {
	return map[string]string{
		"id":     it.ID,
		"from":   it.From,
		"to":     it.To,
		"type":   it.Type,
		"title":  it.Title,
		"status": it.Status,
		"opened": it.Opened,
		"closed": it.Closed,
		"body":   it.Instructions,
	}
}

// routerDumpCmd — `sirsi router dump`: a read-only, store-aware export of every
// router item as JSONL to stdout, one JSON object per line. Built for the
// hypergraph fabric feeder (claude-home, 2026-07-24): the ADR-036/037 cutover
// stopped writing items/*.md, so a file-only reader is blind to store-only
// items — this reads through the dispatch facade (the union of file + store),
// so it sees EVERY item across the cutover boundary. H4 boundary: JSON only,
// exactly the nine agreed fields — no store schema, no internal wake/frontmatter
// state — so the export stays a stable contract the ingester can dedup on.
var routerDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Export every router item as JSONL (store-aware; for the hypergraph feeder)",
	Long: `Streams every router item to stdout as JSONL — one JSON object per line,
with exactly: id, from, to, type, title, status, opened, closed, body.

Read-only and store-aware: it reads through the dispatch facade (file + durable
store union), so unlike a raw items/ scan it sees store-only items written after
the ADR-036/037 cutover. Intended as the hypergraph router feeder; the field set
is a fixed H4-boundary contract (JSON only, no schema exposure).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no .agents/idea-router/ found: %w", err)
		}
		f, err := dispatch.Open(repoRoot)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		items, err := f.ListAll()
		if err != nil {
			return fmt.Errorf("list items: %w", err)
		}
		enc := json.NewEncoder(os.Stdout)
		for _, it := range items {
			if err := enc.Encode(dumpRecord(it)); err != nil {
				return fmt.Errorf("encode %s: %w", it.ID, err)
			}
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
	routerCloseCmd.Flags().StringVar(&closeAgent, "agent", "", "Acting agent id (otherwise resolved from the current session)")
	routerRespondCmd.Flags().StringVar(&respondResult, "result", "", "Response body routed back to the requester (literal text, or @file)")
	routerRespondCmd.Flags().StringVar(&respondTitle, "title", "", "Title for the response inbound (default: RESPONSE: <request title>)")
	routerRespondCmd.Flags().StringVar(&respondAgent, "agent", "", "Acting agent id (otherwise resolved from the current session)")
	routerCloseCmd.Flags().StringVar(&closeProof, "proof", "", "Completion proof JSON path, relative to repo root or absolute (ADR-037)")
	routerCloseCmd.Flags().BoolVar(&closeBlocked, "blocked", false, "Close as explicitly blocked; requires --result and skips proof validation")
	routerCloseCmd.Flags().BoolVar(&closeAck, "ack", false, "Close as coordination/ack only; requires --result and skips proof validation")
	routerStatusCmd.Flags().IntVar(&statusStaleHours, "stale", 24, "Hours after which an open item is flagged as stale (0 disables)")
	routerDoctorCmd.Flags().BoolVar(&routerDoctorFix, "fix", false, "run the safe repair: reap OS-dead thread records (non-destructive)")
	routerQuarantineWorkerCmd.Flags().BoolVar(&quarantineWorkerDryRun, "dry-run", false, "report the full plan without booting out or renaming anything (Rule A1)")
	routerWakeInstallCmd.Flags().Bool("force", false, "arm even if the agent already has an armed watcher (bypasses the duplicate-spawn leak guard)")
	routerPullCmd.Flags().BoolVar(&pullBuildFilter, "build-filter", false, "Skip non-build items (decision/review/proposal) — safe for headless build workers")
	routerWaitCmd.Flags().IntVar(&routerWaitTimeout, "timeout", 50, "Max seconds to block before returning empty (a shell loop calls wait repeatedly)")
	routerCutoverEnableCmd.Flags().Bool("rearm", false, "reinstall headless wake LaunchAgents into store-wake mode now")
	routerCutoverCmd.AddCommand(routerCutoverStatusCmd, routerCutoverEnableCmd, routerCutoverDisableCmd)
	routerPruneCmd.Flags().IntVar(&pruneDays, "days", router.DefaultRetentionDays, "retention window in days (older artifacts are removed)")
	routerPruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "report the bytes that would be reclaimed without deleting anything (Rule A1)")
	routerPruneCmd.Flags().BoolVar(&pruneItemsOnly, "items-only", false, "prune only closed items past the window (skip logs/dumps/queue)")
	routerPruneCmd.Flags().BoolVar(&pruneLogsOnly, "logs-only", false, "prune only the router logs/ directory")
	routerPruneCmd.Flags().BoolVar(&pruneNoHome, "no-home", false, "do not sweep ~/.sirsi runtime logs")
	routerBreakersCmd.Flags().BoolVar(&routerBreakersJSON, "json", false, "emit the breaker states as JSON")
	routerCmd.AddCommand(routerStatusCmd, routerSendCmd, routerPullCmd, routerWaitCmd, routerShowCmd, routerCloseCmd, routerRespondCmd, routerAckCmd, routerDoctorCmd, routerWakeInstallCmd, routerWakeLoopCmd, routerInstallDaemonsCmd, routerBoardCmd, routerFleetCmd, routerQuarantineWorkerCmd, routerQuarantineCmd, routerUnquarantineCmd, routerMigrateCmd, routerCutoverCmd, routerPruneCmd, routerDumpCmd, routerBreakersCmd, routerBreakerResetCmd)
}
