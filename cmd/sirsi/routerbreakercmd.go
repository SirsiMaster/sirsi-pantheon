package main

// Operator surface for the dispatch circuit breakers (ADR-035 §2b axiom 6).
//
// routerstore has shipped Breakers() and ResetBreaker() since the breakers
// landed, but nothing ever called them from a command. That left the breaker
// with no operator lever at all: a tripped domain gates SendGuarded (facade.go
// breakerGateTx on "global" and "sender:<from>"), and the escalation it writes
// tells the operator to run `sirsi router breaker-reset <domain>` — a verb that
// did not exist. On 2026-08-07 sender:claude-home tripped and the router
// conduit could not send a single item, including the report about being
// unable to send.
//
// These two verbs wrap the existing store methods and add nothing else.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

// routerBreakersJSON is the machine contract for `router breakers`.
var routerBreakersJSON bool

// openRouterStore resolves the same database every other router verb uses:
// SIRSI_ROUTER_DB when set, else ~/.sirsi/router.db. Pointing a breaker verb
// at a different store than the dispatcher would let an operator "clear" a
// breaker that is still tripped for everyone else.
func openRouterStore() (routerstore.Store, error) {
	return routerstore.Resolve()
}

var routerBreakersCmd = &cobra.Command{
	Use:   "breakers",
	Short: "List dispatch circuit breakers — tripped domains first",
	Long: `Show every failure domain with recorded failures.

A domain with a tripped_at timestamp is PAUSING DISPATCH through that path:
  sender:<id>  every item that agent tries to send
  target:<id>  every item addressed to that agent
  class:<c>    every item of that failure class
  global       the whole fabric

Clear one with: sirsi router breaker-reset <domain>`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		store, err := openRouterStore()
		if err != nil {
			return err
		}
		defer store.Close()

		breakers, err := store.Breakers()
		if err != nil {
			return err
		}
		if routerBreakersJSON {
			out, mErr := json.MarshalIndent(breakers, "", "  ")
			if mErr != nil {
				return mErr
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		}
		if len(breakers) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no recorded dispatch failures")
			return nil
		}
		for _, b := range breakers {
			state := "ok"
			if b.TrippedAt != "" {
				state = "TRIPPED " + b.TrippedAt
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-28s failures=%-4d %s\n", b.Domain, b.Failures, state)
		}
		return nil
	},
}

var routerBreakerResetCmd = &cobra.Command{
	Use:   "breaker-reset <domain>",
	Short: "Clear one tripped circuit breaker after fixing its cause",
	Long: `Re-arm a dispatch path. Clears the trip AND the failure count.

This is a deliberate operator action: the breaker exists so that a failing
path stops rather than retries into a flood. Fix the cause first, then reset.

  sirsi router breaker-reset sender:claude-home`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := strings.TrimSpace(args[0])
		if domain == "" {
			return fmt.Errorf("domain is required")
		}
		store, err := openRouterStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.ResetBreaker(domain); err != nil {
			return fmt.Errorf("breaker-reset %s: %w", domain, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "breaker %s reset — dispatch resumed\n", domain)
		return nil
	},
}
