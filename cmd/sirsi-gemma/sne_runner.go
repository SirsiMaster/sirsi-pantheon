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
	"strings"
	"time"
)

// SNERunner calls an OpenAI-compatible /v1/chat/completions endpoint.
// Every Generate call is a fresh stateless POST (same contract as MLXRunner).
type SNERunner struct {
	baseURL string // e.g. "http://localhost:11434/v1"
	model   string // model name forwarded in the request body
	client  *http.Client
}

// NewSNERunner constructs a runner pointed at baseURL.
// model defaults to "gemma-2-27b-it" when empty.
func NewSNERunner(baseURL, model string) *SNERunner {
	if model == "" {
		model = "gemma-2-27b-it"
	}
	return &SNERunner{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

type sneRequest struct {
	Model       string       `json:"model"`
	Messages    []sneMessage `json:"messages"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature float64      `json:"temperature,omitempty"`
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
	body := sneRequest{
		Model:       r.model,
		Messages:    []sneMessage{{Role: "user", Content: prompt}},
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("sne runner: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		r.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("sne runner: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sne runner: POST %s/chat/completions: %w", r.baseURL, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("sne runner: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sne runner: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out sneResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("sne runner: decode response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("sne runner: server error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("sne runner: no choices in response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

func (r *SNERunner) Health(ctx context.Context) error {
	_, err := r.Generate(ctx, "ping", 1, 0.01)
	return err
}
