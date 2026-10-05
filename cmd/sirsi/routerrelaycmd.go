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

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

var relaySpool string
var relayRepairAgent string

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
SIRSI_ROUTER_URL=spool://<spool> and need no token.

SIRSI_RELAY_TRUST_GROUP names a group whose members may also own the spool
directory (checked via CheckSpoolDirTrustingGroup instead of the default
single-UID CheckSpoolDir) — for a relay running under a dedicated service
account that must still serve lane clients running as a different uid. Unset
by default: leaving it empty preserves exact single-UID behavior.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		base, err := routerstore.CheckServiceURL(os.Getenv("SIRSI_ROUTER_URL"))
		if err != nil {
			return fmt.Errorf("router relay serve: %w (a spool:// URL is for lanes; the relay needs the service's https URL in its own environment)", err)
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
		rl := &routerstore.Relay{
			Spool:      spool,
			Base:       base,
			Token:      tok,
			TrustGroup: strings.TrimSpace(os.Getenv("SIRSI_RELAY_TRUST_GROUP")),
			Log:        slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil)),
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		trustNote := ""
		if rl.TrustGroup != "" {
			trustNote = fmt.Sprintf(" (trusting group %q)", rl.TrustGroup)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "router relay: spool %s → %s (token held here only)%s\n", spool, base, trustNote)
		return rl.Serve(ctx)
	},
}

var routerRelayInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install (or refresh) the ai.sirsi.router.relay LaunchAgent — the host's only token holder — from this shell's SIRSI_ROUTER_URL/TOKEN",
	RunE: func(cmd *cobra.Command, _ []string) error {
		spool := relaySpool
		if spool == "" {
			home, _ := os.UserHomeDir()
			spool = filepath.Join(home, ".sirsi", "relay")
		}
		changed, path, err := router.InstallRelayLaunchAgent(spool, os.Getenv("SIRSI_ROUTER_URL"), os.Getenv("SIRSI_ROUTER_TOKEN"))
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "relay LaunchAgent %s (%s): %s; spool %s\n", router.RelayLaunchAgentLabel, map[bool]string{true: "written", false: "unchanged"}[changed], path, spool)
		fmt.Fprintln(cmd.OutOrStdout(), "lanes: SIRSI_ROUTER_URL=spool://"+spool+" and no SIRSI_ROUTER_TOKEN; codex lanes add -c sandbox_workspace_write.writable_roots=[\""+spool+"\"]")
		return nil
	},
}

var routerRelayRepairOutboxCmd = &cobra.Command{
	Use:   "repair-outbox --agent AGENT",
	Short: "Restore one locally-owned unreadable relay outbox through retained no-follow descriptors",
	Long: `Repairs only the exact local <spool>/<agent>/outbox directory when it is
owned by the current user and can be opened without following any link. It sets
that retained directory to mode 0700, persists the change, and never drains,
replays, deletes, or changes queued messages. Any ownership, type, link, or
persistence ambiguity is refused without a pathname-based fallback.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		spool := relaySpool
		if spool == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("router relay repair-outbox: determine home directory: %w", err)
			}
			spool = filepath.Join(home, ".sirsi", "relay")
		}
		repair, err := routerstore.RepairSpoolOutbox(spool, relayRepairAgent)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "relay outbox repaired: agent %s mode %s → %s at %s; queued messages were not changed\n", repair.Agent, repair.PreviousMode, repair.RepairedMode, repair.CanonicalSpool)
		return nil
	},
}

func init() {
	routerRelayServeCmd.Flags().StringVar(&relaySpool, "spool", "", "spool directory (default ~/.sirsi/relay)")
	routerRelayInstallCmd.Flags().StringVar(&relaySpool, "spool", "", "spool directory (default ~/.sirsi/relay)")
	routerRelayRepairOutboxCmd.Flags().StringVar(&relaySpool, "spool", "", "spool directory (default ~/.sirsi/relay)")
	routerRelayRepairOutboxCmd.Flags().StringVar(&relayRepairAgent, "agent", "", "relay agent whose unreadable outbox should be repaired")
	_ = routerRelayRepairOutboxCmd.MarkFlagRequired("agent")
	routerRelayCmd.AddCommand(routerRelayServeCmd, routerRelayInstallCmd, routerRelayRepairOutboxCmd)
	routerCmd.AddCommand(routerRelayCmd)
}
