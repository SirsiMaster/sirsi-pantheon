package modelrouter

import (
	"context"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

// stubProvider is a minimal provider.Provider for routing tests.
type stubProvider struct {
	name          string
	tier          provider.Tier
	ctx           int
	avail         bool
	availableCall int
	completeCall  int
	onAvailable   func()
}

func (s *stubProvider) Name() string        { return s.name }
func (s *stubProvider) Tier() provider.Tier { return s.tier }
func (s *stubProvider) Caps() provider.Caps {
	return provider.Caps{ContextTokens: s.ctx, Offline: s.tier == provider.TierLocal}
}
func (s *stubProvider) Available(_ context.Context) bool {
	s.availableCall++
	if s.onAvailable != nil {
		s.onAvailable()
	}
	return s.avail
}
func (s *stubProvider) Complete(_ context.Context, req provider.Request) (provider.Response, error) {
	s.completeCall++
	return provider.Response{Text: "stub:" + s.name, Tier: s.tier, Provider: s.name}, nil
}

func localStub(name string, ctx int, avail bool) provider.Provider {
	return &stubProvider{name: name, tier: provider.TierLocal, ctx: ctx, avail: avail}
}

func remoteStub(name string, avail bool) provider.Provider {
	return &stubProvider{name: name, tier: provider.TierRemote, ctx: 128000, avail: avail}
}

func remoteStubWithContext(name string, contextTokens int, avail bool) *stubProvider {
	return &stubProvider{name: name, tier: provider.TierRemote, ctx: contextTokens, avail: avail}
}

func TestRoute_LocalOnly_UsesLocal(t *testing.T) {
	rt := New([]provider.Provider{localStub("local", 8192, true), remoteStub("remote", true)})
	d, err := rt.Route(context.Background(), Request{Privacy: PrivacyLocalOnly})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Lane != LaneLocal {
		t.Fatalf("got lane=%s, want local", d.Lane)
	}
}

func TestRoute_LocalOnly_FailsWhenLocalDown(t *testing.T) {
	rt := New([]provider.Provider{localStub("local", 8192, false), remoteStub("remote", true)})
	_, err := rt.Route(context.Background(), Request{Privacy: PrivacyLocalOnly})
	if err == nil {
		t.Fatal("expected error when local-only and local is down")
	}
}

func TestRoute_Judgment_PrefersRemote(t *testing.T) {
	rt := New([]provider.Provider{localStub("local", 8192, true), remoteStub("remote", true)})
	d, err := rt.Route(context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Lane != LaneRemote {
		t.Fatalf("got lane=%s, want remote for judgment+shareable", d.Lane)
	}
}

func TestRoute_Judgment_FallsBackToLocalWhenRemoteDown(t *testing.T) {
	rt := New([]provider.Provider{localStub("local", 8192, true), remoteStub("remote", false)})
	d, err := rt.Route(context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Lane != LaneLocal {
		t.Fatalf("got lane=%s, want local fallback", d.Lane)
	}
	if !d.Escalated {
		t.Fatal("expected Escalated=true on sovereignty fallback")
	}
}

func TestRoute_Generation_PrefersLocal(t *testing.T) {
	rt := New([]provider.Provider{localStub("local", 8192, true), remoteStub("remote", true)})
	d, err := rt.Route(context.Background(), Request{Task: TaskGeneration, Privacy: PrivacyShareable})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Lane != LaneLocal {
		t.Fatalf("got lane=%s, want local for generation", d.Lane)
	}
}

func TestRoute_ContextGap_EscalatesToRemote(t *testing.T) {
	rt := New([]provider.Provider{localStub("local", 4096, true), remoteStub("remote", true)})
	d, err := rt.Route(context.Background(), Request{
		Task: TaskGeneration, Privacy: PrivacyShareable, MinContextTokens: 8192,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Lane != LaneRemote {
		t.Fatalf("got lane=%s, want remote when local context too small and shareable", d.Lane)
	}
}

func TestRoute_ContextGap_LocalOnly_Fails(t *testing.T) {
	rt := New([]provider.Provider{localStub("local", 4096, true)})
	_, err := rt.Route(context.Background(), Request{
		Task: TaskGeneration, Privacy: PrivacyLocalOnly, MinContextTokens: 8192,
	})
	if err == nil {
		t.Fatal("expected error: local-only with context gap should fail-closed")
	}
}

func TestRoute_DefaultsLocalOnly(t *testing.T) {
	rt := New([]provider.Provider{localStub("local", 8192, true)})
	d, err := rt.Route(context.Background(), Request{})
	if err != nil {
		t.Fatalf("default request should route to local: %v", err)
	}
	if d.Lane != LaneLocal {
		t.Fatalf("default privacy should be local-only, got lane=%s", d.Lane)
	}
}

func TestRoute_SelectsFirstAvailableQualifiedProvider(t *testing.T) {
	tests := []struct {
		name           string
		providers      []*stubProvider
		request        Request
		wantLane       Lane
		wantProvider   string
		wantEscalated  bool
		wantError      bool
		wantProbeCalls []int
	}{
		{
			name: "second local provider meets context requirement",
			providers: []*stubProvider{
				{name: "local-short", tier: provider.TierLocal, ctx: 4096, avail: true},
				{name: "local-qualified", tier: provider.TierLocal, ctx: 8192, avail: true},
				{name: "remote", tier: provider.TierRemote, ctx: 16384, avail: true},
			},
			request:        Request{Task: TaskGeneration, Privacy: PrivacyShareable, MinContextTokens: 8192},
			wantLane:       LaneLocal,
			wantProvider:   "local-qualified",
			wantProbeCalls: []int{1, 1, 0},
		},
		{
			name: "judgment skips short remote for qualified remote",
			providers: []*stubProvider{
				{name: "remote-short", tier: provider.TierRemote, ctx: 4096, avail: true},
				{name: "remote-qualified", tier: provider.TierRemote, ctx: 8192, avail: true},
				{name: "local", tier: provider.TierLocal, ctx: 8192, avail: true},
			},
			request:        Request{Task: TaskJudgment, Privacy: PrivacyShareable, MinContextTokens: 8192},
			wantLane:       LaneRemote,
			wantProvider:   "remote-qualified",
			wantProbeCalls: []int{1, 1, 0},
		},
		{
			name: "judgment falls back to qualified local when remote is short",
			providers: []*stubProvider{
				{name: "remote-short", tier: provider.TierRemote, ctx: 4096, avail: true},
				{name: "local-qualified", tier: provider.TierLocal, ctx: 8192, avail: true},
			},
			request:        Request{Task: TaskJudgment, Privacy: PrivacyShareable, MinContextTokens: 8192},
			wantLane:       LaneLocal,
			wantProvider:   "local-qualified",
			wantEscalated:  true,
			wantProbeCalls: []int{1, 1},
		},
		{
			name: "generation escalates to qualified remote when local is short",
			providers: []*stubProvider{
				{name: "local-short", tier: provider.TierLocal, ctx: 4096, avail: true},
				{name: "remote-qualified", tier: provider.TierRemote, ctx: 8192, avail: true},
			},
			request:        Request{Task: TaskGeneration, Privacy: PrivacyShareable, MinContextTokens: 8192},
			wantLane:       LaneRemote,
			wantProvider:   "remote-qualified",
			wantEscalated:  true,
			wantProbeCalls: []int{1, 1},
		},
		{
			name: "all short local and remote providers reject positive context requirement",
			providers: []*stubProvider{
				{name: "local-short", tier: provider.TierLocal, ctx: 4096, avail: true},
				{name: "remote-short", tier: provider.TierRemote, ctx: 4096, avail: true},
			},
			request:        Request{Task: TaskGeneration, Privacy: PrivacyShareable, MinContextTokens: 8192},
			wantError:      true,
			wantProbeCalls: []int{1, 1},
		},
		{
			name: "judgment rejects when both remote and local context are short",
			providers: []*stubProvider{
				{name: "remote-short", tier: provider.TierRemote, ctx: 4096, avail: true},
				{name: "local-short", tier: provider.TierLocal, ctx: 4096, avail: true},
			},
			request:        Request{Task: TaskJudgment, Privacy: PrivacyShareable, MinContextTokens: 8192},
			wantError:      true,
			wantProbeCalls: []int{1, 1},
		},
		{
			name: "local-only rejects short local even when remote qualifies",
			providers: []*stubProvider{
				{name: "local-short", tier: provider.TierLocal, ctx: 4096, avail: true},
				{name: "remote-qualified", tier: provider.TierRemote, ctx: 16384, avail: true},
			},
			request:        Request{Privacy: PrivacyLocalOnly, MinContextTokens: 8192},
			wantError:      true,
			wantProbeCalls: []int{1, 0},
		},
		{
			name: "unknown zero context is not qualified for positive requirement",
			providers: []*stubProvider{
				{name: "local-unknown", tier: provider.TierLocal, ctx: 0, avail: true},
				{name: "remote-unknown", tier: provider.TierRemote, ctx: 0, avail: true},
			},
			request:        Request{Task: TaskGeneration, Privacy: PrivacyShareable, MinContextTokens: 1},
			wantError:      true,
			wantProbeCalls: []int{1, 1},
		},
		{
			name: "exactly satisfied context requirement routes locally",
			providers: []*stubProvider{
				{name: "local-qualified", tier: provider.TierLocal, ctx: 8192, avail: true},
				{name: "remote", tier: provider.TierRemote, ctx: 16384, avail: true},
			},
			request:        Request{Task: TaskGeneration, Privacy: PrivacyShareable, MinContextTokens: 8192},
			wantLane:       LaneLocal,
			wantProvider:   "local-qualified",
			wantProbeCalls: []int{1, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providers := make([]provider.Provider, len(tt.providers))
			for i, p := range tt.providers {
				providers[i] = p
			}
			response, decision, err := New(providers).Complete(context.Background(), tt.request, provider.Request{})
			if tt.wantError {
				if err == nil {
					t.Fatal("expected routing error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected routing error: %v", err)
				}
				if decision.Lane != tt.wantLane || decision.Provider.Name() != tt.wantProvider {
					t.Fatalf("got %s/%s, want %s/%s", decision.Lane, decision.Provider.Name(), tt.wantLane, tt.wantProvider)
				}
				if decision.Escalated != tt.wantEscalated {
					t.Fatalf("got Escalated=%t, want %t", decision.Escalated, tt.wantEscalated)
				}
				if response.Provider != tt.wantProvider {
					t.Fatalf("completed provider=%q, want %q", response.Provider, tt.wantProvider)
				}
			}
			for i, p := range tt.providers {
				if p.availableCall != tt.wantProbeCalls[i] {
					t.Errorf("provider %q availability probes=%d, want %d", p.name, p.availableCall, tt.wantProbeCalls[i])
				}
				wantCompleteCalls := 0
				if !tt.wantError && p.name == tt.wantProvider {
					wantCompleteCalls = 1
				}
				if p.completeCall != wantCompleteCalls {
					t.Errorf("provider %q completions=%d, want %d", p.name, p.completeCall, wantCompleteCalls)
				}
			}
		})
	}
}

func TestRoute_InvalidRequestFailsBeforeProbing(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Request)
	}{
		{name: "unknown task", edit: func(r *Request) { r.Task = TaskClass("review") }},
		{name: "unknown privacy", edit: func(r *Request) { r.Privacy = PrivacyClass("remote") }},
		{name: "unknown latency", edit: func(r *Request) { r.Latency = LatencyClass("urgent") }},
		{name: "negative context", edit: func(r *Request) { r.MinContextTokens = -1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := Request{Task: TaskGeneration, Privacy: PrivacyShareable}
			tt.edit(&request)
			local := &stubProvider{name: "local", tier: provider.TierLocal, ctx: 8192, avail: true}
			remote := remoteStubWithContext("remote", 16384, true)
			_, _, err := New([]provider.Provider{local, remote}).Complete(context.Background(), request, provider.Request{})
			if err == nil {
				t.Fatal("expected malformed request to fail closed")
			}
			if local.availableCall != 0 || remote.availableCall != 0 {
				t.Fatalf("malformed request probed providers: local=%d remote=%d", local.availableCall, remote.availableCall)
			}
			if local.completeCall != 0 || remote.completeCall != 0 {
				t.Fatalf("malformed request completed a provider: local=%d remote=%d", local.completeCall, remote.completeCall)
			}
		})
	}
}

func TestRoute_CanceledContextDoesNotProbeOrRoute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	local := &stubProvider{name: "local", tier: provider.TierLocal, ctx: 8192, avail: true}
	remote := remoteStubWithContext("remote", 16384, true)

	_, _, err := New([]provider.Provider{local, remote}).Complete(ctx, Request{
		Task: TaskGeneration, Privacy: PrivacyShareable,
	}, provider.Request{})
	if err == nil {
		t.Fatal("expected canceled request to fail without routing")
	}
	if local.availableCall != 0 || remote.availableCall != 0 {
		t.Fatalf("canceled request probed providers: local=%d remote=%d", local.availableCall, remote.availableCall)
	}
	if local.completeCall != 0 || remote.completeCall != 0 {
		t.Fatalf("canceled request completed a provider: local=%d remote=%d", local.completeCall, remote.completeCall)
	}
}

func TestRoute_CancellationDuringProbeDoesNotRoute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	local := &stubProvider{
		name: "local", tier: provider.TierLocal, ctx: 8192, avail: true,
		onAvailable: cancel,
	}
	remote := remoteStubWithContext("remote", 16384, true)

	_, _, err := New([]provider.Provider{local, remote}).Complete(ctx, Request{
		Task: TaskGeneration, Privacy: PrivacyShareable,
	}, provider.Request{})
	if err == nil {
		t.Fatal("expected cancellation during provider probe to fail without routing")
	}
	if local.availableCall != 1 || remote.availableCall != 0 {
		t.Fatalf("unexpected availability probes: local=%d remote=%d", local.availableCall, remote.availableCall)
	}
	if local.completeCall != 0 || remote.completeCall != 0 {
		t.Fatalf("canceled request completed a provider: local=%d remote=%d", local.completeCall, remote.completeCall)
	}
}
