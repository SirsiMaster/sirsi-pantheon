package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// GenerationReceipt is the Pantheon-owned provenance envelope for a selected
// CLI runner. Configuration identifies the selected connector; the envelope
// deliberately does not claim runtime attestation or expose endpoints/tokens.
type GenerationReceipt struct {
	Schema           string `json:"schema"`
	Engine           string `json:"engine"`
	Model            string `json:"model"`
	Route            string `json:"route"`
	IdentitySource   string `json:"identity_source"`
	RuntimeVerified  bool   `json:"runtime_verified"`
	RequestSHA256    string `json:"request_sha256"`
	CompletionSHA256 string `json:"completion_sha256"`
}

// ReceiptRunner is optional so existing injected and legacy runners retain
// the original text-only MCP contract.
type ReceiptRunner interface {
	Runner
	GenerateWithReceipt(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, GenerationReceipt, error)
}

type configuredReceiptRunner struct {
	Runner
	engine string
	model  string
	route  string
}

func (r *configuredReceiptRunner) GenerateWithReceipt(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, GenerationReceipt, error) {
	out, err := r.Runner.Generate(ctx, prompt, maxTokens, temperature)
	if err != nil {
		return "", GenerationReceipt{}, err
	}
	return out, GenerationReceipt{
		Schema:           "pantheon.gemma-generation-receipt/v1",
		Engine:           r.engine,
		Model:            r.model,
		Route:            r.route,
		IdentitySource:   "configured-engine-selection",
		RuntimeVerified:  false,
		RequestSHA256:    generationRequestDigest(prompt, maxTokens, temperature),
		CompletionSHA256: sha256Hex([]byte(out)),
	}, nil
}

func generationRequestDigest(prompt string, maxTokens int, temperature float64) string {
	b, _ := json.Marshal(struct {
		Prompt      string  `json:"prompt"`
		MaxTokens   int     `json:"max_tokens"`
		Temperature float64 `json:"temperature"`
	}{prompt, maxTokens, temperature})
	return sha256Hex(b)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func marshalGenerationReceipt(receipt GenerationReceipt) string {
	b, _ := json.Marshal(receipt)
	return string(b)
}
