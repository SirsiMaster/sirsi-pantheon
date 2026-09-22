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

func TestDashboardEngineVariantSelectionAndDefaults(t *testing.T) {
	setDashboardEngineIdentity(t, "SIRSI_MLX", "mlx")
	setDashboardEngineIdentity(t, "SIRSI_SNE", "sne")
	t.Setenv("SIRSI_MLX_VARIANT", "mlx-patched")
	t.Setenv("SIRSI_SNE_VARIANT", "sne-mtp")
	t.Setenv("SIRSI_ENGINE_PREFERRED", "sne")
	t.Setenv("SIRSI_ENGINE_PREFERRED_VARIANT", "sne-mtp")
	controller, err := buildDashboardEngineSelection()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := controller.Snapshot()
	if snapshot.PreferredVariant != engine.VariantSNEMTP || snapshot.Connectors[0].Variant != engine.VariantMLXPatched || snapshot.Connectors[1].Variant != engine.VariantSNEMTP {
		t.Fatalf("dashboard variants = %+v", snapshot)
	}

	t.Setenv("SIRSI_MLX_VARIANT", "")
	t.Setenv("SIRSI_SNE_VARIANT", "")
	t.Setenv("SIRSI_ENGINE_PREFERRED_VARIANT", "bad")
	if _, err := buildDashboardEngineSelection(); err == nil {
		t.Fatal("malformed preferred dashboard variant accepted")
	}
}

func TestBuildDashboardEngineSelectionLoadsRawAndPatchedMLXSideBySide(t *testing.T) {
	setDashboardEngineIdentity(t, "SIRSI_MLX_RAW", "mlx")
	setDashboardEngineIdentity(t, "SIRSI_MLX_PATCHED", "mlx")
	t.Setenv("SIRSI_ENGINE_PREFERRED", "mlx")
	t.Setenv("SIRSI_ENGINE_PREFERRED_VARIANT", "mlx-patched")
	controller, err := buildDashboardEngineSelection()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := controller.Snapshot()
	if len(snapshot.Connectors) != 2 || snapshot.Connectors[0].Variant != engine.VariantMLXRaw || snapshot.Connectors[1].Variant != engine.VariantMLXPatched {
		t.Fatalf("co-configured MLX variants = %+v", snapshot.Connectors)
	}
	if snapshot.Preferred != engine.KindMLX || snapshot.PreferredVariant != engine.VariantMLXPatched {
		t.Fatalf("preferred route = %s/%s", snapshot.Preferred, snapshot.PreferredVariant)
	}
}

func TestEnginePromptCanBootstrapExplicitNonDefaultOnlyConnector(t *testing.T) {
	setDashboardEngineIdentity(t, "SIRSI_SNE_MTP", "sne")
	t.Setenv("SIRSI_ENGINE_PREFERRED", "")
	t.Setenv("SIRSI_ENGINE_PREFERRED_VARIANT", "")
	t.Setenv("SIRSI_ENGINE_ALLOW_FALLBACK", "")
	if _, err := buildDashboardEngineSelection(); err == nil {
		t.Fatal("dashboard inferred SNE MTP without an explicit preferred variant")
	}
	controller, err := buildDashboardEngineSelectionForPrompt(enginePromptOptions{Engine: "sne", Variant: "sne-mtp"})
	if err != nil {
		t.Fatalf("explicit CLI engine+variant could not build its controller: %v", err)
	}
	if policy := controller.Policy(); policy.Preferred != engine.KindSNE || policy.PreferredVariant != engine.VariantSNEMTP {
		t.Fatalf("CLI initial policy = %+v", policy)
	}
	controller, err = buildDashboardEngineSelectionForPrompt(enginePromptOptions{Variant: "sne-mtp"})
	if err != nil {
		t.Fatalf("variant-only CLI route could not infer its ABI kind: %v", err)
	}
	if policy := controller.Policy(); policy.Preferred != engine.KindSNE || policy.PreferredVariant != engine.VariantSNEMTP {
		t.Fatalf("variant-only CLI policy = %+v", policy)
	}
}

func TestDashboardRejectsIncompatibleConfiguredVariant(t *testing.T) {
	setDashboardEngineIdentity(t, "SIRSI_MLX", "mlx")
	t.Setenv("SIRSI_MLX_VARIANT", "sne-plain")
	if _, err := buildDashboardEngineSelection(); err == nil {
		t.Fatal("incompatible dashboard variant accepted")
	}
}
