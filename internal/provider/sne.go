package provider

import (
	"context"
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

// SNEProvider adapts the native SNE client to the engine-neutral provider
// ladder. It is deliberately buffered: SNE's native client does not expose a
// streaming method, so the adapter never claims streaming capability.
type SNEProvider struct {
	client  SNEClient
	modelID string
}

// NewSNEProvider creates a local SNE provider bound to one admitted model.
// Readiness and completion both fail closed when the service reports a
// different served model; a configured model string is never trusted alone.
func NewSNEProvider(client SNEClient, modelID string) (*SNEProvider, error) {
	if client == nil {
		return nil, fmt.Errorf("SNE provider: client is required")
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, fmt.Errorf("SNE provider: model is required")
	}
	return &SNEProvider{client: client, modelID: modelID}, nil
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
		Model:       p.modelID,
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
	if completion.Model != p.modelID {
		return Response{}, fmt.Errorf("SNE provider: served model %q does not match admitted model %q", completion.Model, p.modelID)
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
	if servedModel != p.modelID {
		return fmt.Errorf("SNE provider: ready model %q does not match admitted model %q", servedModel, p.modelID)
	}
	return nil
}
