package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
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
	for _, raw := range []string{
		"https://operator:secret@m5.example.test:8734",
		"https://operator@m5.example.test:8734",
	} {
		if _, err := controlEndpointURL(raw); err == nil || !strings.Contains(err.Error(), "embedded credentials") {
			t.Errorf("endpoint %q accepted embedded credentials or returned wrong error: %v", raw, err)
		}
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

func TestRemoteControlDoesNotFollowAuthenticatedRedirects(t *testing.T) {
	var redirected atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect-target" {
			redirected.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Redirect(w, r, "/redirect-target", http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	if _, err := fetchRemoteControl(context.Background(), server.URL, "test-token"); err == nil || !strings.Contains(err.Error(), "HTTP 307") {
		t.Fatalf("redirected control read error = %v, want HTTP 307 refusal", err)
	}
	request := []byte(`{"verb":"delegate","agent":"codex","task_id":"t-1","subject":"ship"}`)
	if _, err := sendRemoteControlAction(context.Background(), server.URL, "test-token", request); err == nil || !strings.Contains(err.Error(), "redirects are not followed") {
		t.Fatalf("redirected control action error = %v, want explicit redirect refusal", err)
	}
	if got := redirected.Load(); got != 0 {
		t.Fatalf("redirect target received %d requests, want none", got)
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
	for name, invalid := range map[string]string{
		"unknown field":      `{"verb":"delegate","agent":"a","task_id":"t","subject":"s","extra":true}`,
		"case variant field": `{"verb":"delegate","Agent":"a","task_id":"t","subject":"s"}`,
		"second JSON value":  `{"verb":"delegate","agent":"a","task_id":"t","subject":"s"}{}`,
		"invalid action":     `{"verb":"delegate","agent":"a","task_id":"t"}`,
	} {
		path := filepath.Join(t.TempDir(), name+".json")
		if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readControlActionRequest(path); err == nil {
			t.Errorf("%s request was accepted before submission", name)
		}
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
	if _, err := sendRemoteControlAction(context.Background(), badResponse.URL, "test-token", requestBody); err == nil || !strings.Contains(err.Error(), "not canonical") {
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
	} else {
		var rejection *routerboard.ControlActionFailureError
		if !errors.As(err, &rejection) || rejection == nil {
			t.Fatalf("canonical rejection lost its typed receipt: %T %v", err, err)
		}
		if err := rejection.Failure.VerifyControlActionFailure(requestBody); err != nil {
			t.Fatalf("returned failure receipt does not bind the original request: %v", err)
		}
	}
}

func TestRemoteControlClientRunsWorkerLifecycleAgainstCanonicalStore(t *testing.T) {
	store, err := routerstore.Open(filepath.Join(t.TempDir(), "canonical-router.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	board := routerboard.New("/bin/false", "", "control-e2e-test")
	handler := routerboard.NewHandlerWithInjectedControlStore(board, t.TempDir(), store, "m5-test-token")
	mux := http.NewServeMux()
	handler.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	send := func(raw string) routerboard.ControlActionResponse {
		t.Helper()
		body := []byte(raw)
		responseBody, sendErr := sendRemoteControlAction(context.Background(), server.URL, "m5-test-token", body)
		if sendErr != nil {
			t.Fatalf("send %s: %v", raw, sendErr)
		}
		var response routerboard.ControlActionResponse
		if decodeErr := json.Unmarshal(responseBody, &response); decodeErr != nil {
			t.Fatalf("decode response for %s: %v; body=%s", raw, decodeErr, responseBody)
		}
		if verifyErr := response.VerifyControlActionResponseForRequest(body); verifyErr != nil {
			t.Fatalf("verify response for %s: %v", raw, verifyErr)
		}
		return response
	}

	message := send(`{"verb":"message","from":"m1","to":"m5","title":"worker check-in","instructions":"inspect current state"}`)
	if message.ItemID == "" {
		t.Fatal("message response omitted canonical item ID")
	}
	review := send(`{"verb":"review_request","from":"m1","to":"ssa","title":"review control-plane handoff","type":"review","instructions":"verify the returned receipt"}`)
	if review.ItemID == "" || review.ItemID == message.ItemID {
		t.Fatalf("review response did not create its own canonical item: message=%q review=%q", message.ItemID, review.ItemID)
	}

	if got, err := store.ListAll(); err != nil || len(got) != 2 {
		t.Fatalf("canonical inbox items = %d, err=%v; want message and review", len(got), err)
	}

	if delegated := send(`{"verb":"delegate","agent":"codex-pantheon","task_id":"m1-handback","subject":"return an explicit handback"}`); delegated.TaskID != "m1-handback" {
		t.Fatalf("delegate response task = %q", delegated.TaskID)
	}
	handbackClaim := `{"verb":"claim","agent":"codex-pantheon","task_id":"m1-handback","worker":"m1-worker","thread_id":"m1-thread","ttl_seconds":60}`
	handbackLease := send(handbackClaim)
	if handbackLease.Lease == nil {
		t.Fatal("handback claim omitted lease")
	}
	retriedHandbackLease := send(handbackClaim)
	if retriedHandbackLease.Lease == nil || retriedHandbackLease.Lease.Token != handbackLease.Lease.Token || retriedHandbackLease.Lease.Attempt != handbackLease.Lease.Attempt || !retriedHandbackLease.Lease.Expires.Equal(handbackLease.Lease.Expires) {
		t.Fatalf("retry changed the handback lease: first=%+v retry=%+v", handbackLease.Lease, retriedHandbackLease.Lease)
	}
	if returned := send(`{"verb":"cancel_handback","agent":"codex-pantheon","task_id":"m1-handback","lease_token":"` + handbackLease.Lease.Token + `","reason":"handoff complete"}`); returned.TaskID != "m1-handback" {
		t.Fatalf("handback response task = %q", returned.TaskID)
	}
	handbackTask, err := store.GetTask("codex-pantheon", "m1-handback")
	if err != nil {
		t.Fatal(err)
	}
	if handbackTask.Status != "pending" || handbackTask.FailureReason != "handoff complete" {
		t.Fatalf("canonical handback state = status %q, reason %q", handbackTask.Status, handbackTask.FailureReason)
	}

	if delegated := send(`{"verb":"delegate","agent":"codex-pantheon","task_id":"m1-result","subject":"return a verified result"}`); delegated.TaskID != "m1-result" {
		t.Fatalf("result task delegation = %+v", delegated)
	}
	resultLease := send(`{"verb":"claim","agent":"codex-pantheon","task_id":"m1-result","worker":"m1-worker","thread_id":"m1-thread","ttl_seconds":60}`)
	if resultLease.Lease == nil {
		t.Fatal("result claim omitted lease")
	}
	result := send(`{"verb":"result_return","agent":"codex-pantheon","task_id":"m1-result","lease_token":"` + resultLease.Lease.Token + `","result_ref":"receipt://m1/result-1"}`)
	if result.ResultRef != "receipt://m1/result-1" {
		t.Fatalf("result response ref = %q", result.ResultRef)
	}
	completedTask, err := store.GetTask("codex-pantheon", "m1-result")
	if err != nil {
		t.Fatal(err)
	}
	if completedTask.Status != "done" {
		t.Fatalf("canonical result state = status %q, want done", completedTask.Status)
	}

	for _, taskID := range []string{"next-a", "next-b"} {
		if delegated := send(`{"verb":"delegate","agent":"codex-client","task_id":"` + taskID + `","subject":"` + taskID + `"}`); delegated.TaskID != taskID {
			t.Fatalf("next-task delegation = %+v, want %q", delegated, taskID)
		}
	}
	nextClaim := `{"verb":"claim","agent":"codex-client","worker":"m1-worker","thread_id":"m1-thread","ttl_seconds":60}`
	nextLease := send(nextClaim)
	if nextLease.Lease == nil || nextLease.Lease.TaskID != "next-a" {
		t.Fatalf("claim-next response = %+v, want next-a lease", nextLease)
	}
	retriedNextLease := send(nextClaim)
	if retriedNextLease.Lease == nil || retriedNextLease.Lease.TaskID != nextLease.Lease.TaskID || retriedNextLease.Lease.Token != nextLease.Lease.Token || retriedNextLease.Lease.Attempt != nextLease.Lease.Attempt || !retriedNextLease.Lease.Expires.Equal(nextLease.Lease.Expires) {
		t.Fatalf("claim-next retry changed the committed lease: first=%+v retry=%+v", nextLease.Lease, retriedNextLease.Lease)
	}
	nextTask, err := store.GetTask("codex-client", "next-b")
	if err != nil {
		t.Fatal(err)
	}
	if nextTask.Status != "pending" {
		t.Fatalf("retry consumed the next queued task: status=%q, want pending", nextTask.Status)
	}
}

func TestSendAndPrintRemoteControlActionPreservesCanonicalFailureReceipt(t *testing.T) {
	requestBody := []byte(`{"verb":"message","from":"m1","to":"ssa","title":"review status"}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/control/action" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		failure := routerboard.ControlActionFailure{
			Schema: routerboard.ControlFailureSchema, Authority: "canonical-routerstore",
			Verb: "message", Error: "canonical store rejected the message",
		}
		if err := failure.SealControlActionFailure(requestBody); err != nil {
			t.Fatalf("seal failure: %v", err)
		}
		w.WriteHeader(http.StatusConflict)
		if err := json.NewEncoder(w).Encode(failure); err != nil {
			t.Fatalf("write failure: %v", err)
		}
	}))
	defer server.Close()

	var output bytes.Buffer
	err := sendAndPrintRemoteControlAction(context.Background(), server.URL, "test-token", requestBody, &output)
	if err == nil || !strings.Contains(err.Error(), "canonical worker action rejected") {
		t.Fatalf("rejected action error = %v", err)
	}
	var failure routerboard.ControlActionFailure
	if err := json.Unmarshal(output.Bytes(), &failure); err != nil {
		t.Fatalf("CLI did not write one JSON failure receipt to its output: %v; output=%s", err, output.String())
	}
	if err := failure.VerifyControlActionFailure(requestBody); err != nil {
		t.Fatalf("CLI failure receipt is not bound to the exact request: %v", err)
	}
	if failure.Error != "canonical store rejected the message" {
		t.Fatalf("CLI failure reason = %q", failure.Error)
	}
}

func TestSendRemoteControlActionRejectsInvalidRequestBeforeNetwork(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	invalid := []byte(`{"verb":"delegate","agent":"a","task_id":"t"}`)
	if _, err := sendRemoteControlAction(context.Background(), server.URL, "test-token", invalid); err == nil || !strings.Contains(err.Error(), "delegate requires agent, task_id, and subject") {
		t.Fatalf("invalid request error = %v", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("invalid action reached remote endpoint %d times", got)
	}
	invalidUTF8 := append([]byte(`{"verb":"delegate","agent":"a","task_id":"t","subject":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`"}`)...)
	if _, err := sendRemoteControlAction(context.Background(), server.URL, "test-token", invalidUTF8); err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("invalid UTF-8 action error = %v", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("invalid UTF-8 action reached remote endpoint %d times", got)
	}
}

func TestRemoteControlRequiresBearerToken(t *testing.T) {
	if _, err := fetchRemoteControl(context.Background(), "https://m5.example.test:8734", ""); err == nil || !strings.Contains(err.Error(), "bearer token is required") {
		t.Fatalf("empty snapshot token was accepted: %v", err)
	}
	if _, err := sendRemoteControlAction(context.Background(), "https://m5.example.test:8734", "", []byte(`{"verb":"delegate","agent":"codex","task_id":"task-1","subject":"review"}`)); err == nil || !strings.Contains(err.Error(), "bearer token is required") {
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

func TestControlRouteFailsClosedUnlessAuthorityIsExplicit(t *testing.T) {
	tests := []struct {
		name          string
		endpoint      string
		clientOnly    bool
		localAuth     bool
		wantLocal     bool
		wantErrPhrase string
	}{
		{name: "remote endpoint selects M5", endpoint: "https://m5.example.test:8734", wantLocal: false},
		{name: "missing configuration rejects", wantErrPhrase: "authority is not configured"},
		{name: "client-only requires endpoint", clientOnly: true, wantErrPhrase: "requires an authenticated M5 endpoint"},
		{name: "explicit local authority", localAuth: true, wantLocal: true},
		{name: "remote and local authorities conflict", endpoint: "https://m5.example.test:8734", localAuth: true, wantErrPhrase: "cannot be combined"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotLocal, err := useLocalControlAuthority(tc.endpoint, tc.clientOnly, tc.localAuth)
			if tc.wantErrPhrase != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrPhrase) {
					t.Fatalf("route = local:%t err:%v, want error containing %q", gotLocal, err, tc.wantErrPhrase)
				}
				return
			}
			if err != nil || gotLocal != tc.wantLocal {
				t.Fatalf("route = local:%t err:%v, want local:%t", gotLocal, err, tc.wantLocal)
			}
		})
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
	invalidUTF8Response := append([]byte(`{"schema":"pantheon.worker-control/v1","authority":"canonical-routerstore","verb":"delegate","task_id":"t-1","invalid":"`), 0xff)
	invalidUTF8Response = append(invalidUTF8Response, []byte(`"}`)...)
	if err := validateRemoteControlActionResponse(request, invalidUTF8Response); err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("accepted invalid UTF-8 control response: %v", err)
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
