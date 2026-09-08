package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

type enginePromptTestProvider struct{}

func (enginePromptTestProvider) Name() string                   { return "test-model" }
func (enginePromptTestProvider) Tier() provider.Tier            { return provider.TierLocal }
func (enginePromptTestProvider) Caps() provider.Caps            { return provider.Caps{} }
func (enginePromptTestProvider) Available(context.Context) bool { return true }
func (enginePromptTestProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Model: "test-model", Text: "unused"}, nil
}

func testEngineIdentity() engine.Identity {
	return engine.Identity{
		Engine: engine.KindMLX, EngineVersion: "test-1", ModelID: "test-model",
		ModelSHA256: strings.Repeat("a", 64), TokenizerID: "test-tokenizer",
		TokenizerSHA256: strings.Repeat("b", 64), Precision: "bf16", CacheNamespace: "test-cache",
	}
}

func testEngineController(t *testing.T) *engine.SelectionController {
	t.Helper()
	connector, err := engine.NewMLXConnector(enginePromptTestProvider{}, testEngineIdentity(), engine.Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := engine.NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := engine.NewSelectionController(router, engine.RoutePolicy{Preferred: engine.KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	return controller
}

func TestParseEnginePromptOptionsRejectsInvalidInput(t *testing.T) {
	for name, args := range map[string][]string{
		"missing prompt":      {"--max-tokens", "8"},
		"empty prompt":        {"--prompt", "   "},
		"invalid engine":      {"--prompt", "hello", "--engine", "cuda"},
		"zero max tokens":     {"--prompt", "hello", "--max-tokens", "0"},
		"negative max tokens": {"--prompt", "hello", "--max-tokens", "-1"},
		"positional argument": {"--prompt", "hello", "unexpected"},
		"unknown flag":        {"--prompt", "hello", "--unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseEnginePromptOptions(args); err == nil {
				t.Fatal("invalid prompt arguments were accepted")
			}
		})
	}
}

func TestParseEnginePromptOptionsDefaultsToPositiveLimit(t *testing.T) {
	opts, err := parseEnginePromptOptions([]string{"--prompt", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.MaxTokens <= 0 {
		t.Fatalf("default max tokens = %d, want positive", opts.MaxTokens)
	}
}

func TestExecuteEnginePromptProjectsCompletionAndRouteReceipt(t *testing.T) {
	controller := testEngineController(t)
	opts := enginePromptOptions{Engine: "mlx", Prompt: "hello", MaxTokens: 8}
	var out bytes.Buffer
	err := executeEnginePrompt(context.Background(), opts, &out,
		func() (*engine.SelectionController, error) { return controller, nil },
		func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
			return engine.Completion{Text: "answer", Model: "test-model", FinishReason: "stop"}, engine.Receipt{
				ABIVersion: engine.ABIVersion, SessionID: "session-1", Identity: testEngineIdentity(),
				IdentityDigest: "identity-digest", RequestSHA256: strings.Repeat("c", 64), CompletionSHA256: strings.Repeat("d", 64),
				StartedAt: "2026-09-08T12:00:00Z", FinishedAt: "2026-09-08T12:00:01Z",
				Route: &engine.RouteDecision{Requested: engine.KindMLX, Selected: engine.KindMLX, Rationale: "test route"},
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var got enginePromptOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Completion.Text != "answer" || got.Receipt.Route == nil || got.Receipt.Route.Selected != engine.KindMLX {
		t.Fatalf("unexpected prompt output: %+v", got)
	}
}
