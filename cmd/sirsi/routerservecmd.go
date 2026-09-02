package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

// ADR-062 §2 (rs-08): `sirsi router serve` is the Ra ledger service. It is the
// ONLY process that opens the database; every node reaches it through
// SIRSI_ROUTER_URL (routerstore.Resolve → RemoteStore). Cloud Run terminates
// TLS in front of it; --tls-cert/--tls-key exist for a self-hosted Ra.

var (
	routerServeListen   string
	routerServeStore    string
	routerServeTokenEnv string
	routerServeTLSCert  string
	routerServeTLSKey   string
	routerServeMaxWait  time.Duration
)

var routerServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve the router ledger over HTTP for every node in the fleet (ADR-062)",
	Long: `Serve the router ledger over HTTP (ADR-062 §2).

Backends (--store):
  postgres://user@host:port/db   the Ra ledger (Cloud SQL, or any Postgres)
  /path/to/router.db             a SQLite file — for a single-host service or tests

The bearer token is read from the environment variable named by --token-env
(default SIRSI_ROUTER_SERVE_TOKEN), never from a flag, so it does not appear in
process listings. Nodes present it as SIRSI_ROUTER_TOKEN.`,
	RunE: runRouterServe,
}

func init() {
	routerServeCmd.Flags().StringVar(&routerServeListen, "listen", ":8080", "address to listen on ($PORT overrides, for Cloud Run)")
	routerServeCmd.Flags().StringVar(&routerServeStore, "store", "", "postgres:// DSN or SQLite path (required)")
	routerServeCmd.Flags().StringVar(&routerServeTokenEnv, "token-env", "SIRSI_ROUTER_SERVE_TOKEN", "env var holding the bearer token")
	routerServeCmd.Flags().StringVar(&routerServeTLSCert, "tls-cert", "", "TLS certificate file (self-hosted; Cloud Run terminates TLS)")
	routerServeCmd.Flags().StringVar(&routerServeTLSKey, "tls-key", "", "TLS key file")
	routerServeCmd.Flags().DurationVar(&routerServeMaxWait, "max-wait", 60*time.Second, "ceiling for a Wait long-poll")
	routerCmd.AddCommand(routerServeCmd)
}

// openServeStore is the one place the service opens its backend. It is a
// serve-only path: the direct-open gate (scripts/check-router-store-open.sh)
// allowlists exactly this file for OpenPostgres/OpenPath.
func openServeStore(spec string) (routerstore.Store, error) {
	switch {
	case spec == "":
		return nil, errors.New("--store is required (postgres:// DSN or a SQLite path)")
	case strings.HasPrefix(spec, "postgres://") || strings.HasPrefix(spec, "postgresql://"):
		return routerstore.OpenPostgres(spec)
	default:
		return routerstore.OpenPath(spec)
	}
}

func runRouterServe(cmd *cobra.Command, _ []string) error {
	token := strings.TrimSpace(os.Getenv(routerServeTokenEnv))
	if token == "" {
		return fmt.Errorf("router serve: %s is empty; refusing to serve an unauthenticated ledger (ADR-062 §3)", routerServeTokenEnv)
	}
	store, err := openServeStore(routerServeStore)
	if err != nil {
		return fmt.Errorf("router serve: open store: %w", err)
	}
	defer func() { _ = store.Close() }()

	h, err := routerstore.Handler(store, routerstore.ServerOptions{Token: token, MaxWait: routerServeMaxWait})
	if err != nil {
		return err
	}
	addr := routerServeListen
	if p := os.Getenv("PORT"); p != "" { // Cloud Run contract
		addr = ":" + p
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		// Wait long-polls may legitimately hold a request for --max-wait.
		WriteTimeout: routerServeMaxWait + 15*time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(cmd.OutOrStdout(), "router serve: listening on %s (store: %s)\n", addr, redactDSN(routerServeStore))
		if routerServeTLSCert != "" {
			errCh <- srv.ListenAndServeTLS(routerServeTLSCert, routerServeTLSKey)
			return
		}
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), routerServeMaxWait+5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// redactDSN hides credentials in a postgres:// DSN for logs.
func redactDSN(s string) string {
	if i := strings.Index(s, "://"); i >= 0 {
		if at := strings.LastIndex(s, "@"); at > i {
			return s[:i+3] + "…@" + s[at+1:]
		}
	}
	return s
}

// ── per-host tokens (ADR-062 §3/§4, rs-11) ────────────────────────────────
// Run ON THE SERVICE HOST against the service's own backend (--store); these
// verbs are never served over the wire. Plaintext is printed exactly once.

var routerTokenStore string

var routerTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Mint, list or revoke per-host bearer tokens for the router service (run on the service host)",
}

var routerTokenMintCmd = &cobra.Command{
	Use:   "mint <host>",
	Short: "Mint a bearer token for one host; prints the plaintext once",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openServeStore(routerTokenStore)
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		label, _ := cmd.Flags().GetString("label")
		plain, rec, err := store.MintHostToken(args[0], label)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "token id: %s  host: %s  created: %s\n", rec.ID, rec.Host, rec.Created)
		fmt.Fprintf(cmd.OutOrStdout(), "SIRSI_ROUTER_TOKEN=%s\n", plain)
		fmt.Fprintln(cmd.ErrOrStderr(), "  (shown once; only its hash is stored)")
		return nil
	},
}

var routerTokenRevokeCmd = &cobra.Command{
	Use:   "revoke <token-id>",
	Short: "Revoke a host token and every session minted under its host",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openServeStore(routerTokenStore)
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		if err := store.RevokeHostToken(args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "revoked %s (effective on that host's next request)\n", args[0])
		return nil
	},
}

var routerTokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List host tokens (ids, hosts, labels; never plaintext)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		store, err := openServeStore(routerTokenStore)
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		list, err := store.ListHostTokens()
		if err != nil {
			return err
		}
		for _, t := range list {
			state := "active"
			if t.Revoked != "" {
				state = "revoked " + t.Revoked
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s  %-16s %-24s %s  %s\n", t.ID, t.Host, t.Label, t.Created, state)
		}
		return nil
	},
}

func init() {
	routerTokenCmd.PersistentFlags().StringVar(&routerTokenStore, "store", "", "postgres:// DSN or SQLite path of the SERVICE's backend (required)")
	routerTokenMintCmd.Flags().String("label", "", "free-text label (e.g. the machine's name)")
	routerTokenCmd.AddCommand(routerTokenMintCmd, routerTokenRevokeCmd, routerTokenListCmd)
	routerCmd.AddCommand(routerTokenCmd)
}

// ── migrate-store (ADR-062 rs-12) ─────────────────────────────────────────
// Copy a ledger into the service's backend with proof: quiesce (fabric
// quarantine marker held for the whole run, released only by this command's
// exit path), canonical dump hashes on both sides, dry-run, full diff,
// idempotent re-import. Run ON THE SERVICE HOST.

var (
	migrateStoreFrom   string
	migrateStoreTo     string
	migrateStoreDryRun bool
	migrateStoreJSON   bool
	migrateStoreScrub  bool
)

var routerMigrateStoreCmd = &cobra.Command{
	Use:   "migrate-store",
	Short: "Copy a SQLite ledger into the service backend with hash proof (ADR-062 rs-12; run on the service host)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if migrateStoreFrom == "" || migrateStoreTo == "" {
			return errors.New("--from <sqlite path> and --to <postgres:// DSN | sqlite path> are required")
		}
		home, _ := os.UserHomeDir()
		marker := router.FabricQuarantineMarkerPath(home)
		created := false
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(marker, []byte("migrate-store "+time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
				return fmt.Errorf("set fabric quarantine marker: %w", err)
			}
			created = true
			fmt.Fprintf(cmd.ErrOrStderr(), "quiesce: fabric quarantine marker set (%s)\n", marker)
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), "quiesce: fabric quarantine marker already present (%s); left in place\n", marker)
		}
		// Released ONLY here, success or failure, and only if we set it.
		defer func() {
			if created {
				_ = os.Remove(marker)
				fmt.Fprintf(cmd.ErrOrStderr(), "quiesce: marker released by migrate-store exit path\n")
			}
		}()

		src, err := routerstore.OpenPath(migrateStoreFrom)
		if err != nil {
			return fmt.Errorf("open source: %w", err)
		}
		defer func() { _ = src.Close() }()
		dstStore, err := openServeStore(migrateStoreTo)
		if err != nil {
			return fmt.Errorf("open destination: %w", err)
		}
		defer func() { _ = dstStore.Close() }()
		dst, ok := dstStore.(*routerstore.SQLiteStore)
		if !ok {
			return errors.New("destination must be a direct backend (postgres:// or a SQLite path), not a service URL")
		}

		rep, err := routerstore.MigrateStore(src, dst, routerstore.MigrateOptions{DryRun: migrateStoreDryRun, ScrubNUL: migrateStoreScrub})
		if migrateStoreJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(rep)
		} else {
			mode := "IMPORT"
			if migrateStoreDryRun {
				mode = "DRY RUN"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s  source %s  (%d tables)\n", mode, rep.Source.SHA256, len(rep.Source.Tables))
			for _, t := range rep.Source.Tables {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-16s rows=%-6d would_write=%-6d wrote=%-6d %s\n", t.Table, t.Rows, rep.WouldWrite[t.Table], rep.Wrote[t.Table], t.SHA256[:12])
			}
			if !migrateStoreDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "destination %s\n", rep.Destination.SHA256)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "source after %s\n", rep.SourceAfter)
			if len(rep.NULCells) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "NUL cells in source: %d (first: %s)%s\n", len(rep.NULCells), rep.NULCells[0], map[bool]string{true: " — scrubbed", false: " — REFUSED; pass --scrub-nul to strip them"}[migrateStoreScrub])
			}
			if rep.TriggerExtrasRemoved > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "trigger-minted wake events removed from destination: %d\n", rep.TriggerExtrasRemoved)
			}
		}
		if err != nil {
			return err
		}
		if !migrateStoreDryRun {
			fmt.Fprintln(cmd.OutOrStdout(), "OK: destination dump hash equals source dump hash; run again to prove idempotence (expect wrote=0 everywhere)")
		}
		return nil
	},
}

func init() {
	routerMigrateStoreCmd.Flags().StringVar(&migrateStoreFrom, "from", "", "source SQLite ledger path")
	routerMigrateStoreCmd.Flags().StringVar(&migrateStoreTo, "to", "", "destination: postgres:// DSN or SQLite path")
	routerMigrateStoreCmd.Flags().BoolVar(&migrateStoreDryRun, "dry-run", false, "report what would be written; write nothing")
	routerMigrateStoreCmd.Flags().BoolVar(&migrateStoreJSON, "json", false, "machine-readable report")
	routerMigrateStoreCmd.Flags().BoolVar(&migrateStoreScrub, "scrub-nul", false, "strip 0x00 bytes from text cells (Postgres cannot store them); the report lists every cell touched")
	routerCmd.AddCommand(routerMigrateStoreCmd)
}
