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

func (f *fakeEngineSelection) Snapshot() engine.SelectionSnapshot { return f.snapshot }

func (f *fakeEngineSelection) Select(policy engine.RoutePolicy) (engine.SelectionSnapshot, error) {
	f.selected = policy
	f.snapshot.Preferred = policy.Preferred
	f.snapshot.AllowFallback = policy.AllowFallback
	f.snapshot.RequiredCapabilities = policy.RequiredCapabilities
	return f.snapshot, nil
}

func TestEngineSelectionAPIProjectsAndChangesOnlyPolicy(t *testing.T) {
	selection := &fakeEngineSelection{snapshot: engine.SelectionSnapshot{
		Schema: engine.SelectionSchema, Preferred: engine.KindMLX,
		Connectors: []engine.ConnectorSummary{{Kind: engine.KindMLX}, {Kind: engine.KindOMLX}, {Kind: engine.KindSNE}},
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
	if snapshot.Preferred != engine.KindMLX || len(snapshot.Connectors) != 3 {
		t.Fatalf("unexpected engine snapshot: %+v", snapshot)
	}

	body := `{"preferred":"sne","allow_fallback":true}`
	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/engine/select", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer engine-token-abcdefghijklmnopqrstuvwxyz")
	request.Header.Set("Host", "127.0.0.1")
	selected, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer selected.Body.Close()
	if selected.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/engine/select = %d", selected.StatusCode)
	}
	if selection.selected.Preferred != engine.KindSNE || !selection.selected.AllowFallback {
		t.Fatalf("selection policy = %+v", selection.selected)
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
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/engine/select", strings.NewReader(`{"preferred":"sne","unexpected":true}`))
	request.Header.Set("Authorization", "Bearer engine-token-abcdefghijklmnopqrstuvwxyz")
	server.apiEngineSelect(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", recorder.Code)
	}
}
