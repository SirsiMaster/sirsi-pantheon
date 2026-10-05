package provider

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

// SNEClient is the narrow native SNE surface Pantheon consumes. Keeping this
// seam smaller than sne.Client makes the adapter deterministic in tests while
// leaving SNE's service implementation and lifecycle ownership untouched.
type SNEClient interface {
	ReadinessIdentity(context.Context) (sne.ServiceReadinessIdentity, error)
	Complete(context.Context, sne.CompletionRequest) (*sne.CompletionResponse, error)
}

// SNEIdentityExpectation is the immutable runtime tuple admitted by the
// Pantheon engine configuration. Readiness is rejected unless the native
// service reports the same model, runtime, native-runtime, and manifest
// identities.
type SNEIdentityExpectation struct {
	ModelID             string
	RuntimeSHA256       string
	NativeRuntimeSHA256 string
	ManifestSHA256      string
	ExecutionMode       string
	Assistant           *sne.AssistantIdentity
}

// SNEProvider adapts the native SNE client to the engine-neutral provider
// ladder. It is deliberately buffered: SNE's native client does not expose a
// streaming method, so the adapter never claims streaming capability.
type SNEProvider struct {
	client      SNEClient
	expectation SNEIdentityExpectation
}

// NewSNEProvider creates a local SNE provider bound to one admitted identity
// tuple. Readiness and completion fail closed when the service reports a
// different model or runtime generation; configuration is never trusted alone.
func NewSNEProvider(client SNEClient, expectation SNEIdentityExpectation) (*SNEProvider, error) {
	if client == nil {
		return nil, fmt.Errorf("SNE provider: client is required")
	}
	expectation.ModelID = strings.TrimSpace(expectation.ModelID)
	expectation.RuntimeSHA256 = strings.TrimSpace(expectation.RuntimeSHA256)
	expectation.NativeRuntimeSHA256 = strings.TrimSpace(expectation.NativeRuntimeSHA256)
	expectation.ManifestSHA256 = strings.TrimSpace(expectation.ManifestSHA256)
	if expectation.ModelID == "" {
		return nil, fmt.Errorf("SNE provider: model is required")
	}
	if expectation.ExecutionMode != sne.ExecutionModePlain && expectation.ExecutionMode != sne.ExecutionModeMTP {
		return nil, fmt.Errorf("SNE provider: explicit execution mode plain or mtp is required; got %q", expectation.ExecutionMode)
	}
	if expectation.ExecutionMode == sne.ExecutionModeMTP {
		if expectation.Assistant == nil {
			return nil, fmt.Errorf("SNE provider: MTP requires an admitted assistant identity")
		}
		if err := expectation.Assistant.Validate(); err != nil {
			return nil, fmt.Errorf("SNE provider: %w", err)
		}
		assistant := *expectation.Assistant
		assistant.ModelID = strings.TrimSpace(assistant.ModelID)
		assistant.Revision = strings.TrimSpace(assistant.Revision)
		assistant.CheckpointSHA256 = strings.TrimSpace(assistant.CheckpointSHA256)
		assistant.Precision = strings.TrimSpace(assistant.Precision)
		expectation.Assistant = &assistant
	} else if expectation.Assistant != nil {
		return nil, fmt.Errorf("SNE provider: assistant identity is only valid for MTP execution")
	}
	for _, field := range []struct{ name, value string }{
		{"runtime", expectation.RuntimeSHA256},
		{"native runtime", expectation.NativeRuntimeSHA256},
		{"manifest", expectation.ManifestSHA256},
	} {
		if !validSNEHash(field.value) {
			return nil, fmt.Errorf("SNE provider: %s identity must be lowercase SHA-256", field.name)
		}
	}
	return &SNEProvider{client: client, expectation: expectation}, nil
}

func validSNEHash(value string) bool {
	value = strings.TrimSpace(value)
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && strings.ToLower(value) == value
}

func (p *SNEProvider) Name() string { return "sne" }

func (p *SNEProvider) Tier() Tier { return TierLocal }

func (p *SNEProvider) Caps() Caps {
	return Caps{Temperature: true, Streaming: false, Cancellation: true, Offline: true}
}

func (p *SNEProvider) CapabilitiesForContext(ctx context.Context) (Caps, error) {
	identity, err := p.admitReadiness(ctx)
	if err != nil {
		return Caps{}, err
	}
	caps := p.Caps()
	caps.MTP = p.expectation.ExecutionMode == sne.ExecutionModeMTP && identity.ReadyCapabilities.SupportsExecutionMode(sne.ExecutionModeMTP)
	return caps, nil
}

func (p *SNEProvider) Available(ctx context.Context) bool {
	return p.Readiness(ctx) == nil
}

func (p *SNEProvider) Readiness(ctx context.Context) error {
	_, err := p.admitReadiness(ctx)
	return err
}

func (p *SNEProvider) Complete(ctx context.Context, req Request) (Response, error) {
	if ctx == nil {
		return Response{}, fmt.Errorf("SNE provider: context is required")
	}
	if p == nil || p.client == nil {
		return Response{}, fmt.Errorf("SNE provider: client is required")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return Response{}, fmt.Errorf("SNE provider: prompt is required")
	}
	readiness, err := p.admitReadiness(ctx)
	if err != nil {
		return Response{}, err
	}
	messages := make([]sne.Message, 0, 2)
	if req.System != "" {
		messages = append(messages, sne.Message{Role: "system", Content: req.System})
	}
	messages = append(messages, sne.Message{Role: "user", Content: req.Prompt})
	temperature := 0.0
	if req.Temperature != nil {
		temperature = *req.Temperature
	}
	completion, err := p.client.Complete(ctx, sne.CompletionRequest{
		Model:         p.expectation.ModelID,
		ExecutionMode: p.expectation.ExecutionMode,
		Messages:      messages,
		MaxTokens:     req.MaxTokens,
		Temperature:   temperature,
		Stream:        false,
	})
	if err != nil {
		return Response{}, fmt.Errorf("SNE provider: completion: %w", err)
	}
	if completion == nil || len(completion.Choices) == 0 {
		return Response{}, fmt.Errorf("SNE provider: completion returned no choices")
	}
	if completion.Model != p.expectation.ModelID {
		return Response{}, fmt.Errorf("SNE provider: served model %q does not match admitted model %q", completion.Model, p.expectation.ModelID)
	}
	if completion.SNE.Execution.Mode != p.expectation.ExecutionMode {
		return Response{}, fmt.Errorf("SNE provider: served execution mode %q does not match requested mode %q", completion.SNE.Execution.Mode, p.expectation.ExecutionMode)
	}
	if p.expectation.ExecutionMode == sne.ExecutionModeMTP {
		if completion.SNE.Execution.Assistant == nil || !sameSNEAssistantIdentity(*completion.SNE.Execution.Assistant, *readiness.ReadyCapabilities.Assistant) {
			return Response{}, fmt.Errorf("SNE provider: served MTP assistant identity does not match readiness")
		}
	} else if completion.SNE.Execution.Assistant != nil {
		return Response{}, fmt.Errorf("SNE provider: plain completion unexpectedly reported an assistant identity")
	}
	for _, check := range []struct {
		name, actual, expected string
	}{
		{"runtime", completion.SNE.RuntimeSHA256, p.expectation.RuntimeSHA256},
		{"native runtime", completion.SNE.NativeRuntimeSHA256, p.expectation.NativeRuntimeSHA256},
		{"manifest", completion.SNE.ModelManifestSHA256, p.expectation.ManifestSHA256},
	} {
		actual := strings.TrimSpace(check.actual)
		if actual != check.expected {
			return Response{}, fmt.Errorf("SNE provider: completion %s identity %q does not match admitted identity %q", check.name, actual, check.expected)
		}
	}
	return Response{
		Text:         completion.Choices[0].Message.Content,
		Tier:         TierLocal,
		Provider:     p.Name(),
		Model:        completion.Model,
		ToolsHonored: false,
		FinishReason: "stop",
		PromptTokens: completion.Usage.PromptTokens,
		OutputTokens: completion.Usage.CompletionTokens,
	}, nil
}

func (p *SNEProvider) admitReadiness(ctx context.Context) (sne.ServiceReadinessIdentity, error) {
	if ctx == nil {
		return sne.ServiceReadinessIdentity{}, fmt.Errorf("SNE provider: context is required")
	}
	if p == nil || p.client == nil {
		return sne.ServiceReadinessIdentity{}, fmt.Errorf("SNE provider: client is required")
	}
	identity, err := p.client.ReadinessIdentity(ctx)
	if err != nil {
		return sne.ServiceReadinessIdentity{}, fmt.Errorf("SNE provider: readiness: %w", err)
	}
	if err := identity.ValidateContract(); err != nil {
		return sne.ServiceReadinessIdentity{}, fmt.Errorf("SNE provider: %w", err)
	}
	if !identity.ReadyCapabilities.SupportsExecutionMode(p.expectation.ExecutionMode) {
		return sne.ServiceReadinessIdentity{}, fmt.Errorf("SNE provider: readiness does not advertise requested execution mode %q", p.expectation.ExecutionMode)
	}
	if p.expectation.ExecutionMode == sne.ExecutionModeMTP && !sameSNEAssistantIdentity(*identity.ReadyCapabilities.Assistant, *p.expectation.Assistant) {
		return sne.ServiceReadinessIdentity{}, fmt.Errorf("SNE provider: ready MTP assistant identity does not match admitted assistant")
	}
	servedModel := strings.TrimSpace(identity.ReadyModelID)
	loadedModel := strings.TrimSpace(identity.LoadedModel)
	if servedModel != p.expectation.ModelID || loadedModel != p.expectation.ModelID {
		return sne.ServiceReadinessIdentity{}, fmt.Errorf("SNE provider: ready/loaded model identities %q/%q do not match admitted model %q", servedModel, loadedModel, p.expectation.ModelID)
	}
	for _, field := range []struct {
		name, ready, status, expected string
	}{
		{"runtime", strings.TrimSpace(identity.ReadyRuntimeSHA256), strings.TrimSpace(identity.RuntimeSHA256), p.expectation.RuntimeSHA256},
		{"native runtime", strings.TrimSpace(identity.ReadyNativeRuntimeSHA256), strings.TrimSpace(identity.NativeRuntimeSHA256), p.expectation.NativeRuntimeSHA256},
	} {
		if field.ready != field.expected || field.status != field.expected {
			return sne.ServiceReadinessIdentity{}, fmt.Errorf("SNE provider: ready/status %s identities %q/%q do not match admitted identity %q", field.name, field.ready, field.status, field.expected)
		}
	}
	readyManifest := strings.TrimSpace(identity.ReadyManifestSHA256)
	matchingModels := 0
	modelManifest := ""
	for _, model := range identity.Models {
		if strings.TrimSpace(model.ID) != servedModel {
			continue
		}
		matchingModels++
		modelManifest = strings.TrimSpace(model.ManifestSHA256)
	}
	if matchingModels != 1 || readyManifest != p.expectation.ManifestSHA256 || modelManifest != p.expectation.ManifestSHA256 {
		return sne.ServiceReadinessIdentity{}, fmt.Errorf("SNE provider: ready/catalog manifest identities %q/%q for model %q do not match admitted manifest %q", readyManifest, modelManifest, servedModel, p.expectation.ManifestSHA256)
	}
	return identity, nil
}

func sameSNEAssistantIdentity(a, b sne.AssistantIdentity) bool {
	return a.ModelID == b.ModelID && a.Revision == b.Revision && a.CheckpointSHA256 == b.CheckpointSHA256 && a.Precision == b.Precision
}
