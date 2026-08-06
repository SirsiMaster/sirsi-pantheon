package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// `sirsi router close-resolved` — retire acknowledgement items whose work has
// demonstrably landed, and SURFACE (never close) everything else that looks done.
//
// WHY THIS EXISTS. The queue sat at ~49 open items for one agent and growing.
// Measured before building: 48 of 49 titles were DISTINCT after normalization —
// one duplicate pair. So the queue is not spam, and a rate limit or dedupe at
// `router send` would have dropped real findings while fixing nothing. The
// accumulation mechanism is RETENTION: 14 of 49 items (29%) referenced only
// pull requests that were already merged or closed. The work had landed;
// nothing retired the item.
//
// WHY AUTO-CLOSE IS RESTRICTED TO ACKNOWLEDGEMENTS. The first draft closed
// anything whose referenced PRs had all landed. A dry run over the live queue
// proposed 32 closures — and among them:
//
//	"thoth payload fix is merged BUT BYPASSABLE: binary predates it…"
//	                                     → #401 merged, defect LIVE
//	"router doctor: 'N already-armed' is the open-ITEM count, not agents"
//	                                     → #79/#80 merged, NEW bug
//	"binding-hold wedge: RULE ON timeout-minutes vs …"
//	                                     → merged PRs, ask undelivered
//
// An item can reference merged PRs while raising a new problem ABOUT them.
// Closing those would bury exactly the findings the router exists to surface —
// the failure mode this command prevents, inverted. So it closes only items
// whose TITLE announces them as an ACK or RESPONSE (no outstanding ask by
// construction) AND whose referenced PRs have all landed. Everything else that
// merely looks resolved is listed for judgement. Surfacing is safe; closing on
// an inference is not.
var routerCloseResolvedApply bool

var routerCloseResolvedCmd = &cobra.Command{
	Use:   "close-resolved",
	Short: "List items whose referenced PRs all landed, for a reader to judge (surface-only; --apply refuses)",
	RunE:  runRouterCloseResolved,
}

func init() {
	routerCloseResolvedCmd.Flags().BoolVar(&routerCloseResolvedApply, "apply", false, "actually close the resolved acknowledgements")
	routerCmd.AddCommand(routerCloseResolvedCmd)
}

// prRef matches a PR reference: #123. Hash-prefixed only — bare numbers in an
// item ("swap at 93%", "18432 MB") must never be harvested as PR references.
var prRef = regexp.MustCompile(`#(\d{2,5})\b`)

func prStates() (map[int]string, error) {
	out, err := exec.Command("gh", "pr", "list", "--state", "all", "--limit", "400",
		"--json", "number,state").Output()
	if err != nil {
		return nil, fmt.Errorf("read PR states (is gh authenticated?): %w", err)
	}
	var rows []struct {
		Number int    `json:"number"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("parse PR states: %w", err)
	}
	m := make(map[int]string, len(rows))
	for _, r := range rows {
		m[r.Number] = r.State
	}
	return m, nil
}

// resolvedBy reports the PRs an item names and whether ALL of them have landed.
// An unknown number counts as NOT landed: a PR this repo has never seen is more
// likely a typo or another repo's number, and treating unknown as done would
// close items on absence of evidence.
func resolvedBy(text string, states map[int]string) (refs []int, allLanded bool) {
	seen := map[int]bool{}
	for _, m := range prRef.FindAllStringSubmatch(text, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil || seen[n] {
			continue
		}
		seen[n] = true
		refs = append(refs, n)
	}
	if len(refs) == 0 {
		return nil, false
	}
	sort.Ints(refs)
	for _, n := range refs {
		switch states[n] {
		case "MERGED", "CLOSED":
		default:
			return refs, false
		}
	}
	return refs, true
}

// isAcknowledgement reports whether a title ANNOUNCES the item as a pure ack or
// response — a record that something happened, carrying no outstanding ask.
// Anchored to the start deliberately: a body may contain the word "response"
// while the item demands work; only a self-announcing title qualifies.
func isAcknowledgement(title string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	for _, p := range []string{"ack:", "ack ", "response:", "re:", "acknowledged:", "confirmed:"} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

func runRouterCloseResolved(_ *cobra.Command, _ []string) error {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return fmt.Errorf("no idea-router found: %w", err)
	}
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

	items, err := router.OpenItems(routerRoot, "")
	if err != nil {
		return fmt.Errorf("read open items: %w", err)
	}
	states, err := prStates()
	if err != nil {
		return err
	}

	type cand struct {
		item work.Item
		refs []int
	}
	var closable, review []cand
	for _, it := range items {
		// Title AND instructions: a title may carry no number while the body
		// names the PR that closed the work.
		refs, landed := resolvedBy(it.Title+" "+it.Instructions, states)
		if !landed {
			continue
		}
		if isAcknowledgement(it.Title) {
			closable = append(closable, cand{it, refs})
		} else {
			review = append(review, cand{it, refs})
		}
	}

	if len(closable) == 0 && len(review) == 0 {
		fmt.Printf("𓂀 %d open item(s); none reference an exclusively merged/closed PR set — nothing to retire.\n", len(items))
		return nil
	}

	if routerCloseResolvedApply {
		// SURFACE-ONLY until typed acknowledgements exist (codex post-merge
		// review of #405, two P1s):
		//  1. resolvedBy matches bare #NNN against THIS repo's PRs while items
		//     span repo-scoped lanes — "ACK: FinalWishes PR #24 merged" would
		//     close on Pantheon's #24. A number is not an identity.
		//  2. Title vocabulary is not structural proof of no-outstanding-ask:
		//     "Re: defect persists after PR #401" announces live work and
		//     qualifies. The honest gate is a router-generated typed ack with an
		//     explicit resolves_item relation — which does not exist yet.
		// A close is destructive and irreversible in practice (nobody re-reads
		// closed items), so the flag refuses rather than trusts inference.
		return fmt.Errorf("close-resolved is surface-only: closing on inferred PR numbers and title prefixes can retire live work " +
			"(cross-repo #NNN collision; ack titles that announce new defects). It lists candidates for a reader; " +
			"close individual items with `sirsi router close <id> --result @file` after reading them")
	}
	verb := "WOULD-CLOSE"
	closed := 0
	for _, c := range closable {
		nums := make([]string, 0, len(c.refs))
		for _, n := range c.refs {
			nums = append(nums, fmt.Sprintf("#%d(%s)", n, strings.ToLower(states[n])))
		}
		fmt.Printf("%s %s → %s\n    %s\n", verb, c.item.ID, strings.Join(nums, " "), truncateAt(c.item.Title, 96))
		if !routerCloseResolvedApply {
			continue
		}
		result := fmt.Sprintf(
			"CLOSED by `sirsi router close-resolved`: this acknowledgement's referenced pull requests all landed (%s).\n\n"+
				"A state reconciliation, not a review — an ack carries no outstanding ask, and its PRs are\n"+
				"demonstrably complete in the repository. Reopen if that is wrong.",
			strings.Join(nums, ", "))
		f, ferr := dispatch.OpenRoot(routerRoot)
		if ferr != nil {
			return fmt.Errorf("open router store: %w", ferr)
		}
		cerr := f.CloseItem(c.item.To, c.item.ID, result)
		_ = f.Close()
		if cerr != nil {
			fmt.Printf("    ✘ %v\n", cerr)
			continue
		}
		closed++
	}

	if len(review) > 0 {
		fmt.Printf("\n%d item(s) reference only landed PRs but are NOT acknowledgements — NOT closed, listed for judgement:\n", len(review))
		for _, c := range review {
			fmt.Printf("    · %s\n      %s\n", c.item.ID, truncateAt(c.item.Title, 96))
		}
		fmt.Println("  An item can name a merged PR while reporting a NEW problem about it.")
		fmt.Println("  Read each one; close by hand if its ask is genuinely delivered.")
	}
	fmt.Printf("\nretired %d/%d acknowledgement(s) of %d open; %d flagged for judgement; apply=%v\n",
		closed, len(closable), len(items), len(review), routerCloseResolvedApply)
	if !routerCloseResolvedApply && len(closable) > 0 {
		fmt.Println("Re-run with --apply to close the acknowledgements.")
	}
	return nil
}

func truncateAt(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
