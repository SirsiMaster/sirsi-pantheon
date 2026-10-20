package reason

import (
	"context"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

type toolTurnProvider struct {
	requests  []provider.Request
	responses []provider.Response
}

func (p *toolTurnProvider) Name() string        { return "apollo-test" }
func (p *toolTurnProvider) Tier() provider.Tier { return provider.TierLocal }
func (p *toolTurnProvider) Caps() provider.Caps {
	return provider.Caps{Tools: true, RequiredToolChoice: true}
}
func (p *toolTurnProvider) Available(context.Context) bool { return true }
func (p *toolTurnProvider) Complete(_ context.Context, r provider.Request) (provider.Response, error) {
	p.requests = append(p.requests, r)
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func TestToolTurnExecutesOnlyExactAllowlistedToolAndReturnsFinalAnswer(t *testing.T) {
	r := NewRegistry()
	ran := 0
	if err := r.Register(Tool{Name: "machine.state", Does: "read current machine state", Tier: TierObserve, Run: func(context.Context) (Result, error) {
		ran++
		return Result{Summary: "healthy", Evidence: map[string]any{"state": "healthy"}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	p := &toolTurnProvider{responses: []provider.Response{
		{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: toolWireName("machine.state"), Args: map[string]any{}}}, ToolsHonored: true},
		{Text: "The machine is healthy."},
	}}
	out, err := (ToolTurn{Provider: p, Registry: r, Policy: PolicyReadOnly, Allowed: []string{"machine.state"}, System: "answer only from the tool result"}).Complete(context.Background(), "status?", 64)
	if err != nil {
		t.Fatal(err)
	}
	if !out.UsedTool || out.Invocation.Tool != "machine.state" || ran != 1 || out.Final.Text != "The machine is healthy." {
		t.Fatalf("unexpected result: %+v ran=%d", out, ran)
	}
	if len(p.requests) != 2 || len(p.requests[0].Tools) != 1 || len(p.requests[1].Messages) != 4 {
		t.Fatalf("expected one offered tool and a four-message follow-up, got %+v", p.requests)
	}
	if p.requests[0].Tools[0].Name != toolWireName("machine.state") {
		t.Fatalf("wire tool name = %q", p.requests[0].Tools[0].Name)
	}
	if p.requests[1].Messages[3].Role != "tool" || p.requests[1].Messages[3].ToolCallID != "call-1" {
		t.Fatalf("follow-up did not bind the tool result: %+v", p.requests[1].Messages[3])
	}
	if !p.requests[0].RequireTool {
		t.Fatal("initial bounded tool turn did not require its one offered tool")
	}
	if p.requests[1].Messages[2].ToolCalls[0].Name != toolWireName("machine.state") {
		t.Fatalf("follow-up assistant history rewrote wire alias: %+v", p.requests[1].Messages[2].ToolCalls)
	}
}

func TestToolTurnWireAliasesAreDistinctAndBoundToCanonicalNames(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"process.census", "process_census"} {
		if err := r.Register(Tool{Name: name, Does: "read", Tier: TierObserve, Run: func(context.Context) (Result, error) { return Result{}, nil }}); err != nil {
			t.Fatal(err)
		}
	}
	specs, names, err := (ToolTurn{Registry: r, Allowed: []string{"process.census", "process_census"}}).specs()
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 || specs[0].Name == specs[1].Name || names[specs[0].Name] != "process.census" || names[specs[1].Name] != "process_census" {
		t.Fatalf("wire aliases do not uniquely bind canonical names: specs=%+v map=%+v", specs, names)
	}
}

func TestToolTurnRejectsUnknownWireAliasWithoutInvocation(t *testing.T) {
	r := NewRegistry()
	ran := 0
	if err := r.Register(Tool{Name: "process.census", Does: "read", Tier: TierObserve, Run: func(context.Context) (Result, error) { ran++; return Result{}, nil }}); err != nil {
		t.Fatal(err)
	}
	p := &toolTurnProvider{responses: []provider.Response{{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "pantheon_666f72676564", Args: map[string]any{}}}, ToolsHonored: true}}}
	if _, err := (ToolTurn{Provider: p, Registry: r, Policy: PolicyReadOnly, Allowed: []string{"process.census"}}).Complete(context.Background(), "status?", 64); err == nil {
		t.Fatal("expected unknown wire alias to be refused")
	}
	if ran != 0 || len(p.requests) != 1 {
		t.Fatalf("unknown wire alias invoked or followed up: ran=%d requests=%d", ran, len(p.requests))
	}
}

func TestToolTurnRejectsMultipleWireCallsBeforeInvocation(t *testing.T) {
	r := NewRegistry()
	ran := 0
	if err := r.Register(Tool{Name: "process.census", Does: "read", Tier: TierObserve, Run: func(context.Context) (Result, error) { ran++; return Result{}, nil }}); err != nil {
		t.Fatal(err)
	}
	wire := toolWireName("process.census")
	p := &toolTurnProvider{responses: []provider.Response{{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: wire, Args: map[string]any{}}, {ID: "call-2", Name: wire, Args: map[string]any{}}}, ToolsHonored: true}}}
	if _, err := (ToolTurn{Provider: p, Registry: r, Policy: PolicyReadOnly, Allowed: []string{"process.census"}}).Complete(context.Background(), "status?", 64); err == nil {
		t.Fatal("expected multiple wire calls to be refused")
	}
	if ran != 0 || len(p.requests) != 1 {
		t.Fatalf("multiple wire calls invoked or followed up: ran=%d requests=%d", ran, len(p.requests))
	}
}

func TestToolTurnRejectsUnallowlistedArgumentsAndDoesNotInvoke(t *testing.T) {
	r := NewRegistry()
	ran := 0
	if err := r.Register(Tool{Name: "machine.state", Does: "read", Tier: TierObserve, Run: func(context.Context) (Result, error) { ran++; return Result{}, nil }}); err != nil {
		t.Fatal(err)
	}
	p := &toolTurnProvider{responses: []provider.Response{{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: toolWireName("machine.state"), Args: map[string]any{"path": "/tmp"}}}, ToolsHonored: true}}}
	if _, err := (ToolTurn{Provider: p, Registry: r, Policy: PolicyReadOnly, Allowed: []string{"machine.state"}}).Complete(context.Background(), "status?", 64); err == nil {
		t.Fatal("expected arguments to be refused")
	}
	if ran != 0 || len(p.requests) != 1 {
		t.Fatalf("unexpected invocation ran=%d requests=%d", ran, len(p.requests))
	}
}

func TestToolTurnNeverFollowsUpAfterDeniedRepair(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Tool{Name: "repair", Does: "repair", Tier: TierRepair, Run: func(context.Context) (Result, error) { return Result{}, nil }, Verify: func(context.Context, Result) (Result, error) { return Result{}, nil }}); err != nil {
		t.Fatal(err)
	}
	p := &toolTurnProvider{responses: []provider.Response{{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: toolWireName("repair"), Args: map[string]any{}}}, ToolsHonored: true}}}
	if _, err := (ToolTurn{Provider: p, Registry: r, Policy: PolicyReadOnly, Allowed: []string{"repair"}}).Complete(context.Background(), "repair?", 64); err == nil {
		t.Fatal("expected repair to be refused by read-only policy")
	}
	if len(p.requests) != 1 {
		t.Fatalf("denied repair must not get a model follow-up: %d requests", len(p.requests))
	}
}
