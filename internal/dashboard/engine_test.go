package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
)

type fakeEngineSelection struct {
	snapshot engine.SelectionSnapshot
	selected engine.RoutePolicy
}

type legacyEngineSelection struct{ delegate *fakeEngineSelection }

func (f legacyEngineSelection) Snapshot() engine.SelectionSnapshot { return f.delegate.Snapshot() }
func (f legacyEngineSelection) Select(policy engine.RoutePolicy) (engine.SelectionSnapshot, error) {
	return f.delegate.Select(policy)
}

func (f *fakeEngineSelection) Snapshot() engine.SelectionSnapshot { return f.snapshot }

func (f *fakeEngineSelection) Select(policy engine.RoutePolicy) (engine.SelectionSnapshot, error) {
	f.selected = policy
	f.snapshot.Preferred = policy.Preferred
	f.snapshot.AllowFallback = policy.AllowFallback
	f.snapshot.RequiredCapabilities = policy.RequiredCapabilities
	return f.snapshot, nil
}

func (f *fakeEngineSelection) SelectRoute(kind engine.Kind, variant engine.BackendVariant) (engine.SelectionSnapshot, error) {
	return f.Select(engine.RoutePolicy{
		Preferred: kind, PreferredVariant: variant, AllowFallback: f.snapshot.AllowFallback,
		RequiredCapabilities: append([]engine.Capability(nil), f.snapshot.RequiredCapabilities...),
	})
}

func TestEngineSelectionAPIProjectsAndChangesOnlyPolicy(t *testing.T) {
	selection := &fakeEngineSelection{snapshot: engine.SelectionSnapshot{
		Schema: engine.SelectionSchema, Preferred: engine.KindMLX,
		Connectors: []engine.ConnectorSummary{{Kind: engine.KindMLX}, {Kind: engine.KindOMLX}, {DisplayName: "Apollo (Plain)", Kind: engine.KindSNE, Variant: engine.VariantSNEPlain}},
	}}
	server := New(Config{EngineSelection: selection, SNELocalAccessToken: "engine-token-abcdefghijklmnopqrstuvwxyz"})
	ts := testServer(t, server.cfg)
	defer ts.Close()
	response, err := http.Get(ts.URL + "/api/engine")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/engine = %d", response.StatusCode)
	}
	var snapshot engine.SelectionSnapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Preferred != engine.KindMLX || len(snapshot.Connectors) != 3 || snapshot.Connectors[2].DisplayName != "Apollo (Plain)" {
		t.Fatalf("unexpected engine snapshot: %+v", snapshot)
	}

	body := `{"preferred":"sne","preferred_variant":"sne-mtp","allow_fallback":true,"required_capabilities":[]}`
	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/engine/select", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer engine-token-abcdefghijklmnopqrstuvwxyz")
	request.Header.Set("Host", "127.0.0.1")
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	selected, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer selected.Body.Close()
	if selected.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/engine/select = %d", selected.StatusCode)
	}
	if selection.selected.Preferred != engine.KindSNE || selection.selected.PreferredVariant != engine.VariantSNEMTP || !selection.selected.AllowFallback {
		t.Fatalf("selection policy = %+v", selection.selected)
	}
}

func TestEngineSelectionAPIRouteOnlyUpdatePreservesOtherPolicy(t *testing.T) {
	selection := &fakeEngineSelection{snapshot: engine.SelectionSnapshot{
		Schema: engine.SelectionSchema, Preferred: engine.KindMLX, AllowFallback: true,
		RequiredCapabilities: []engine.Capability{engine.CapabilitySessions},
	}}
	server := New(Config{EngineSelection: selection})
	request := httptest.NewRequest(http.MethodPost, "/api/engine/select", strings.NewReader(`{"preferred":"sne","preferred_variant":"sne-plain"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.apiEngineSelect(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("route-only selection status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if selection.selected.Preferred != engine.KindSNE || selection.selected.PreferredVariant != engine.VariantSNEPlain || !selection.selected.AllowFallback || len(selection.selected.RequiredCapabilities) != 1 || selection.selected.RequiredCapabilities[0] != engine.CapabilitySessions {
		t.Fatalf("route-only selection changed unrelated policy: %+v", selection.selected)
	}
}

func TestEngineSelectionAPIRouteOnlyRejectsNonAtomicController(t *testing.T) {
	delegate := &fakeEngineSelection{snapshot: engine.SelectionSnapshot{
		Schema: engine.SelectionSchema, Preferred: engine.KindMLX, AllowFallback: true,
		RequiredCapabilities: []engine.Capability{engine.CapabilitySessions},
	}}
	server := New(Config{EngineSelection: legacyEngineSelection{delegate: delegate}})
	request := httptest.NewRequest(http.MethodPost, "/api/engine/select", strings.NewReader(`{"preferred":"sne","preferred_variant":"sne-plain"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.apiEngineSelect(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("non-atomic route selection status = %d, want %d; body=%s", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
	if delegate.selected.Preferred != "" || delegate.snapshot.Preferred != engine.KindMLX || !delegate.snapshot.AllowFallback || len(delegate.snapshot.RequiredCapabilities) != 1 || delegate.snapshot.RequiredCapabilities[0] != engine.CapabilitySessions {
		t.Fatalf("unsupported controller route update mutated canonical policy: %+v", delegate.snapshot)
	}
}

func TestEngineSelectionAPIRouteOnlyUsesCanonicalController(t *testing.T) {
	identityFor := func(kind engine.Kind, variant engine.BackendVariant, model string) engine.Identity {
		return engine.Identity{
			Engine: kind, Variant: variant, EngineVersion: "test-1", ModelID: model,
			ModelSHA256: strings.Repeat("a", 64), TokenizerID: "test-tokenizer",
			TokenizerSHA256: strings.Repeat("b", 64), Precision: "bf16", CacheNamespace: "dashboard-selection-test",
		}
	}
	mlxProvider := &askRouteProvider{model: "mlx-test-model"}
	mlx, err := engine.NewMLXConnector(mlxProvider, identityFor(engine.KindMLX, engine.VariantMLXRaw, "mlx-test-model"), engine.Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	sneProvider := &askRouteProvider{model: "sne-test-model"}
	sne, err := engine.NewSNEConnector(sneProvider, identityFor(engine.KindSNE, engine.VariantSNEPlain, "sne-test-model"), engine.Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := engine.NewRouter(mlx, sne)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := engine.NewSelectionController(router, engine.RoutePolicy{
		Preferred: engine.KindMLX, PreferredVariant: engine.VariantMLXRaw,
		AllowFallback: true, RequiredCapabilities: []engine.Capability{engine.CapabilitySessions},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{
		EngineSelection:     controller,
		SNELocalAccessToken: "pantheon-dashboard-route-flow-capability",
	})
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/engine/select", strings.NewReader(`{"preferred":"sne","preferred_variant":"sne-plain"}`))
	request.Host = "127.0.0.1"
	request.Header.Set("Authorization", "Bearer pantheon-dashboard-route-flow-capability")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("canonical route selection status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	policy := controller.Policy()
	if policy.Preferred != engine.KindSNE || policy.PreferredVariant != engine.VariantSNEPlain || !policy.AllowFallback || len(policy.RequiredCapabilities) != 1 || policy.RequiredCapabilities[0] != engine.CapabilitySessions {
		t.Fatalf("canonical controller route update lost policy: %+v", policy)
	}
	var snapshot engine.SelectionSnapshot
	if err := json.NewDecoder(recorder.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Preferred != policy.Preferred || snapshot.PreferredVariant != policy.PreferredVariant || snapshot.AllowFallback != policy.AllowFallback || len(snapshot.RequiredCapabilities) != 1 || snapshot.RequiredCapabilities[0] != engine.CapabilitySessions {
		t.Fatalf("route update response differs from canonical policy: %+v vs %+v", snapshot, policy)
	}

	const question = "which configured route handled this request?"
	askRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/ask", strings.NewReader(`{"question":"`+question+`"}`))
	askRequest.Host = "127.0.0.1"
	askRequest.Header.Set("Authorization", "Bearer pantheon-dashboard-route-flow-capability")
	askRequest.Header.Set("Content-Type", "application/json")
	askRecorder := httptest.NewRecorder()
	server.handler.ServeHTTP(askRecorder, askRequest)
	if askRecorder.Code != http.StatusOK {
		t.Fatalf("prompt through API-selected route status = %d, want %d; body=%s", askRecorder.Code, http.StatusOK, askRecorder.Body.String())
	}
	var answer askResponse
	if err := json.NewDecoder(askRecorder.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if len(sneProvider.requests) != 1 || sneProvider.requests[0].Prompt != question {
		t.Fatalf("selected SNE connector requests = %+v, want one request for %q", sneProvider.requests, question)
	}
	if len(mlxProvider.requests) != 0 {
		t.Fatalf("unselected MLX connector received requests: %+v", mlxProvider.requests)
	}
	if answer.Receipt == nil || answer.Receipt.Route == nil || answer.Receipt.Route.Requested != engine.KindSNE || answer.Receipt.Route.RequestedVariant != engine.VariantSNEPlain || answer.Receipt.Route.Selected != engine.KindSNE || answer.Receipt.Route.SelectedVariant != engine.VariantSNEPlain || answer.Receipt.Identity.ModelID != "sne-test-model" || answer.Receipt.RequestSHA256 == "" || answer.Receipt.CompletionSHA256 == "" {
		t.Fatalf("prompt response receipt does not prove the API-selected route: %+v", answer.Receipt)
	}
}

func TestEngineSelectionAPIRejectsUnknownFieldsAndMissingController(t *testing.T) {
	server := New(Config{SNELocalAccessToken: "engine-token-abcdefghijklmnopqrstuvwxyz"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/engine", nil)
	server.apiEngine(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing controller status = %d", recorder.Code)
	}

	selection := &fakeEngineSelection{snapshot: engine.SelectionSnapshot{Schema: engine.SelectionSchema, Preferred: engine.KindMLX}}
	server = New(Config{EngineSelection: selection, SNELocalAccessToken: "engine-token-abcdefghijklmnopqrstuvwxyz"})
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "unknown field", body: `{"preferred":"sne","unexpected":true}`},
		{name: "duplicate key", body: `{"preferred":"sne","preferred":"mlx"}`},
		{name: "case alias", body: `{"Preferred":"sne"}`},
		{name: "null preferred variant", body: `{"preferred":"sne","preferred_variant":null}`},
		{name: "null fallback policy", body: `{"preferred":"sne","allow_fallback":null}`},
		{name: "null capability list", body: `{"preferred":"sne","required_capabilities":null}`},
		{name: "missing route variant", body: `{"preferred":"sne"}`},
		{name: "partial fallback replacement", body: `{"preferred":"sne","preferred_variant":"sne-plain","allow_fallback":true}`},
		{name: "partial capability replacement", body: `{"preferred":"sne","preferred_variant":"sne-plain","required_capabilities":[]}`},
		{name: "non-object", body: `[]`},
		{name: "trailing value", body: `{"preferred":"sne"}{"preferred":"mlx"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/engine/select", strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer engine-token-abcdefghijklmnopqrstuvwxyz")
			request.Header.Set("Content-Type", "application/json")
			server.apiEngineSelect(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if selection.selected.Preferred != "" {
				t.Fatalf("invalid request changed policy to %+v", selection.selected)
			}
		})
	}
}

func TestEngineSelectionAPIRequiresJSONContentTypeBeforeMutation(t *testing.T) {
	selection := &fakeEngineSelection{snapshot: engine.SelectionSnapshot{Schema: engine.SelectionSchema, Preferred: engine.KindMLX}}
	server := New(Config{EngineSelection: selection, SNELocalAccessToken: "engine-token-abcdefghijklmnopqrstuvwxyz"})
	for _, test := range []struct {
		name        string
		contentType string
	}{
		{name: "missing"},
		{name: "text plain", contentType: "text/plain"},
		{name: "malformed", contentType: "application/json; charset"},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection.selected = engine.RoutePolicy{}
			request := httptest.NewRequest(http.MethodPost, "/api/engine/select", strings.NewReader(`{"preferred":"sne","preferred_variant":"sne-plain"}`))
			request.Header.Set("Content-Type", test.contentType)
			recorder := httptest.NewRecorder()
			server.apiEngineSelect(recorder, request)
			if recorder.Code != http.StatusUnsupportedMediaType {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusUnsupportedMediaType, recorder.Body.String())
			}
			if selection.selected.Preferred != "" {
				t.Fatalf("invalid media type mutated engine policy: %+v", selection.selected)
			}
		})
	}
}

func TestEngineSelectionAPIRejectsOversizedBodyBeforeMutation(t *testing.T) {
	selection := &fakeEngineSelection{snapshot: engine.SelectionSnapshot{Schema: engine.SelectionSchema, Preferred: engine.KindMLX}}
	server := New(Config{EngineSelection: selection, SNELocalAccessToken: "engine-token-abcdefghijklmnopqrstuvwxyz"})
	body := `{"preferred":"sne","preferred_variant":"sne-plain"}` + strings.Repeat(" ", (64<<10)+1)
	request := httptest.NewRequest(http.MethodPost, "/api/engine/select", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer engine-token-abcdefghijklmnopqrstuvwxyz")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	server.apiEngineSelect(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized request status = %d, want %d; body=%s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
	if selection.selected.Preferred != "" {
		t.Fatalf("oversized request mutated engine policy: %+v", selection.selected)
	}
}

func TestRegisteredEngineSelectionRequiresCapabilityBeforeMutation(t *testing.T) {
	selection := &fakeEngineSelection{snapshot: engine.SelectionSnapshot{Schema: engine.SelectionSchema, Preferred: engine.KindMLX}}
	server := New(Config{EngineSelection: selection, SNELocalAccessToken: "engine-token-abcdefghijklmnopqrstuvwxyz"})
	for _, test := range []struct {
		name       string
		credential string
		wantStatus int
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized},
		{name: "invalid", credential: "Bearer wrong-token-abcdefghijklmnopqrstuvwxyz", wantStatus: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection.selected = engine.RoutePolicy{}
			request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9119/api/engine/select", strings.NewReader(`{"preferred":"sne","preferred_variant":"sne-plain"}`))
			request.Host = "127.0.0.1:9119"
			request.Header.Set("Content-Type", "application/json")
			if test.credential != "" {
				request.Header.Set("Authorization", test.credential)
			}
			recorder := httptest.NewRecorder()
			server.handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if selection.selected.Preferred != "" {
				t.Fatalf("unauthorized request mutated engine policy: %+v", selection.selected)
			}
		})
	}
}

func TestEngineSelectionAPIPreflightAllowsNexusRouteChange(t *testing.T) {
	selection := &fakeEngineSelection{snapshot: engine.SelectionSnapshot{Schema: engine.SelectionSchema, Preferred: engine.KindMLX}}
	server := New(Config{EngineSelection: selection, SNELocalAccessToken: "engine-token-abcdefghijklmnopqrstuvwxyz"})
	ts := testServer(t, server.cfg)
	defer ts.Close()

	preflight, err := http.NewRequest(http.MethodOptions, ts.URL+"/api/engine/select", nil)
	if err != nil {
		t.Fatal(err)
	}
	preflight.Header.Set("Origin", "http://127.0.0.1:5173")
	preflight.Header.Set("Access-Control-Request-Method", "POST")
	preflight.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	preflightResponse, err := http.DefaultClient.Do(preflight)
	if err != nil {
		t.Fatal(err)
	}
	defer preflightResponse.Body.Close()
	if preflightResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", preflightResponse.StatusCode, http.StatusNoContent)
	}
	if got := preflightResponse.Header.Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("preflight allow-origin = %q", got)
	}
	if got := preflightResponse.Header.Get("Access-Control-Allow-Methods"); got != "POST, OPTIONS" {
		t.Fatalf("preflight allow-methods = %q", got)
	}
	if got := preflightResponse.Header.Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
		t.Fatalf("preflight allow-headers = %q", got)
	}
	if selection.selected.Preferred != "" {
		t.Fatalf("preflight mutated engine policy: %+v", selection.selected)
	}

	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/engine/select", strings.NewReader(`{"preferred":"sne","preferred_variant":"sne-plain"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	request.Header.Set("Authorization", "Bearer engine-token-abcdefghijklmnopqrstuvwxyz")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || selection.selected.Preferred != engine.KindSNE || selection.selected.PreferredVariant != engine.VariantSNEPlain {
		t.Fatalf("cross-origin route change status=%d selected=%+v", response.StatusCode, selection.selected)
	}
	if got := response.Header.Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("route change allow-origin = %q", got)
	}
}

func TestEngineSelectionRejectsUntrustedOriginBeforePolicyMutation(t *testing.T) {
	selection := &fakeEngineSelection{snapshot: engine.SelectionSnapshot{Schema: engine.SelectionSchema, Preferred: engine.KindMLX}}
	server := New(Config{EngineSelection: selection, SNELocalAccessToken: "engine-token-abcdefghijklmnopqrstuvwxyz"})
	ts := testServer(t, server.cfg)
	defer ts.Close()

	preflight, err := http.NewRequest(http.MethodOptions, ts.URL+"/api/engine/select", nil)
	if err != nil {
		t.Fatal(err)
	}
	preflight.Header.Set("Origin", "https://attacker.example")
	preflight.Header.Set("Access-Control-Request-Method", "POST")
	preflight.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	preflightResponse, err := http.DefaultClient.Do(preflight)
	if err != nil {
		t.Fatal(err)
	}
	_ = preflightResponse.Body.Close()
	if preflightResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("untrusted preflight status = %d, want %d", preflightResponse.StatusCode, http.StatusForbidden)
	}
	if selection.selected.Preferred != "" {
		t.Fatalf("untrusted preflight mutated engine policy: %+v", selection.selected)
	}

	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/engine/select", strings.NewReader(`{"preferred":"sne","preferred_variant":"sne-plain"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://attacker.example")
	request.Header.Set("Authorization", "Bearer engine-token-abcdefghijklmnopqrstuvwxyz")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("untrusted route-change status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
	if selection.selected.Preferred != "" {
		t.Fatalf("untrusted origin with a valid bearer mutated engine policy: %+v", selection.selected)
	}
}

func TestEngineSelectionAPIPreflightRejectsUnneededHeadersAndMethods(t *testing.T) {
	for _, test := range []struct {
		name    string
		method  string
		headers string
	}{
		{name: "other method", method: "DELETE", headers: "authorization,content-type"},
		{name: "unneeded header", method: "POST", headers: "authorization,content-type,x-untrusted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodOptions, "/api/engine/select", nil)
			request.Header.Set("Access-Control-Request-Method", test.method)
			request.Header.Set("Access-Control-Request-Headers", test.headers)
			recorder := httptest.NewRecorder()
			if prepareEngineSelectionPreflight(recorder, request) {
				t.Fatal("invalid preflight was accepted")
			}
			if recorder.Code == http.StatusNoContent || recorder.Code == http.StatusOK {
				t.Fatalf("invalid preflight status = %d", recorder.Code)
			}
		})
	}
}
