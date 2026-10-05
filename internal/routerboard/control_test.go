package routerboard

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestControlEnvelopeUsesCanonicalBoardStateAndCapabilities(t *testing.T) {
	b := New("/bin/false", "", "test-build")
	b.mu.Lock()
	b.version = 4
	b.payload = []byte(`{"build":"test-build","generated_at":"2026-09-07T12:00:00Z","evidence":[],"fleet":[],"activity":[],"data_errors":[],"threads":[],"registration_gaps":[],"tasks":[],"board":{},"ledger":{},"counters":{}}`)
	b.mu.Unlock()

	body, version, err := b.SnapshotControl()
	if err != nil {
		t.Fatalf("SnapshotControl: %v", err)
	}
	if version != 4 {
		t.Fatalf("version = %d, want 4", version)
	}
	var got ControlEnvelope
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode control envelope: %v", err)
	}
	if got.Schema != ControlSchema || got.Authority != "canonical-routerstore" {
		t.Fatalf("identity = %q/%q, want %q/canonical-routerstore", got.Schema, got.Authority, ControlSchema)
	}
	if got.State.Build != "test-build" || got.GeneratedAt != got.State.GeneratedAt {
		t.Fatalf("state was not preserved: %+v", got)
	}
	if got.Revision != 4 {
		t.Fatalf("revision = %d, want 4", got.Revision)
	}
	canonicalState, err := json.Marshal(got.State)
	if err != nil {
		t.Fatal(err)
	}
	stateSum := sha256.Sum256(canonicalState)
	if got.StateSHA256 != hex.EncodeToString(stateSum[:]) {
		t.Fatalf("state digest = %q, want canonical state digest", got.StateSHA256)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}
	if len(got.Capabilities) != 7 {
		t.Fatalf("capability count = %d, want 7", len(got.Capabilities))
	}
	seen := map[string]bool{}
	for _, capability := range got.Capabilities {
		if capability.Verb == "" || capability.Command == "" || seen[capability.Verb] {
			t.Fatalf("invalid or duplicate capability: %+v", capability)
		}
		seen[capability.Verb] = true
	}
}

func TestControlEnvelopeRejectsRegistryAndStateDrift(t *testing.T) {
	state := Payload{GeneratedAt: "2026-09-07T12:00:00Z"}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	stateSum := sha256.Sum256(stateBytes)
	valid := ControlEnvelope{
		Schema: ControlSchema, Authority: "canonical-routerstore", Revision: 1,
		GeneratedAt: state.GeneratedAt, StateSHA256: hex.EncodeToString(stateSum[:]), State: state,
		Capabilities: ControlCapabilities(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}
	cases := map[string]func(*ControlEnvelope){
		"missing capability":   func(e *ControlEnvelope) { e.Capabilities = e.Capabilities[:len(e.Capabilities)-1] },
		"duplicate capability": func(e *ControlEnvelope) { e.Capabilities = append(e.Capabilities, e.Capabilities[0]) },
		"unknown capability":   func(e *ControlEnvelope) { e.Capabilities[0].Verb = "unknown" },
		"mutated capability":   func(e *ControlEnvelope) { e.Capabilities[0].Command = "sirsi forged" },
		"state drift":          func(e *ControlEnvelope) { e.State.Build = "drift" },
		"identity drift":       func(e *ControlEnvelope) { e.Authority = "untrusted" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Capabilities = append([]ControlCapability(nil), valid.Capabilities...)
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("drifted control envelope was accepted")
			}
		})
	}
}

func TestSnapshotControlRefusesUnpolledBoard(t *testing.T) {
	b := New("/bin/false", "", "test-build")
	body, version, err := b.SnapshotControl()
	if err != nil {
		t.Fatalf("SnapshotControl: %v", err)
	}
	if body != nil || version != 0 {
		t.Fatalf("unpolled board returned data: version=%d body=%q", version, body)
	}
}

func TestSnapshotControlRejectsMalformedObservationTimestamp(t *testing.T) {
	b := New("/bin/false", "", "test-build")
	b.mu.Lock()
	b.version = 1
	b.payload = []byte(`{"generated_at":"not-a-timestamp","evidence":[],"fleet":[],"activity":[],"data_errors":[],"threads":[],"registration_gaps":[],"tasks":[],"board":{},"ledger":{},"counters":{}}`)
	b.mu.Unlock()
	if body, version, err := b.SnapshotControl(); err == nil || body != nil || version != 1 || !strings.Contains(err.Error(), "RFC3339") {
		t.Fatalf("malformed timestamp was accepted: body=%q version=%d err=%v", body, version, err)
	}
}

func TestDefaultControlEndpointWithoutTokenFailsClosed(t *testing.T) {
	t.Setenv("SIRSI_CONTROL_TOKEN", "")
	b := New("/bin/false", "", "test-build")
	b.mu.Lock()
	b.version = 1
	b.payload = []byte(`{"generated_at":"2026-09-07T12:00:00Z","evidence":[],"fleet":[],"activity":[],"data_errors":[],"threads":[],"registration_gaps":[],"tasks":[],"board":{},"ledger":{},"counters":{}}`)
	b.mu.Unlock()
	h := NewHandler(b, t.TempDir())
	mux := http.NewServeMux()
	h.Register(mux)

	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/control", nil))
	if get.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET status = %d, want 503 without SIRSI_CONTROL_TOKEN: %s", get.Code, get.Body.String())
	}

	post := httptest.NewRecorder()
	mux.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/control", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", post.Code)
	}
	if post.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST Allow = %q, want GET", post.Header().Get("Allow"))
	}
}

func TestDefaultControlEndpointRequiresExactBearerWhenConfigured(t *testing.T) {
	t.Setenv("SIRSI_CONTROL_TOKEN", "test-token")
	b := New("/bin/false", "", "test-build")
	b.mu.Lock()
	b.version = 1
	b.payload = []byte(`{"generated_at":"2026-09-07T12:00:00Z","evidence":[],"fleet":[],"activity":[],"data_errors":[],"threads":[],"registration_gaps":[],"tasks":[],"board":{},"ledger":{},"counters":{}}`)
	b.mu.Unlock()
	h := NewHandler(b, t.TempDir())
	mux := http.NewServeMux()
	h.Register(mux)

	missing := httptest.NewRecorder()
	mux.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/control", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer status = %d, want 401", missing.Code)
	}

	wrong := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	wrong.Header.Set("Authorization", "Bearer test-token-extra")
	wrongResponse := httptest.NewRecorder()
	mux.ServeHTTP(wrongResponse, wrong)
	if wrongResponse.Code != http.StatusUnauthorized {
		t.Fatalf("wrong bearer status = %d, want 401", wrongResponse.Code)
	}

	exact := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	exact.Header.Set("Authorization", "Bearer test-token")
	exactResponse := httptest.NewRecorder()
	mux.ServeHTTP(exactResponse, exact)
	if exactResponse.Code != http.StatusOK {
		t.Fatalf("exact bearer status = %d, want 200: %s", exactResponse.Code, exactResponse.Body.String())
	}
	var envelope ControlEnvelope
	if err := json.Unmarshal(exactResponse.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("exact bearer body is not control JSON: %v", err)
	}
	if envelope.Schema != ControlSchema || envelope.Authority != "canonical-routerstore" {
		t.Fatalf("exact bearer identity = %q/%q", envelope.Schema, envelope.Authority)
	}
}

func TestAuthenticatedControlActionsUseCanonicalStoreAndLeaseFence(t *testing.T) {
	store, err := routerstore.Open(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b := New("/bin/false", "", "test-build")
	h := NewHandlerWithInjectedControlStore(b, t.TempDir(), store, "test-token")
	mux := http.NewServeMux()
	h.Register(mux)

	unauthorized := postControlAction(t, mux, "", ControlActionRequest{
		Verb: "delegate", Agent: "codex-pantheon", TaskID: "task-1", Subject: "ship control plane",
	})
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorized.Code)
	}

	delegated := postControlAction(t, mux, "test-token", ControlActionRequest{
		Verb: "delegate", Agent: "codex-pantheon", TaskID: "task-1", Subject: "ship control plane",
	})
	if delegated.Code != http.StatusOK {
		t.Fatalf("delegate status = %d: %s", delegated.Code, delegated.Body.String())
	}

	var claimed ControlActionResponse
	claimRequest := ControlActionRequest{
		Verb: "claim", Agent: "codex-pantheon", TaskID: "task-1", Worker: "m1-worker", ThreadID: "thread-1", TTLSeconds: 60,
	}
	claimedResponse := postControlAction(t, mux, "test-token", claimRequest)
	if claimedResponse.Code != http.StatusOK {
		t.Fatalf("claim status = %d: %s", claimedResponse.Code, claimedResponse.Body.String())
	}
	if err := json.Unmarshal(claimedResponse.Body.Bytes(), &claimed); err != nil || claimed.Lease == nil || claimed.Lease.Token == "" {
		t.Fatalf("claim response = %s, err=%v", claimedResponse.Body.String(), err)
	}
	retriedClaimResponse := postControlAction(t, mux, "test-token", claimRequest)
	if retriedClaimResponse.Code != http.StatusOK {
		t.Fatalf("retried claim status = %d: %s", retriedClaimResponse.Code, retriedClaimResponse.Body.String())
	}
	var retriedClaim ControlActionResponse
	if err := json.Unmarshal(retriedClaimResponse.Body.Bytes(), &retriedClaim); err != nil || retriedClaim.Lease == nil {
		t.Fatalf("retried claim response = %s, err=%v", retriedClaimResponse.Body.String(), err)
	}
	if retriedClaim.Lease.Token != claimed.Lease.Token || retriedClaim.Lease.TaskID != claimed.Lease.TaskID || retriedClaim.Lease.Attempt != claimed.Lease.Attempt || !retriedClaim.Lease.Expires.Equal(claimed.Lease.Expires) {
		t.Fatalf("retry did not return the committed lease: first=%+v retry=%+v", claimed.Lease, retriedClaim.Lease)
	}

	completed := postControlAction(t, mux, "test-token", ControlActionRequest{
		Verb: "result_return", Agent: "codex-pantheon", TaskID: "task-1", LeaseToken: claimed.Lease.Token, ResultRef: "receipt://task-1",
	})
	if completed.Code != http.StatusOK {
		t.Fatalf("result_return status = %d: %s", completed.Code, completed.Body.String())
	}
	var completedBody ControlActionResponse
	if err := json.Unmarshal(completed.Body.Bytes(), &completedBody); err != nil || completedBody.ResultRef != "receipt://task-1" {
		t.Fatalf("result return response = %s, err=%v", completed.Body.String(), err)
	}
	got, err := store.GetTask("codex-pantheon", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "done" {
		t.Fatalf("task status = %q, want done", got.Status)
	}
}

func TestInjectedControlStoreRequiresConfiguredTokenForReadSnapshot(t *testing.T) {
	store, err := routerstore.Open(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b := New("/bin/false", "", "test-build")
	b.mu.Lock()
	b.version = 1
	b.payload = []byte(`{"generated_at":"2026-09-07T12:00:00Z","evidence":[],"fleet":[],"activity":[],"data_errors":[],"threads":[],"registration_gaps":[],"tasks":[],"board":{},"ledger":{},"counters":{}}`)
	b.mu.Unlock()
	h := NewHandlerWithInjectedControlStore(b, t.TempDir(), store, "test-token")
	mux := http.NewServeMux()
	h.Register(mux)

	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/control", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized snapshot status = %d, want 401", unauthorized.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer test-token")
	authorized := httptest.NewRecorder()
	mux.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized snapshot status = %d: %s", authorized.Code, authorized.Body.String())
	}
}

func TestControlActionRejectsUnknownFieldsAndMissingAuthorization(t *testing.T) {
	store, err := routerstore.Open(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h := NewHandlerWithInjectedControlStore(New("/bin/false", "", "test-build"), t.TempDir(), store, "test-token")
	mux := http.NewServeMux()
	h.Register(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/control/action", bytes.NewBufferString(`{"verb":"delegate","agent":"a","task_id":"t","subject":"s","unexpected":true}`))
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want 400", response.Code)
	}

	duplicate := httptest.NewRequest(http.MethodPost, "/api/control/action", bytes.NewBufferString(`{"verb":"delegate","agent":"a","agent":"b","task_id":"t","subject":"s"}`))
	duplicate.Header.Set("Authorization", "Bearer test-token")
	duplicate.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, duplicate)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte("duplicate object key")) {
		t.Fatalf("duplicate field response = %d %s", response.Code, response.Body.String())
	}

	caseAlias := httptest.NewRequest(http.MethodPost, "/api/control/action", bytes.NewBufferString(`{"verb":"delegate","Agent":"other","agent":"a","task_id":"t","subject":"s"}`))
	caseAlias.Header.Set("Authorization", "Bearer test-token")
	caseAlias.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, caseAlias)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("case-variant action field status = %d, want 400: %s", response.Code, response.Body.String())
	}

	invalidUTF8 := append([]byte(`{"verb":"delegate","agent":"a","task_id":"invalid-utf8","subject":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`"}`)...)
	invalidRequest := httptest.NewRequest(http.MethodPost, "/api/control/action", bytes.NewReader(invalidUTF8))
	invalidRequest.Header.Set("Authorization", "Bearer test-token")
	invalidRequest.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, invalidRequest)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid UTF-8 action status = %d, want 400: %s", response.Code, response.Body.String())
	}
	if _, err := store.GetTask("a", "invalid-utf8"); err == nil {
		t.Fatal("invalid UTF-8 action mutated the canonical task store")
	}

	semantic := httptest.NewRequest(http.MethodPost, "/api/control/action", bytes.NewBufferString(`{"verb":"message","from":"a","to":"b","title":"hello","task_id":"not-a-message-field"}`))
	semantic.Header.Set("Authorization", "Bearer test-token")
	semantic.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, semantic)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte("does not accept task_id")) {
		t.Fatalf("cross-verb field response = %d %s", response.Code, response.Body.String())
	}

	conflictingReviewType := httptest.NewRequest(http.MethodPost, "/api/control/action", bytes.NewBufferString(`{"verb":"review_request","from":"a","to":"b","title":"review","type":"message"}`))
	conflictingReviewType.Header.Set("Authorization", "Bearer test-token")
	conflictingReviewType.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, conflictingReviewType)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte("only supports type")) {
		t.Fatalf("conflicting review type status/body = %d %s", response.Code, response.Body.String())
	}
	if inbox, err := store.Inbox("b"); err != nil || len(inbox) != 0 {
		t.Fatalf("conflicting review type mutated the canonical store: inbox=%+v err=%v", inbox, err)
	}
	if _, err := DecodeControlActionRequest([]byte(`{"verb":"review_request","from":"a","to":"b","title":"review","type":"review"}`)); err != nil {
		t.Fatalf("fixed review type was rejected: %v", err)
	}

	noToken := NewHandlerWithInjectedControlStore(New("/bin/false", "", "test-build"), t.TempDir(), store, "")
	noTokenMux := http.NewServeMux()
	noToken.Register(noTokenMux)
	response = postControlAction(t, noTokenMux, "", ControlActionRequest{Verb: "delegate", Agent: "a", TaskID: "t", Subject: "s"})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing configured token status = %d, want 503", response.Code)
	}
}

func TestControlActionBodyReadErrorsFailBeforeStoreMutation(t *testing.T) {
	store, err := routerstore.Open(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h := NewHandlerWithInjectedControlStore(New("/bin/false", "", "test-build"), t.TempDir(), store, "test-token")
	mux := http.NewServeMux()
	h.Register(mux)

	oversized := []byte(`{"verb":"delegate","agent":"a","task_id":"oversized","subject":"` + strings.Repeat("x", ControlActionBodyLimit) + `"}`)
	oversizedRequest := httptest.NewRequest(http.MethodPost, "/api/control/action", bytes.NewReader(oversized))
	oversizedRequest.Header.Set("Authorization", "Bearer test-token")
	oversizedRequest.Header.Set("Content-Type", "application/json")
	oversizedResponse := httptest.NewRecorder()
	mux.ServeHTTP(oversizedResponse, oversizedRequest)
	if oversizedResponse.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized action status = %d, want 413: %s", oversizedResponse.Code, oversizedResponse.Body.String())
	}
	if _, err := store.GetTask("a", "oversized"); err == nil {
		t.Fatal("oversized action mutated the canonical task store")
	}

	readErrorRequest := httptest.NewRequest(http.MethodPost, "/api/control/action", nil)
	readErrorRequest.Body = controlActionFailingBody{}
	readErrorRequest.Header.Set("Authorization", "Bearer test-token")
	readErrorRequest.Header.Set("Content-Type", "application/json")
	readErrorResponse := httptest.NewRecorder()
	mux.ServeHTTP(readErrorResponse, readErrorRequest)
	if readErrorResponse.Code != http.StatusBadRequest || !strings.Contains(readErrorResponse.Body.String(), "could not read control action body") {
		t.Fatalf("unreadable action status/body = %d %s", readErrorResponse.Code, readErrorResponse.Body.String())
	}
}

type controlActionFailingBody struct{}

func (controlActionFailingBody) Read([]byte) (int, error) {
	return 0, errors.New("injected body read failure")
}
func (controlActionFailingBody) Close() error { return nil }

func TestDecodeControlActionRejectsInvalidUTF8(t *testing.T) {
	body := append([]byte(`{"verb":"message","title":"`), 0xff)
	body = append(body, []byte(`"}`)...)
	if _, err := DecodeControlActionRequest(body); err == nil {
		t.Fatal("DecodeControlActionRequest accepted invalid UTF-8")
	}
}

func TestDecodeControlActionRejectsNullInsteadOfOmittedFields(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		field string
	}{
		{
			name:  "optional string",
			body:  `{"verb":"message","from":"m1","to":"m5","title":"status","instructions":null}`,
			field: "instructions",
		},
		{
			name:  "optional number",
			body:  `{"verb":"claim","agent":"codex","worker":"m1","thread_id":"thread-1","ttl_seconds":null}`,
			field: "ttl_seconds",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeControlActionRequest([]byte(test.body))
			if err == nil || !strings.Contains(err.Error(), `field "`+test.field+`" must be omitted instead of null`) {
				t.Fatalf("DecodeControlActionRequest error = %v, want explicit null rejection for %s", err, test.field)
			}
		})
	}
}

func TestValidateJSONNoDuplicateKeysRejectsInvalidUTF8(t *testing.T) {
	body := append([]byte(`{"value":"`), 0xff)
	body = append(body, []byte(`"}`)...)
	if err := ValidateJSONNoDuplicateKeys(body); err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("invalid UTF-8 JSON error = %v", err)
	}
}

func TestControlActionRequiresJSONContentType(t *testing.T) {
	store, err := routerstore.Open(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h := NewHandlerWithInjectedControlStore(New("/bin/false", "", "test-build"), t.TempDir(), store, "test-token")
	mux := http.NewServeMux()
	h.Register(mux)
	request := httptest.NewRequest(http.MethodPost, "/api/control/action", bytes.NewBufferString(`{"verb":"delegate","agent":"a","task_id":"t","subject":"s"}`))
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("missing content type status = %d, want 415", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/control/action", bytes.NewBufferString(`{"verb":"delegate","agent":"a","task_id":"t","subject":"s"}`))
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("parameterized JSON content type status = %d: %s", response.Code, response.Body.String())
	}
}

func TestControlActionCanonicalizesLedgerFieldsAndRequiresHandbackReason(t *testing.T) {
	store, err := routerstore.Open(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	response, err := ApplyControlAction(store, ControlActionRequest{
		Verb: " delegate ", Agent: " worker ", TaskID: " task-1 ", Subject: " ship the control plane ",
	})
	if err != nil {
		t.Fatalf("delegate: %v", err)
	}
	if response.TaskID != "task-1" {
		t.Fatalf("task id = %q, want task-1", response.TaskID)
	}
	task, err := store.GetTask("worker", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if task.Subject != "ship the control plane" {
		t.Fatalf("subject = %q, want canonical whitespace", task.Subject)
	}

	lease, err := ApplyControlAction(store, ControlActionRequest{
		Verb: "claim", Agent: "worker", TaskID: "task-1", Worker: "m1", ThreadID: "thread-1",
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if lease.Lease == nil || lease.Lease.Token == "" {
		t.Fatalf("claim response missing lease: %+v", lease)
	}
	if _, err := ApplyControlAction(store, ControlActionRequest{
		Verb: "cancel_handback", Agent: "worker", TaskID: "task-1", LeaseToken: lease.Lease.Token,
	}); err == nil {
		t.Fatal("cancel_handback without reason unexpectedly succeeded")
	}
	if _, err := ApplyControlAction(store, ControlActionRequest{
		Verb: "cancel_handback", Agent: " worker ", TaskID: " task-1 ", LeaseToken: " " + lease.Lease.Token + " ", Reason: " retry after review ",
	}); err != nil {
		t.Fatalf("cancel_handback: %v", err)
	}
	task, err = store.GetTask("worker", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "pending" {
		t.Fatalf("status = %q, want pending", task.Status)
	}
	if task.FailureReason != "retry after review" {
		t.Fatalf("failure reason = %q, want canonical handback reason", task.FailureReason)
	}
}

func TestControlActionReceiptBindsExactRequestAndResponse(t *testing.T) {
	response := ControlActionResponse{
		Schema: ControlSchema, Authority: "canonical-routerstore", Verb: "message", ItemID: "item-1",
	}
	request := []byte(`{"verb":"message","from":"m1","to":"m5","title":"inspect"}`)
	if err := response.SealControlActionResponse(request); err != nil {
		t.Fatal(err)
	}
	if err := response.VerifyControlActionResponse(request); err != nil {
		t.Fatalf("verify receipt: %v", err)
	}
	if err := response.VerifyControlActionResponseForRequest(request); err != nil {
		t.Fatalf("verify action-specific outcome: %v", err)
	}
	if err := response.VerifyControlActionResponse([]byte(`{"verb":"message","from":"m1","to":"m5","title":"tampered"}`)); err == nil {
		t.Fatal("tampered request accepted by receipt")
	}

	tampered := response
	tampered.ItemID = "item-2"
	if err := tampered.VerifyControlActionResponse(request); err == nil {
		t.Fatal("tampered response accepted by receipt")
	}
}

func TestControlActionReceiptRequiresRequestBoundOutcome(t *testing.T) {
	tests := []struct {
		name     string
		request  string
		response ControlActionResponse
	}{
		{
			name:    "message requires item identity",
			request: `{"verb":"message","from":"m1","to":"m5","title":"inspect"}`,
			response: ControlActionResponse{
				Schema: ControlSchema, Authority: "canonical-routerstore", Verb: "message",
			},
		},
		{
			name:    "task result must match requested task",
			request: `{"verb":"delegate","agent":"worker","task_id":"task-1","subject":"review"}`,
			response: ControlActionResponse{
				Schema: ControlSchema, Authority: "canonical-routerstore", Verb: "delegate", TaskID: "task-2",
			},
		},
		{
			name:    "claim lease must match worker identity",
			request: `{"verb":"claim","agent":"worker","worker":"m1","thread_id":"thread-1"}`,
			response: ControlActionResponse{
				Schema: ControlSchema, Authority: "canonical-routerstore", Verb: "claim", TaskID: "task-1",
				Lease: &routerstore.TaskLease{
					TaskID: "task-1", Agent: "worker", Worker: "different-worker", ThreadID: "thread-1",
					Token: "lease-token", Expires: time.Now().Add(time.Minute), Attempt: 1,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestBody := []byte(test.request)
			if err := test.response.SealControlActionResponse(requestBody); err != nil {
				t.Fatalf("seal response: %v", err)
			}
			if err := test.response.VerifyControlActionResponseForRequest(requestBody); err == nil {
				t.Fatal("request-detached outcome was accepted")
			}
		})
	}
}

func TestControlActionClaimReceiptRequiresExactLeaseIdentity(t *testing.T) {
	requestBody := []byte(`{"verb":"claim","agent":"worker","worker":"m1","thread_id":"thread-1","task_id":"task-1"}`)
	valid := ControlActionResponse{
		Schema: ControlSchema, Authority: "canonical-routerstore", Verb: "claim", TaskID: "task-1",
		Lease: &routerstore.TaskLease{
			Agent: "worker", TaskID: "task-1", Worker: "m1", ThreadID: "thread-1",
			Token: "lease-token", Expires: time.Now().Add(time.Minute), Attempt: 1,
		},
	}
	tests := []struct {
		name   string
		mutate func(*ControlActionResponse)
	}{
		{name: "missing lease", mutate: func(r *ControlActionResponse) { r.Lease = nil }},
		{name: "missing lease token", mutate: func(r *ControlActionResponse) { r.Lease.Token = " " }},
		{name: "agent mismatch", mutate: func(r *ControlActionResponse) { r.Lease.Agent = "other-agent" }},
		{name: "worker mismatch", mutate: func(r *ControlActionResponse) { r.Lease.Worker = "other-worker" }},
		{name: "thread mismatch", mutate: func(r *ControlActionResponse) { r.Lease.ThreadID = "other-thread" }},
		{name: "requested task mismatch", mutate: func(r *ControlActionResponse) { r.Lease.TaskID = "other-task" }},
		{name: "response task detached from lease", mutate: func(r *ControlActionResponse) { r.TaskID = "other-task" }},
		{name: "zero expiry", mutate: func(r *ControlActionResponse) { r.Lease.Expires = time.Time{} }},
		{name: "zero attempt", mutate: func(r *ControlActionResponse) { r.Lease.Attempt = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := valid
			lease := *valid.Lease
			response.Lease = &lease
			test.mutate(&response)
			if err := response.SealControlActionResponse(requestBody); err != nil {
				t.Fatalf("seal response: %v", err)
			}
			if err := response.VerifyControlActionResponseForRequest(requestBody); err == nil {
				t.Fatal("claim response with detached or incomplete lease proof was accepted")
			}
		})
	}
}

func TestControlActionReceiptRejectsNonCanonicalIdentity(t *testing.T) {
	request := []byte(`{"verb":"message","from":"m1","to":"m5","title":"inspect"}`)
	valid := ControlActionResponse{
		Schema: ControlSchema, Authority: "canonical-routerstore", Verb: "message", ItemID: "item-1",
	}
	if err := valid.SealControlActionResponse(request); err != nil {
		t.Fatal(err)
	}
	for name, mutated := range map[string]ControlActionResponse{
		"wrong schema":      func() ControlActionResponse { v := valid; v.Schema = "other/v1"; return v }(),
		"wrong authority":   func() ControlActionResponse { v := valid; v.Authority = "other"; return v }(),
		"unknown verb":      func() ControlActionResponse { v := valid; v.Verb = "unknown"; return v }(),
		"noncanonical verb": func() ControlActionResponse { v := valid; v.Verb = " message "; return v }(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := mutated.VerifyControlActionResponse(request); err == nil {
				t.Fatal("non-canonical response identity was accepted")
			}
		})
	}

	failure := ControlActionFailure{
		Schema: ControlFailureSchema, Authority: "canonical-routerstore", Verb: "message", Error: "rejected",
	}
	if err := failure.SealControlActionFailure(request); err != nil {
		t.Fatal(err)
	}
	if err := failure.VerifyControlActionFailure(request); err != nil {
		t.Fatalf("valid failure receipt rejected: %v", err)
	}
	for name, mutated := range map[string]ControlActionFailure{
		"wrong schema":      func() ControlActionFailure { v := failure; v.Schema = "other/v1"; return v }(),
		"wrong authority":   func() ControlActionFailure { v := failure; v.Authority = "other"; return v }(),
		"unknown verb":      func() ControlActionFailure { v := failure; v.Verb = "unknown"; return v }(),
		"noncanonical verb": func() ControlActionFailure { v := failure; v.Verb = " message "; return v }(),
	} {
		t.Run("failure "+name, func(t *testing.T) {
			if err := mutated.VerifyControlActionFailure(request); err == nil {
				t.Fatal("non-canonical failure identity was accepted")
			}
		})
	}

	preDecodeFailure := failure
	preDecodeFailure.Verb = ""
	preDecodeFailure.ReceiptSHA256 = ""
	if err := preDecodeFailure.SealControlActionFailure(request); err != nil {
		t.Fatal(err)
	}
	if err := preDecodeFailure.VerifyControlActionFailure(request); err != nil {
		t.Fatalf("empty pre-decode failure verb rejected: %v", err)
	}
}

func TestProtectedControlInspectionRequiresBearerToken(t *testing.T) {
	store, err := routerstore.Open(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h := NewHandlerWithControlAuth(New("/bin/false", "", "test-build"), t.TempDir(), "test-token", true)
	h.openControlStore = func() (*routerstore.Store, bool, error) { return store, false, nil }
	h.board.mu.Lock()
	h.board.version = 1
	h.board.payload = []byte(`{"generated_at":"2026-09-07T12:00:00Z","evidence":[],"fleet":[],"activity":[],"data_errors":[],"threads":[],"registration_gaps":[],"tasks":[],"board":{},"ledger":{},"counters":{}}`)
	h.board.mu.Unlock()
	mux := http.NewServeMux()
	h.Register(mux)

	missing := httptest.NewRecorder()
	mux.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/control", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("protected unauthenticated GET = %d, want 401", missing.Code)
	}
	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer test-token")
	authorized := httptest.NewRecorder()
	mux.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("protected authenticated GET = %d: %s", authorized.Code, authorized.Body.String())
	}
}

func TestControlAuthorizationRejectsDuplicateOrNonExactBearerHeaders(t *testing.T) {
	store, err := routerstore.Open(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h := NewHandlerWithControlAuth(New("/bin/false", "", "test-build"), t.TempDir(), "test-token", true)
	h.openControlStore = func() (*routerstore.Store, bool, error) { return store, false, nil }
	h.board.mu.Lock()
	h.board.version = 1
	h.board.payload = []byte(`{"generated_at":"2026-09-07T12:00:00Z","evidence":[],"fleet":[],"activity":[],"data_errors":[],"threads":[],"registration_gaps":[],"tasks":[],"board":{},"ledger":{},"counters":{}}`)
	h.board.mu.Unlock()
	mux := http.NewServeMux()
	h.Register(mux)

	duplicate := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	duplicate.Header.Add("Authorization", "Bearer test-token")
	duplicate.Header.Add("Authorization", "Bearer test-token")
	duplicateResponse := httptest.NewRecorder()
	mux.ServeHTTP(duplicateResponse, duplicate)
	if duplicateResponse.Code != http.StatusUnauthorized {
		t.Fatalf("duplicate authorization status = %d, want 401", duplicateResponse.Code)
	}

	trailing := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	trailing.Header.Set("Authorization", "Bearer test-token ")
	trailingResponse := httptest.NewRecorder()
	mux.ServeHTTP(trailingResponse, trailing)
	if trailingResponse.Code != http.StatusUnauthorized {
		t.Fatalf("non-exact authorization status = %d, want 401", trailingResponse.Code)
	}
}

func postControlAction(t *testing.T, mux *http.ServeMux, token string, request ControlActionRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/api/control/action", bytes.NewReader(body))
	if token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+token)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httpRequest)
	return response
}
