package main

// `sirsi router wing authority grant|revoke` — rs-31b (ADR-066): issuing a
// wing-authority grant is server-side only, exactly like `sirsi router token
// mint|revoke` (routerservecmd.go) — run ON THE SERVICE HOST against its own
// backend via --store (default $SIRSI_ROUTER_STORE). Never reachable from a
// node (see routerstore/serve.go's notServed); RegisterWing is the only
// node-reachable half of wing admission.

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var routerWingAuthorityStore string

var routerWingAuthorityCmd = &cobra.Command{
	Use:   "authority",
	Short: "Issue or revoke wing-authority grants (run on the service host)",
}

var routerWingAuthorityGrantIssuer string
var routerWingAuthorityGrantEvidence []string

var routerWingAuthorityGrantCmd = &cobra.Command{
	Use:   "grant <principal> <project-id> <namespace> <repository-root>",
	Short: "Bind principal to admit wings for (project-id, namespace) rooted at repository-root",
	Long: `Issues a grant: principal may admit (RegisterWing) a Stack Lab wing record
for project-id/namespace whose workspace roots (repository_root, every
writable_root, evidence_root) resolve inside repository-root or one of
--evidence-root.

The bootstrap grant (no grants exist yet in this backend) is the one ungated
step — it requires direct access to the service's own backend, which running
this command already requires. Every later grant needs --issuer to already
hold an active grant whose roots contain repository-root and every
--evidence-root: a second owner requires delegation from authority that
already covers the requested scope, never a bare assertion.`,
	Args: cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openServeStore(routerWingAuthorityStore)
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		principal, projectID, namespace, repoRoot := args[0], args[1], args[2], args[3]
		grant, err := store.GrantWingAuthority(routerWingAuthorityGrantIssuer, principal, projectID, namespace, repoRoot, routerWingAuthorityGrantEvidence)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "granted %s: %s may admit wings for %s/%s rooted at %s (issuer=%s created=%s)\n",
			grant.GrantID, grant.Principal, grant.ProjectID, grant.RouterNamespace, grant.RepositoryRoot, grant.Issuer, grant.Created)
		return nil
	},
}

var routerWingAuthorityRevokeCmd = &cobra.Command{
	Use:   "revoke <grant-id>",
	Short: "Revoke a wing-authority grant (idempotent)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openServeStore(routerWingAuthorityStore)
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		if err := store.RevokeWingAuthority(args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "revoked %s\n", args[0])
		return nil
	},
}

func init() {
	routerWingAuthorityCmd.PersistentFlags().StringVar(&routerWingAuthorityStore, "store", os.Getenv("SIRSI_ROUTER_STORE"), "postgres:// DSN or SQLite path of the SERVICE's backend (required; default $SIRSI_ROUTER_STORE)")
	routerWingAuthorityGrantCmd.Flags().StringVar(&routerWingAuthorityGrantIssuer, "issuer", "", "Agent id delegating this grant (must already hold a covering active grant, unless this is the bootstrap grant)")
	routerWingAuthorityGrantCmd.Flags().StringArrayVar(&routerWingAuthorityGrantEvidence, "evidence-root", nil, "Additional permitted evidence root (repeatable)")
	routerWingAuthorityCmd.AddCommand(routerWingAuthorityGrantCmd, routerWingAuthorityRevokeCmd)
	routerWingCmd.AddCommand(routerWingAuthorityCmd)
}
