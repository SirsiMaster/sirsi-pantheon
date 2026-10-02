package main

// `sirsi maat submit --kind tag|release|release-edit|merge --repo OWNER/REPO
// --ref REF` — Ma'at admission and attribution for GitHub submissions
// (Phase 1, owner directive 2026-09-27, router item 20260928-021707). Every
// agent pushes to GitHub as the one SirsiMaster account, so GitHub itself
// cannot tell lanes apart; this is the layer that can. The requester is
// derived from the caller's registered session→agent marker
// (internal/router.ReadSessionAgentMarker), then checked against the
// declared agent registry (dispatch.ValidateAgent, ADR-054) — never a flag
// or self-declared name.
//
// Scope honestly (codex-pantheon review 20260930-231226, item
// 20260930-230911): this is registry-declaration eligibility, the same
// boundary dispatch.ValidateAgent enforces everywhere else in this codebase
// — NOT live-thread/session authentication. A forged local marker file
// naming a real registered agent id still passes. Phase 1 is attribution
// against local host state, not a cryptographic identity proof; strengthen
// this later only alongside the rest of the router's session-binding work,
// not invented ad hoc here.
//
// Phase 1 scope only: admission + attribution. No watcher, no automatic
// release/tag mutation — those are explicitly deferred to a later phase with
// their own independent review (router item 20260930-225059).

import (
	"fmt"
	"os"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/decision"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

var (
	submitKind string
	submitRepo string
	submitRef  string
)

// submitRepoPolicy is the Phase 1 allowlist: canonical lowercase "owner/repo"
// -> the only agent ids permitted to submit tag/release/release-edit/merge
// admissions for it. Repos not listed here have no restriction defined yet
// and are admitted with a recorded decision rather than refused — Phase 1
// does not invent policy the owner hasn't stated (see router item
// 20260930-225059).
var submitRepoPolicy = map[string][]string{
	"sirsimaster/sirsi-hermes": {"hermes"},
	// Hermes is now Mercury (owner 2026-10-02): the repo is being renamed sirsi-hermes -> sirsi-mercury. The new name
	// must carry the same policy BEFORE the rename, or an unlisted repo is admitted for any lane. "mercury" is the
	// lane's router id once the registry adds it; until then the lane submits as "hermes".
	"sirsimaster/sirsi-mercury": {"hermes", "mercury"},
	"sirsimaster/sirsi-photon":  {"hermes"},
}

var submitValidKinds = map[string]bool{
	"tag": true, "release": true, "release-edit": true, "merge": true,
}

// canonicalRepo lowercases "OWNER/REPO" so GitHub's case-insensitive naming
// can't be used to slip past a case-sensitive policy lookup (codex-pantheon
// finding 2, item 20260930-231226: "sirsimaster/SIRSI-HERMES" must match the
// same policy row as "SirsiMaster/sirsi-hermes"). Also validates the shape.
func canonicalRepo(repo string) (string, error) {
	repo = strings.TrimSpace(repo)
	owner, name, ok := strings.Cut(repo, "/")
	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", fmt.Errorf("--repo must be OWNER/REPO, got %q", repo)
	}
	return strings.ToLower(owner) + "/" + strings.ToLower(name), nil
}

// resolveSubmitRequester derives the caller's identity from its registered
// Claude session marker, then confirms that identity is a declared agent in
// the repo's agent registry (dispatch.ValidateAgent, ADR-054) — the same
// eligibility boundary used everywhere else in this codebase. It never
// falls back to a flag, an env var, or a guessed PID; an unregistered
// session or an unregistered agent id is a refusal, not a guess.
func resolveSubmitRequester() (string, error) {
	sid := router.CurrentSessionID()
	if sid == "" {
		return "", fmt.Errorf("no active session id (%s unset); refusing to guess requester identity", router.SessionIDEnv)
	}
	agent := router.ReadSessionAgentMarker(sid)
	if agent == "" {
		return "", fmt.Errorf("session %s has no registered agent marker (sirsi thread register); refusing to guess requester identity", sid)
	}
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return "", fmt.Errorf("resolve router repo root: %w", err)
	}
	f, err := dispatch.Open(repoRoot)
	if err != nil {
		return "", fmt.Errorf("open router facade: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := f.ValidateAgent("requester", agent); err != nil {
		return "", fmt.Errorf("session %s marker names %q, which is not a declared agent: %w", sid, agent, err)
	}
	return agent, nil
}

// checkSubmitPolicy evaluates submitRepoPolicy for repo/requester. repo must
// already be canonicalRepo-normalized. A repo with no entry is granted
// (Phase 1 doesn't invent policy the owner hasn't stated); a repo with an
// entry grants only a listed requester.
func checkSubmitPolicy(repo, requester string) (determination, why string) {
	allowed, policyDefined := submitRepoPolicy[repo]
	if !policyDefined {
		return "grant", "no policy defined for repo; admitted with attribution"
	}
	for _, a := range allowed {
		if a == requester {
			return "grant", fmt.Sprintf("requester %q matches repo policy %v", requester, allowed)
		}
	}
	return "refuse", fmt.Sprintf("requester %q not in repo policy %v", requester, allowed)
}

// appendSubmitDecision writes the outcome to the Ma'at decision ledger and
// returns any write failure instead of swallowing it (codex-pantheon finding
// 4, item 20260930-231226): a ledger append failure must not be reported
// alongside a successful grant.
func appendSubmitDecision(requester, resource, repo, kind, determination, why string) error {
	return decision.Append("", decision.New("submit-admission", requester, resource, repo, kind, determination, why, ""))
}

var maatSubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "𓆄 Ma'at admission for a GitHub tag/release/merge submission (Phase 1: attribution only)",
	Long: `𓆄 Ma'at Submit — attribution layer for GitHub submissions (Phase 1)

Every Sirsi agent pushes to GitHub as the one SirsiMaster account, so GitHub
cannot distinguish which lane tagged, released, or merged something. This
records who actually did it and checks it against any per-repo policy.

Phase 1 is admission and attribution only: it derives the requester from your
registered session, confirms that identity is a declared agent (registry
eligibility, not live-session authentication — see dispatch.ValidateAgent),
checks repo policy, and writes a decision record. It does not watch GitHub
and does not mutate any tag, release, or PR.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		kind := strings.ToLower(strings.TrimSpace(submitKind))
		ref := strings.TrimSpace(submitRef)

		if !submitValidKinds[kind] {
			return fmt.Errorf("--kind must be one of tag|release|release-edit|merge, got %q", submitKind)
		}
		if ref == "" {
			return fmt.Errorf("--ref is required")
		}
		repo, err := canonicalRepo(submitRepo)
		if err != nil {
			return err
		}

		resource := fmt.Sprintf("%s#%s@%s", repo, kind, ref)

		requester, err := resolveSubmitRequester()
		if err != nil {
			// codex-pantheon review (router item 20261001-041706): this branch
			// discarded the ledger-append error while the policy branch below
			// correctly propagates it, making the documented "every grant/
			// refusal is written" contract false specifically for identity
			// refusals. The determination itself stays refuse either way (a
			// denied operation stays denied) — but an append failure must be
			// VISIBLE, not silently coexist with a clean-looking refusal.
			appendErr := appendSubmitDecision("unknown", resource, repo, kind, "refuse", err.Error())
			if maatJSON {
				payload := map[string]any{"determination": "refuse", "why": err.Error()}
				if appendErr != nil {
					payload["ledger_error"] = appendErr.Error()
				}
				if jsonErr := emitJSON(payload); jsonErr != nil {
					fmt.Fprintf(os.Stderr, "𓆄 WARNING: failed to emit JSON result: %s\n", jsonErr.Error())
				}
			} else {
				fmt.Printf("𓆄 refuse: %s\n", err.Error())
				if appendErr != nil {
					fmt.Printf("𓆄 WARNING: decision ledger append failed, this refusal was NOT recorded: %s\n", appendErr.Error())
				}
			}
			os.Exit(admissionRefusedExit)
		}

		determination, why := checkSubmitPolicy(repo, requester)
		if err := appendSubmitDecision(requester, resource, repo, kind, determination, why); err != nil {
			return fmt.Errorf("append decision ledger (determination was %q): %w", determination, err)
		}

		if maatJSON {
			if err := emitJSON(map[string]any{
				"requester":     requester,
				"repo":          repo,
				"kind":          kind,
				"ref":           ref,
				"determination": determination,
				"why":           why,
			}); err != nil {
				return err
			}
		} else {
			fmt.Printf("𓆄 %s: %s %s %s@%s — %s\n", determination, requester, repo, kind, ref, why)
		}

		if determination == "refuse" {
			os.Exit(admissionRefusedExit)
		}
		return nil
	},
}

func init() {
	maatSubmitCmd.Flags().StringVar(&submitKind, "kind", "", "tag|release|release-edit|merge")
	maatSubmitCmd.Flags().StringVar(&submitRepo, "repo", "", "OWNER/REPO")
	maatSubmitCmd.Flags().StringVar(&submitRef, "ref", "", "the tag, release, or ref being submitted")
	maatSubmitCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	maatCmd.AddCommand(maatSubmitCmd)
}
