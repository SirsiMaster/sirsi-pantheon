package main

// `sirsi router relay serve` — the host-side half of the filesystem spool
// (ADR-062 step 20a.1b): the one process on a host that holds the router
// service token, forwarding requests that no-network lanes drop as files under
// the spool directory. Everything else about a call (session, nonce, runtime,
// HMAC) is produced by the lane and verified by the service unchanged.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

var relaySpool string

var routerRelayCmd = &cobra.Command{
	Use:   "relay",
	Short: "Filesystem relay for lanes without network (ADR-062 20a.1b): lanes write request files, the relay forwards them with the host token",
}

var routerRelayServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Forward spooled requests to the router service; the ONLY process on this host that needs SIRSI_ROUTER_TOKEN",
	Long: `Reads SIRSI_ROUTER_URL (https://…) and SIRSI_ROUTER_TOKEN from its own environment,
polls <spool>/<agent>/req/*.json, forwards each to the service replacing only the
Authorization header, and writes <spool>/<agent>/res/<id>.json atomically.
MintHostToken, RevokeHostToken and ListHostTokens are refused by name. Lanes set
SIRSI_ROUTER_URL=spool://<spool> and need no token.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		base := strings.TrimSpace(os.Getenv("SIRSI_ROUTER_URL"))
		if base == "" || routerstore.SpoolDir(base) != "" {
			return errors.New("router relay serve: SIRSI_ROUTER_URL must be the service's https URL in the relay's own environment (a spool:// URL is for lanes)")
		}
		tok := strings.TrimSpace(os.Getenv("SIRSI_ROUTER_TOKEN"))
		if tok == "" {
			return errors.New("router relay serve: SIRSI_ROUTER_TOKEN is empty; the relay is the token holder")
		}
		spool := relaySpool
		if spool == "" {
			home, _ := os.UserHomeDir()
			spool = filepath.Join(home, ".sirsi", "relay")
		}
		rl := &routerstore.Relay{Spool: spool, Base: base, Token: tok, Log: slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil))}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		fmt.Fprintf(cmd.ErrOrStderr(), "router relay: spool %s → %s (token held here only)\n", spool, base)
		return rl.Serve(ctx)
	},
}

func init() {
	routerRelayServeCmd.Flags().StringVar(&relaySpool, "spool", "", "spool directory (default ~/.sirsi/relay)")
	routerRelayCmd.AddCommand(routerRelayServeCmd)
	routerCmd.AddCommand(routerRelayCmd)
}
