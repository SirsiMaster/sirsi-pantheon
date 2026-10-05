package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
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
)

// recoveryServeConfig contains only public operator-admission authority. It
// contains no RFB credential or private signing material, and never changes
// macOS screen-sharing, TCC, FileVault, SIP, or Tailscale configuration.
type recoveryServeConfig struct {
	Nodes               []desktoprecovery.Node `json:"nodes"`
	AdmissionPublicKeys map[string]string      `json:"admission_public_keys"`
	SessionTTLSeconds   int                    `json:"session_ttl_seconds"`
}

var (
	recoveryConfigPath string
	recoveryListenAddr string
)

var recoveryServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve the loopback-only noVNC recovery bridge behind Tailscale Serve",
	Long: `Starts a browser-to-RFB recovery bridge only on loopback. Put it behind
authenticated Tailscale Serve for private encrypted transport. The JSON config
names approved private port-5900 nodes, exact HTTPS origins, and public keys
for short-lived Pantheon operator admissions. It never trusts identity headers,
accepts a destination from a browser, or contains a desktop credential.

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
		keys, err := recoveryPublicKeys(cfg.AdmissionPublicKeys)
		if err != nil {
			return err
		}
		ttl := 0 * time.Second
		if cfg.SessionTTLSeconds != 0 {
			ttl = time.Duration(cfg.SessionTTLSeconds) * time.Second
		}
		gateway, err := desktoprecovery.New(desktoprecovery.Config{
			Nodes:      cfg.Nodes,
			Authorizer: desktoprecovery.CapabilityAuthorizer{PublicKeys: keys},
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

func recoveryPublicKeys(encoded map[string]string) (map[string]ed25519.PublicKey, error) {
	if len(encoded) == 0 {
		return nil, errors.New("recovery serve: admission_public_keys is required")
	}
	keys := make(map[string]ed25519.PublicKey, len(encoded))
	for id, text := range encoded {
		if strings.TrimSpace(id) == "" {
			return nil, errors.New("recovery serve: admission_public_keys contains an empty key id")
		}
		bytes, err := base64.RawURLEncoding.DecodeString(text)
		if err != nil || len(bytes) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("recovery serve: admission public key %q must be base64url Ed25519", id)
		}
		keys[id] = ed25519.PublicKey(bytes)
	}
	return keys, nil
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
}
