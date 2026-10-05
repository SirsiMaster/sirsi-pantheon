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
	"math"
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

// BackendVariant is the closed backend implementation identity. A variant is
// part of the admitted engine identity; it is never inferred from provider
// behavior or silently substituted at execution time.
type BackendVariant string

// Variant is retained as a concise compatibility name for callers that use
// the ABI's variant terminology.
type Variant = BackendVariant

const (
	VariantMLXRaw     BackendVariant = "mlx-raw"
	VariantMLXPatched BackendVariant = "mlx-patched"
	VariantOMLXPublic BackendVariant = "omlx-public"
	VariantSNEPlain   BackendVariant = "sne-plain"
	VariantSNEMTP     BackendVariant = "sne-mtp"
)

func (v BackendVariant) Validate() error {
	switch v {
	case VariantMLXRaw, VariantMLXPatched, VariantOMLXPublic, VariantSNEPlain, VariantSNEMTP:
		return nil
	default:
		return fmt.Errorf("engine variant: unsupported variant %q", v)
	}
}

func (v BackendVariant) ValidateForEngine(kind Kind) error {
	if err := v.Validate(); err != nil {
		return err
	}
	compatible := (kind == KindMLX && (v == VariantMLXRaw || v == VariantMLXPatched)) ||
		(kind == KindOMLX && v == VariantOMLXPublic) ||
		(kind == KindSNE && (v == VariantSNEPlain || v == VariantSNEMTP))
	if !compatible {
		return fmt.Errorf("engine variant %q is incompatible with engine %q", v, kind)
	}
	return nil
}

func DefaultVariant(kind Kind) BackendVariant {
	switch kind {
	case KindMLX:
		return VariantMLXRaw
	case KindOMLX:
		return VariantOMLXPublic
	case KindSNE:
		return VariantSNEPlain
	default:
		return ""
	}
}

// RouteDisplayName returns Pantheon's stable product-facing name for a
// configured engine route. Kind and variant remain the machine-readable
// identity; this label is only for operator-facing surfaces.
func RouteDisplayName(kind Kind, variant BackendVariant) string {
	switch {
	case kind == KindMLX && variant == VariantMLXRaw:
		return "MLX · Raw"
	case kind == KindMLX && variant == VariantMLXPatched:
		return "MLX · Patched"
	case kind == KindOMLX && variant == VariantOMLXPublic:
		return "oMLX · Public"
	case kind == KindSNE && variant == VariantSNEPlain:
		return "Apollo (Plain)"
	case kind == KindSNE && variant == VariantSNEMTP:
		return "Apollo Flash (Speculative)"
	default:
		if variant == "" {
			return string(kind)
		}
		return string(kind) + " · " + string(variant)
	}
}

func ParseVariant(value string) (BackendVariant, error) {
	variant := BackendVariant(strings.ToLower(strings.TrimSpace(value)))
	if err := variant.Validate(); err != nil {
		return "", err
	}
	return variant, nil
}

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

func variantRequiredCapabilities(variant BackendVariant) []Capability {
	if variant == VariantSNEMTP {
		return []Capability{CapabilityMTP}
	}
	return nil
}

type Identity struct {
	Engine          Kind               `json:"engine"`
	Variant         BackendVariant     `json:"variant"`
	EngineVersion   string             `json:"engine_version"`
	ModelID         string             `json:"model_id"`
	ModelSHA256     string             `json:"model_sha256"`
	TokenizerID     string             `json:"tokenizer_id"`
	TokenizerSHA256 string             `json:"tokenizer_sha256"`
	Precision       string             `json:"precision"`
	CacheNamespace  string             `json:"cache_namespace"`
	Assistant       *AssistantIdentity `json:"assistant,omitempty"`
}

// AssistantIdentity binds the separately loaded assistant checkpoint used by
// SNE MTP. It is part of the selected engine identity and therefore of every
// session and receipt digest, rather than a mutable service-side annotation.
type AssistantIdentity struct {
	ModelID          string `json:"model_id"`
	Revision         string `json:"revision"`
	CheckpointSHA256 string `json:"checkpoint_sha256"`
	Precision        string `json:"precision"`
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (i Identity) Validate() error {
	if i.Engine != KindMLX && i.Engine != KindOMLX && i.Engine != KindSNE {
		return fmt.Errorf("engine identity: unsupported engine %q", i.Engine)
	}
	variant := i.EffectiveVariant()
	if err := variant.ValidateForEngine(i.Engine); err != nil {
		return fmt.Errorf("engine identity: %w", err)
	}
	for _, field := range []struct{ name, value string }{
		{"engine_version", i.EngineVersion},
		{"model_id", i.ModelID},
		{"tokenizer_id", i.TokenizerID},
		{"precision", i.Precision},
		{"cache_namespace", i.CacheNamespace},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("engine identity: %s is required", field.name)
		}
	}
	for _, field := range []struct{ name, value string }{
		{"model_sha256", i.ModelSHA256},
		{"tokenizer_sha256", i.TokenizerSHA256},
	} {
		if !sha256Pattern.MatchString(field.value) {
			return fmt.Errorf("engine identity: %s must be lowercase SHA-256", field.name)
		}
	}
	if variant == VariantSNEMTP {
		if i.Assistant == nil {
			return fmt.Errorf("engine identity: assistant identity is required for sne-mtp")
		}
		for _, field := range []struct{ name, value string }{
			{"assistant.model_id", i.Assistant.ModelID},
			{"assistant.revision", i.Assistant.Revision},
			{"assistant.precision", i.Assistant.Precision},
		} {
			if strings.TrimSpace(field.value) == "" {
				return fmt.Errorf("engine identity: %s is required", field.name)
			}
		}
		if !sha256Pattern.MatchString(i.Assistant.CheckpointSHA256) {
			return fmt.Errorf("engine identity: assistant.checkpoint_sha256 must be lowercase SHA-256")
		}
	} else if i.Assistant != nil {
		return fmt.Errorf("engine identity: assistant identity is only valid for sne-mtp")
	}
	return nil
}

func (i Identity) EffectiveVariant() BackendVariant {
	if i.Variant != "" {
		return i.Variant
	}
	return DefaultVariant(i.Engine)
}

func (i Identity) canonical() Identity {
	i.Variant = i.EffectiveVariant()
	if i.Assistant != nil {
		assistant := *i.Assistant
		i.Assistant = &assistant
	}
	return i
}

func (i Identity) Equal(other Identity) bool {
	a, b := i.canonical(), other.canonical()
	if a.Assistant == nil || b.Assistant == nil {
		return a.Assistant == nil && b.Assistant == nil && a.withoutAssistant() == b.withoutAssistant()
	}
	return *a.Assistant == *b.Assistant && a.withoutAssistant() == b.withoutAssistant()
}

func (i Identity) withoutAssistant() Identity {
	i.Assistant = nil
	return i
}

// MarshalJSON makes legacy empty-variant literals emit the explicit canonical
// variant required by session and receipt identities.
func (i Identity) MarshalJSON() ([]byte, error) {
	type identityJSON Identity
	return json.Marshal(identityJSON(i.canonical()))
}

// Digest is the stable identity key used by sessions and receipts. JSON field
// order is fixed by the struct declaration, so equivalent identities hash
// identically across connectors.
func (i Identity) Digest() (string, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(i.canonical())
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

// GenerateRequestDigest hashes the exact JSON representation bound to an
// execution receipt. It is shared by provider adapters and the router so a
// connector cannot substitute a valid-looking receipt for different inputs.
func GenerateRequestDigest(request GenerateRequest) (string, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("engine request: encode receipt input: %w", err)
	}
	sum := sha256.Sum256(requestBytes)
	return hex.EncodeToString(sum[:]), nil
}

// SnapshotGenerateRequest creates an immutable-by-ownership request value for
// an execution boundary and returns the digest of its normalized JSON form.
// The JSON round trip detaches nested tool-schema maps and slices from caller
// mutation; hashing the decoded snapshot binds the receipt to the value the
// backend will actually receive.
func SnapshotGenerateRequest(request GenerateRequest) (GenerateRequest, string, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return GenerateRequest{}, "", fmt.Errorf("engine request: encode snapshot: %w", err)
	}
	var snapshot GenerateRequest
	if err := json.Unmarshal(requestBytes, &snapshot); err != nil {
		return GenerateRequest{}, "", fmt.Errorf("engine request: decode snapshot: %w", err)
	}
	snapshotBytes, err := json.Marshal(snapshot)
	if err != nil {
		return GenerateRequest{}, "", fmt.Errorf("engine request: encode normalized snapshot: %w", err)
	}
	sum := sha256.Sum256(snapshotBytes)
	return snapshot, hex.EncodeToString(sum[:]), nil
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
	for _, capability := range variantRequiredCapabilities(session.Identity.EffectiveVariant()) {
		if !capabilities.Has(capability) {
			return fmt.Errorf("%w: %s required by variant %s", ErrUnsupportedCapability, capability, session.Identity.EffectiveVariant())
		}
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
	if r.Temperature != nil && (math.IsNaN(*r.Temperature) || math.IsInf(*r.Temperature, 0) || *r.Temperature < 0 || *r.Temperature > 2) {
		return fmt.Errorf("engine request: temperature must be finite and within [0,2]")
	}
	if r.TopP != nil && (math.IsNaN(*r.TopP) || math.IsInf(*r.TopP, 0) || *r.TopP <= 0 || *r.TopP > 1) {
		return fmt.Errorf("engine request: top_p must be finite and within (0,1]")
	}
	if r.Stream {
		if !capabilities.Has(CapabilityStreaming) {
			return errors.New("engine request: streaming is unsupported by the selected engine")
		}
		if !capabilities.Has(CapabilityCancellation) {
			return fmt.Errorf("%w: cancellation for streaming", ErrUnsupportedCapability)
		}
	}
	if !capabilities.Has(CapabilityReceipts) {
		return fmt.Errorf("%w: receipts", ErrUnsupportedCapability)
	}
	if _, err := GenerateRequestDigest(r); err != nil {
		return err
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
	Model      string    `json:"model,omitempty"`
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
	if e.Kind == EventError && e.ErrorCode == "cancelled" && (e.Receipt == nil || !e.Receipt.Cancelled) {
		return errors.New("engine event: cancellation errors require a cancelled receipt")
	}
	if e.Kind == EventError && e.ErrorCode != "cancelled" && e.Receipt != nil && e.Receipt.Cancelled {
		return errors.New("engine event: non-cancellation errors cannot carry a cancelled receipt")
	}
	if e.Kind == EventCompleted && e.Receipt == nil {
		return errors.New("engine event: completed events require a receipt")
	}
	if e.Kind == EventCompleted && e.Receipt != nil && e.Receipt.Cancelled {
		return errors.New("engine event: completed event cannot carry a cancelled receipt")
	}
	if e.Receipt != nil && e.Receipt.SessionID != e.SessionID {
		return errors.New("engine event: receipt session does not match event session")
	}
	if e.Kind == EventCompleted && strings.TrimSpace(e.Model) == "" {
		return errors.New("engine event: completed events require served model identity")
	}
	return nil
}

type Receipt struct {
	ABIVersion       string         `json:"abi_version"`
	SessionID        string         `json:"session_id"`
	Identity         Identity       `json:"identity"`
	IdentityDigest   string         `json:"identity_digest"`
	RequestSHA256    string         `json:"request_sha256"`
	CompletionSHA256 string         `json:"completion_sha256"`
	StartedAt        string         `json:"started_at"`
	FinishedAt       string         `json:"finished_at"`
	Cancelled        bool           `json:"cancelled"`
	Route            *RouteDecision `json:"route,omitempty"`
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
	Variant() BackendVariant
	Capabilities() Capabilities
	OpenSession(context.Context, string) (Session, error)
	Complete(context.Context, Session, GenerateRequest) (Completion, Receipt, error)
	Stream(context.Context, Session, GenerateRequest) (<-chan Event, error)
}

// ContextualCapabilityResolver handles capabilities whose truth depends on a
// live service identity. Static connector snapshots stay conservative; only
// session admission may resolve these values.
type ContextualCapabilityResolver interface {
	CapabilitiesForContext(context.Context) (Capabilities, error)
	CanResolveCapability(Capability) bool
}

// ProviderConnector adapts the existing provider ladder into the ABI. MLX,
// OMLX, and SNE OpenAI-compatible endpoints can use this bridge while their
// engine-specific lifecycle remains outside this package.
type ProviderConnector struct {
	Backend provider.Provider
	Engine  Kind
	Model   Identity
	Caps    Capabilities
	// RequestBoundary is a privacy-safe classification for providers that do
	// not expose their configured endpoint through the provider interface (for
	// example, the SNE adapter). OpenAICompat boundaries are always derived from
	// its endpoint. This is endpoint metadata, not a trace of intermediaries.
	RequestBoundary string
	Now             func() time.Time
}

func (c ProviderConnector) Kind() Kind { return c.Engine }

func (c ProviderConnector) Variant() BackendVariant { return c.Model.EffectiveVariant() }

func (c ProviderConnector) Capabilities() Capabilities { return c.Caps }

func (c ProviderConnector) RequestDataBoundary() string {
	if backend, ok := c.Backend.(*provider.OpenAICompat); ok && backend != nil {
		return boundaryForProviderTier(provider.EndpointTier(backend.Endpoint))
	}
	if c.RequestBoundary != "" {
		switch c.RequestBoundary {
		case "local", DataBoundaryOnDevice:
			return DataBoundaryOnDevice
		case "remote":
			return DataBoundaryRemote
		default:
			return DataBoundaryNotDisclosed
		}
	}
	return DataBoundaryNotDisclosed
}

func boundaryForProviderTier(tier provider.Tier) string {
	switch tier {
	case provider.TierLocal:
		return DataBoundaryOnDevice
	case provider.TierRemote:
		return DataBoundaryRemote
	default:
		return DataBoundaryNotDisclosed
	}
}

func (c ProviderConnector) CanResolveCapability(capability Capability) bool {
	_, ok := c.Backend.(provider.ContextualCapabilities)
	return ok && capability == CapabilityMTP && c.Engine == KindSNE && c.Variant() == VariantSNEMTP
}

func (c ProviderConnector) CapabilitiesForContext(ctx context.Context) (Capabilities, error) {
	resolver, ok := c.Backend.(provider.ContextualCapabilities)
	if !ok {
		return c.Caps, nil
	}
	resolved, err := resolver.CapabilitiesForContext(ctx)
	if err != nil {
		return Capabilities{}, err
	}
	caps := c.Caps
	caps.MTP = resolved.MTP
	return caps, nil
}

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
	if ctx == nil {
		return Session{}, errors.New("engine connector: context is required")
	}
	if err := ctx.Err(); err != nil {
		return Session{}, fmt.Errorf("engine connector: session cancelled before availability check: %w", err)
	}
	if c.Backend == nil {
		return Session{}, errors.New("engine connector: provider is required")
	}
	if err := c.Model.Validate(); err != nil {
		return Session{}, err
	}
	variantRequirements := variantRequiredCapabilities(c.Model.EffectiveVariant())
	if err := requireCapabilities(c.Caps, variantRequirements); err != nil {
		resolver, canResolve := c.Backend.(provider.ContextualCapabilities)
		resolvable := canResolve
		for _, capability := range variantRequirements {
			if !c.Caps.Has(capability) && capability != CapabilityMTP {
				resolvable = false
			}
		}
		if !resolvable || c.Engine != KindSNE || c.Model.EffectiveVariant() != VariantSNEMTP {
			return Session{}, fmt.Errorf("engine connector: configured variant is not supported: %w", err)
		}
		resolved, resolveErr := resolver.CapabilitiesForContext(ctx)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Session{}, fmt.Errorf("engine connector: session cancelled during contextual capability admission: %w", ctxErr)
		}
		if resolveErr != nil {
			return Session{}, fmt.Errorf("engine connector: contextual capability admission: %w", resolveErr)
		}
		validationCapabilities := c.Caps
		validationCapabilities.MTP = resolved.MTP
		if resolveErr = requireCapabilities(validationCapabilities, variantRequirements); resolveErr != nil {
			return Session{}, fmt.Errorf("engine connector: configured variant is not supported: %w", resolveErr)
		}
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
	var availabilityErr error
	if checker, ok := c.Backend.(provider.ReadinessChecker); ok {
		availabilityErr = checker.Readiness(ctx)
	} else if !c.Backend.Available(ctx) {
		availabilityErr = fmt.Errorf("%s is unavailable", c.Backend.Name())
	}
	if err := ctx.Err(); err != nil {
		return Session{}, fmt.Errorf("engine connector: session cancelled during availability check: %w", err)
	}
	if availabilityErr != nil {
		return Session{}, fmt.Errorf("engine connector: %s readiness: %w", c.Backend.Name(), availabilityErr)
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	identity := c.Model.canonical()
	return Session{ID: id, Identity: identity, CreatedAt: now().UTC().Format(time.RFC3339Nano)}, nil
}

func (c ProviderConnector) Complete(ctx context.Context, session Session, req GenerateRequest) (Completion, Receipt, error) {
	if ctx == nil {
		return Completion{}, Receipt{}, errors.New("engine connector: context is required")
	}
	if req.Stream {
		return Completion{}, Receipt{}, errors.New("engine connector: Complete requires request stream=false")
	}
	snapshot, requestDigest, err := SnapshotGenerateRequest(req)
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	req = snapshot
	if c.Backend == nil {
		return Completion{}, Receipt{}, errors.New("engine connector: provider is required")
	}
	if err := session.Validate(); err != nil {
		return Completion{}, Receipt{}, err
	}
	if session.Identity.Engine != c.Engine || !session.Identity.Equal(c.Model) {
		return Completion{}, Receipt{}, errors.New("engine connector: session identity does not match this connector")
	}
	if req.SessionID != session.ID || !req.Identity.Equal(session.Identity) {
		return Completion{}, Receipt{}, errors.New("engine request: identity or session does not match the admitted session")
	}
	validationCapabilities := c.Caps
	requiredCapabilities := append([]Capability(nil), req.RequiredCapabilities...)
	for _, capability := range variantRequiredCapabilities(session.Identity.EffectiveVariant()) {
		requiredCapabilities = includeCapability(requiredCapabilities, capability)
	}
	if err := requireCapabilities(validationCapabilities, requiredCapabilities); err != nil {
		resolver, canResolve := c.Backend.(provider.ContextualCapabilities)
		resolvable := canResolve
		for _, capability := range requiredCapabilities {
			if !validationCapabilities.Has(capability) && capability != CapabilityMTP {
				resolvable = false
			}
		}
		if !resolvable || c.Engine != KindSNE || session.Identity.EffectiveVariant() != VariantSNEMTP {
			return Completion{}, Receipt{}, err
		}
		resolved, resolveErr := resolver.CapabilitiesForContext(ctx)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Completion{}, Receipt{}, fmt.Errorf("engine connector: completion cancelled during contextual capability admission: %w", ctxErr)
		}
		if resolveErr != nil {
			return Completion{}, Receipt{}, fmt.Errorf("engine connector: contextual capability admission: %w", resolveErr)
		}
		validationCapabilities.MTP = resolved.MTP
		if err := requireCapabilities(validationCapabilities, requiredCapabilities); err != nil {
			return Completion{}, Receipt{}, err
		}
	}
	if err := req.Validate(session, validationCapabilities); err != nil {
		return Completion{}, Receipt{}, err
	}
	started := time.Now
	if c.Now != nil {
		started = c.Now
	}
	start := started().UTC()
	backendRequest, err := providerRequest(req)
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine connector: completion cancelled before provider call: %w", err)
	}
	response, err := c.Backend.Complete(ctx, backendRequest)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine connector: completion cancelled during provider call: %w", ctxErr)
	}
	if err != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine connector %s: %w", c.Engine, err)
	}
	if response.Model != session.Identity.ModelID {
		return Completion{}, Receipt{}, fmt.Errorf("engine connector: served model %q does not match admitted model %q", response.Model, session.Identity.ModelID)
	}
	completionSum := sha256.Sum256([]byte(response.Text))
	finished := started().UTC()
	receipt := Receipt{
		ABIVersion:       ABIVersion,
		SessionID:        session.ID,
		Identity:         session.Identity,
		RequestSHA256:    requestDigest,
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
	if ctx == nil {
		return nil, errors.New("engine connector: context is required")
	}
	if !req.Stream {
		return nil, errors.New("engine connector: Stream requires request stream=true")
	}
	snapshot, _, err := SnapshotGenerateRequest(req)
	if err != nil {
		return nil, err
	}
	req = snapshot
	if err := req.Validate(session, c.Caps); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("engine connector: stream cancelled before start: %w", err)
	}
	streaming, ok := c.Backend.(provider.StreamingProvider)
	if !ok {
		return nil, fmt.Errorf("%w: %s connector has no streaming transport", ErrUnsupportedCapability, c.Engine)
	}
	backendRequest, err := providerRequest(req)
	if err != nil {
		return nil, err
	}
	raw, err := streaming.Stream(ctx, backendRequest)
	if err != nil {
		return nil, fmt.Errorf("engine connector %s: %w", c.Engine, err)
	}
	// One slot lets the cancellation terminal event be retained even after the
	// caller cancels its context and stops receiving immediately. The provider
	// still receives the same context, so network-backed streams can terminate.
	events := make(chan Event, 1)
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
		finishReason := ""
		emitCancellation := func() {
			sequence++
			receipt, err := streamReceipt(session, req, text.String(), started, now().UTC(), true)
			if err != nil {
				return
			}
			event := Event{Kind: EventError, SessionID: session.ID, Sequence: sequence, ErrorCode: "cancelled", Error: "stream cancelled", Receipt: &receipt}
			publishTerminalEngineEvent(events, event, func(pending Event) Event {
				if pending.Sequence > 0 && pending.Sequence < event.Sequence {
					event.Sequence = pending.Sequence
				}
				if pending.Kind == EventDelta && pending.Text != "" && strings.HasSuffix(text.String(), pending.Text) {
					delivered := strings.TrimSuffix(text.String(), pending.Text)
					if adjusted, err := streamReceipt(session, req, delivered, started, now().UTC(), true); err == nil {
						event.Receipt = &adjusted
					}
				}
				return event
			})
		}
		emitTerminal := func(event Event) {
			if emitEngineEvent(ctx, events, event) {
				return
			}
			if ctx.Err() != nil {
				sequence-- // The failed terminal was not published; reuse its sequence for cancellation.
				emitCancellation()
			}
		}
		for {
			var chunk provider.StreamChunk
			var ok bool
			select {
			case <-ctx.Done():
				emitCancellation()
				return
			case chunk, ok = <-raw:
				if !ok {
					if ctx.Err() != nil {
						emitCancellation()
						return
					}
					sequence++
					event := Event{Kind: EventError, SessionID: session.ID, Sequence: sequence, ErrorCode: "stream_incomplete", Error: "provider stream closed before completion"}
					publishTerminalEngineEvent(events, event, func(pending Event) Event {
						if pending.Sequence > 0 && pending.Sequence < event.Sequence {
							event.Sequence = pending.Sequence
						}
						return event
					})
					return
				}
			}
			if ctx.Err() != nil {
				emitCancellation()
				return
			}
			if chunk.Err != nil {
				sequence++
				emitTerminal(Event{Kind: EventError, SessionID: session.ID, Sequence: sequence, ErrorCode: "stream_error", Error: chunk.Err.Error()})
				return
			}
			if chunk.Model != "" {
				if model != "" && model != chunk.Model {
					sequence++
					emitTerminal(Event{Kind: EventError, SessionID: session.ID, Sequence: sequence, ErrorCode: "model_identity_mismatch", Error: "stream changed served model"})
					return
				}
				model = chunk.Model
				if model != session.Identity.ModelID {
					sequence++
					emitTerminal(Event{Kind: EventError, SessionID: session.ID, Sequence: sequence, ErrorCode: "model_identity_mismatch", Error: "stream model does not match admitted identity"})
					return
				}
			}
			if chunk.FinishReason != "" {
				finishReason = chunk.FinishReason
			}
			if chunk.Text != "" {
				sequence++
				if !emitEngineEvent(ctx, events, Event{Kind: EventDelta, SessionID: session.ID, Sequence: sequence, Text: chunk.Text}) {
					if ctx.Err() != nil {
						sequence-- // The blocked delta was not published; reuse its sequence for cancellation.
						emitCancellation()
					}
					return
				}
				text.WriteString(chunk.Text)
			}
			if chunk.Done {
				if ctx.Err() != nil {
					emitCancellation()
					return
				}
				if model == "" {
					sequence++
					emitTerminal(Event{Kind: EventError, SessionID: session.ID, Sequence: sequence, ErrorCode: "model_identity_missing", Error: "stream completed without a served model identity"})
					return
				}
				receipt, err := streamReceipt(session, req, text.String(), started, now().UTC(), false)
				if err != nil {
					sequence++
					emitTerminal(Event{Kind: EventError, SessionID: session.ID, Sequence: sequence, ErrorCode: "receipt_invalid", Error: err.Error()})
					return
				}
				sequence++
				if !emitEngineEvent(ctx, events, Event{Kind: EventCompleted, SessionID: session.ID, Sequence: sequence, Model: model, Text: text.String(), Finish: finishReason, Receipt: &receipt}) {
					if ctx.Err() != nil {
						sequence-- // Completion was not published; cancellation is the terminal outcome.
						emitCancellation()
					}
					return
				}
				return
			}
		}
	}()
	return events, nil
}

// publishTerminalEngineEvent gives terminal state priority over a buffered
// delta when a consumer has stopped reading. Otherwise cancellation could
// silently close the stream without the required terminal event and receipt.
func publishTerminalEngineEvent(events chan Event, event Event, rebase func(Event) Event) {
	select {
	case events <- event:
		return
	default:
	}
	select {
	case pending := <-events:
		if rebase != nil {
			event = rebase(pending)
		}
	default:
	}
	select {
	case events <- event:
	default:
	}
}

func streamReceipt(session Session, req GenerateRequest, text string, started, finished time.Time, cancelled bool) (Receipt, error) {
	requestDigest, err := GenerateRequestDigest(req)
	if err != nil {
		return Receipt{}, err
	}
	completionSum := sha256.Sum256([]byte(text))
	receipt := Receipt{
		ABIVersion:       ABIVersion,
		SessionID:        session.ID,
		Identity:         session.Identity,
		RequestSHA256:    requestDigest,
		CompletionSHA256: hex.EncodeToString(completionSum[:]),
		StartedAt:        started.Format(time.RFC3339Nano),
		FinishedAt:       finished.Format(time.RFC3339Nano),
		Cancelled:        cancelled,
	}
	receipt.IdentityDigest, err = session.Identity.Digest()
	if err != nil {
		return Receipt{}, err
	}
	if err := receipt.Validate(session); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func providerTools(tools []ToolSpec) ([]provider.ToolSpec, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	out := make([]provider.ToolSpec, len(tools))
	for i, tool := range tools {
		encodedSchema, err := json.Marshal(tool.Schema)
		if err != nil {
			return nil, fmt.Errorf("engine connector: encode tool schema %q: %w", tool.Name, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(encodedSchema, &schema); err != nil {
			return nil, fmt.Errorf("engine connector: snapshot tool schema %q: %w", tool.Name, err)
		}
		out[i] = provider.ToolSpec{Name: tool.Name, Description: tool.Description, Schema: schema}
	}
	return out, nil
}

func providerRequest(req GenerateRequest) (provider.Request, error) {
	tools, err := providerTools(req.Tools)
	if err != nil {
		return provider.Request{}, err
	}
	return provider.Request{
		System:      req.System,
		Prompt:      req.Prompt,
		MaxTokens:   req.MaxTokens,
		Temperature: copyFloat64(req.Temperature),
		TopP:        copyFloat64(req.TopP),
		Seed:        copyInt64(req.Seed),
		Tools:       tools,
	}, nil
}

func copyFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
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
	if r.Route != nil {
		if r.Route.DataBoundary == "" {
			return errors.New("engine receipt: route data boundary is required; use not-disclosed when unknown")
		}
		if err := r.Route.validate(session.Identity.Engine, session.Identity.EffectiveVariant()); err != nil {
			return err
		}
	}
	if r.IdentityDigest != digest {
		return errors.New("engine receipt: identity digest mismatch")
	}
	for name, value := range map[string]string{"request_sha256": r.RequestSHA256, "completion_sha256": r.CompletionSHA256} {
		if !sha256Pattern.MatchString(value) {
			return fmt.Errorf("engine receipt: %s must be lowercase SHA-256", name)
		}
	}
	startedAt, err := time.Parse(time.RFC3339Nano, r.StartedAt)
	if err != nil {
		return fmt.Errorf("engine receipt: started_at: %w", err)
	}
	finishedAt, err := time.Parse(time.RFC3339Nano, r.FinishedAt)
	if err != nil {
		return fmt.Errorf("engine receipt: finished_at: %w", err)
	}
	if finishedAt.Before(startedAt) {
		return errors.New("engine receipt: finished_at precedes started_at")
	}
	return nil
}
