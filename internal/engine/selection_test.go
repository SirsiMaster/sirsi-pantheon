package engine

import (
	"context"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

type selectionProvider struct {
	name  string
	caps  provider.Caps
	ready bool
}

func (p selectionProvider) Name() string                   { return p.name }
func (p selectionProvider) Tier() provider.Tier            { return provider.TierLocal }
func (p selectionProvider) Caps() provider.Caps            { return p.caps }
func (p selectionProvider) Available(context.Context) bool { return p.ready }
func (p selectionProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Model: p.name}, nil
}

func selectionIdentity(kind Kind) Identity {
	return Identity{Engine: kind, EngineVersion: "1", ModelID: "model", ModelSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", TokenizerID: "tokenizer", TokenizerSHA256: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", Precision: "bf16", CacheNamespace: "cache"}
}

func TestSelectionControllerPublishesConfiguredEnginePolicy(t *testing.T) {
	mlx, err := NewMLXConnector(selectionProvider{name: "mlx", ready: true}, selectionIdentity(KindMLX), Capabilities{Sessions: true, Streaming: true})
	if err != nil {
		t.Fatal(err)
	}
	sne, err := NewSNEConnector(selectionProvider{name: "sne", ready: true}, selectionIdentity(KindSNE), Capabilities{Sessions: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(mlx, sne)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{Preferred: KindMLX, AllowFallback: false, RequiredCapabilities: []Capability{CapabilitySessions}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := controller.Snapshot()
	if snapshot.Schema != SelectionSchema || snapshot.Preferred != KindMLX || len(snapshot.Connectors) != 2 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if _, err := controller.Select(RoutePolicy{Preferred: KindSNE, AllowFallback: false, RequiredCapabilities: []Capability{CapabilitySessions}}); err != nil {
		t.Fatal(err)
	}
	if got := controller.Policy().Preferred; got != KindSNE {
		t.Fatalf("preferred engine = %q, want sne", got)
	}
}

func TestSelectionControllerRejectsUnsupportedPolicy(t *testing.T) {
	mlx, err := NewMLXConnector(selectionProvider{name: "mlx", ready: true}, selectionIdentity(KindMLX), Capabilities{Sessions: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(mlx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSelectionController(router, RoutePolicy{Preferred: KindSNE}); err == nil {
		t.Fatal("expected unconfigured preferred connector rejection")
	}
	controller, err := NewSelectionController(router, RoutePolicy{Preferred: KindMLX, RequiredCapabilities: []Capability{CapabilityStreaming}})
	if err == nil || controller != nil {
		t.Fatal("expected unsupported preferred capability rejection")
	}
	if _, err := NewSelectionController(router, RoutePolicy{Preferred: KindMLX, AllowFallback: true, RequiredCapabilities: []Capability{CapabilityStreaming}}); err == nil {
		t.Fatal("expected no-capable-connector rejection")
	}
}
