package routerboard

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestControlEndpointIsReadOnlyAndReturnsTheEnvelope(t *testing.T) {
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
	if get.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200: %s", get.Code, get.Body.String())
	}
	var envelope ControlEnvelope
	if err := json.Unmarshal(get.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("GET body is not control JSON: %v", err)
	}
	if envelope.Schema != ControlSchema || envelope.Authority != "canonical-routerstore" {
		t.Fatalf("GET identity = %q/%q", envelope.Schema, envelope.Authority)
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

func TestAuthenticatedControlActionsUseCanonicalStoreAndLeaseFence(t *testing.T) {
	store, err := routerstore.Open(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b := New("/bin/false", "", "test-build")
	h := NewHandlerWithControlStore(b, t.TempDir(), store, "test-token")
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
	claimedResponse := postControlAction(t, mux, "test-token", ControlActionRequest{
		Verb: "claim", Agent: "codex-pantheon", TaskID: "task-1", Worker: "m1-worker", ThreadID: "thread-1", TTLSeconds: 60,
	})
	if claimedResponse.Code != http.StatusOK {
		t.Fatalf("claim status = %d: %s", claimedResponse.Code, claimedResponse.Body.String())
	}
	if err := json.Unmarshal(claimedResponse.Body.Bytes(), &claimed); err != nil || claimed.Lease == nil || claimed.Lease.Token == "" {
		t.Fatalf("claim response = %s, err=%v", claimedResponse.Body.String(), err)
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
	h := NewHandlerWithControlStore(b, t.TempDir(), store, "test-token")
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
	h := NewHandlerWithControlStore(New("/bin/false", "", "test-build"), t.TempDir(), store, "test-token")
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

	semantic := httptest.NewRequest(http.MethodPost, "/api/control/action", bytes.NewBufferString(`{"verb":"message","from":"a","to":"b","title":"hello","task_id":"not-a-message-field"}`))
	semantic.Header.Set("Authorization", "Bearer test-token")
	semantic.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, semantic)
	if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte("does not accept task_id")) {
		t.Fatalf("cross-verb field response = %d %s", response.Code, response.Body.String())
	}

	noToken := NewHandlerWithControlStore(New("/bin/false", "", "test-build"), t.TempDir(), store, "")
	noTokenMux := http.NewServeMux()
	noToken.Register(noTokenMux)
	response = postControlAction(t, noTokenMux, "", ControlActionRequest{Verb: "delegate", Agent: "a", TaskID: "t", Subject: "s"})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing configured token status = %d, want 503", response.Code)
	}
}

func TestControlActionRequiresJSONContentType(t *testing.T) {
	store, err := routerstore.Open(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h := NewHandlerWithControlStore(New("/bin/false", "", "test-build"), t.TempDir(), store, "test-token")
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
	if err := response.VerifyControlActionResponse([]byte(`{"verb":"message","from":"m1","to":"m5","title":"tampered"}`)); err == nil {
		t.Fatal("tampered request accepted by receipt")
	}

	tampered := response
	tampered.ItemID = "item-2"
	if err := tampered.VerifyControlActionResponse(request); err == nil {
		t.Fatal("tampered response accepted by receipt")
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
