package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
