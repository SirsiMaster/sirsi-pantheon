// Package sne implements Pantheon's product-neutral SNE service client.
// Pantheon owns admission and supervision; the engine remains replaceable
// behind the OpenAI-compatible contract.
package sne

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
	token   string
}

const OpenAIChatContractV3 = "sne.openai-chat.v3"

const (
	ExecutionModePlain = "plain"
	ExecutionModeMTP   = "mtp"
)

type AssistantIdentity struct {
	ModelID          string `json:"model_id"`
	Revision         string `json:"revision"`
	CheckpointSHA256 string `json:"checkpoint_sha256"`
	Precision        string `json:"precision"`
}

func (a *AssistantIdentity) UnmarshalJSON(data []byte) error {
	if err := validateUniqueJSONKeys(data); err != nil {
		return fmt.Errorf("SNE assistant identity: %w", err)
	}
	type wire AssistantIdentity
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	*a = AssistantIdentity(decoded)
	return nil
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompletionRequest struct {
	Model         string    `json:"model"`
	ExecutionMode string    `json:"execution_mode"`
	Messages      []Message `json:"messages"`
	MaxTokens     int       `json:"max_tokens,omitempty"`
	Temperature   float64   `json:"temperature"`
	Stream        bool      `json:"stream"`
}

type CompletionResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	SNE struct {
		RuntimeSHA256             string  `json:"runtime_sha256"`
		NativeRuntimeSHA256       string  `json:"native_runtime_sha256"`
		ModelManifestSHA256       string  `json:"model_manifest_sha256"`
		Profile                   string  `json:"profile"`
		TTFTMilliseconds          float64 `json:"ttft_ms"`
		GenerationTokensPerSecond float64 `json:"generation_tokens_per_second"`
		Execution                 struct {
			Mode      string             `json:"mode"`
			Assistant *AssistantIdentity `json:"assistant,omitempty"`
		} `json:"execution"`
	} `json:"sne"`
}

type ReadinessCapabilities struct {
	ExecutionModes []string           `json:"execution_modes"`
	Assistant      *AssistantIdentity `json:"assistant,omitempty"`
}

func (c *ReadinessCapabilities) UnmarshalJSON(data []byte) error {
	if err := validateUniqueJSONKeys(data); err != nil {
		return fmt.Errorf("SNE readiness capabilities: %w", err)
	}
	type wire ReadinessCapabilities
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	*c = ReadinessCapabilities(decoded)
	return nil
}

func (c ReadinessCapabilities) Validate() error {
	if len(c.ExecutionModes) == 0 {
		return fmt.Errorf("SNE readiness capabilities: execution_modes is empty")
	}
	seen := make(map[string]struct{}, len(c.ExecutionModes))
	mtp := false
	for _, mode := range c.ExecutionModes {
		if mode != ExecutionModePlain && mode != ExecutionModeMTP {
			return fmt.Errorf("SNE readiness capabilities: unsupported execution mode %q", mode)
		}
		if _, exists := seen[mode]; exists {
			return fmt.Errorf("SNE readiness capabilities: duplicate execution mode %q", mode)
		}
		seen[mode] = struct{}{}
		mtp = mtp || mode == ExecutionModeMTP
	}
	if mtp != (c.Assistant != nil) {
		return fmt.Errorf("SNE readiness capabilities: assistant identity must be present exactly when MTP is advertised")
	}
	if c.Assistant != nil {
		if err := c.Assistant.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c ReadinessCapabilities) SupportsExecutionMode(mode string) bool {
	for _, supported := range c.ExecutionModes {
		if supported == mode {
			return true
		}
	}
	return false
}

func (a AssistantIdentity) Validate() error {
	if strings.TrimSpace(a.ModelID) == "" || strings.TrimSpace(a.Revision) == "" || strings.TrimSpace(a.Precision) == "" {
		return fmt.Errorf("SNE assistant identity requires model_id, revision, and precision")
	}
	if !validLowerSHA256(a.CheckpointSHA256) {
		return fmt.Errorf("SNE assistant identity checkpoint_sha256 must be lowercase SHA-256")
	}
	return nil
}

func validLowerSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && strings.ToLower(value) == value
}

type Model struct {
	ID             string `json:"id"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

type ServiceReadinessIdentity struct {
	Status                     string
	ServiceVersion             string
	APIVersion                 string
	APIContract                string
	Profile                    string
	RuntimeSHA256              string
	NativeRuntimeSHA256        string
	LoadedModel                string
	Models                     []Model
	ReadyProfile               string
	ReadyRuntimeSHA256         string
	ReadyNativeRuntimeSHA256   string
	ReadyModelID               string
	ReadyManifestSHA256        string
	ReadyAPIContract           string
	ReadyCapabilities          ReadinessCapabilities
	CacheTopology              string
	ServingCacheCapacity       int
	PrefixSessionsMaximum      int
	MaxConcurrentRequests      int
	MaxQueuedRequests          int
	QueueDiscipline            string
	RequestTimeoutMS           int64
	ReadyMaxConcurrentRequests int
	ReadyMaxQueuedRequests     int
	ReadyQueueDiscipline       string
	ReadyRequestTimeoutMS      int64
}

// ValidateContract checks the shared v3 readiness contract before any
// consumer interprets service identity or capabilities. Lifecycle owners may
// then apply their stricter model/runtime/profile expectations.
func (i ServiceReadinessIdentity) ValidateContract() error {
	if i.Status != "ready" {
		return fmt.Errorf("SNE readiness status %q is not ready", i.Status)
	}
	if i.APIContract != OpenAIChatContractV3 || i.ReadyAPIContract != OpenAIChatContractV3 || i.APIContract != i.ReadyAPIContract {
		return fmt.Errorf("SNE readiness/status API contracts must both equal %q", OpenAIChatContractV3)
	}
	if err := i.ValidateEndpointConsistency(); err != nil {
		return err
	}
	if err := i.ReadyCapabilities.Validate(); err != nil {
		return fmt.Errorf("SNE readiness capabilities: %w", err)
	}
	return nil
}

// ValidateEndpointConsistency rejects contradictory values when readiness
// and status readbacks both provide the same identity field, and ambiguous
// duplicate catalog rows for the ready model. Missing fields remain the
// responsibility of the consuming route's stricter policy.
func (i ServiceReadinessIdentity) ValidateEndpointConsistency() error {
	for _, field := range []struct{ name, ready, status string }{
		{"profile", strings.TrimSpace(i.ReadyProfile), strings.TrimSpace(i.Profile)},
		{"runtime", strings.TrimSpace(i.ReadyRuntimeSHA256), strings.TrimSpace(i.RuntimeSHA256)},
		{"native runtime", strings.TrimSpace(i.ReadyNativeRuntimeSHA256), strings.TrimSpace(i.NativeRuntimeSHA256)},
		{"model", strings.TrimSpace(i.ReadyModelID), strings.TrimSpace(i.LoadedModel)},
	} {
		if field.ready != "" && field.status != "" && field.ready != field.status {
			return fmt.Errorf("SNE readiness/status %s identities disagree: %q != %q", field.name, field.ready, field.status)
		}
	}

	readyModel := strings.TrimSpace(i.ReadyModelID)
	readyManifest := strings.TrimSpace(i.ReadyManifestSHA256)
	if readyModel == "" {
		return nil
	}
	matchingModels := 0
	for _, model := range i.Models {
		if strings.TrimSpace(model.ID) != readyModel {
			continue
		}
		matchingModels++
		if readyManifest != "" && strings.TrimSpace(model.ManifestSHA256) != readyManifest {
			return fmt.Errorf("SNE readiness/catalog manifest identities disagree for model %q", readyModel)
		}
	}
	if matchingModels > 1 {
		return fmt.Errorf("SNE model catalog contains duplicate identity for ready model %q", readyModel)
	}
	if readyManifest != "" && matchingModels != 1 {
		return fmt.Errorf("SNE model catalog has %d identities for ready model %q, want exactly one", matchingModels, readyModel)
	}
	return nil
}

type ServiceMetrics struct {
	RequestsActive        int64  `json:"requests_active"`
	RequestsQueued        int    `json:"requests_queued"`
	MaxConcurrentRequests int    `json:"max_concurrent_requests"`
	MaxQueuedRequests     int    `json:"max_queued_requests"`
	QueueDiscipline       string `json:"queue_discipline"`
	RequestTimeoutMS      int64  `json:"request_timeout_ms"`
}

type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Retryable  bool
}

func (e *APIError) Error() string {
	return fmt.Sprintf("SNE request failed with HTTP %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

func IsRestartRequired(err error) bool {
	var apiError *APIError
	return errors.As(err, &apiError) && apiError.Code == "restart_required"
}

func NewClient(baseURL string) (*Client, error) {
	return NewAuthenticatedClient(baseURL, "")
}

func NewAuthenticatedClient(baseURL, token string) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid SNE base URL %q", baseURL)
	}
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Minute},
		token:   token,
	}, nil
}

func (c *Client) authorize(request *http.Request) {
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func (c *Client) Ready(ctx context.Context) bool {
	identity, err := c.ReadinessIdentity(ctx)
	return err == nil && identity.ValidateContract() == nil
}

func (c *Client) ReadinessIdentity(ctx context.Context) (ServiceReadinessIdentity, error) {
	var ready struct {
		Status                string                `json:"status"`
		ServiceVersion        string                `json:"service_version"`
		APIVersion            string                `json:"api_version"`
		APIContract           string                `json:"api_contract"`
		Capabilities          ReadinessCapabilities `json:"capabilities"`
		Profile               string                `json:"profile"`
		RuntimeSHA256         string                `json:"runtime_sha256"`
		NativeRuntimeSHA256   string                `json:"native_runtime_sha256"`
		ModelID               string                `json:"model_id"`
		ModelManifestSHA256   string                `json:"model_manifest_sha256"`
		CacheTopology         string                `json:"cache_topology"`
		ServingCacheCapacity  int                   `json:"serving_cache_capacity"`
		PrefixSessionsMaximum int                   `json:"prefix_sessions_maximum"`
		MaxConcurrentRequests int                   `json:"max_concurrent_requests"`
		MaxQueuedRequests     int                   `json:"max_queued_requests"`
		QueueDiscipline       string                `json:"queue_discipline"`
		RequestTimeoutMS      int64                 `json:"request_timeout_ms"`
	}
	if err := c.getJSON(ctx, "/health/ready", &ready); err != nil {
		return ServiceReadinessIdentity{}, err
	}
	var status struct {
		Profile               string  `json:"profile"`
		RuntimeSHA256         string  `json:"runtime_sha256"`
		NativeRuntimeSHA256   string  `json:"native_runtime_sha256"`
		LoadedModel           *string `json:"loaded_model"`
		MaxConcurrentRequests int     `json:"max_concurrent_requests"`
		MaxQueuedRequests     int     `json:"max_queued_requests"`
		QueueDiscipline       string  `json:"queue_discipline"`
		RequestTimeoutMS      int64   `json:"request_timeout_ms"`
		APIContract           string  `json:"api_contract"`
	}
	if err := c.getJSON(ctx, "/v1/sne/status", &status); err != nil {
		return ServiceReadinessIdentity{}, err
	}
	models, err := c.Models(ctx)
	if err != nil {
		return ServiceReadinessIdentity{}, err
	}
	loaded := ""
	if status.LoadedModel != nil {
		loaded = *status.LoadedModel
	}
	return ServiceReadinessIdentity{
		Status: ready.Status, ServiceVersion: ready.ServiceVersion, APIVersion: ready.APIVersion, APIContract: status.APIContract,
		Profile: status.Profile, RuntimeSHA256: status.RuntimeSHA256, NativeRuntimeSHA256: status.NativeRuntimeSHA256, LoadedModel: loaded, Models: models,
		ReadyProfile: ready.Profile, ReadyRuntimeSHA256: ready.RuntimeSHA256, ReadyNativeRuntimeSHA256: ready.NativeRuntimeSHA256,
		ReadyModelID: ready.ModelID, ReadyManifestSHA256: ready.ModelManifestSHA256,
		ReadyAPIContract:  ready.APIContract,
		ReadyCapabilities: ready.Capabilities,
		CacheTopology:     ready.CacheTopology, ServingCacheCapacity: ready.ServingCacheCapacity,
		PrefixSessionsMaximum: ready.PrefixSessionsMaximum,
		MaxConcurrentRequests: status.MaxConcurrentRequests, MaxQueuedRequests: status.MaxQueuedRequests,
		QueueDiscipline: status.QueueDiscipline, RequestTimeoutMS: status.RequestTimeoutMS,
		ReadyMaxConcurrentRequests: ready.MaxConcurrentRequests, ReadyMaxQueuedRequests: ready.MaxQueuedRequests,
		ReadyQueueDiscipline: ready.QueueDiscipline, ReadyRequestTimeoutMS: ready.RequestTimeoutMS,
	}, nil
}

func (c *Client) getJSON(ctx context.Context, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	c.authorize(request)
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return responseError(response)
	}
	body, err := readServiceJSON(response.Body)
	if err != nil {
		return fmt.Errorf("read SNE %s: %w", path, err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode SNE %s: %w", path, err)
	}
	return nil
}

func (c *Client) Models(ctx context.Context) ([]Model, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	c.authorize(request)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, responseError(response)
	}
	body, err := readServiceJSON(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read SNE models: %w", err)
	}
	var envelope struct {
		Data []Model `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode SNE models: %w", err)
	}
	return envelope.Data, nil
}

func (c *Client) Metrics(ctx context.Context) (ServiceMetrics, error) {
	var metrics ServiceMetrics
	if err := c.getJSON(ctx, "/v1/sne/metrics", &metrics); err != nil {
		return ServiceMetrics{}, err
	}
	return metrics, nil
}

func (c *Client) Complete(ctx context.Context, request CompletionRequest) (*CompletionResponse, error) {
	if request.Model == "" || len(request.Messages) == 0 {
		return nil, fmt.Errorf("SNE completion requires a model and messages")
	}
	if request.ExecutionMode != ExecutionModePlain && request.ExecutionMode != ExecutionModeMTP {
		return nil, fmt.Errorf("SNE completion requires explicit execution_mode plain or mtp")
	}
	if request.Stream {
		return nil, fmt.Errorf("streaming requires the Pantheon stream adapter")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	c.authorize(httpRequest)
	response, err := c.http.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, responseError(response)
	}
	responseBody, err := readServiceJSON(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read SNE completion: %w", err)
	}
	var completion CompletionResponse
	if err := json.Unmarshal(responseBody, &completion); err != nil {
		return nil, fmt.Errorf("decode SNE completion: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("SNE returned no completion choices")
	}
	if completion.Model != request.Model {
		return nil, fmt.Errorf("SNE completion model %q does not match requested model %q", completion.Model, request.Model)
	}
	if completion.SNE.Execution.Mode != request.ExecutionMode {
		return nil, fmt.Errorf("SNE completion mode %q does not match requested mode %q", completion.SNE.Execution.Mode, request.ExecutionMode)
	}
	if request.ExecutionMode == ExecutionModeMTP {
		if completion.SNE.Execution.Assistant == nil {
			return nil, fmt.Errorf("SNE MTP completion omitted assistant identity")
		}
		if err := completion.SNE.Execution.Assistant.Validate(); err != nil {
			return nil, fmt.Errorf("SNE MTP completion assistant identity: %w", err)
		}
	} else if completion.SNE.Execution.Assistant != nil {
		return nil, fmt.Errorf("SNE plain completion unexpectedly included assistant identity")
	}
	for _, field := range []struct{ name, value string }{
		{"runtime_sha256", completion.SNE.RuntimeSHA256},
		{"native_runtime_sha256", completion.SNE.NativeRuntimeSHA256},
		{"model_manifest_sha256", completion.SNE.ModelManifestSHA256},
	} {
		if !validLowerSHA256(field.value) {
			return nil, fmt.Errorf("SNE completion %s must be lowercase SHA-256", field.name)
		}
	}
	return &completion, nil
}

const maxSNEJSONResponse = 8 << 20

func readServiceJSON(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxSNEJSONResponse+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxSNEJSONResponse {
		return nil, fmt.Errorf("response exceeds %d-byte limit", maxSNEJSONResponse)
	}
	if err := validateUniqueJSONKeys(body); err != nil {
		return nil, fmt.Errorf("invalid service JSON: %w", err)
	}
	return body, nil
}

func (c *Client) modelLifecycle(ctx context.Context, model, action string) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("SNE %s requires a model", action)
	}
	if action != "load" && action != "unload" && action != "reload" {
		return fmt.Errorf("unsupported SNE lifecycle action %q", action)
	}
	body, err := json.Marshal(map[string]string{"model": model})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/sne/model/"+action, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	c.authorize(request)
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return responseError(response)
	}
	return nil
}

func (c *Client) LoadModel(ctx context.Context, model string) error {
	return c.modelLifecycle(ctx, model, "load")
}

func (c *Client) UnloadModel(ctx context.Context, model string) error {
	return c.modelLifecycle(ctx, model, "unload")
}

func (c *Client) ReloadModel(ctx context.Context, model string) error {
	return c.modelLifecycle(ctx, model, "reload")
}

func responseError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error.Code != "" {
		return &APIError{StatusCode: response.StatusCode, Code: envelope.Error.Code, Message: envelope.Error.Message, Retryable: envelope.Error.Retryable}
	}
	return fmt.Errorf("SNE request failed with HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
}
