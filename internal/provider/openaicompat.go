package provider

import (
	"bytes"
	"context"
	"encoding/json"
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
	if strings.TrimSpace(o.Endpoint) == "" {
		return false
	}
	if o.UseRealCompletionProbe {
		return o.probeCompletion(ctx)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(o.Endpoint, "/")+"/models", nil)
	if err != nil {
		return false
	}
	o.auth(req)
	resp, err := o.client().Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// probeCompletion sends a 1-token chat completion to verify the engine can
// actually infer, not just bind a port. The SNE design specifies this over
// /health because two incidents proved a wedged model passes /health while
// returning 500s on real prompts.
func (o *OpenAICompat) probeCompletion(ctx context.Context) bool {
	// 30s timeout: a cold model load can be slow but >30s means it is wedged.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	model, err := o.ProbeServedModel(ctx)
	if err != nil {
		return false
	}

	body, err := json.Marshal(ccRequest{
		Model:     model,
		Messages:  []ccMessage{{Role: "user", Content: "1"}},
		MaxTokens: 1,
	})
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(o.Endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return false
	}
	o.auth(req)
	resp, err := o.client().Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	// 200 means the engine completed. Any 5xx means it is serving but broken.
	return resp.StatusCode == http.StatusOK
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
		return Response{}, fmt.Errorf("%s: http %d: %v", o.ProviderName, hresp.StatusCode, cc.Error)
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
