package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

func TestDashboardEngineCapabilitiesPreserveProviderDeclarations(t *testing.T) {
	got := dashboardEngineCapabilities(provider.Caps{
		Tools: true, Temperature: true, TopP: true, Seed: true, Streaming: true,
		Cancellation: true, Prefill: true, Decode: true, MTP: true, KVState: true, Telemetry: true,
	})
	want := engine.Capabilities{
		Sessions: true, Receipts: true, Tools: true, Temperature: true, TopP: true, Seed: true,
		Streaming: true, Cancellation: true, Prefill: true, Decode: true, MTP: true, KVState: true, Telemetry: true,
	}
	if got != want {
		t.Fatalf("dashboard capabilities = %+v, want %+v", got, want)
	}
}

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

func TestDashboardConnectorIdentityReportsMissingFieldsInStableOrder(t *testing.T) {
	const prefix = "SIRSI_MLX_RAW"
	t.Setenv(prefix+"_ENDPOINT", "http://127.0.0.1:8000")
	for _, suffix := range []string{"MODEL", "ENGINE_VERSION", "MODEL_SHA256", "TOKENIZER_ID", "TOKENIZER_SHA256", "PRECISION", "CACHE_NAMESPACE"} {
		t.Setenv(prefix+"_"+suffix, "")
	}
	_, _, err := buildDashboardConnectorVariant(engine.KindMLX, prefix, engine.VariantMLXRaw)
	if err == nil || !strings.Contains(err.Error(), "model identity is missing") {
		t.Fatalf("multiple missing identity fields should report model first, got %v", err)
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

func TestDashboardSelectionReportsRemoteBoundaryWithoutExposingEndpoint(t *testing.T) {
	setDashboardEngineIdentity(t, "SIRSI_OMLX", "omlx")
	t.Setenv("SIRSI_OMLX_ENDPOINT", "https://gateway.example.test/v1?token=must-not-appear")
	t.Setenv("SIRSI_ENGINE_PREFERRED", "omlx")
	controller, err := buildDashboardEngineSelection()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := controller.Snapshot()
	if len(snapshot.Connectors) != 1 || snapshot.Connectors[0].DataBoundary != "remote" {
		t.Fatalf("remote endpoint boundary = %+v", snapshot.Connectors)
	}
	wire, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "gateway.example.test") || strings.Contains(string(wire), "must-not-appear") {
		t.Fatalf("engine snapshot exposed endpoint configuration: %s", wire)
	}
}

func TestDashboardHTTPStreamingCapabilityRequiresExplicitOptIn(t *testing.T) {
	setDashboardEngineIdentity(t, "SIRSI_MLX_RAW", "mlx")
	t.Setenv("SIRSI_ENGINE_PREFERRED", "mlx")
	t.Setenv("SIRSI_ENGINE_PREFERRED_VARIANT", "mlx-raw")
	controller, err := buildDashboardEngineSelection()
	if err != nil {
		t.Fatal(err)
	}
	capabilities := controller.Snapshot().Connectors[0].Capabilities
	if capabilities.Streaming {
		t.Fatal("OpenAI-compatible endpoint advertised streaming without explicit configuration")
	}
	if !capabilities.Cancellation {
		t.Fatal("context-aware HTTP endpoint did not retain cancellation capability")
	}

	t.Setenv("SIRSI_MLX_RAW_STREAMING", "true")
	controller, err = buildDashboardEngineSelection()
	if err != nil {
		t.Fatal(err)
	}
	if !controller.Snapshot().Connectors[0].Capabilities.Streaming {
		t.Fatal("explicitly enabled HTTP streaming capability was not exposed")
	}

	t.Setenv("SIRSI_MLX_RAW_STREAMING", "sometimes")
	if _, err := buildDashboardEngineSelection(); err == nil || !strings.Contains(err.Error(), "SIRSI_MLX_RAW_STREAMING must be boolean") {
		t.Fatalf("malformed streaming capability was not rejected: %v", err)
	}
}

func TestDashboardEngineVariantSelectionAndDefaults(t *testing.T) {
	setDashboardEngineIdentity(t, "SIRSI_MLX", "mlx")
	setDashboardEngineIdentity(t, "SIRSI_SNE", "sne")
	t.Setenv("SIRSI_MLX_VARIANT", "mlx-patched")
	t.Setenv("SIRSI_SNE_VARIANT", "sne-plain")
	t.Setenv("SIRSI_ENGINE_PREFERRED", "sne")
	t.Setenv("SIRSI_ENGINE_PREFERRED_VARIANT", "sne-plain")
	t.Setenv("SIRSI_ENGINE_ALLOW_FALLBACK", "true")
	controller, err := buildDashboardEngineSelection()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := controller.Snapshot()
	if snapshot.PreferredVariant != engine.VariantSNEPlain || snapshot.Connectors[0].Variant != engine.VariantMLXPatched || snapshot.Connectors[1].Variant != engine.VariantSNEPlain {
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

func TestEnginePromptDefersExplicitMTPCapabilityToLiveReadiness(t *testing.T) {
	setDashboardEngineIdentity(t, "SIRSI_SNE_MTP", "sne")
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	t.Setenv("SIRSI_SNE_MTP_ASSISTANT_MODEL_ID", "assistant-model")
	t.Setenv("SIRSI_SNE_MTP_ASSISTANT_REVISION", "r7")
	t.Setenv("SIRSI_SNE_MTP_ASSISTANT_CHECKPOINT_SHA256", sha)
	t.Setenv("SIRSI_SNE_MTP_ASSISTANT_PRECISION", "int8")
	t.Setenv("SIRSI_ENGINE_PREFERRED", "")
	t.Setenv("SIRSI_ENGINE_PREFERRED_VARIANT", "")
	t.Setenv("SIRSI_ENGINE_ALLOW_FALLBACK", "")
	if _, err := buildDashboardEngineSelection(); err == nil {
		t.Fatal("dashboard inferred SNE MTP without an explicit preferred variant")
	}
	for _, opts := range []enginePromptOptions{
		{Engine: "sne", Variant: "sne-mtp"},
		{Variant: "sne-mtp"},
	} {
		controller, err := buildDashboardEngineSelectionForPrompt(opts)
		if err != nil {
			t.Fatalf("explicit MTP policy should wait for live capability proof: %v", err)
		}
		snapshot := controller.Snapshot()
		if snapshot.Preferred != engine.KindSNE || snapshot.PreferredVariant != engine.VariantSNEMTP || len(snapshot.Connectors) != 1 {
			t.Fatalf("MTP policy was not selected exactly: %+v", snapshot)
		}
		connector := snapshot.Connectors[0]
		if connector.Capabilities.MTP {
			t.Fatal("MTP capability was claimed before live service readiness")
		}
		resolvable := false
		for _, capability := range connector.ContextualCapabilities {
			resolvable = resolvable || capability == engine.CapabilityMTP
		}
		if !resolvable {
			t.Fatal("explicit MTP policy lacks a readiness-time capability resolver")
		}
	}
}

func TestDashboardMTPRouteBindsAssistantIdentityAndCapability(t *testing.T) {
	setDashboardEngineIdentity(t, "SIRSI_SNE_MTP", "sne")
	t.Setenv("SIRSI_ENGINE_ALLOW_FALLBACK", "false")
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	t.Setenv("SIRSI_SNE_MTP_ASSISTANT_MODEL_ID", "assistant-model")
	t.Setenv("SIRSI_SNE_MTP_ASSISTANT_REVISION", "r7")
	t.Setenv("SIRSI_SNE_MTP_ASSISTANT_CHECKPOINT_SHA256", sha)
	t.Setenv("SIRSI_SNE_MTP_ASSISTANT_PRECISION", "int8")
	controller, err := buildDashboardEngineSelectionForPrompt(enginePromptOptions{Engine: "sne", Variant: "sne-mtp"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := controller.Snapshot()
	if snapshot.PreferredVariant != engine.VariantSNEMTP || len(snapshot.Connectors) != 1 {
		t.Fatalf("MTP route selection = %+v", snapshot)
	}
	connector := snapshot.Connectors[0]
	if connector.Capabilities.MTP {
		t.Fatal("MTP capability was claimed before live service readiness")
	}
	contextual, configured, err := buildDashboardConnectorVariant(engine.KindSNE, "SIRSI_SNE_MTP", engine.VariantSNEMTP)
	if err != nil || !configured {
		t.Fatalf("MTP connector construction: configured=%v err=%v", configured, err)
	}
	resolver, ok := contextual.(engine.ContextualCapabilityResolver)
	if !ok || !resolver.CanResolveCapability(engine.CapabilityMTP) {
		t.Fatal("explicit MTP connector has no readiness-time capability resolver")
	}
	identity, _, err := dashboardEngineIdentityVariant(engine.KindSNE, "SIRSI_SNE_MTP", engine.VariantSNEMTP)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Assistant == nil || identity.Assistant.ModelID != "assistant-model" || identity.Assistant.Revision != "r7" || identity.Assistant.CheckpointSHA256 != sha || identity.Assistant.Precision != "int8" {
		t.Fatalf("MTP assistant identity = %+v", identity.Assistant)
	}

	t.Setenv("SIRSI_SNE_MTP_ASSISTANT_CHECKPOINT_SHA256", "invalid")
	if _, err := buildDashboardEngineSelectionForPrompt(enginePromptOptions{Engine: "sne", Variant: "sne-mtp"}); err == nil || !strings.Contains(err.Error(), "assistant.checkpoint_sha256") {
		t.Fatalf("malformed assistant checkpoint was accepted: %v", err)
	}
}

func TestDashboardRejectsIncompatibleConfiguredVariant(t *testing.T) {
	setDashboardEngineIdentity(t, "SIRSI_MLX", "mlx")
	t.Setenv("SIRSI_MLX_VARIANT", "sne-plain")
	if _, err := buildDashboardEngineSelection(); err == nil {
		t.Fatal("incompatible dashboard variant accepted")
	}
}
