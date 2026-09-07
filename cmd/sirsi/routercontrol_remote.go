package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const remoteControlBodyLimit = 8 << 20

func firstNonEmptyControlEndpoint(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func controlEndpointURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("control endpoint must be an absolute URL: %q", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("control endpoint scheme %q is unsupported", parsed.Scheme)
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/api/control"
	} else if parsed.Path != "/api/control" {
		return "", fmt.Errorf("control endpoint path must be /api/control, got %q", parsed.Path)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("control endpoint must not include query or fragment")
	}
	return parsed.String(), nil
}

func fetchRemoteControl(ctx context.Context, rawEndpoint, token string) ([]byte, error) {
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
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
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
	if !json.Valid(body) {
		return nil, fmt.Errorf("control snapshot is not valid JSON")
	}
	return body, nil
}

func printControlJSON(body []byte) error {
	var out map[string]json.RawMessage
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("validate control snapshot: %w", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
