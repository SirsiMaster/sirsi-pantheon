package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

// buildDashboardEngineSelection constructs only explicitly configured,
// identity-bound connectors. It performs no network probe: session opening
// remains the point where provider availability/readiness is established.
func buildDashboardEngineSelection() (*engine.SelectionController, error) {
	return buildDashboardEngineSelectionWithOverrides("", "", false)
}

func buildDashboardEngineSelectionForPrompt(opts enginePromptOptions) (*engine.SelectionController, error) {
	preferred, variant := opts.Engine, opts.Variant
	if preferred == "" && variant != "" {
		parsed, err := engine.ParseVariant(variant)
		if err != nil {
			return nil, fmt.Errorf("engine prompt variant: %w", err)
		}
		for _, kind := range []engine.Kind{engine.KindMLX, engine.KindOMLX, engine.KindSNE} {
			if parsed.ValidateForEngine(kind) == nil {
				preferred = string(kind)
				break
			}
		}
	}
	if preferred == "" && variant == "" {
		return buildDashboardEngineSelection()
	}
	return buildDashboardEngineSelectionWithOverrides(preferred, variant, true)
}

func buildDashboardEngineSelectionWithOverrides(engineOverride, variantOverride string, override bool) (*engine.SelectionController, error) {
	connectors := make([]engine.Connector, 0, 7)
	for _, spec := range []struct {
		kind    engine.Kind
		prefix  string
		variant engine.BackendVariant
	}{
		{kind: engine.KindMLX, prefix: "SIRSI_MLX"}, // legacy single-variant configuration
		{kind: engine.KindOMLX, prefix: "SIRSI_OMLX"},
		{kind: engine.KindSNE, prefix: "SIRSI_SNE"},
		{kind: engine.KindMLX, prefix: "SIRSI_MLX_RAW", variant: engine.VariantMLXRaw},
		{kind: engine.KindMLX, prefix: "SIRSI_MLX_PATCHED", variant: engine.VariantMLXPatched},
		{kind: engine.KindSNE, prefix: "SIRSI_SNE_PLAIN", variant: engine.VariantSNEPlain},
		{kind: engine.KindSNE, prefix: "SIRSI_SNE_MTP", variant: engine.VariantSNEMTP},
	} {
		connector, configured, err := buildDashboardConnectorVariant(spec.kind, spec.prefix, spec.variant)
		if err != nil {
			return nil, err
		}
		if configured {
			connectors = append(connectors, connector)
		}
	}
	if len(connectors) == 0 {
		return nil, nil
	}
	router, err := engine.NewRouter(connectors...)
	if err != nil {
		return nil, err
	}
	preferred := engine.Kind(strings.ToLower(strings.TrimSpace(os.Getenv("SIRSI_ENGINE_PREFERRED"))))
	if override && engineOverride != "" {
		preferred = engine.Kind(strings.ToLower(strings.TrimSpace(engineOverride)))
	}
	if preferred == "" {
		preferred = connectors[0].Kind()
	}
	allowFallback, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("SIRSI_ENGINE_ALLOW_FALLBACK")))
	if err != nil && strings.TrimSpace(os.Getenv("SIRSI_ENGINE_ALLOW_FALLBACK")) != "" {
		return nil, fmt.Errorf("SIRSI_ENGINE_ALLOW_FALLBACK must be boolean: %w", err)
	}
	preferredVariant := engine.BackendVariant("")
	if override {
		preferredVariant, err = dashboardExplicitVariant(preferred, variantOverride)
	} else {
		preferredVariant, err = dashboardPreferredVariant(preferred)
	}
	if err != nil {
		return nil, err
	}
	return engine.NewSelectionController(router, engine.RoutePolicy{Preferred: preferred, PreferredVariant: preferredVariant, AllowFallback: allowFallback})
}

func dashboardExplicitVariant(preferred engine.Kind, raw string) (engine.BackendVariant, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	variant, err := engine.ParseVariant(raw)
	if err != nil {
		return "", fmt.Errorf("engine prompt --variant: %w", err)
	}
	if err := variant.ValidateForEngine(preferred); err != nil {
		return "", fmt.Errorf("engine prompt --variant: %w", err)
	}
	return variant, nil
}

func dashboardPreferredVariant(preferred engine.Kind) (engine.BackendVariant, error) {
	raw := strings.TrimSpace(os.Getenv("SIRSI_ENGINE_PREFERRED_VARIANT"))
	if raw == "" {
		return "", nil
	}
	variant, err := engine.ParseVariant(raw)
	if err != nil {
		return "", fmt.Errorf("SIRSI_ENGINE_PREFERRED_VARIANT: %w", err)
	}
	if err := variant.ValidateForEngine(preferred); err != nil {
		return "", fmt.Errorf("SIRSI_ENGINE_PREFERRED_VARIANT: %w", err)
	}
	return variant, nil
}

func buildDashboardConnector(kind engine.Kind, prefix string) (engine.Connector, bool, error) {
	return buildDashboardConnectorVariant(kind, prefix, "")
}

func buildDashboardConnectorVariant(kind engine.Kind, prefix string, fixedVariant engine.BackendVariant) (engine.Connector, bool, error) {
	endpoint := strings.TrimSpace(os.Getenv(prefix + "_ENDPOINT"))
	if endpoint == "" {
		return nil, false, nil
	}
	identity, modelID, err := dashboardEngineIdentityVariant(kind, prefix, fixedVariant)
	if err != nil {
		return nil, false, err
	}
	var backend provider.Provider
	switch kind {
	case engine.KindMLX, engine.KindOMLX:
		backend = &provider.OpenAICompat{
			ProviderName: strings.ToLower(string(kind)), Endpoint: endpoint, Model: modelID,
			TierValue: provider.TierLocal, HTTP: http.DefaultClient,
			SupportsStreaming:      true,
			UseRealCompletionProbe: true,
		}
	case engine.KindSNE:
		client, err := sne.NewAuthenticatedClient(endpoint, os.Getenv(prefix+"_TOKEN"))
		if err != nil {
			return nil, false, fmt.Errorf("%s client: %w", prefix, err)
		}
		backend, err = provider.NewSNEProvider(client, provider.SNEIdentityExpectation{
			ModelID:             modelID,
			RuntimeSHA256:       os.Getenv(prefix + "_RUNTIME_SHA256"),
			NativeRuntimeSHA256: os.Getenv(prefix + "_NATIVE_RUNTIME_SHA256"),
			ManifestSHA256:      os.Getenv(prefix + "_MANIFEST_SHA256"),
		})
		if err != nil {
			return nil, false, err
		}
	default:
		return nil, false, fmt.Errorf("unsupported dashboard engine %q", kind)
	}
	backendCaps := backend.Caps()
	caps := engine.Capabilities{
		Sessions: true, Cancellation: true, Receipts: true,
		Tools: backendCaps.Tools, Temperature: backendCaps.Temperature,
		TopP: backendCaps.TopP, Seed: backendCaps.Seed,
	}
	caps.Streaming = backendCaps.Streaming
	connector, err := engine.NewProviderConnector(backend, kind, identity, caps)
	if err != nil {
		return nil, false, fmt.Errorf("%s connector: %w", prefix, err)
	}
	return connector, true, nil
}

func dashboardEngineIdentity(kind engine.Kind, prefix string) (engine.Identity, string, error) {
	return dashboardEngineIdentityVariant(kind, prefix, "")
}

func dashboardEngineIdentityVariant(kind engine.Kind, prefix string, fixedVariant engine.BackendVariant) (engine.Identity, string, error) {
	variant, err := dashboardConfiguredVariant(kind, prefix)
	if err != nil {
		return engine.Identity{}, "", err
	}
	if fixedVariant != "" {
		if err := fixedVariant.ValidateForEngine(kind); err != nil {
			return engine.Identity{}, "", err
		}
		if rawVariant := strings.TrimSpace(os.Getenv(prefix + "_VARIANT")); rawVariant != "" && variant != fixedVariant {
			return engine.Identity{}, "", fmt.Errorf("%s variant %q conflicts with fixed variant %q", prefix, variant, fixedVariant)
		}
		variant = fixedVariant
	}
	values := map[string]string{
		"model":            strings.TrimSpace(os.Getenv(prefix + "_MODEL")),
		"engine_version":   strings.TrimSpace(os.Getenv(prefix + "_ENGINE_VERSION")),
		"model_sha256":     strings.TrimSpace(os.Getenv(prefix + "_MODEL_SHA256")),
		"tokenizer_id":     strings.TrimSpace(os.Getenv(prefix + "_TOKENIZER_ID")),
		"tokenizer_sha256": strings.TrimSpace(os.Getenv(prefix + "_TOKENIZER_SHA256")),
		"precision":        strings.TrimSpace(os.Getenv(prefix + "_PRECISION")),
		"cache_namespace":  strings.TrimSpace(os.Getenv(prefix + "_CACHE_NAMESPACE")),
	}
	for name, value := range values {
		if value == "" {
			return engine.Identity{}, "", fmt.Errorf("%s is configured but %s identity is missing", prefix, name)
		}
	}
	identity := engine.Identity{
		Engine:          kind,
		Variant:         variant,
		EngineVersion:   values["engine_version"],
		ModelID:         values["model"],
		ModelSHA256:     values["model_sha256"],
		TokenizerID:     values["tokenizer_id"],
		TokenizerSHA256: values["tokenizer_sha256"],
		Precision:       values["precision"],
		CacheNamespace:  values["cache_namespace"],
	}
	if err := identity.Validate(); err != nil {
		return engine.Identity{}, "", fmt.Errorf("%s identity: %w", prefix, err)
	}
	return identity, values["model"], nil
}

func dashboardConfiguredVariant(kind engine.Kind, prefix string) (engine.BackendVariant, error) {
	raw := strings.TrimSpace(os.Getenv(prefix + "_VARIANT"))
	if raw == "" {
		return engine.DefaultVariant(kind), nil
	}
	variant, err := engine.ParseVariant(raw)
	if err != nil {
		return "", fmt.Errorf("%s variant: %w", prefix, err)
	}
	if err := variant.ValidateForEngine(kind); err != nil {
		return "", fmt.Errorf("%s variant: %w", prefix, err)
	}
	return variant, nil
}
