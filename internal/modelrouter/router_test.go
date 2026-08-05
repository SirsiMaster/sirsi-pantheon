package modelrouter

import (
	"context"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

// stubProvider is a minimal provider.Provider for routing tests.
type stubProvider struct {
	name  string
	tier  provider.Tier
	ctx   int
	avail bool
}

func (s *stubProvider) Name() string        { return s.name }
func (s *stubProvider) Tier() provider.Tier { return s.tier }
func (s *stubProvider) Caps() provider.Caps {
	return provider.Caps{ContextTokens: s.ctx, Offline: s.tier == provider.TierLocal}
}
func (s *stubProvider) Available(_ context.Context) bool { return s.avail }
func (s *stubProvider) Complete(_ context.Context, req provider.Request) (provider.Response, error) {
	return provider.Response{Text: "stub:" + s.name, Tier: s.tier, Provider: s.name}, nil
}

func localStub(name string, ctx int, avail bool) provider.Provider {
	return &stubProvider{name: name, tier: provider.TierLocal, ctx: ctx, avail: avail}
}

func remoteStub(name string, avail bool) provider.Provider {
	return &stubProvider{name: name, tier: provider.TierRemote, ctx: 128000, avail: avail}
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
