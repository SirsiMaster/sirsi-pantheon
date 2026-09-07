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
	for name, value := range map[string]string{
		"runtime":        expectation.RuntimeSHA256,
		"native runtime": expectation.NativeRuntimeSHA256,
		"manifest":       expectation.ManifestSHA256,
	} {
		if !validSNEHash(value) {
			return nil, fmt.Errorf("SNE provider: %s identity must be lowercase SHA-256", name)
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
	return Caps{Streaming: false, Offline: true}
}

func (p *SNEProvider) Available(ctx context.Context) bool {
	return p.readiness(ctx) == nil
}

func (p *SNEProvider) Complete(ctx context.Context, req Request) (Response, error) {
	if p == nil || p.client == nil {
		return Response{}, fmt.Errorf("SNE provider: client is required")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return Response{}, fmt.Errorf("SNE provider: prompt is required")
	}
	if err := p.readiness(ctx); err != nil {
		return Response{}, err
	}
	messages := make([]sne.Message, 0, 2)
	if req.System != "" {
		messages = append(messages, sne.Message{Role: "system", Content: req.System})
	}
	messages = append(messages, sne.Message{Role: "user", Content: req.Prompt})
	completion, err := p.client.Complete(ctx, sne.CompletionRequest{
		Model:       p.expectation.ModelID,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: 0,
		Stream:      false,
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

func (p *SNEProvider) readiness(ctx context.Context) error {
	if p == nil || p.client == nil {
		return fmt.Errorf("SNE provider: client is required")
	}
	identity, err := p.client.ReadinessIdentity(ctx)
	if err != nil {
		return fmt.Errorf("SNE provider: readiness: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(identity.Status), "ready") {
		return fmt.Errorf("SNE provider: service status %q is not ready", identity.Status)
	}
	servedModel := strings.TrimSpace(identity.ReadyModelID)
	if servedModel == "" {
		servedModel = strings.TrimSpace(identity.LoadedModel)
	}
	if servedModel != p.expectation.ModelID {
		return fmt.Errorf("SNE provider: ready model %q does not match admitted model %q", servedModel, p.expectation.ModelID)
	}
	runtimeSHA256 := strings.TrimSpace(identity.ReadyRuntimeSHA256)
	if runtimeSHA256 == "" {
		runtimeSHA256 = strings.TrimSpace(identity.RuntimeSHA256)
	}
	if runtimeSHA256 != p.expectation.RuntimeSHA256 {
		return fmt.Errorf("SNE provider: runtime identity %q does not match admitted runtime %q", runtimeSHA256, p.expectation.RuntimeSHA256)
	}
	nativeRuntimeSHA256 := strings.TrimSpace(identity.ReadyNativeRuntimeSHA256)
	if nativeRuntimeSHA256 == "" {
		nativeRuntimeSHA256 = strings.TrimSpace(identity.NativeRuntimeSHA256)
	}
	if nativeRuntimeSHA256 != p.expectation.NativeRuntimeSHA256 {
		return fmt.Errorf("SNE provider: native runtime identity %q does not match admitted native runtime %q", nativeRuntimeSHA256, p.expectation.NativeRuntimeSHA256)
	}
	manifest := strings.TrimSpace(identity.ReadyManifestSHA256)
	if manifest == "" {
		for _, model := range identity.Models {
			if strings.TrimSpace(model.ID) == servedModel {
				manifest = strings.TrimSpace(model.ManifestSHA256)
				break
			}
		}
	}
	if manifest != p.expectation.ManifestSHA256 {
		return fmt.Errorf("SNE provider: manifest identity %q does not match admitted manifest %q", manifest, p.expectation.ManifestSHA256)
	}
	return nil
}
