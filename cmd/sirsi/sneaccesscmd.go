package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/spf13/cobra"
)

var (
	sneAccessReveal  bool
	sneRotateConfirm bool
)

var sneCmd = &cobra.Command{Use: "sne", Short: "Use and manage the local Sirsi Nexus Engine"}
var sneAccessCmd = &cobra.Command{Use: "access", Short: "Manage the private local SNE API capability"}

var sneAccessStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show local SNE capability readiness without revealing the secret",
	RunE: func(_ *cobra.Command, _ []string) error {
		path, err := dashboard.DefaultSNELocalAccessTokenPath()
		if err != nil {
			return err
		}
		token, err := dashboard.LoadOrCreateSNELocalAccessToken(path)
		if err != nil {
			return err
		}
		if JsonOutput {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"configured": true, "path": path, "token_bytes": len(token), "revealed": false})
		}
		fmt.Printf("SNE local access is configured.\nCredential: private and not displayed\nPath: %s\n", path)
		return nil
	},
}

var sneAccessTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Reveal the local API token only with --reveal",
	RunE: func(_ *cobra.Command, _ []string) error {
		if !sneAccessReveal {
			return fmt.Errorf("refusing to print the local capability without --reveal")
		}
		token, err := dashboard.LoadOrCreateDefaultSNELocalAccessToken()
		if err != nil {
			return err
		}
		fmt.Println(token)
		return nil
	},
}

var sneAccessRotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Rotate through the running Pantheon server and revoke the old token",
	RunE: func(_ *cobra.Command, _ []string) error {
		if !sneRotateConfirm {
			return fmt.Errorf("rotation immediately revokes existing Nexus and API sessions; rerun with --confirm")
		}
		oldToken, err := dashboard.LoadOrCreateDefaultSNELocalAccessToken()
		if err != nil {
			return err
		}
		newToken, err := rotateSNELocalCapability("http://127.0.0.1:9119", oldToken, &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}})
		if err != nil {
			return err
		}
		persisted, err := dashboard.LoadOrCreateDefaultSNELocalAccessToken()
		if err != nil || persisted != newToken {
			return fmt.Errorf("Pantheon rotated memory but durable verification failed; restart Pantheon before using SNE")
		}
		fmt.Println("SNE local capability rotated. Existing Nexus/API sessions are revoked; reopen Nexus from Pantheon.")
		return nil
	},
}

var sneOpenCmd = &cobra.Command{
	Use:   "open",
	Short: "Open Nexus with this Mac's private local SNE capability",
	RunE: func(_ *cobra.Command, _ []string) error {
		token, err := dashboard.LoadOrCreateDefaultSNELocalAccessToken()
		if err != nil {
			return err
		}
		launchURL, err := dashboard.BuildNexusCapabilityURL(dashboard.NexusLocalAIURL, token)
		if err != nil {
			return err
		}
		return exec.Command("open", launchURL).Start()
	},
}

func init() {
	sneAccessTokenCmd.Flags().BoolVar(&sneAccessReveal, "reveal", false, "Print the secret token to stdout")
	sneAccessRotateCmd.Flags().BoolVar(&sneRotateConfirm, "confirm", false, "Immediately revoke the current token")
	sneAccessCmd.AddCommand(sneAccessStatusCmd, sneAccessTokenCmd, sneAccessRotateCmd)
	sneCmd.AddCommand(sneAccessCmd, sneOpenCmd)
	rootCmd.AddCommand(sneCmd)
}

func rotateSNELocalCapability(base, token string, client *http.Client) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "http" || !isCLIRequestLoopbackHost(parsed.Hostname()) {
		return "", fmt.Errorf("rotation endpoint must be loopback HTTP")
	}
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(base, "/")+"/api/sne/access/rotate", bytes.NewReader(nil))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("Pantheon rotation request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Pantheon rejected capability rotation with HTTP %d", response.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || payload.TokenType != "Bearer" || len(payload.AccessToken) != 43 {
		return "", fmt.Errorf("Pantheon returned an invalid rotation response")
	}
	return payload.AccessToken, nil
}

func isCLIRequestLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
