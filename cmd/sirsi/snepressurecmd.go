package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/spf13/cobra"
)

var (
	snePressureBaseURL    = "http://127.0.0.1:9119"
	snePressureLoadToken  = dashboard.LoadOrCreateDefaultSNELocalAccessToken
	snePressureHTTPClient = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}}
	snePressureRequestID  string
	snePressureSHA        string
	snePressureToken      string
	snePressureActionHash string
	snePressureCleanupID  string
)

var snePrefixCachePressureCmd = &cobra.Command{
	Use:   "prefix-cache-pressure",
	Short: "Review or explicitly authorize a receipt-bound SNE prefix-cache pressure action",
}

var snePrefixCachePressurePrepareCmd = &cobra.Command{
	Use:   "prepare",
	Short: "Measure pressure and print the owner-confirmation receipt; does not authorize or mutate SNE",
	RunE: func(_ *cobra.Command, _ []string) error {
		view, err := requestPrefixCachePressure(http.MethodGet, nil)
		if err != nil {
			return err
		}
		return writePrefixCachePressureView(view)
	},
}

var snePrefixCachePressureAuthorizeCmd = &cobra.Command{
	Use:   "authorize",
	Short: "Consume one explicit confirmation token and print the bound authorization receipt",
	RunE: func(_ *cobra.Command, _ []string) error {
		if strings.TrimSpace(snePressureRequestID) == "" || strings.TrimSpace(snePressureSHA) == "" || strings.TrimSpace(snePressureToken) == "" {
			return fmt.Errorf("request-id, observation-sha256, and confirm-token are required from `sirsi sne prefix-cache-pressure prepare`")
		}
		view, err := requestPrefixCachePressure(http.MethodPost, map[string]string{
			"request_id": snePressureRequestID, "observation_sha256": snePressureSHA,
			"confirm_token": snePressureToken, "action_hash": snePressureActionHash,
		})
		if err != nil {
			return err
		}
		return writePrefixCachePressureView(view)
	},
}

var snePrefixCachePressureReceiptCmd = &cobra.Command{
	Use:   "receipt",
	Short: "Read an SNE-owned prefix-cache pressure execution receipt",
	RunE: func(_ *cobra.Command, _ []string) error {
		if strings.TrimSpace(snePressureRequestID) == "" {
			return fmt.Errorf("request-id is required")
		}
		view, err := requestPrefixCachePressureEvidence("receipts/" + snePressureRequestID)
		if err != nil {
			return err
		}
		return writePrefixCachePressureEvidence(view)
	},
}

var snePrefixCachePressureRetentionCmd = &cobra.Command{
	Use:   "retention",
	Short: "Read an SNE-owned prefix-cache pressure retention receipt",
	RunE: func(_ *cobra.Command, _ []string) error {
		if strings.TrimSpace(snePressureCleanupID) == "" {
			return fmt.Errorf("cleanup-id is required")
		}
		view, err := requestPrefixCachePressureEvidence("retention/" + snePressureCleanupID)
		if err != nil {
			return err
		}
		return writePrefixCachePressureEvidence(view)
	},
}

func init() {
	snePrefixCachePressureAuthorizeCmd.Flags().StringVar(&snePressureRequestID, "request-id", "", "Exact request_id from prepare")
	snePrefixCachePressureAuthorizeCmd.Flags().StringVar(&snePressureSHA, "observation-sha256", "", "Exact observation_sha256 from prepare")
	snePrefixCachePressureAuthorizeCmd.Flags().StringVar(&snePressureToken, "confirm-token", "", "Single-use confirmation token from prepare")
	snePrefixCachePressureAuthorizeCmd.Flags().StringVar(&snePressureActionHash, "action-hash", "", "Action hash from prepare")
	snePrefixCachePressureReceiptCmd.Flags().StringVar(&snePressureRequestID, "request-id", "", "Exact SNE request_id")
	snePrefixCachePressureRetentionCmd.Flags().StringVar(&snePressureCleanupID, "cleanup-id", "", "Exact SNE cleanup_id")
	snePrefixCachePressureCmd.AddCommand(snePrefixCachePressurePrepareCmd, snePrefixCachePressureAuthorizeCmd, snePrefixCachePressureReceiptCmd, snePrefixCachePressureRetentionCmd)
	sneCmd.AddCommand(snePrefixCachePressureCmd)
}

func requestPrefixCachePressure(method string, body any) (dashboard.PrefixCachePressureAuthorizationView, error) {
	var view dashboard.PrefixCachePressureAuthorizationView
	base, err := url.Parse(snePressureBaseURL)
	if err != nil || base.Scheme != "http" || !isCLIRequestLoopbackHost(base.Hostname()) {
		return view, fmt.Errorf("prefix-cache pressure endpoint must be loopback HTTP")
	}
	token, err := snePressureLoadToken()
	if err != nil {
		return view, err
	}
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		data, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return view, marshalErr
		}
		payload = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, strings.TrimRight(snePressureBaseURL, "/")+"/api/sne/prefix-cache-pressure", payload)
	if err != nil {
		return view, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := snePressureHTTPClient.Do(request)
	if err != nil {
		return view, fmt.Errorf("request prefix-cache pressure authorization: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return view, fmt.Errorf("Pantheon rejected prefix-cache pressure authorization with HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&view); err != nil {
		return view, fmt.Errorf("decode prefix-cache pressure response: %w", err)
	}
	return view, nil
}

func requestPrefixCachePressureEvidence(suffix string) (dashboard.PrefixCachePressureEvidenceView, error) {
	var view dashboard.PrefixCachePressureEvidenceView
	base, err := url.Parse(snePressureBaseURL)
	if err != nil || base.Scheme != "http" || !isCLIRequestLoopbackHost(base.Hostname()) || strings.Contains(suffix, "..") || strings.Contains(suffix, "/") && strings.Count(suffix, "/") != 1 {
		return view, fmt.Errorf("prefix-cache pressure evidence endpoint is invalid")
	}
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(snePressureBaseURL, "/")+"/api/sne/prefix-cache-pressure/"+suffix, nil)
	if err != nil {
		return view, err
	}
	response, err := snePressureHTTPClient.Do(request)
	if err != nil {
		return view, fmt.Errorf("read prefix-cache pressure evidence: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return view, fmt.Errorf("Pantheon rejected prefix-cache pressure evidence with HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&view); err != nil {
		return view, fmt.Errorf("decode prefix-cache pressure evidence: %w", err)
	}
	return view, nil
}

func writePrefixCachePressureView(view dashboard.PrefixCachePressureAuthorizationView) error {
	if JsonOutput {
		return json.NewEncoder(os.Stdout).Encode(view)
	}
	fmt.Printf("Prefix-cache pressure: %s\nHost: %s\nRequest: %s\nEvidence: %s\n", view.State, view.Receipt.Observation.HostID, view.Receipt.Observation.RequestID, view.Receipt.ObservationSHA256)
	if view.Confirmation != nil {
		fmt.Println("Owner confirmation required. Re-run with the exact values returned in JSON; SNE has not been changed.")
	}
	if view.Authorization != nil {
		fmt.Printf("Authorization: %s (expires %d)\nSNE decision/execution/retention evidence is not yet available.\n", view.Authorization.State, view.Authorization.ExpiresAtUnix)
	}
	return nil
}

func writePrefixCachePressureEvidence(view dashboard.PrefixCachePressureEvidenceView) error {
	if JsonOutput {
		return json.NewEncoder(os.Stdout).Encode(view)
	}
	fmt.Printf("Prefix-cache %s evidence: %s\nIdentity: %s\n", view.EvidenceType, view.State, view.Identity)
	if view.State == "unavailable" {
		fmt.Println("SNE-owned receipt evidence is unavailable; no execution state is inferred.")
	}
	return nil
}
