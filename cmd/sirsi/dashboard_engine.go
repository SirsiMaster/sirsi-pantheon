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
	connectors := make([]engine.Connector, 0, 3)
	for _, spec := range []struct {
		kind   engine.Kind
		prefix string
	}{
		{kind: engine.KindMLX, prefix: "SIRSI_MLX"},
		{kind: engine.KindOMLX, prefix: "SIRSI_OMLX"},
		{kind: engine.KindSNE, prefix: "SIRSI_SNE"},
	} {
		connector, configured, err := buildDashboardConnector(spec.kind, spec.prefix)
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
	if preferred == "" {
		preferred = connectors[0].Kind()
	}
	allowFallback, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("SIRSI_ENGINE_ALLOW_FALLBACK")))
	if err != nil && strings.TrimSpace(os.Getenv("SIRSI_ENGINE_ALLOW_FALLBACK")) != "" {
		return nil, fmt.Errorf("SIRSI_ENGINE_ALLOW_FALLBACK must be boolean: %w", err)
	}
	return engine.NewSelectionController(router, engine.RoutePolicy{Preferred: preferred, AllowFallback: allowFallback})
}

func buildDashboardConnector(kind engine.Kind, prefix string) (engine.Connector, bool, error) {
	endpoint := strings.TrimSpace(os.Getenv(prefix + "_ENDPOINT"))
	if endpoint == "" {
		return nil, false, nil
	}
	identity, modelID, err := dashboardEngineIdentity(kind, prefix)
	if err != nil {
		return nil, false, err
	}
	var backend provider.Provider
	switch kind {
	case engine.KindMLX, engine.KindOMLX:
		backend = &provider.OpenAICompat{
			ProviderName: strings.ToLower(string(kind)), Endpoint: endpoint, Model: modelID,
			TierValue: provider.TierLocal, HTTP: http.DefaultClient,
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
	if _, ok := backend.(provider.StreamingProvider); ok {
		caps.Streaming = true
	}
	connector, err := engine.NewProviderConnector(backend, kind, identity, caps)
	if err != nil {
		return nil, false, fmt.Errorf("%s connector: %w", prefix, err)
	}
	return connector, true, nil
}

func dashboardEngineIdentity(kind engine.Kind, prefix string) (engine.Identity, string, error) {
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
