package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
)

func TestDashboardControlSnapshotProducerIsExplicitAndAuthenticated(t *testing.T) {
	if got := dashboardControlSnapshotProducer("", "", false); got != nil {
		t.Fatal("default local dashboard unexpectedly selected remote control")
	}

	missingEndpoint := dashboardControlSnapshotProducer("", "", true)
	if missingEndpoint == nil {
		t.Fatal("client-only dashboard did not install a fail-closed producer")
	}
	if _, err := missingEndpoint(context.Background()); err == nil || !strings.Contains(err.Error(), "requires an authenticated M5 endpoint") {
		t.Fatalf("client-only dashboard error = %v", err)
	}

	state := routerboard.Payload{
		GeneratedAt: "2026-09-23T12:00:00Z",
		Evidence:    []routerboard.EvidenceRef{}, Activity: []routerboard.Event{}, Fleet: []routerboard.Lane{},
		DataErrors: []string{}, Threads: []routerboard.Thread{}, RegistrationGaps: []string{}, Tasks: []routerboard.TaskDetail{},
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(stateBytes)
	body, err := json.Marshal(routerboard.ControlEnvelope{
		Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Revision: 1,
		GeneratedAt: state.GeneratedAt, StateSHA256: hex.EncodeToString(digest[:]),
		Capabilities: routerboard.ControlCapabilities(), State: state,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/control" || r.Header.Get("Authorization") != "Bearer m5-token" {
			t.Errorf("request = %s %s authorization=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	producer := dashboardControlSnapshotProducer(server.URL, "m5-token", false)
	if producer == nil {
		t.Fatal("configured M5 endpoint did not select canonical remote source")
	}
	got, err := producer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := routerboard.DecodeControlEnvelope(got); err != nil {
		t.Fatalf("dashboard producer returned invalid canonical envelope: %v", err)
	}
	if !json.Valid(got) {
		t.Fatal("dashboard producer returned invalid JSON")
	}
}

func TestDashboardControlActionProducerUsesAuthenticatedM5Receipt(t *testing.T) {
	if got := dashboardControlActionProducer("", "", false); got != nil {
		t.Fatal("default local dashboard unexpectedly selected remote worker mutations")
	}

	missingEndpoint := dashboardControlActionProducer("", "", true)
	if missingEndpoint == nil {
		t.Fatal("client-only dashboard did not install a fail-closed action producer")
	}
	if _, err := missingEndpoint(context.Background(), []byte(`{"verb":"message","from":"m1","to":"ssa","title":"hello"}`)); err == nil || !strings.Contains(err.Error(), "requires an authenticated M5 endpoint") {
		t.Fatalf("client-only action error = %v", err)
	}

	requestBody := []byte(`{"verb":"message","from":"m1","to":"ssa","title":"hello"}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/control/action" || r.Header.Get("Authorization") != "Bearer m5-token" {
			t.Errorf("M5 action request = %s %s authorization=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var request routerboard.ControlActionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode action: %v", err)
		}
		response := routerboard.ControlActionResponse{
			Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Verb: request.Verb, ItemID: "item-42",
		}
		if err := response.SealControlActionResponse(requestBody); err != nil {
			t.Errorf("seal receipt: %v", err)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	producer := dashboardControlActionProducer(server.URL, "m5-token", false)
	got, err := producer(context.Background(), requestBody)
	if err != nil {
		t.Fatal(err)
	}
	var receipt routerboard.ControlActionResponse
	if err := json.Unmarshal(got, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.ItemID != "item-42" || receipt.Verb != "message" {
		t.Fatalf("receipt = %#v", receipt)
	}
	if err := receipt.VerifyControlActionResponse(requestBody); err != nil {
		t.Fatalf("request-bound receipt rejected: %v", err)
	}
}
