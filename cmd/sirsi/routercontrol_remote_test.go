package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/rolereceipt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
)

func TestControlEndpointURLCanonicalizesHostOnlyEndpoint(t *testing.T) {
	got, err := controlEndpointURL("https://m5.example.test:8734/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://m5.example.test:8734/api/control" {
		t.Fatalf("endpoint = %q", got)
	}
	if _, err := controlEndpointURL("file:///tmp/control"); err == nil {
		t.Fatal("accepted non-HTTP control endpoint")
	}
	if _, err := controlEndpointURL("https://m5.example.test/other"); err == nil {
		t.Fatal("accepted non-control endpoint path")
	}
}

func TestControlEndpointURLRestrictsPlainHTTPToLoopback(t *testing.T) {
	for _, raw := range []string{
		"http://localhost:8734",
		"http://127.0.0.1:8734",
		"http://127.255.255.254:8734/",
		"http://[::1]:8734",
	} {
		if got, err := controlEndpointURL(raw); err != nil {
			t.Errorf("loopback endpoint %q rejected: %v", raw, err)
		} else if !strings.HasSuffix(got, "/api/control") {
			t.Errorf("loopback endpoint %q = %q, missing canonical path", raw, got)
		}
	}
	for _, raw := range []string{
		"http://m5.example.test:8734",
		"http://100.92.193.26:8734",
		"http://[2001:db8::1]:8734",
	} {
		if _, err := controlEndpointURL(raw); err == nil || !strings.Contains(err.Error(), "use HTTPS") {
			t.Errorf("remote HTTP endpoint %q was accepted or had the wrong error: %v", raw, err)
		}
	}
}

func TestControlActionRequestAndRemoteSubmissionUseClosedEndpoint(t *testing.T) {
	requestBody := []byte(`{"verb":"delegate","agent":"codex","task_id":"t-1","subject":"ship"}`)
	requestFile := t.TempDir() + "/request.json"
	if err := os.WriteFile(requestFile, requestBody, 0o600); err != nil {
		t.Fatal(err)
	}
	gotBody, err := readControlActionRequest(requestFile)
	if err != nil || !bytes.Equal(gotBody, requestBody) {
		t.Fatalf("request body = %s, err=%v", gotBody, err)
	}
	duplicateFile := filepath.Join(t.TempDir(), "duplicate.json")
	if err := os.WriteFile(duplicateFile, []byte(`{"verb":"delegate","agent":"a","agent":"b","task_id":"t-1","subject":"ship"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readControlActionRequest(duplicateFile); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("duplicate request file was accepted: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/control/action" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("headers = %#v", r.Header)
		}
		received, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(received, requestBody) {
			t.Fatalf("received body = %s, err=%v", received, err)
		}
		response := routerboard.ControlActionResponse{
			Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Verb: "delegate", TaskID: "t-1",
		}
		if err := response.SealControlActionResponse(received); err != nil {
			t.Fatalf("seal response: %v", err)
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()
	response, err := sendRemoteControlAction(context.Background(), server.URL, "test-token", requestBody)
	if err != nil || !strings.Contains(string(response), `"task_id":"t-1"`) {
		t.Fatalf("response = %s, err=%v", response, err)
	}
	misbound := routerboard.ControlActionResponse{
		Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Verb: "delegate", TaskID: "different-task",
	}
	if err := misbound.SealControlActionResponse(requestBody); err != nil {
		t.Fatal(err)
	}
	misboundBody, err := json.Marshal(misbound)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteControlActionResponse(requestBody, misboundBody); err == nil || !strings.Contains(err.Error(), "does not match requested task_id") {
		t.Fatalf("accepted receipt-bound response for a different task: %v", err)
	}

	badResponse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"schema":"pantheon.worker-control/v1","authority":"untrusted","verb":"delegate","task_id":"t-1"}`))
	}))
	defer badResponse.Close()
	if _, err := sendRemoteControlAction(context.Background(), badResponse.URL, "test-token", requestBody); err == nil || !strings.Contains(err.Error(), "authority") {
		t.Fatalf("accepted untrusted action response: %v", err)
	}

	failureServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		failure := routerboard.ControlActionFailure{
			Schema: routerboard.ControlFailureSchema, Authority: "canonical-routerstore", Verb: "delegate", Error: "task already exists",
		}
		if err := failure.SealControlActionFailure(requestBody); err != nil {
			t.Fatalf("seal failure: %v", err)
		}
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(failure)
	}))
	defer failureServer.Close()
	if _, err := sendRemoteControlAction(context.Background(), failureServer.URL, "test-token", requestBody); err == nil || !strings.Contains(err.Error(), "task already exists") {
		t.Fatalf("failure receipt was not returned: %v", err)
	}
}

func TestRemoteControlRequiresBearerToken(t *testing.T) {
	if _, err := fetchRemoteControl(context.Background(), "https://m5.example.test:8734", ""); err == nil || !strings.Contains(err.Error(), "bearer token is required") {
		t.Fatalf("empty snapshot token was accepted: %v", err)
	}
	if _, err := sendRemoteControlAction(context.Background(), "https://m5.example.test:8734", "", []byte(`{"verb":"inspect"}`)); err == nil || !strings.Contains(err.Error(), "bearer token is required") {
		t.Fatalf("empty action token was accepted: %v", err)
	}
}

func TestControlClientOnlyEnvironmentIsClosed(t *testing.T) {
	for _, value := range []string{"1", "true", "YES", "on"} {
		if !controlClientOnlyEnv(value) {
			t.Errorf("%q was not recognized as client-only", value)
		}
	}
	for _, value := range []string{"", "0", "false", "off", "operator"} {
		if controlClientOnlyEnv(value) {
			t.Errorf("%q was incorrectly recognized as client-only", value)
		}
	}
}

func TestFetchRemoteControlUsesBearerAndRejectsInvalidResponse(t *testing.T) {
	state := routerboard.Payload{GeneratedAt: "2026-09-07T12:00:00Z", Evidence: []routerboard.EvidenceRef{}, Fleet: []routerboard.Lane{}, Activity: []routerboard.Event{}, DataErrors: []string{}, Threads: []routerboard.Thread{}, RegistrationGaps: []string{}, Tasks: []routerboard.TaskDetail{}}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	stateSum := sha256.Sum256(stateBytes)
	envelope := routerboard.ControlEnvelope{
		Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Revision: 7,
		GeneratedAt: state.GeneratedAt, StateSHA256: hex.EncodeToString(stateSum[:]), State: state,
		Capabilities: routerboard.ControlCapabilities(),
	}
	validBody, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/control" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(validBody)
	}))
	defer server.Close()

	body, err := fetchRemoteControl(context.Background(), server.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"revision":7`) {
		t.Fatalf("body = %s", body)
	}

	digestMismatch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		broken := envelope
		broken.StateSHA256 = strings.Repeat("0", 64)
		payload, _ := json.Marshal(broken)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer digestMismatch.Close()
	if _, err := fetchRemoteControl(context.Background(), digestMismatch.URL, "test-token"); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("accepted digest-mismatched snapshot: %v", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer bad.Close()
	if _, err := fetchRemoteControl(context.Background(), bad.URL, "test-token"); err == nil {
		t.Fatal("accepted invalid remote control JSON")
	}
}

func TestRemoteControlValidationRejectsDuplicateAndTrailingJSON(t *testing.T) {
	request := []byte(`{"verb":"delegate","agent":"codex","task_id":"t-1","subject":"ship"}`)
	duplicateResponse := []byte(`{"schema":"pantheon.worker-control/v1","authority":"canonical-routerstore","verb":"delegate","task_id":"t-1","task_id":"t-2"}`)
	if err := validateRemoteControlActionResponse(request, duplicateResponse); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("accepted duplicate action response: %v", err)
	}
	trailingResponse := []byte(`{"schema":"pantheon.worker-control/v1","authority":"canonical-routerstore","verb":"delegate","task_id":"t-1"} {}`)
	if err := validateRemoteControlActionResponse(request, trailingResponse); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("accepted trailing action response: %v", err)
	}
	if err := validateRemoteControlSnapshot([]byte(`{"schema":"pantheon.worker-control/v1","schema":"evil"}`)); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("accepted duplicate control snapshot: %v", err)
	}
}

func TestRemoteControlSnapshotBindsEnvelopeTimestampToState(t *testing.T) {
	state := routerboard.Payload{GeneratedAt: "2026-09-07T12:00:00Z"}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	stateSum := sha256.Sum256(stateBytes)
	envelope := routerboard.ControlEnvelope{
		Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Revision: 1,
		GeneratedAt: "2026-09-07T12:00:01Z", StateSHA256: hex.EncodeToString(stateSum[:]), State: state,
		Capabilities: routerboard.ControlCapabilities(),
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteControlSnapshot(body); err == nil || !strings.Contains(err.Error(), "generated_at") {
		t.Fatalf("accepted timestamp-mismatched snapshot: %v", err)
	}
}

func TestRemoteControlSnapshotRejectsMalformedObservationTimestamp(t *testing.T) {
	state := routerboard.Payload{GeneratedAt: "not-a-timestamp"}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	stateSum := sha256.Sum256(stateBytes)
	body, err := json.Marshal(routerboard.ControlEnvelope{
		Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Revision: 1,
		GeneratedAt: state.GeneratedAt, StateSHA256: hex.EncodeToString(stateSum[:]), State: state,
		Capabilities: routerboard.ControlCapabilities(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteControlSnapshot(body); err == nil || !strings.Contains(err.Error(), "RFC3339") {
		t.Fatalf("malformed observation timestamp was accepted: %v", err)
	}
}

func TestRemoteControlActionCanRequireExactRoleReceipt(t *testing.T) {
	request := []byte(`{"verb":"delegate","agent":"codex","task_id":"t-1","subject":"ship"}`)
	expectedID := "rr-m1-exact"
	expectedSHA := strings.Repeat("a", 64)
	response := routerboard.ControlActionResponse{
		Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Verb: "delegate", TaskID: "t-1",
		RoleReceiptID: expectedID, RoleReceiptSHA256: expectedSHA,
	}
	if err := response.SealControlActionResponse(request); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteControlActionResponseWithRole(request, body, expectedID, expectedSHA); err != nil {
		t.Fatalf("exact role receipt was rejected: %v", err)
	}
	response.RoleReceiptID = ""
	response.RoleReceiptSHA256 = ""
	if err := response.SealControlActionResponse(request); err != nil {
		t.Fatal(err)
	}
	body, err = json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteControlActionResponseWithRole(request, body, expectedID, expectedSHA); err == nil || !strings.Contains(err.Error(), "role receipt") {
		t.Fatalf("accepted response without the expected role receipt: %v", err)
	}
}

func TestExpectedControlRoleReferenceRejectsPartialOrNonCanonicalEnvironment(t *testing.T) {
	t.Setenv("SIRSI_CONTROL_ROLE_RECEIPT_ID", "rr-m1")
	t.Setenv("SIRSI_CONTROL_ROLE_RECEIPT_SHA256", "")
	if _, _, err := expectedControlRoleReference(); err == nil || !strings.Contains(err.Error(), "supplied together") {
		t.Fatalf("accepted partial role expectation: %v", err)
	}
	t.Setenv("SIRSI_CONTROL_ROLE_RECEIPT_ID", " rr-m1")
	t.Setenv("SIRSI_CONTROL_ROLE_RECEIPT_SHA256", strings.Repeat("a", 64))
	if _, _, err := expectedControlRoleReference(); err == nil || !strings.Contains(err.Error(), "whitespace") {
		t.Fatalf("accepted non-canonical role expectation: %v", err)
	}
}

func controlRoleReceiptFixture(t *testing.T) []byte {
	t.Helper()
	now := time.Date(2026, 9, 12, 7, 0, 0, 0, time.UTC)
	receipt := rolereceipt.Receipt{
		Schema: rolereceipt.Schema, ReceiptID: "rr-m1-transport", Role: rolereceipt.ConstrainedClient,
		HostProfile: rolereceipt.HostProfile{ID: "m1-test", OS: "macOS", Toolchain: "go", Transport: "tailscale"},
		Scope:       []string{"inspect", "delegate"}, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
		RevocationReference: "revocations-v1", Issuer: "ssa", KeyID: "test-key", PolicyVersion: "v1",
		ObservedState: rolereceipt.ObservedState{
			RouterNamespace: "sirsi-primary", ProtectedProcesses: []string{"codex"},
			ResourceFacts: []rolereceipt.ResourceFact{{Name: "swap", Value: "1"}},
		},
		Signature: "test-signature",
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestLoadControlRoleProofBindsExactReceiptFileBytes(t *testing.T) {
	raw := controlRoleReceiptFixture(t)
	path := filepath.Join(t.TempDir(), "role-receipt.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(controlRoleReceiptFileEnv, path)
	t.Setenv("SIRSI_CONTROL_ROLE_RECEIPT_ID", "")
	t.Setenv("SIRSI_CONTROL_ROLE_RECEIPT_SHA256", "")
	proof, err := loadControlRoleProof(true)
	if err != nil {
		t.Fatalf("loadControlRoleProof: %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(proof.header)
	if err != nil || !bytes.Equal(decoded, raw) {
		t.Fatalf("header bytes differ from receipt file: err=%v", err)
	}
	sum := sha256.Sum256(raw)
	if proof.expectedID != "rr-m1-transport" || proof.expectedSHA != hex.EncodeToString(sum[:]) {
		t.Fatalf("proof reference = %q/%q", proof.expectedID, proof.expectedSHA)
	}
	t.Setenv("SIRSI_CONTROL_ROLE_RECEIPT_ID", "rr-other")
	t.Setenv("SIRSI_CONTROL_ROLE_RECEIPT_SHA256", strings.Repeat("a", 64))
	if _, err := loadControlRoleProof(true); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("accepted mismatched expected reference: %v", err)
	}
}

func TestLoadControlRoleProofRequiresFileForAuthenticatedAction(t *testing.T) {
	t.Setenv(controlRoleReceiptFileEnv, "")
	t.Setenv("SIRSI_CONTROL_ROLE_RECEIPT_ID", "")
	t.Setenv("SIRSI_CONTROL_ROLE_RECEIPT_SHA256", "")
	if _, err := loadControlRoleProof(true); err == nil || !strings.Contains(err.Error(), controlRoleReceiptFileEnv) {
		t.Fatalf("accepted authenticated action without a receipt file: %v", err)
	}
}

func TestFetchRemoteControlWithRoleAndHeaderSendsExactReceipt(t *testing.T) {
	raw := controlRoleReceiptFixture(t)
	sum := sha256.Sum256(raw)
	roleSHA := hex.EncodeToString(sum[:])
	state := routerboard.Payload{GeneratedAt: "2026-09-12T07:00:00Z"}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	stateSum := sha256.Sum256(stateBytes)
	responseBody, err := json.Marshal(routerboard.ControlEnvelope{
		Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Revision: 1,
		GeneratedAt: state.GeneratedAt, StateSHA256: hex.EncodeToString(stateSum[:]),
		RoleReceiptID: "rr-m1-transport", RoleReceiptSHA256: roleSHA,
		Capabilities: routerboard.ControlCapabilities(), State: state,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, err := base64.RawURLEncoding.DecodeString(r.Header.Get(routerboard.ControlRoleReceiptHeader)); err != nil || !bytes.Equal(got, raw) {
			t.Fatalf("role receipt header differs from exact file: err=%v", err)
		}
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()
	proof := controlRoleProof{header: base64.RawURLEncoding.EncodeToString(raw), expectedID: "rr-m1-transport", expectedSHA: roleSHA}
	if _, err := fetchRemoteControlWithRoleAndHeader(context.Background(), server.URL, "test-token", proof.expectedID, proof.expectedSHA, proof.header); err != nil {
		t.Fatalf("fetchRemoteControlWithRoleAndHeader: %v", err)
	}
}

func TestRemoteControlActionArmResponseBindsRequestedAgent(t *testing.T) {
	request := []byte(`{"verb":"arm","agent":"claude-pantheon"}`)
	response := routerboard.ControlActionResponse{
		Schema: routerboard.ControlSchema, Authority: "canonical-routerstore", Verb: "arm", Agent: "claude-pantheon",
	}
	if err := response.SealControlActionResponse(request); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteControlActionResponse(request, body); err != nil {
		t.Fatalf("valid arm response rejected: %v", err)
	}
	response.Agent = "other-agent"
	if err := response.SealControlActionResponse(request); err != nil {
		t.Fatal(err)
	}
	body, err = json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteControlActionResponse(request, body); err == nil || !strings.Contains(err.Error(), "agent") {
		t.Fatalf("accepted arm response for another agent: %v", err)
	}
}

func TestRemoteControlFailureCanRequireExactRoleReceipt(t *testing.T) {
	request := []byte(`{"verb":"delegate","agent":"codex","task_id":"t-1","subject":"ship"}`)
	expectedID := "rr-m1-failure"
	expectedSHA := strings.Repeat("b", 64)
	failure := routerboard.ControlActionFailure{
		Schema: routerboard.ControlFailureSchema, Authority: "canonical-routerstore", Verb: "delegate", Error: "task unavailable",
		RoleReceiptID: expectedID, RoleReceiptSHA256: expectedSHA,
	}
	if err := failure.SealControlActionFailure(request); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(failure)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteControlActionFailureWithRole(request, body, expectedID, expectedSHA); err != nil {
		t.Fatalf("exact failure role receipt was rejected: %v", err)
	}
	failure.RoleReceiptID = ""
	failure.RoleReceiptSHA256 = ""
	if err := failure.SealControlActionFailure(request); err != nil {
		t.Fatal(err)
	}
	body, err = json.Marshal(failure)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteControlActionFailureWithRole(request, body, expectedID, expectedSHA); err == nil || !strings.Contains(err.Error(), "role receipt") {
		t.Fatalf("accepted failure without the expected role receipt: %v", err)
	}
}
