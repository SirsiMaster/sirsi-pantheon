package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OpenAICompat serves any endpoint speaking the OpenAI chat-completions shape.
//
// This one implementation covers the local gemma broker, LM Studio, Ollama's
// compatible endpoint, vLLM, and every hosted OpenAI-compatible gateway —
// which is why sirsi-brain.sh chose that shape and why it is the right seam to
// promote. Tier is a field rather than a constant because the SAME protocol is
// local when it points at 127.0.0.1 and remote when it points at a vendor.
type OpenAICompat struct {
	ProviderName string
	Endpoint     string // base URL, e.g. http://127.0.0.1:8765/v1
	Model        string
	APIKey       string
	TierValue    Tier
	HTTP         *http.Client
	// SupportsTools is declared, not assumed. mlx_lm.server does not implement
	// tool-calling; claiming otherwise would make the loop believe a silent
	// no-op was the model declining to act.
	SupportsTools       bool
	SupportsStreaming   bool
	SupportsDeterminism bool
	SupportsJSON        bool
	ContextTokens       int
	// UseRealCompletionProbe changes Available() from a /v1/models check to a
	// real 1-token completion. Required for the SNE local lane per
	// MODEL-ROUTER-DESIGN.md: "a serving process that cannot complete is DOWN".
	// /v1/models proves the process is bound, not that inference works.
	UseRealCompletionProbe bool
}

type sirsiCapabilities struct {
	ContractVersion string `json:"contract_version"`
	Model           struct {
		ID                    string `json:"id"`
		QualifiedPromptTokens int    `json:"qualified_prompt_tokens"`
	} `json:"model"`
	Serving struct {
		Streaming bool `json:"streaming"`
	} `json:"serving"`
	Determinism struct {
		BatchInvariantActive bool `json:"batch_invariant_active"`
	} `json:"determinism"`
}

func (o *OpenAICompat) DiscoverCapabilities(ctx context.Context) (Caps, string, error) {
	caps := o.Caps()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(o.Endpoint, "/")+"/sirsi/capabilities", nil)
	if err != nil {
		return Caps{}, "", err
	}
	o.auth(req)
	resp, err := o.client().Do(req)
	if err != nil {
		return Caps{}, "", fmt.Errorf("%w: %s capability discovery: %v", ErrUnavailable, o.ProviderName, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		caps.ContextTokens = 0
		caps.Deterministic = false
		caps.JSONMode = false
		caps.Tools = false
		return caps, "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return Caps{}, "", classifyHTTPError(o.ProviderName+" capability discovery", resp.StatusCode)
	}
	var discovered sirsiCapabilities
	if err := json.NewDecoder(resp.Body).Decode(&discovered); err != nil {
		return Caps{}, "", fmt.Errorf("%s: decode capabilities: %w", o.ProviderName, err)
	}
	if discovered.ContractVersion != "1" && !strings.HasPrefix(discovered.ContractVersion, "1.") {
		return Caps{}, "", fmt.Errorf("%s: unsupported capabilities contract %q", o.ProviderName, discovered.ContractVersion)
	}
	caps.ContextTokens = discovered.Model.QualifiedPromptTokens
	caps.Streaming = discovered.Serving.Streaming
	caps.Deterministic = discovered.Determinism.BatchInvariantActive
	caps.JSONMode = false
	caps.Tools = false
	return caps, discovered.Model.ID, nil
}

func classifyHTTPError(op string, status int) error {
	switch status {
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s: http %d", ErrRateLimited, op, status)
	case http.StatusPaymentRequired:
		return fmt.Errorf("%w: %s: http %d", ErrBudgetExhausted, op, status)
	default:
		return fmt.Errorf("%w: %s: http %d", ErrUnavailable, op, status)
	}
}

func (o *OpenAICompat) Name() string { return o.ProviderName }
func (o *OpenAICompat) Tier() Tier   { return o.TierValue }

func (o *OpenAICompat) Caps() Caps {
	return Caps{
		Tools:         o.SupportsTools,
		Streaming:     o.SupportsStreaming,
		Deterministic: o.SupportsDeterminism,
		JSONMode:      o.SupportsJSON,
		ContextTokens: o.ContextTokens,
		Offline:       o.TierValue == TierLocal,
	}
}

// ProbeCompletion is the readiness gate for an inference lane. A health route,
// listening socket, or loaded model name does not prove that inference works.
func (o *OpenAICompat) ProbeCompletion(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := o.Complete(ctx, Request{Prompt: "Reply with OK.", MaxTokens: 1})
	if err != nil {
		if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrBudgetExhausted) {
			return fmt.Errorf("%s one-token completion: %w", o.ProviderName, err)
		}
		return fmt.Errorf("%w: %s one-token completion: %v", ErrUnavailable, o.ProviderName, err)
	}
	return nil
}

func (o *OpenAICompat) client() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	// Generous: a cold local model pays a full weight-load on first token, and
	// timing that load as if it were decode is a false-DEGRADED this fabric has
	// already recorded twice.
	return &http.Client{Timeout: 180 * time.Second}
}

// Available probes liveness. When UseRealCompletionProbe is set (SNE local lane),
// it sends a 1-token real completion — a serving process that cannot complete
// is DOWN per MODEL-ROUTER-DESIGN.md. Otherwise it probes /v1/models, which
// proves a model is loaded but not that inference works.
func (o *OpenAICompat) Available(ctx context.Context) bool {
	return o.Availability(ctx) == Available
}

func (o *OpenAICompat) Availability(ctx context.Context) Availability {
	if strings.TrimSpace(o.Endpoint) == "" {
		return Offline
	}
	if o.UseRealCompletionProbe {
		err := o.ProbeCompletion(ctx)
		switch {
		case err == nil:
			return Available
		case errors.Is(err, ErrRateLimited):
			return RateLimit
		case errors.Is(err, ErrBudgetExhausted):
			return Budgeted
		default:
			return Offline
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(o.Endpoint, "/")+"/models", nil)
	if err != nil {
		return Offline
	}
	o.auth(req)
	resp, err := o.client().Do(req)
	if err != nil {
		return Offline
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return Available
	case http.StatusTooManyRequests:
		return RateLimit
	case http.StatusPaymentRequired:
		return Budgeted
	default:
		return Offline
	}
}

func (o *OpenAICompat) auth(r *http.Request) {
	if o.APIKey != "" {
		r.Header.Set("Authorization", "Bearer "+o.APIKey)
	}
	r.Header.Set("Content-Type", "application/json")
}

type ccMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ccRequest struct {
	Model     string      `json:"model"`
	Messages  []ccMessage `json:"messages"`
	MaxTokens int         `json:"max_tokens,omitempty"`
}

type ccResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role    string `json:"role"`
			Content string `json:"content"`
			// Reasoning models return their chain here and leave Content empty
			// when they hit the token cap mid-thought. A probe asserting on
			// non-empty Content calls that "wedged" — a false alarm this fabric
			// has recorded against gemma-4 specifically.
			Reasoning string `json:"reasoning"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error any `json:"error"`
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// ServedModel RESOLVES the model name to use for a request. It short-circuits to
// the configured model when one is set, so it is cheap — and therefore it is NOT
// a liveness probe: with a model configured it never touches the network and
// cannot tell a healthy broker from a dead one.
//
// An earlier version of this comment claimed it "proves the ENDPOINT answers".
// It does not, and a restart verifier built on that claim would report a healthy
// model over a dead broker (codex-pantheon, router item 20260729-193639). Use
// ProbeServedModel when the point is to prove the endpoint is alive.
func (o *OpenAICompat) ServedModel(ctx context.Context) (string, error) {
	if strings.TrimSpace(o.Model) != "" {
		return o.Model, nil
	}
	return o.ProbeServedModel(ctx)
}

// ProbeServedModel ALWAYS performs the request. It is the honest liveness probe:
// it proves the endpoint answers and names what the broker actually loaded,
// which no process check and no configured string can. A configured model must
// never be able to satisfy it — that bypass is what let a wedged server read as
// healthy.
func (o *OpenAICompat) ProbeServedModel(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(o.Endpoint, "/")+"/models", nil)
	if err != nil {
		return "", err
	}
	o.auth(req)
	resp, err := o.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrUnavailable, o.ProviderName, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: models: http %d", o.ProviderName, resp.StatusCode)
	}
	var models modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return "", fmt.Errorf("%s: decode models: %w", o.ProviderName, err)
	}
	if len(models.Data) == 0 || strings.TrimSpace(models.Data[0].ID) == "" {
		return "", fmt.Errorf("%s: no served model", o.ProviderName)
	}
	return models.Data[0].ID, nil
}

func (o *OpenAICompat) Complete(ctx context.Context, req Request) (Response, error) {
	if strings.TrimSpace(o.Endpoint) == "" {
		return Response{}, fmt.Errorf("%w: %s has no endpoint", ErrUnavailable, o.ProviderName)
	}

	msgs := make([]ccMessage, 0, 2)
	if req.System != "" {
		msgs = append(msgs, ccMessage{Role: "system", Content: req.System})
	}
	msgs = append(msgs, ccMessage{Role: "user", Content: req.Prompt})

	model, err := o.ServedModel(ctx)
	if err != nil {
		return Response{}, err
	}
	body, err := json.Marshal(ccRequest{Model: model, Messages: msgs, MaxTokens: req.MaxTokens})
	if err != nil {
		return Response{}, err
	}

	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(o.Endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	o.auth(hreq)

	hresp, err := o.client().Do(hreq)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %s: %v", ErrUnavailable, o.ProviderName, err)
	}
	defer func() { _ = hresp.Body.Close() }()

	var cc ccResponse
	if derr := json.NewDecoder(hresp.Body).Decode(&cc); derr != nil {
		return Response{}, fmt.Errorf("%s: decode: %w", o.ProviderName, derr)
	}
	if hresp.StatusCode != http.StatusOK {
		// Surface the backend's own words. A 404 here once read "cannot find an
		// appropriate cached snapshot ... HF_HUB_OFFLINE" — which was a wrong
		// model NAME in the request, not a broken broker. Swallowing it would
		// have sent an operator hunting the wrong fault.
		return Response{}, fmt.Errorf("%w: %v", classifyHTTPError(o.ProviderName, hresp.StatusCode), cc.Error)
	}
	if len(cc.Choices) == 0 {
		return Response{}, fmt.Errorf("%s: no choices in response", o.ProviderName)
	}

	ch := cc.Choices[0]
	text := ch.Message.Content
	if text == "" && ch.Message.Reasoning != "" {
		// Honest degradation: the model produced reasoning and ran out of room
		// before the answer. Return it rather than an empty string, and let the
		// finish_reason explain why it is partial.
		text = ch.Message.Reasoning
	}

	return Response{
		Text:         text,
		Tier:         o.TierValue,
		Provider:     o.ProviderName,
		Model:        cc.Model,
		ToolsHonored: false, // this transport does not carry tool calls yet
		FinishReason: ch.FinishReason,
		PromptTokens: cc.Usage.PromptTokens,
		OutputTokens: cc.Usage.CompletionTokens,
	}, nil
}
