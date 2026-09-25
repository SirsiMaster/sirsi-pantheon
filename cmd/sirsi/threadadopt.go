package main

// `sirsi thread adopt` — ADR-067 (rs-42): bind THIS machine's stable id to the
// host token it already holds, so the lane's identity survives hostname drift
// (DHCP/mDNS renames) instead of orphaning its threads. The credential is the
// token the session already presents; the server injects the authenticated host
// and records the machine id once. This is the SANCTIONED migration — never a
// runtime shape guess (IDENTITY_ARCHITECTURE.md §4).

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/machineid"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

var threadAdoptMachineID string

var threadAdoptCmd = &cobra.Command{
	Use:   "adopt",
	Short: "Bind this machine's stable id to its host token (ADR-067) so identity survives hostname drift",
	Long: `Adopt this machine's stable hardware id onto the host token this lane
authenticates with. After adoption, sessions may claim the machine id (via
SIRSI_ROUTER_USE_MACHINE_ID) and still resolve to this host's threads even after
the hostname changes — the fix for the "thread authority: a session may only …
on its own host" refusals that hostname drift causes.

The host is taken from the authenticated session (never a client argument), and
the id is recorded ONCE: re-adopting the same id is a no-op, a different id is
refused (mint a fresh token instead), and an id already held by another live
token is refused (revoke that token first).`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		machineID := threadAdoptMachineID
		if machineID == "" {
			machineID = machineid.MachineID()
		}
		if machineID == "" {
			return fmt.Errorf("no stable machine id available on this platform; pass --machine-id explicitly")
		}
		store, err := routerstore.Resolve()
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()

		// host is injected server-side from the authenticated session; the arg is ignored.
		err = store.AdoptTokenMachineID("", machineID)
		out := cmd.OutOrStdout()
		if JsonOutput {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			res := map[string]any{"machine_id": machineID, "adopted": err == nil}
			if err != nil {
				res["error"] = err.Error()
			}
			_ = enc.Encode(res)
			return err
		}
		switch {
		case err == nil:
			fmt.Fprintf(out, "adopted: this host token now also answers to machine id %s\n", machineID)
		case errors.Is(err, routerstore.ErrNoTokenForHost):
			fmt.Fprintln(out, "no host token for this session's host — mint one first (`sirsi router token mint <host>`, on the service host) and re-run from a session that presents it")
		case errors.Is(err, routerstore.ErrMachineIDClaimed):
			fmt.Fprintf(out, "machine id %s is already adopted by another live token — revoke that token first (`sirsi router token revoke <id>`)\n", machineID)
		case errors.Is(err, routerstore.ErrMachineIDAdopted):
			fmt.Fprintln(out, "this token already adopted a different machine id (adoption is one-way) — mint a fresh token to rebind")
		}
		return err
	},
}

func init() {
	threadAdoptCmd.Flags().StringVar(&threadAdoptMachineID, "machine-id", "", "machine id to adopt (default: this machine's hardware id)")
	threadCmd.AddCommand(threadAdoptCmd)
}
