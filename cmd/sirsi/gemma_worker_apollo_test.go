package main

import (
	"context"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
	"github.com/SirsiMaster/sirsi-pantheon/internal/reason"
)

type apolloWorkerFake struct {
	responses []provider.Response
	requests  []provider.Request
}

func (p *apolloWorkerFake) Name() string                   { return "apollo-worker-fake" }
func (p *apolloWorkerFake) Tier() provider.Tier            { return provider.TierLocal }
func (p *apolloWorkerFake) Caps() provider.Caps            { return provider.Caps{Tools: true} }
func (p *apolloWorkerFake) Available(context.Context) bool { return true }
func (p *apolloWorkerFake) Complete(_ context.Context, r provider.Request) (provider.Response, error) {
	p.requests = append(p.requests, r)
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func TestGemmaWorkerApolloCompleteReturnsCorrelatedObserveEvidence(t *testing.T) {
	registry := reason.NewRegistry()
	if err := registry.Register(reason.Tool{Name: "machine.state", Does: "read state", Tier: reason.TierObserve, Run: func(context.Context) (reason.Result, error) {
		return reason.Result{Summary: "healthy", Evidence: map[string]any{"state": "healthy"}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	backend := &apolloWorkerFake{responses: []provider.Response{
		{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "machine.state", Args: map[string]any{}}}, ToolsHonored: true},
		{Text: "The machine is healthy."},
	}}
	answer, err := gemmaWorkerApolloComplete(context.Background(), backend, registry, []string{"machine.state"}, "report status", 64)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "The machine is healthy.\n\nREAD-ONLY TOOL EVIDENCE:\n{\"tool\":\"machine.state\",\"summary\":\"healthy\",\"evidence\":{\"state\":\"healthy\"}}" {
		t.Fatalf("answer = %q", answer)
	}
	if len(backend.requests) != 2 || len(backend.requests[1].Messages) != 4 {
		t.Fatalf("expected a bounded correlated follow-up, got %+v", backend.requests)
	}
}

func TestGemmaWorkerObserveAllowlistRejectsRepairBeforeProviderCall(t *testing.T) {
	registry := reason.NewRegistry()
	if err := registry.Register(reason.Tool{Name: "repair", Does: "repair", Tier: reason.TierRepair, Run: func(context.Context) (reason.Result, error) { return reason.Result{}, nil }, Verify: func(context.Context, reason.Result) (reason.Result, error) { return reason.Result{}, nil }}); err != nil {
		t.Fatal(err)
	}
	if err := gemmaWorkerObserveAllowlist(registry, []string{"repair"}); err == nil {
		t.Fatal("repair tool must be rejected before it reaches a provider")
	}
}
