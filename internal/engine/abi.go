// Package engine defines the engine-neutral Pantheon inference ABI.
//
// Connectors for MLX, OMLX, and SNE must present the same identity, session,
// request, event, and receipt semantics. This package contains no backend or
// process-launch code: it is the executable boundary that prevents a
// connector from silently changing model, tokenizer, precision, or cache.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

const ABIVersion = "pantheon.engine/v1"

var ErrUnsupportedCapability = errors.New("engine capability unsupported")

type Kind string

const (
	KindMLX  Kind = "mlx"
	KindOMLX Kind = "omlx"
	KindSNE  Kind = "sne"
)

type Capability string

const (
	CapabilityTools        Capability = "tools"
	CapabilityTemperature  Capability = "temperature"
	CapabilityTopP         Capability = "top_p"
	CapabilitySeed         Capability = "seed"
	CapabilityStreaming    Capability = "streaming"
	CapabilityCancellation Capability = "cancellation"
	CapabilityPrefill      Capability = "prefill"
	CapabilityDecode       Capability = "decode"
	CapabilityMTP          Capability = "mtp"
	CapabilityKVState      Capability = "kv_state"
	CapabilitySessions     Capability = "sessions"
	CapabilityTelemetry    Capability = "telemetry"
	CapabilityReceipts     Capability = "receipts"
)

// Capabilities are declarations, not inferred behavior. A false capability
// must produce an explicit error before a request reaches a connector.
type Capabilities struct {
	Tools        bool `json:"tools"`
	Temperature  bool `json:"temperature"`
	TopP         bool `json:"top_p"`
	Seed         bool `json:"seed"`
	Streaming    bool `json:"streaming"`
	Cancellation bool `json:"cancellation"`
	Prefill      bool `json:"prefill"`
	Decode       bool `json:"decode"`
	MTP          bool `json:"mtp"`
	KVState      bool `json:"kv_state"`
	Sessions     bool `json:"sessions"`
	Telemetry    bool `json:"telemetry"`
	Receipts     bool `json:"receipts"`
}

func (c Capabilities) Has(want Capability) bool {
	switch want {
	case CapabilityTools:
		return c.Tools
	case CapabilityTemperature:
		return c.Temperature
	case CapabilityTopP:
		return c.TopP
	case CapabilitySeed:
		return c.Seed
	case CapabilityStreaming:
		return c.Streaming
	case CapabilityCancellation:
		return c.Cancellation
	case CapabilityPrefill:
		return c.Prefill
	case CapabilityDecode:
		return c.Decode
	case CapabilityMTP:
		return c.MTP
	case CapabilityKVState:
		return c.KVState
	case CapabilitySessions:
		return c.Sessions
	case CapabilityTelemetry:
		return c.Telemetry
	case CapabilityReceipts:
		return c.Receipts
	default:
		return false
	}
}

type Identity struct {
	Engine          Kind   `json:"engine"`
	EngineVersion   string `json:"engine_version"`
	ModelID         string `json:"model_id"`
	ModelSHA256     string `json:"model_sha256"`
	TokenizerID     string `json:"tokenizer_id"`
	TokenizerSHA256 string `json:"tokenizer_sha256"`
	Precision       string `json:"precision"`
	CacheNamespace  string `json:"cache_namespace"`
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (i Identity) Validate() error {
	if i.Engine != KindMLX && i.Engine != KindOMLX && i.Engine != KindSNE {
		return fmt.Errorf("engine identity: unsupported engine %q", i.Engine)
	}
	for name, value := range map[string]string{
		"engine_version":  i.EngineVersion,
		"model_id":        i.ModelID,
		"tokenizer_id":    i.TokenizerID,
		"precision":       i.Precision,
		"cache_namespace": i.CacheNamespace,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("engine identity: %s is required", name)
		}
	}
	for name, value := range map[string]string{"model_sha256": i.ModelSHA256, "tokenizer_sha256": i.TokenizerSHA256} {
		if !sha256Pattern.MatchString(value) {
			return fmt.Errorf("engine identity: %s must be lowercase SHA-256", name)
		}
	}
	return nil
}

func (i Identity) Equal(other Identity) bool { return i == other }

// Digest is the stable identity key used by sessions and receipts. JSON field
// order is fixed by the struct declaration, so equivalent identities hash
// identically across connectors.
func (i Identity) Digest() (string, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(i)
	if err != nil {
		return "", fmt.Errorf("engine identity: marshal: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

type Session struct {
	ID        string   `json:"id"`
	Identity  Identity `json:"identity"`
	CreatedAt string   `json:"created_at"`
}

func (s Session) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("engine session: id is required")
	}
	if err := s.Identity.Validate(); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339Nano, s.CreatedAt); err != nil {
		return fmt.Errorf("engine session: created_at: %w", err)
	}
	return nil
}

// ToolSpec is the engine-neutral declaration passed to a connector. A
// connector may reject it explicitly when its transport cannot encode tools;
// it must never silently drop the request.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
}

type GenerateRequest struct {
	SessionID      string     `json:"session_id"`
	Identity       Identity   `json:"identity"`
	System         string     `json:"system,omitempty"`
	Prompt         string     `json:"prompt"`
	MaxTokens      int        `json:"max_tokens"`
	Tools          []ToolSpec `json:"tools,omitempty"`
	Temperature    *float64   `json:"temperature,omitempty"`
	TopP           *float64   `json:"top_p,omitempty"`
	Seed           *int64     `json:"seed,omitempty"`
	Stream         bool       `json:"stream"`
	CacheNamespace string     `json:"cache_namespace"`
	// RequiredCapabilities are checked before the request reaches a provider.
	// This prevents a connector from silently dropping prefill/decode/MTP/KV or
	// receipt requirements when engines expose different subsets.
	RequiredCapabilities []Capability `json:"required_capabilities,omitempty"`
}

func (r GenerateRequest) Validate(session Session, capabilities Capabilities) error {
	if err := session.Validate(); err != nil {
		return err
	}
	if r.SessionID != session.ID {
		return fmt.Errorf("engine request: session mismatch: %q != %q", r.SessionID, session.ID)
	}
	if !r.Identity.Equal(session.Identity) {
		return errors.New("engine request: identity does not match the session; silent model/tokenizer/precision changes are forbidden")
	}
	if r.CacheNamespace != session.Identity.CacheNamespace {
		return errors.New("engine request: cache namespace does not match the session")
	}
	if len(r.Tools) > 0 && !capabilities.Has(CapabilityTools) {
		return fmt.Errorf("%w: tools", ErrUnsupportedCapability)
	}
	if r.Temperature != nil && !capabilities.Has(CapabilityTemperature) {
		return fmt.Errorf("%w: temperature", ErrUnsupportedCapability)
	}
	if r.TopP != nil && !capabilities.Has(CapabilityTopP) {
		return fmt.Errorf("%w: top_p", ErrUnsupportedCapability)
	}
	if r.Seed != nil && !capabilities.Has(CapabilitySeed) {
		return fmt.Errorf("%w: seed", ErrUnsupportedCapability)
	}
	seenCapabilities := make(map[Capability]struct{}, len(r.RequiredCapabilities))
	for _, capability := range r.RequiredCapabilities {
		if _, seen := seenCapabilities[capability]; seen {
			return fmt.Errorf("engine request: duplicate required capability %q", capability)
		}
		seenCapabilities[capability] = struct{}{}
		if !capabilities.Has(capability) {
			return fmt.Errorf("%w: %s", ErrUnsupportedCapability, capability)
		}
	}
	if strings.TrimSpace(r.Prompt) == "" {
		return errors.New("engine request: prompt is required")
	}
	if r.MaxTokens <= 0 {
		return errors.New("engine request: max_tokens must be positive")
	}
	if r.Temperature != nil && (*r.Temperature < 0 || *r.Temperature > 2) {
		return fmt.Errorf("engine request: temperature %.3f outside [0,2]", *r.Temperature)
	}
	if r.TopP != nil && (*r.TopP <= 0 || *r.TopP > 1) {
		return fmt.Errorf("engine request: top_p %.3f outside (0,1]", *r.TopP)
	}
	if r.Stream && !capabilities.Has(CapabilityStreaming) {
		return errors.New("engine request: streaming is unsupported by the selected engine")
	}
	return nil
}

type EventKind string

const (
	EventDelta     EventKind = "delta"
	EventCompleted EventKind = "completed"
	EventError     EventKind = "error"
)

type Event struct {
	Kind       EventKind `json:"kind"`
	SessionID  string    `json:"session_id"`
	Sequence   uint64    `json:"sequence"`
	Text       string    `json:"text,omitempty"`
	Finish     string    `json:"finish,omitempty"`
	ErrorCode  string    `json:"error_code,omitempty"`
	Error      string    `json:"error,omitempty"`
	ReceiptSHA string    `json:"receipt_sha256,omitempty"`
	Receipt    *Receipt  `json:"receipt,omitempty"`
}

func (e Event) Validate(previous uint64) error {
	if e.Kind != EventDelta && e.Kind != EventCompleted && e.Kind != EventError {
		return fmt.Errorf("engine event: unsupported kind %q", e.Kind)
	}
	if strings.TrimSpace(e.SessionID) == "" {
		return errors.New("engine event: session_id is required")
	}
	if e.Sequence == 0 || e.Sequence <= previous {
		return fmt.Errorf("engine event: sequence %d is not after %d", e.Sequence, previous)
	}
	if e.Kind == EventError && strings.TrimSpace(e.ErrorCode) == "" {
		return errors.New("engine event: error_code is required for error events")
	}
	return nil
}

type Receipt struct {
	ABIVersion       string   `json:"abi_version"`
	SessionID        string   `json:"session_id"`
	Identity         Identity `json:"identity"`
	IdentityDigest   string   `json:"identity_digest"`
	RequestSHA256    string   `json:"request_sha256"`
	CompletionSHA256 string   `json:"completion_sha256"`
	StartedAt        string   `json:"started_at"`
	FinishedAt       string   `json:"finished_at"`
	Cancelled        bool     `json:"cancelled"`
}

// Completion is the normalized non-streaming result shared by all provider
// adapters. Streaming connectors emit Event values instead; they still use
// the same Session and Identity checks.
type Completion struct {
	Text         string `json:"text"`
	Model        string `json:"model"`
	FinishReason string `json:"finish_reason"`
	PromptTokens int    `json:"prompt_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

// Connector is the backend-neutral execution seam. It intentionally exposes
// no process, URL, shell, or backend-specific configuration.
type Connector interface {
	Kind() Kind
	Capabilities() Capabilities
	OpenSession(context.Context, string) (Session, error)
	Complete(context.Context, Session, GenerateRequest) (Completion, Receipt, error)
	Stream(context.Context, Session, GenerateRequest) (<-chan Event, error)
}

// ProviderConnector adapts the existing provider ladder into the ABI. MLX,
// OMLX, and SNE OpenAI-compatible endpoints can use this bridge while their
// engine-specific lifecycle remains outside this package.
type ProviderConnector struct {
	Backend provider.Provider
	Engine  Kind
	Model   Identity
	Caps    Capabilities
	Now     func() time.Time
}

func (c ProviderConnector) Kind() Kind { return c.Engine }

func (c ProviderConnector) Capabilities() Capabilities { return c.Caps }

func NewProviderConnector(backend provider.Provider, kind Kind, identity Identity, capabilities Capabilities) (ProviderConnector, error) {
	connector := ProviderConnector{Backend: backend, Engine: kind, Model: identity, Caps: capabilities}
	if backend == nil {
		return ProviderConnector{}, errors.New("engine connector: provider is required")
	}
	if err := identity.Validate(); err != nil {
		return ProviderConnector{}, err
	}
	if identity.Engine != kind {
		return ProviderConnector{}, fmt.Errorf("engine connector: engine %q does not match identity %q", kind, identity.Engine)
	}
	return connector, nil
}

func NewMLXConnector(backend provider.Provider, identity Identity, capabilities Capabilities) (ProviderConnector, error) {
	return NewProviderConnector(backend, KindMLX, identity, capabilities)
}

func NewOMLXConnector(backend provider.Provider, identity Identity, capabilities Capabilities) (ProviderConnector, error) {
	return NewProviderConnector(backend, KindOMLX, identity, capabilities)
}

func NewSNEConnector(backend provider.Provider, identity Identity, capabilities Capabilities) (ProviderConnector, error) {
	return NewProviderConnector(backend, KindSNE, identity, capabilities)
}

func (c ProviderConnector) OpenSession(ctx context.Context, id string) (Session, error) {
	if c.Backend == nil {
		return Session{}, errors.New("engine connector: provider is required")
	}
	if err := c.Model.Validate(); err != nil {
		return Session{}, err
	}
	if c.Engine != c.Model.Engine {
		return Session{}, fmt.Errorf("engine connector: engine %q does not match identity %q", c.Engine, c.Model.Engine)
	}
	if !c.Caps.Has(CapabilitySessions) {
		return Session{}, errors.New("engine connector: sessions are unsupported by the selected engine")
	}
	if strings.TrimSpace(id) == "" {
		return Session{}, errors.New("engine connector: session id is required")
	}
	if !c.Backend.Available(ctx) {
		return Session{}, fmt.Errorf("engine connector: %s is unavailable", c.Backend.Name())
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	return Session{ID: id, Identity: c.Model, CreatedAt: now().UTC().Format(time.RFC3339Nano)}, nil
}

func (c ProviderConnector) Complete(ctx context.Context, session Session, req GenerateRequest) (Completion, Receipt, error) {
	if err := req.Validate(session, c.Caps); err != nil {
		return Completion{}, Receipt{}, err
	}
	if c.Backend == nil {
		return Completion{}, Receipt{}, errors.New("engine connector: provider is required")
	}
	started := time.Now
	if c.Now != nil {
		started = c.Now
	}
	start := started().UTC()
	response, err := c.Backend.Complete(ctx, providerRequest(req))
	if err != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine connector %s: %w", c.Engine, err)
	}
	if response.Model != "" && response.Model != session.Identity.ModelID {
		return Completion{}, Receipt{}, fmt.Errorf("engine connector: served model %q does not match admitted model %q", response.Model, session.Identity.ModelID)
	}
	requestBytes, err := json.Marshal(req)
	if err != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine connector: request digest: %w", err)
	}
	requestSum := sha256.Sum256(requestBytes)
	completionSum := sha256.Sum256([]byte(response.Text))
	finished := started().UTC()
	receipt := Receipt{
		ABIVersion:       ABIVersion,
		SessionID:        session.ID,
		Identity:         session.Identity,
		RequestSHA256:    hex.EncodeToString(requestSum[:]),
		CompletionSHA256: hex.EncodeToString(completionSum[:]),
		StartedAt:        start.Format(time.RFC3339Nano),
		FinishedAt:       finished.Format(time.RFC3339Nano),
	}
	receipt.IdentityDigest, err = session.Identity.Digest()
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	if err := receipt.Validate(session); err != nil {
		return Completion{}, Receipt{}, err
	}
	return Completion{Text: response.Text, Model: response.Model, FinishReason: response.FinishReason, PromptTokens: response.PromptTokens, OutputTokens: response.OutputTokens}, receipt, nil
}

// Stream is intentionally explicit for the current OpenAI-compatible bridge:
// provider.Provider has no streaming method, so claiming streaming here would
// silently turn a requested stream into a buffered completion. Future
// streaming adapters implement this method without changing the ABI.
func (c ProviderConnector) Stream(ctx context.Context, session Session, req GenerateRequest) (<-chan Event, error) {
	if err := req.Validate(session, c.Caps); err != nil {
		return nil, err
	}
	streaming, ok := c.Backend.(provider.StreamingProvider)
	if !ok {
		return nil, fmt.Errorf("%w: %s connector has no streaming transport", ErrUnsupportedCapability, c.Engine)
	}
	raw, err := streaming.Stream(ctx, providerRequest(req))
	if err != nil {
		return nil, fmt.Errorf("engine connector %s: %w", c.Engine, err)
	}
	events := make(chan Event)
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	started := now().UTC()
	go func() {
		defer close(events)
		var sequence uint64
		var text strings.Builder
		model := ""
		for chunk := range raw {
			if chunk.Err != nil {
				sequence++
				emitEngineEvent(ctx, events, Event{Kind: EventError, SessionID: session.ID, Sequence: sequence, ErrorCode: "stream_error", Error: chunk.Err.Error()})
				return
			}
			if chunk.Model != "" {
				if model != "" && model != chunk.Model {
					sequence++
					emitEngineEvent(ctx, events, Event{Kind: EventError, SessionID: session.ID, Sequence: sequence, ErrorCode: "model_identity_mismatch", Error: "stream changed served model"})
					return
				}
				model = chunk.Model
				if model != session.Identity.ModelID {
					sequence++
					emitEngineEvent(ctx, events, Event{Kind: EventError, SessionID: session.ID, Sequence: sequence, ErrorCode: "model_identity_mismatch", Error: "stream model does not match admitted identity"})
					return
				}
			}
			if chunk.Text != "" {
				text.WriteString(chunk.Text)
				sequence++
				if !emitEngineEvent(ctx, events, Event{Kind: EventDelta, SessionID: session.ID, Sequence: sequence, Text: chunk.Text}) {
					return
				}
			}
			if chunk.Done {
				sequence++
				requestBytes, _ := json.Marshal(req)
				requestSum := sha256.Sum256(requestBytes)
				completionSum := sha256.Sum256([]byte(text.String()))
				receipt := Receipt{ABIVersion: ABIVersion, SessionID: session.ID, Identity: session.Identity, RequestSHA256: hex.EncodeToString(requestSum[:]), CompletionSHA256: hex.EncodeToString(completionSum[:]), StartedAt: started.Format(time.RFC3339Nano), FinishedAt: now().UTC().Format(time.RFC3339Nano)}
				receipt.IdentityDigest, _ = session.Identity.Digest()
				if err := receipt.Validate(session); err != nil {
					sequence++
					emitEngineEvent(ctx, events, Event{Kind: EventError, SessionID: session.ID, Sequence: sequence, ErrorCode: "receipt_invalid", Error: err.Error()})
					return
				}
				if !emitEngineEvent(ctx, events, Event{Kind: EventCompleted, SessionID: session.ID, Sequence: sequence, Text: text.String(), Receipt: &receipt}) {
					return
				}
				return
			}
		}
	}()
	return events, nil
}

func providerTools(tools []ToolSpec) []provider.ToolSpec {
	if len(tools) == 0 {
		return nil
	}
	out := make([]provider.ToolSpec, len(tools))
	for i, tool := range tools {
		out[i] = provider.ToolSpec{Name: tool.Name, Description: tool.Description, Schema: tool.Schema}
	}
	return out
}

func providerRequest(req GenerateRequest) provider.Request {
	return provider.Request{
		System:      req.System,
		Prompt:      req.Prompt,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Seed:        req.Seed,
		Tools:       providerTools(req.Tools),
	}
}

func emitEngineEvent(ctx context.Context, events chan<- Event, event Event) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r Receipt) Validate(session Session) error {
	if r.ABIVersion != ABIVersion {
		return fmt.Errorf("engine receipt: abi_version %q does not match %q", r.ABIVersion, ABIVersion)
	}
	if r.SessionID != session.ID || !r.Identity.Equal(session.Identity) {
		return errors.New("engine receipt: session identity mismatch")
	}
	digest, err := r.Identity.Digest()
	if err != nil {
		return err
	}
	if r.IdentityDigest != digest {
		return errors.New("engine receipt: identity digest mismatch")
	}
	for name, value := range map[string]string{"request_sha256": r.RequestSHA256, "completion_sha256": r.CompletionSHA256} {
		if !sha256Pattern.MatchString(value) {
			return fmt.Errorf("engine receipt: %s must be lowercase SHA-256", name)
		}
	}
	for name, value := range map[string]string{"started_at": r.StartedAt, "finished_at": r.FinishedAt} {
		if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
			return fmt.Errorf("engine receipt: %s: %w", name, err)
		}
	}
	return nil
}
