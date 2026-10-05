package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
)

func testControlEnvelopeBody(t *testing.T) []byte {
	t.Helper()
	state := routerboard.Payload{
		GeneratedAt:      "2026-09-23T12:00:00Z",
		Evidence:         []routerboard.EvidenceRef{{TaskID: "task-7", Agent: "codex", Label: "review", URL: "receipt://review", Status: "complete"}},
		Counters:         routerboard.Counters{Completed: 1, InProgressNow: 2, Total: 4, Pending: 1, Done: 1},
		Activity:         []routerboard.Event{{At: "2026-09-23T11:59:00Z", Agent: "codex", TaskID: "task-7", Subject: "review", From: "in-progress", To: "done"}},
		Fleet:            []routerboard.Lane{{Agent: "codex", Activity: "active now", OpenItems: 1, Counts: map[string]int{"in-progress": 1}}},
		Tasks:            []routerboard.TaskDetail{{TaskID: "task-7", Agent: "codex", Subject: "review", Status: "done", Updated: "2026-09-23T11:59:00Z"}},
		DataErrors:       []string{},
		Threads:          []routerboard.Thread{},
		RegistrationGaps: []string{},
		Board:            routerboard.BoardSummary{TotalTasks: 4, DoneTasks: 1, ActiveTasks: 2, PctDone: 25},
		Ledger:           routerboard.BoardSummary{TotalTasks: 4, DoneTasks: 1, ActiveTasks: 2, PctDone: 25},
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	stateHash := sha256.Sum256(stateBytes)
	body, err := json.Marshal(routerboard.ControlEnvelope{
		Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Revision: 42,
		GeneratedAt: state.GeneratedAt, StateSHA256: hex.EncodeToString(stateHash[:]),
		Capabilities: routerboard.ControlCapabilities(), State: state,
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestControlActionAPIRequiresLocalCapabilityAndPreservesCanonicalReceipt(t *testing.T) {
	const capability = "pantheon-dashboard-local-capability-test"
	requestBody := []byte(`{"verb":"message","from":"m1","to":"ssa","title":"review status"}`)
	called := false
	ts := testServer(t, Config{
		SNELocalAccessToken: capability,
		ControlActionFn: func(ctx context.Context, body []byte) ([]byte, error) {
			called = true
			if ctx == nil || string(body) != string(requestBody) {
				t.Fatalf("action callback context=%v body=%s", ctx, body)
			}
			response := routerboard.ControlActionResponse{
				Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Verb: "message", ItemID: "item-7",
			}
			if err := response.SealControlActionResponse(body); err != nil {
				return nil, err
			}
			return json.Marshal(response)
		},
	})
	defer ts.Close()

	unauthorized, err := http.Post(ts.URL+"/api/control/action", "application/json", strings.NewReader(string(requestBody)))
	if err != nil {
		t.Fatal(err)
	}
	_ = unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized || called {
		t.Fatalf("unauthorized action status=%d callback=%t", unauthorized.StatusCode, called)
	}

	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/control/action", strings.NewReader(string(requestBody)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+capability)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("authorized action status=%d cache=%q", response.StatusCode, response.Header.Get("Cache-Control"))
	}
	var receipt routerboard.ControlActionResponse
	if err := json.NewDecoder(response.Body).Decode(&receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.ItemID != "item-7" || receipt.Verb != "message" {
		t.Fatalf("receipt = %#v", receipt)
	}
	if err := receipt.VerifyControlActionResponse(requestBody); err != nil {
		t.Fatalf("receipt failed request binding: %v", err)
	}
	if !called {
		t.Fatal("authorized action did not reach canonical producer")
	}
}

func TestControlActionProxyRejectsHashedButIncompleteOutcome(t *testing.T) {
	requestBody := []byte(`{"verb":"message","from":"m1","to":"m5","title":"inspect"}`)
	response := routerboard.ControlActionResponse{
		Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Verb: "message",
	}
	if err := response.SealControlActionResponse(requestBody); err != nil {
		t.Fatal(err)
	}
	responseBody, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateControlActionProxyReceipt(requestBody, responseBody); err == nil {
		t.Fatal("hash-valid message receipt without item identity was accepted")
	}
}

func TestControlActionAPIRejectsInvalidRequestBeforeProducer(t *testing.T) {
	called := false
	ts := testServer(t, Config{
		SNELocalAccessToken: "local-capability",
		ControlActionFn: func(context.Context, []byte) ([]byte, error) {
			called = true
			return nil, errors.New("should not be called")
		},
	})
	defer ts.Close()
	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/control/action", strings.NewReader(`{"verb":"message","verb":"claim"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer local-capability")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest || called {
		t.Fatalf("duplicate-key action status=%d callback=%t", response.StatusCode, called)
	}
}

func TestControlActionAPISeparatesBodyLimitFromReadFailure(t *testing.T) {
	for _, test := range []struct {
		name       string
		body       io.ReadCloser
		wantStatus int
	}{
		{
			name:       "over limit",
			body:       io.NopCloser(strings.NewReader(strings.Repeat("x", routerboard.ControlActionBodyLimit+1))),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "read failure",
			body:       io.NopCloser(iotest.ErrReader(errors.New("injected read failure"))),
			wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			server := New(Config{
				SNELocalAccessToken: "local-capability",
				ControlActionFn: func(context.Context, []byte) ([]byte, error) {
					called = true
					return nil, errors.New("producer must not be called")
				},
			})
			request := httptest.NewRequest(http.MethodPost, "/api/control/action", nil)
			request.Header.Set("Content-Type", "application/json")
			request.Body = test.body
			response := httptest.NewRecorder()
			server.apiControlAction(response, request)
			if response.Code != test.wantStatus || called {
				t.Fatalf("body result status=%d producer-called=%t body=%s", response.Code, called, response.Body.String())
			}
		})
	}
}

func TestControlActionAPIRejectsNonJSONBeforeProducer(t *testing.T) {
	called := false
	ts := testServer(t, Config{
		SNELocalAccessToken: "local-capability",
		ControlActionFn: func(context.Context, []byte) ([]byte, error) {
			called = true
			return nil, errors.New("producer must not be called")
		},
	})
	defer ts.Close()

	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/control/action", strings.NewReader(`{"verb":"message"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer local-capability")
	request.Header.Set("Content-Type", "text/plain")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnsupportedMediaType || called {
		t.Fatalf("non-JSON action status=%d producer-called=%t", response.StatusCode, called)
	}
}

func TestControlActionAPIProxiesRequestBoundCanonicalFailure(t *testing.T) {
	const capability = "pantheon-dashboard-local-capability-test"
	requestBody := []byte(`{"verb":"message","from":"m1","to":"ssa","title":"review status"}`)
	ts := testServer(t, Config{
		SNELocalAccessToken: capability,
		ControlActionFn: func(_ context.Context, body []byte) ([]byte, error) {
			if string(body) != string(requestBody) {
				t.Fatalf("action callback body = %s", body)
			}
			failure := routerboard.ControlActionFailure{
				Schema: routerboard.ControlFailureSchema, Authority: "canonical-routerstore",
				Verb: "message", Error: "canonical router rejected the message",
			}
			if err := failure.SealControlActionFailure(body); err != nil {
				t.Fatalf("seal canonical failure receipt: %v", err)
			}
			return nil, &routerboard.ControlActionFailureError{Failure: failure}
		},
	})
	defer ts.Close()

	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/control/action", strings.NewReader(string(requestBody)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+capability)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("canonical rejection status=%d cache=%q", response.StatusCode, response.Header.Get("Cache-Control"))
	}
	var failure routerboard.ControlActionFailure
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if err := failure.VerifyControlActionFailure(requestBody); err != nil {
		t.Fatalf("proxied failure receipt is not request-bound: %v", err)
	}
}

func TestControlActionAPIRejectsTamperedCanonicalFailureReceipt(t *testing.T) {
	const capability = "pantheon-dashboard-local-capability-test"
	requestBody := []byte(`{"verb":"message","from":"m1","to":"ssa","title":"review status"}`)
	ts := testServer(t, Config{
		SNELocalAccessToken: capability,
		ControlActionFn: func(_ context.Context, body []byte) ([]byte, error) {
			failure := routerboard.ControlActionFailure{
				Schema: routerboard.ControlFailureSchema, Authority: "canonical-routerstore",
				Verb: "message", Error: "canonical router rejected the message",
			}
			if err := failure.SealControlActionFailure(body); err != nil {
				t.Fatalf("seal canonical failure receipt: %v", err)
			}
			failure.Error = "altered after receipt sealing"
			return nil, &routerboard.ControlActionFailureError{Failure: failure}
		},
	})
	defer ts.Close()

	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/control/action", strings.NewReader(string(requestBody)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+capability)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("tampered canonical failure status=%d, want 502", response.StatusCode)
	}
	var failure routerboard.ControlActionFailure
	if err := json.NewDecoder(response.Body).Decode(&failure); err == nil && failure.Schema == routerboard.ControlFailureSchema {
		t.Fatal("tampered canonical failure receipt escaped as a canonical response")
	}
}

func TestControlActionAPIRejectsFailureReceiptForDifferentVerb(t *testing.T) {
	const capability = "pantheon-dashboard-local-capability-test"
	requestBody := []byte(`{"verb":"message","from":"m1","to":"ssa","title":"review status"}`)
	ts := testServer(t, Config{
		SNELocalAccessToken: capability,
		ControlActionFn: func(_ context.Context, body []byte) ([]byte, error) {
			failure := routerboard.ControlActionFailure{
				Schema: routerboard.ControlFailureSchema, Authority: "canonical-routerstore",
				Verb: "delegate", Error: "a different action was rejected",
			}
			if err := failure.SealControlActionFailure(body); err != nil {
				t.Fatalf("seal canonical failure receipt: %v", err)
			}
			return nil, &routerboard.ControlActionFailureError{Failure: failure}
		},
	})
	defer ts.Close()

	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/control/action", strings.NewReader(string(requestBody)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+capability)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("misattributed failure status=%d, want 502", response.StatusCode)
	}
}

func TestControlAPIProxiesCanonicalSnapshotWithoutChangingFleetContract(t *testing.T) {
	body := testControlEnvelopeBody(t)
	localCalled := false
	ts := testServer(t, Config{
		ControlSnapshotFn: func(ctx context.Context) ([]byte, error) {
			if ctx == nil {
				t.Fatal("canonical control producer received nil request context")
			}
			return body, nil
		},
		FleetFn: func() (ledger.Snapshot, error) {
			localCalled = true
			return ledger.Snapshot{}, nil
		},
	})
	defer ts.Close()

	response, err := http.Get(ts.URL + "/api/control")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/control = %d, want 200", response.StatusCode)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", response.Header.Get("Cache-Control"))
	}
	got, err := json.Marshal(json.RawMessage(body))
	if err != nil {
		t.Fatal(err)
	}
	var returned json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&returned); err != nil {
		t.Fatal(err)
	}
	var wantEnvelope, gotEnvelope routerboard.ControlEnvelope
	if _, err := routerboard.DecodeControlEnvelope(body); err != nil {
		t.Fatalf("test envelope invalid: %v", err)
	}
	if err := json.Unmarshal(got, &wantEnvelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(returned, &gotEnvelope); err != nil {
		t.Fatal(err)
	}
	if gotEnvelope.Revision != 42 || gotEnvelope.StateSHA256 != wantEnvelope.StateSHA256 || gotEnvelope.State.Tasks[0].TaskID != "task-7" || gotEnvelope.State.Evidence[0].URL != "receipt://review" {
		t.Fatalf("canonical envelope was not preserved: %#v", gotEnvelope)
	}
	if localCalled {
		t.Fatal("local FleetFn ran while serving /api/control")
	}
	fleetResponse, err := http.Get(ts.URL + "/api/fleet")
	if err != nil {
		t.Fatal(err)
	}
	defer fleetResponse.Body.Close()
	if fleetResponse.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/fleet = %d, want legacy local fleet contract 200", fleetResponse.StatusCode)
	}
	var fleetBody map[string]json.RawMessage
	if err := json.NewDecoder(fleetResponse.Body).Decode(&fleetBody); err != nil {
		t.Fatal(err)
	}
	if _, ok := fleetBody["summary"]; !ok {
		t.Fatal("/api/fleet response lost its existing summary contract")
	}
	if !localCalled {
		t.Fatal("/api/fleet did not use its separately configured local producer")
	}
}

func TestControlAPIRequiresLocalCapabilityBeforeM5Read(t *testing.T) {
	const capability = "pantheon-dashboard-local-capability-test"
	body := testControlEnvelopeBody(t)
	called := false
	ts := testServer(t, Config{
		SNELocalAccessToken: capability,
		ControlSnapshotFn: func(context.Context) ([]byte, error) {
			called = true
			return body, nil
		},
	})
	defer ts.Close()

	unauthorized, err := http.Get(ts.URL + "/api/control")
	if err != nil {
		t.Fatal(err)
	}
	_ = unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized || called {
		t.Fatalf("unauthorized worker snapshot status=%d producer-called=%t", unauthorized.StatusCode, called)
	}

	request, err := http.NewRequest(http.MethodGet, ts.URL+"/api/control", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+capability)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !called {
		t.Fatalf("authorized worker snapshot status=%d producer-called=%t", response.StatusCode, called)
	}
}

func TestControlAPIFailureDoesNotBecomeLocalFleetData(t *testing.T) {
	ts := testServer(t, Config{
		ControlSnapshotFn: func(context.Context) ([]byte, error) {
			return nil, errors.New("M5 unavailable")
		},
	})
	defer ts.Close()

	response, err := http.Get(ts.URL + "/api/control")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("GET /api/control = %d, want 502", response.StatusCode)
	}
	var failure map[string]string
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if failure["error"] != "canonical worker snapshot unavailable" {
		t.Fatalf("failure = %#v", failure)
	}
	localResponse, err := http.Get(ts.URL + "/api/fleet")
	if err != nil {
		t.Fatal(err)
	}
	defer localResponse.Body.Close()
	if localResponse.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET /api/fleet = %d, want 503 when client mode has no local producer", localResponse.StatusCode)
	}
}

func TestControlAPIRejectsNonGETWithoutContactingM5(t *testing.T) {
	called := false
	ts := testServer(t, Config{
		ControlSnapshotFn: func(context.Context) ([]byte, error) {
			called = true
			return []byte(`{}`), nil
		},
	})
	defer ts.Close()

	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/control", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed || response.Header.Get("Allow") != http.MethodGet {
		t.Fatalf("POST /api/control status=%d allow=%q", response.StatusCode, response.Header.Get("Allow"))
	}
	if called {
		t.Fatal("non-GET request reached canonical M5 snapshot producer")
	}
}

func TestControlAPINotConfiguredHasDistinctFallbackStatus(t *testing.T) {
	ts := testServer(t, Config{})
	defer ts.Close()
	response, err := http.Get(ts.URL + "/api/control")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/control = %d, want 404 for absent remote source", response.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body["error"], "not configured") {
		t.Fatalf("not-configured response = %#v", body)
	}
}

func TestControlAPIRejectsInvalidCanonicalEnvelope(t *testing.T) {
	body := testControlEnvelopeBody(t)
	var envelope routerboard.ControlEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.StateSHA256 = strings.Repeat("0", 64)
	badBody, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	ts := testServer(t, Config{ControlSnapshotFn: func(context.Context) ([]byte, error) { return badBody, nil }})
	defer ts.Close()

	response, err := http.Get(ts.URL + "/api/control")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("GET /api/control = %d, want 502", response.StatusCode)
	}
	var failure map[string]string
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if failure["error"] != "canonical worker snapshot rejected" {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestControlAPIRedactsSnapshotProducerError(t *testing.T) {
	ts := testServer(t, Config{
		ControlSnapshotFn: func(context.Context) ([]byte, error) {
			return nil, errors.New("bearer=secret-token internal-host=router.local")
		},
	})
	defer ts.Close()

	response, err := http.Get(ts.URL + "/api/control")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadGateway || strings.Contains(string(body), "secret-token") || strings.Contains(string(body), "router.local") {
		t.Fatalf("snapshot producer failure was not safely redacted: status=%d body=%s", response.StatusCode, body)
	}
	if !strings.Contains(string(body), "canonical worker snapshot unavailable") {
		t.Fatalf("snapshot failure response lost stable client message: %s", body)
	}
}

func TestControlActionAPIRedactsUnexpectedProducerError(t *testing.T) {
	const capability = "local-capability"
	requestBody := `{"verb":"message","from":"m1","to":"ssa","title":"status"}`
	ts := testServer(t, Config{
		SNELocalAccessToken: capability,
		ControlActionFn: func(context.Context, []byte) ([]byte, error) {
			return nil, errors.New("bearer=secret-token internal-host=router.local")
		},
	})
	defer ts.Close()

	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/control/action", strings.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+capability)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadGateway || strings.Contains(string(body), "secret-token") || strings.Contains(string(body), "router.local") {
		t.Fatalf("action producer failure was not safely redacted: status=%d body=%s", response.StatusCode, body)
	}
	if !strings.Contains(string(body), "canonical worker action failed") {
		t.Fatalf("action failure response lost stable client message: %s", body)
	}
}
