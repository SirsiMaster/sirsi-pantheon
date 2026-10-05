package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
)

const remoteControlBodyLimit = 8 << 20
const remoteControlActionBodyLimit = routerboard.ControlActionBodyLimit

func remoteControlHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		// Control requests carry a bearer credential, and action POSTs mutate
		// canonical router state. Never let an endpoint redirect either request
		// to a second authority or replay a mutation under redirect semantics.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func firstNonEmptyControlEndpoint(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func controlEndpointURL(raw string) (string, error) {
	return controlURL(raw, "/api/control")
}

func controlActionEndpointURL(raw string) (string, error) {
	return controlURL(raw, "/api/control/action")
}

func controlURL(raw, path string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("control endpoint must be an absolute URL: %q", raw)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("control endpoint must not contain embedded credentials")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("control endpoint scheme %q is unsupported", parsed.Scheme)
	}
	if parsed.Scheme == "http" && !isLoopbackControlHost(parsed.Hostname()) {
		return "", fmt.Errorf("plain HTTP control endpoints are restricted to loopback hosts; use HTTPS for %q", parsed.Hostname())
	}
	if parsed.Path != "" && parsed.Path != "/" && parsed.Path != "/api/control" {
		return "", fmt.Errorf("control endpoint path must be empty, /, or /api/control, got %q", parsed.Path)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("control endpoint must not include query or fragment")
	}
	parsed.Path = path
	return parsed.String(), nil
}

func isLoopbackControlHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 127
	}
	return ip.Equal(net.ParseIP("::1"))
}

func fetchRemoteControl(ctx context.Context, rawEndpoint, token string) ([]byte, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("fetch control snapshot: bearer token is required for remote control")
	}
	endpoint, err := controlEndpointURL(rawEndpoint)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build control request: %w", err)
	}
	if token = strings.TrimSpace(token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := remoteControlHTTPClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch control snapshot: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, remoteControlBodyLimit+1))
	if readErr != nil {
		return nil, fmt.Errorf("read control snapshot: %w", readErr)
	}
	if len(body) > remoteControlBodyLimit {
		return nil, fmt.Errorf("control snapshot exceeds %d-byte limit", remoteControlBodyLimit)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("control snapshot returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := validateRemoteControlSnapshot(body); err != nil {
		return nil, err
	}
	return body, nil
}

func validateRemoteControlSnapshot(body []byte) error {
	if _, err := routerboard.DecodeControlEnvelope(body); err != nil {
		return fmt.Errorf("control snapshot envelope: %w", err)
	}
	return nil
}

func readControlActionRequest(source string) ([]byte, error) {
	var reader io.Reader = os.Stdin
	var file *os.File
	if strings.TrimSpace(source) != "" && source != "-" {
		clean := filepath.Clean(source)
		opened, err := os.Open(clean) //nolint:gosec // explicit operator-selected request file
		if err != nil {
			return nil, fmt.Errorf("open control action request %q: %w", clean, err)
		}
		file = opened
		defer func() { _ = file.Close() }()
		reader = file
	}
	body, err := io.ReadAll(io.LimitReader(reader, remoteControlActionBodyLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read control action request: %w", err)
	}
	if len(body) > remoteControlActionBodyLimit {
		return nil, fmt.Errorf("control action request exceeds %d-byte limit", remoteControlActionBodyLimit)
	}
	if err := validateControlActionRequest(body); err != nil {
		return nil, fmt.Errorf("invalid control action request: %w", err)
	}
	return body, nil
}

func validateControlActionRequest(body []byte) error {
	if len(body) > remoteControlActionBodyLimit {
		return fmt.Errorf("request exceeds %d-byte limit", remoteControlActionBodyLimit)
	}
	_, err := routerboard.DecodeControlActionRequest(body)
	return err
}

func sendRemoteControlAction(ctx context.Context, rawEndpoint, token string, body []byte) ([]byte, error) {
	if err := validateControlActionRequest(body); err != nil {
		return nil, fmt.Errorf("send control action: invalid request: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("send control action: bearer token is required for remote control")
	}
	endpoint, err := controlActionEndpointURL(rawEndpoint)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("build control action request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token = strings.TrimSpace(token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := remoteControlHTTPClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("send control action: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	result, readErr := io.ReadAll(io.LimitReader(response.Body, remoteControlBodyLimit+1))
	if readErr != nil {
		return nil, fmt.Errorf("read control action response: %w", readErr)
	}
	if len(result) > remoteControlBodyLimit {
		return nil, fmt.Errorf("control action response exceeds %d-byte limit", remoteControlBodyLimit)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < 400 {
			return nil, fmt.Errorf("control action endpoint returned redirect HTTP %d; redirects are not followed", response.StatusCode)
		}
		if err := validateRemoteControlActionFailure(body, result); err != nil {
			return nil, fmt.Errorf("control action returned HTTP %d with invalid failure receipt: %w", response.StatusCode, err)
		}
		var failure routerboard.ControlActionFailure
		if err := json.Unmarshal(result, &failure); err != nil {
			return nil, fmt.Errorf("control action returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(result)))
		}
		return nil, &routerboard.ControlActionFailureError{Failure: failure}
	}
	if err := validateRemoteControlActionResponse(body, result); err != nil {
		return nil, err
	}
	return result, nil
}

func validateRemoteControlActionFailure(requestBody, responseBody []byte) error {
	if err := routerboard.ValidateJSONNoDuplicateKeys(responseBody); err != nil {
		return fmt.Errorf("control action failure JSON is ambiguous: %w", err)
	}
	var request routerboard.ControlActionRequest
	requestDecoder := json.NewDecoder(bytes.NewReader(requestBody))
	requestDecoder.DisallowUnknownFields()
	requestErr := requestDecoder.Decode(&request)
	var failure routerboard.ControlActionFailure
	responseDecoder := json.NewDecoder(bytes.NewReader(responseBody))
	responseDecoder.DisallowUnknownFields()
	if err := responseDecoder.Decode(&failure); err != nil {
		return fmt.Errorf("control action failure is not valid JSON: %w", err)
	}
	var trailing any
	if err := responseDecoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("control action failure contains multiple JSON values")
		}
		return fmt.Errorf("control action failure contains trailing JSON: %w", err)
	}
	if requestErr == nil && strings.TrimSpace(failure.Verb) != strings.TrimSpace(request.Verb) {
		return fmt.Errorf("control action failure verb %q does not match %q", failure.Verb, strings.TrimSpace(request.Verb))
	}
	if err := failure.VerifyControlActionFailure(requestBody); err != nil {
		return err
	}
	return nil
}

func validateRemoteControlActionResponse(requestBody, responseBody []byte) error {
	if err := routerboard.ValidateJSONNoDuplicateKeys(responseBody); err != nil {
		return fmt.Errorf("control action response JSON is ambiguous: %w", err)
	}
	var response routerboard.ControlActionResponse
	responseDecoder := json.NewDecoder(bytes.NewReader(responseBody))
	responseDecoder.DisallowUnknownFields()
	if err := responseDecoder.Decode(&response); err != nil {
		return fmt.Errorf("control action response is not valid JSON: %w", err)
	}
	var responseTrailing any
	if err := responseDecoder.Decode(&responseTrailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("control action response contains multiple JSON values")
		}
		return fmt.Errorf("control action response contains trailing JSON: %w", err)
	}
	if err := response.VerifyControlActionResponseForRequest(requestBody); err != nil {
		return fmt.Errorf("control action response receipt invalid: %w", err)
	}
	return nil
}

func printControlJSON(output io.Writer, body []byte) error {
	var out map[string]json.RawMessage
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("validate control snapshot: %w", err)
	}
	enc := json.NewEncoder(output)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
