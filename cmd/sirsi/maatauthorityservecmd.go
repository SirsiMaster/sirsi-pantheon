package main

// `sirsi maat authority serve` — ADR-120 step 3 (adr120-s3-01-server-cmd):
// serves a pod-local Ma'at booking authority over HTTP for every member of
// its pod (ADR-120 §2, §6), the rs-08-serve pattern at the scope §6 asks
// for: a pod secret bearer, not ADR-062's full node-session chain.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/authority"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

var (
	authServeListen      string
	authServePod         string
	authServeLocalStore  string
	authServeJournal     string
	authServeSecretEnv   string
	authServeTLSCert     string
	authServeTLSKey      string
	authServeRenewEvery  time.Duration
	authServeCallTimeout time.Duration
	authServeMaxInFlight int
)

var maatAuthorityCmd = &cobra.Command{
	Use:   "authority",
	Short: "Pod-local Ma'at booking authority (ADR-120)",
}

var maatAuthorityServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve a pod-local Ma'at booking authority over HTTP (ADR-120 §2, §6)",
	Long: `Serves the pod authority named by --pod: the existing schedule.Ledger
admission (floor share, cedes, conflicts unchanged) over a LOCAL SQLite
store, journaled and write-through to the canonical ledger when reachable
(ADR-120 §2-5).

Starting this command requires the cloud ledger to be reachable once, to
learn --pod's current delegation (epoch, resources, not_after) — the
authority has no identity of its own until then. Once running, it keeps
granting through its own fence until not_after even if the cloud goes away.

Pod members authenticate with the pod secret read from the environment
variable named by --secret-env (default SIRSI_MAAT_POD_SECRET) — never a
flag, so it does not appear in process listings (mirrors "sirsi router
serve"'s --token-env).`,
	RunE: runMaatAuthorityServe,
}

func init() {
	maatAuthorityServeCmd.Flags().StringVar(&authServeListen, "listen", ":8443", "address to listen on")
	maatAuthorityServeCmd.Flags().StringVar(&authServePod, "pod", "", "pod id this authority serves (required, e.g. studio-m5)")
	maatAuthorityServeCmd.Flags().StringVar(&authServeLocalStore, "local-store", "", "SQLite path for this authority's local ledger (required)")
	maatAuthorityServeCmd.Flags().StringVar(&authServeJournal, "journal", "", "path for the durable grant journal (required)")
	maatAuthorityServeCmd.Flags().StringVar(&authServeSecretEnv, "secret-env", "SIRSI_MAAT_POD_SECRET", "env var holding the pod secret")
	maatAuthorityServeCmd.Flags().StringVar(&authServeTLSCert, "tls-cert", "", "TLS certificate file")
	maatAuthorityServeCmd.Flags().StringVar(&authServeTLSKey, "tls-key", "", "TLS key file")
	maatAuthorityServeCmd.Flags().DurationVar(&authServeRenewEvery, "renew-every", 15*time.Minute, "cloud renewal interval (ADR-120 §4)")
	maatAuthorityServeCmd.Flags().DurationVar(&authServeCallTimeout, "call-timeout", 10*time.Second, "ceiling for a Reserve/Renew/CatchUp call")
	maatAuthorityServeCmd.Flags().IntVar(&authServeMaxInFlight, "max-inflight", 8, "max concurrent calls before refusing with 503")
	maatAuthorityCmd.AddCommand(maatAuthorityServeCmd)
	maatCmd.AddCommand(maatAuthorityCmd)
}

func runMaatAuthorityServe(cmd *cobra.Command, _ []string) error {
	secret := strings.TrimSpace(os.Getenv(authServeSecretEnv))
	if secret == "" {
		return fmt.Errorf("maat authority serve: %s is empty; refusing to serve unauthenticated (ADR-120 §6)", authServeSecretEnv)
	}
	if authServePod == "" || authServeLocalStore == "" || authServeJournal == "" {
		return errors.New("maat authority serve: --pod, --local-store, and --journal are required")
	}

	localStore, err := routerstore.OpenPath(authServeLocalStore)
	if err != nil {
		return fmt.Errorf("maat authority serve: open local store: %w", err)
	}
	defer func() { _ = localStore.Close() }()
	local := schedule.NewLedger(localStore)

	journal, err := authority.OpenFileJournal(authServeJournal)
	if err != nil {
		return fmt.Errorf("maat authority serve: open journal: %w", err)
	}

	cloud, err := maatLedger()
	if err != nil {
		return fmt.Errorf("maat authority serve: this authority must start online to learn its delegation (ADR-120 §1): %w", err)
	}
	d, err := cloud.GetDelegation(authServePod)
	if err != nil {
		return fmt.Errorf("maat authority serve: look up delegation for pod %q: %w", authServePod, err)
	}
	if d == nil {
		return fmt.Errorf("maat authority serve: no delegation for pod %q; run `sirsi maat delegate` first", authServePod)
	}

	auth, err := authority.New(*d, local, cloud, journal)
	if err != nil {
		return fmt.Errorf("maat authority serve: %w", err)
	}

	h, err := authority.Handler(auth, authority.ServerOptions{
		PodSecret:   secret,
		CallTimeout: authServeCallTimeout,
		MaxInFlight: authServeMaxInFlight,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              authServeListen,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      authServeCallTimeout + 5*time.Second,
		IdleTimeout:       120 * time.Second,
	}

	stopRenew := make(chan struct{})
	go renewLoop(cmd.ErrOrStderr(), auth, authServeRenewEvery, stopRenew)
	defer close(stopRenew)

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(cmd.OutOrStdout(), "maat authority serve: pod %q listening on %s\n", authServePod, authServeListen)
		if authServeTLSCert != "" {
			errCh <- srv.ListenAndServeTLS(authServeTLSCert, authServeTLSKey)
			return
		}
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), authServeCallTimeout+5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// renewLoop keeps the delegation's not_after extended while the cloud is
// reachable (ADR-120 §4). A failed renew is logged, never fatal: the
// existing not_after still holds until Authority.Fenced observes real expiry
// or a self-fence (superseded/revoked) stops granting immediately.
func renewLoop(w io.Writer, auth *authority.Authority, every time.Duration, stop <-chan struct{}) {
	if every <= 0 {
		every = 15 * time.Minute
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if err := auth.Renew(); err != nil {
				fmt.Fprintf(w, "maat authority serve: renew: %v\n", err)
			}
		}
	}
}
