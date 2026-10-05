package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/desktoprecovery"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

// recoveryServeConfig is intentionally small: it contains only node and
// operator allowlists. It contains no RFB credential, and never changes macOS
// screen-sharing, TCC, FileVault, SIP, or Tailscale configuration.
type recoveryServeConfig struct {
	Nodes                  []desktoprecovery.Node `json:"nodes"`
	AllowedTailnetLogins   []string               `json:"allowed_tailnet_logins"`
	OperatorPasswordHashes map[string]string      `json:"operator_password_hashes"`
	SessionTTLSeconds      int                    `json:"session_ttl_seconds"`
}

var (
	recoveryConfigPath string
	recoveryListenAddr string
)

var recoveryCmd = &cobra.Command{
	Use:   "recovery",
	Short: "Authenticated browser recovery for approved private Mac desktops",
}

var recoveryServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve the loopback-only noVNC recovery bridge behind Tailscale Serve",
	Long: `Starts a browser-to-RFB recovery bridge only on loopback. Put it behind
authenticated Tailscale Serve and use its authenticated identity headers. The
JSON config names approved private port-5900 nodes, exact HTTPS origins, and
operator logins. It never accepts a destination or credential from a browser.

This command does not configure Screen Sharing, Tailscale Serve, permissions,
or launchd. Those are separately qualified host operations.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadRecoveryConfig(recoveryConfigPath)
		if err != nil {
			return err
		}
		if !isLoopbackListen(recoveryListenAddr) {
			return errors.New("recovery serve: --listen must be a literal loopback address; place the bridge behind authenticated Tailscale Serve")
		}
		allow := make(map[string]struct{}, len(cfg.AllowedTailnetLogins))
		for _, login := range cfg.AllowedTailnetLogins {
			login = strings.TrimSpace(login)
			if login == "" {
				return errors.New("recovery serve: allowed_tailnet_logins contains an empty login")
			}
			hash := cfg.OperatorPasswordHashes[login]
			cost, err := bcrypt.Cost([]byte(hash))
			if err != nil || cost < 10 || cost > 14 {
				return errors.New("recovery serve: each operator requires a bcrypt password hash with cost 10 through 14")
			}
			allow[login] = struct{}{}
		}
		if len(allow) == 0 {
			return errors.New("recovery serve: allowed_tailnet_logins is required")
		}
		ttl := 0 * time.Second
		if cfg.SessionTTLSeconds != 0 {
			ttl = time.Duration(cfg.SessionTTLSeconds) * time.Second
		}
		gateway, err := desktoprecovery.New(desktoprecovery.Config{
			Nodes:      cfg.Nodes,
			Authorizer: desktoprecovery.TailnetHeaderAuthorizer{AllowedLogins: allow, PasswordHashes: cfg.OperatorPasswordHashes},
			SessionTTL: ttl,
		})
		if err != nil {
			return err
		}
		listener, err := net.Listen("tcp", recoveryListenAddr)
		if err != nil {
			return fmt.Errorf("recovery serve: listen: %w", err)
		}
		defer listener.Close()
		server := &http.Server{Handler: gateway.Handler(), ReadHeaderTimeout: 5 * time.Second}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		errCh := make(chan error, 1)
		go func() { errCh <- server.Serve(listener) }()
		fmt.Fprintf(cmd.OutOrStdout(), "Pantheon recovery bridge listening at http://%s/recovery/novnc/vnc.html (loopback only; authenticated Tailscale Serve required)\n", listener.Addr())
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("recovery serve: %w", err)
			}
			return nil
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return server.Shutdown(shutdown)
		}
	},
}

func loadRecoveryConfig(path string) (recoveryServeConfig, error) {
	if strings.TrimSpace(path) == "" {
		return recoveryServeConfig{}, errors.New("recovery serve: --config is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return recoveryServeConfig{}, fmt.Errorf("recovery serve: read config: %w", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	var cfg recoveryServeConfig
	if err := dec.Decode(&cfg); err != nil {
		return recoveryServeConfig{}, fmt.Errorf("recovery serve: decode config: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return recoveryServeConfig{}, errors.New("recovery serve: config contains more than one JSON value")
		}
		return recoveryServeConfig{}, fmt.Errorf("recovery serve: trailing config data: %w", err)
	}
	return cfg, nil
}

func isLoopbackListen(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func init() {
	recoveryServeCmd.Flags().StringVar(&recoveryConfigPath, "config", "", "approved recovery node/operator JSON config")
	recoveryServeCmd.Flags().StringVar(&recoveryListenAddr, "listen", "127.0.0.1:9188", "literal loopback address to listen on")
	recoveryCmd.AddCommand(recoveryServeCmd)
	rootCmd.AddCommand(recoveryCmd)
}
