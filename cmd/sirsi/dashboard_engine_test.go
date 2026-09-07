package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
)

func setDashboardEngineIdentity(t *testing.T, prefix, kind string) {
	t.Helper()
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	t.Setenv(prefix+"_ENDPOINT", "http://127.0.0.1:8477/v1")
	t.Setenv(prefix+"_MODEL", "model-"+kind)
	t.Setenv(prefix+"_ENGINE_VERSION", "engine-1")
	t.Setenv(prefix+"_MODEL_SHA256", sha)
	t.Setenv(prefix+"_TOKENIZER_ID", "tokenizer-1")
	t.Setenv(prefix+"_TOKENIZER_SHA256", sha)
	t.Setenv(prefix+"_PRECISION", "bf16")
	t.Setenv(prefix+"_CACHE_NAMESPACE", "pantheon-test")
	if kind == "sne" {
		t.Setenv(prefix+"_RUNTIME_SHA256", sha)
		t.Setenv(prefix+"_NATIVE_RUNTIME_SHA256", sha)
		t.Setenv(prefix+"_MANIFEST_SHA256", sha)
	}
}

func TestBuildDashboardEngineSelectionRequiresExplicitIdentity(t *testing.T) {
	t.Setenv("SIRSI_MLX_ENDPOINT", "http://127.0.0.1:8477/v1")
	if _, err := buildDashboardEngineSelection(); err == nil {
		t.Fatal("expected configured endpoint without identity to fail closed")
	}
}

func TestBuildDashboardEngineSelectionBindsConfiguredConnectors(t *testing.T) {
	setDashboardEngineIdentity(t, "SIRSI_MLX", "mlx")
	setDashboardEngineIdentity(t, "SIRSI_SNE", "sne")
	t.Setenv("SIRSI_ENGINE_PREFERRED", "sne")
	t.Setenv("SIRSI_ENGINE_ALLOW_FALLBACK", "true")
	controller, err := buildDashboardEngineSelection()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := controller.Snapshot()
	if snapshot.Preferred != engine.KindSNE || !snapshot.AllowFallback || len(snapshot.Connectors) != 2 {
		t.Fatalf("unexpected configured selection: %+v", snapshot)
	}
	if snapshot.Connectors[0].Kind != engine.KindMLX || snapshot.Connectors[1].Kind != engine.KindSNE {
		t.Fatalf("connectors are not deterministic: %+v", snapshot.Connectors)
	}
}
