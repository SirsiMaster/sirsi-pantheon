package main

// `sirsi maat submit --kind tag|release|release-edit|merge --repo OWNER/REPO
// --ref REF` — Ma'at admission and attribution for GitHub submissions
// (Phase 1, owner directive 2026-09-27, router item 20260928-021707). Every
// agent pushes to GitHub as the one SirsiMaster account, so GitHub itself
// cannot tell lanes apart; this is the layer that can. The requester is
// derived from the caller's registered session→agent marker
// (internal/router.ReadSessionAgentMarker), never a flag or self-declared
// name — an unregistered session is refused, not guessed at (Truth Vector
// A23). The decision is written to the existing host-local Ma'at decision
// ledger (internal/maat/decision) so `sirsi maat decisions` shows it.
//
// Phase 1 scope only: admission + attribution. No watcher, no automatic
// release/tag mutation — those are explicitly deferred to a later phase with
// their own independent review (router item 20260930-225059).

import (
	"fmt"
	"os"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

var (
	submitKind string
	submitRepo string
	submitRef  string
)

// submitRepoPolicy is the Phase 1 allowlist: repo -> the only agent ids
// permitted to submit tag/release/release-edit/merge admissions for it.
// Repos not listed here have no restriction defined yet and are admitted
// with a recorded decision rather than refused — Phase 1 does not invent
// policy the owner hasn't stated (see router item 20260930-225059).
var submitRepoPolicy = map[string][]string{
	"SirsiMaster/sirsi-hermes": {"hermes"},
	"SirsiMaster/sirsi-photon": {"hermes"},
}

var submitValidKinds = map[string]bool{
	"tag": true, "release": true, "release-edit": true, "merge": true,
}

// resolveSubmitRequester derives the caller's identity from its registered
// Claude session marker. It never falls back to a flag, an env var, or a
// guessed PID — an unregistered session is a refusal, not a guess.
func resolveSubmitRequester() (string, error) {
	sid := router.CurrentSessionID()
	if sid == "" {
		return "", fmt.Errorf("no active session id (%s unset); refusing to guess requester identity", router.SessionIDEnv)
	}
	agent := router.ReadSessionAgentMarker(sid)
	if agent == "" {
		return "", fmt.Errorf("session %s has no registered agent marker (sirsi thread register); refusing to guess requester identity", sid)
	}
	return agent, nil
}

// checkSubmitPolicy evaluates submitRepoPolicy for repo/requester. A repo
// with no entry is granted (Phase 1 doesn't invent policy the owner hasn't
// stated); a repo with an entry grants only a listed requester.
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

var maatSubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "𓆄 Ma'at admission for a GitHub tag/release/merge submission (Phase 1: attribution only)",
	Long: `𓆄 Ma'at Submit — attribution layer for GitHub submissions (Phase 1)

Every Sirsi agent pushes to GitHub as the one SirsiMaster account, so GitHub
cannot distinguish which lane tagged, released, or merged something. This
records who actually did it and checks it against any per-repo policy.

Phase 1 is admission and attribution only: it derives the requester from your
registered session, checks policy, and writes a decision record. It does not
watch GitHub and does not mutate any tag, release, or PR.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		kind := strings.ToLower(strings.TrimSpace(submitKind))
		repo := strings.TrimSpace(submitRepo)
		ref := strings.TrimSpace(submitRef)

		if !submitValidKinds[kind] {
			return fmt.Errorf("--kind must be one of tag|release|release-edit|merge, got %q", submitKind)
		}
		if repo == "" {
			return fmt.Errorf("--repo is required (OWNER/REPO)")
		}
		if ref == "" {
			return fmt.Errorf("--ref is required")
		}

		resource := fmt.Sprintf("%s#%s@%s", repo, kind, ref)

		requester, err := resolveSubmitRequester()
		if err != nil {
			recordDecision("submit-admission", "unknown", resource, repo, kind, "refuse", err.Error(), "")
			if maatJSON {
				_ = emitJSON(map[string]any{"determination": "refuse", "why": err.Error()})
			}
			os.Exit(admissionRefusedExit)
			return nil
		}

		determination, why := checkSubmitPolicy(repo, requester)
		recordDecision("submit-admission", requester, resource, repo, kind, determination, why, "")

		if maatJSON {
			return emitJSON(map[string]any{
				"requester":     requester,
				"repo":          repo,
				"kind":          kind,
				"ref":           ref,
				"determination": determination,
				"why":           why,
			})
		}
		fmt.Printf("𓆄 %s: %s %s %s@%s — %s\n", determination, requester, repo, kind, ref, why)
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
