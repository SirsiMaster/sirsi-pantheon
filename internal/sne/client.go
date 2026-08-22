// Package sne implements Pantheon's product-neutral SNE service client.
// Pantheon owns admission and supervision; the engine remains replaceable
// behind the OpenAI-compatible contract.
package sne

import (
	"bytes"
	"context"
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

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature"`
	Stream      bool      `json:"stream"`
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
	} `json:"sne"`
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
	_, err := c.ReadinessIdentity(ctx)
	return err == nil
}

func (c *Client) ReadinessIdentity(ctx context.Context) (ServiceReadinessIdentity, error) {
	var ready struct {
		Status                string `json:"status"`
		ServiceVersion        string `json:"service_version"`
		APIVersion            string `json:"api_version"`
		APIContract           string `json:"api_contract"`
		Profile               string `json:"profile"`
		RuntimeSHA256         string `json:"runtime_sha256"`
		NativeRuntimeSHA256   string `json:"native_runtime_sha256"`
		ModelID               string `json:"model_id"`
		ModelManifestSHA256   string `json:"model_manifest_sha256"`
		CacheTopology         string `json:"cache_topology"`
		ServingCacheCapacity  int    `json:"serving_cache_capacity"`
		PrefixSessionsMaximum int    `json:"prefix_sessions_maximum"`
		MaxConcurrentRequests int    `json:"max_concurrent_requests"`
		MaxQueuedRequests     int    `json:"max_queued_requests"`
		QueueDiscipline       string `json:"queue_discipline"`
		RequestTimeoutMS      int64  `json:"request_timeout_ms"`
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
		ReadyAPIContract: ready.APIContract,
		CacheTopology:    ready.CacheTopology, ServingCacheCapacity: ready.ServingCacheCapacity,
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
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
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
	var envelope struct {
		Data []Model `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
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
	var completion CompletionResponse
	if err := json.NewDecoder(response.Body).Decode(&completion); err != nil {
		return nil, fmt.Errorf("decode SNE completion: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("SNE returned no completion choices")
	}
	return &completion, nil
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
