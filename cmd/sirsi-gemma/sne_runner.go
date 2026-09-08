package main

// sne_runner.go — SNE seam for sirsi-gemma (ADR-003 in sirsi-inference).
//
// SNERunner replaces the MLX subprocess with a plain HTTP POST to an
// OpenAI-compatible /v1/chat/completions endpoint — the SNE seam contract
// defined in docs/adr/ADR-003-anubis-sne-seam.md (sirsi-inference repo,
// currently under codex review).
//
// Activation: set `sne_url = http://localhost:11434/v1` in ~/.config/sirsi/gemma.toml
// (or wherever the seam ADR specifies the default port). Leave unset to keep
// the current MLX subprocess runner.
//
// ADR-002 boundary: no engine source crosses into Pantheon. This runner speaks
// only to SNE's HTTP surface; it never imports sirsi-inference packages.
//
// ponytail: plain net/http, no retries. If SNE is down the caller sees the
// error immediately — no silent fallback so the operator knows to check SNE.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// SNERunner calls an OpenAI-compatible /v1/chat/completions endpoint. SNE and
// oMLX share this wire implementation so engine selection does not change the
// Pantheon tool workflow.
type SNERunner struct {
	backend     string
	baseURL     string // e.g. "http://localhost:11434/v1"
	model       string // model name forwarded in the request body
	client      *http.Client
	apiToken    string
	apiTokenErr error
}

// NewSNERunner constructs a runner pointed at baseURL.
// model defaults to "gemma-2-27b-it" when empty.
func NewSNERunner(baseURL, model string) *SNERunner {
	return NewSNERunnerWithTokenFile(baseURL, model, "")
}

// NewSNERunnerWithTokenFile constructs an SNE runner with an optional bearer
// token loaded from a private local file. The token is never included in
// errors or logs; invalid configuration is reported when the runner is used.
func NewSNERunnerWithTokenFile(baseURL, model, tokenFile string) *SNERunner {
	if model == "" {
		model = "gemma-2-27b-it"
	}
	token, tokenErr := readSNEAPIToken(tokenFile)
	return &SNERunner{
		backend:     "sne",
		baseURL:     strings.TrimRight(baseURL, "/"),
		model:       model,
		client:      &http.Client{Timeout: 5 * time.Minute},
		apiToken:    token,
		apiTokenErr: tokenErr,
	}
}

// NewOMLXRunner uses oMLX's OpenAI-compatible server through the same Pantheon
// request and response contract as SNE.
func NewOMLXRunner(baseURL, model string) *SNERunner {
	if model == "" {
		model = "gemma-4-12b-it"
	}
	return &SNERunner{
		backend: "omlx",
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

func readSNEAPIToken(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read SNE_API_TOKEN_FILE: %w", err)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", fmt.Errorf("SNE_API_TOKEN_FILE is empty")
	}
	return token, nil
}

type sneRequest struct {
	Model       string       `json:"model"`
	Messages    []sneMessage `json:"messages"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature float64      `json:"temperature"`
}

type sneMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type sneResponse struct {
	Choices []struct {
		Message sneMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (r *SNERunner) Generate(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error) {
	prefix := r.backend + " runner"
	if r.backend == "sne" && r.apiTokenErr != nil {
		return "", fmt.Errorf("%s: invalid SNE API token configuration: %w", prefix, r.apiTokenErr)
	}
	body := sneRequest{
		Model:       r.model,
		Messages:    []sneMessage{{Role: "user", Content: prompt}},
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("%s: marshal request: %w", prefix, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		r.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("%s: build request: %w", prefix, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if r.backend == "sne" && r.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.apiToken)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: POST %s/chat/completions: %w", prefix, r.baseURL, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%s: read response: %w", prefix, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: HTTP %d: %s", prefix, resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out sneResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("%s: decode response: %w", prefix, err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("%s: server error: %s", prefix, out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("%s: no choices in response", prefix)
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

func (r *SNERunner) Health(ctx context.Context) error {
	temperature := 0.01
	if r.backend == "sne" {
		temperature = 0
	}
	_, err := r.Generate(ctx, "ping", 1, temperature)
	return err
}
