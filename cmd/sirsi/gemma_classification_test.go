package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGemmaStructuredBrokerClassification(t *testing.T) {
	for _, tc := range []struct {
		status          int
		code            string
		permanent, busy bool
	}{{400, "context_length_exceeded", true, false}, {422, "context_mismatch", true, false}, {503, "BUSY", false, true}, {401, "unauthorized", true, false}} {
		t.Run(tc.code, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(tc.status)
				fmt.Fprintf(w, `{"error":{"code":%q,"type":"inference_error","message":"bounded structured evidence"}}`, tc.code)
			}))
			defer srv.Close()
			_, err := gemmaWarmCompleteProfile(context.Background(), srv.URL, "exact-model", "input", 256, "")
			var e *gemmaInferenceError
			if !errors.As(err, &e) || e.Code != tc.code || e.Permanent != tc.permanent || e.Busy != tc.busy {
				t.Fatalf("classification mismatch: %v", err)
			}
			if e.Busy && e.RetryAfter != 2*time.Second {
				t.Fatal("Retry-After lost")
			}
		})
	}
}

func TestGemmaWorkerProfileTimeoutRenewsAndRetriesExactLease(t *testing.T) {
	oldProfile, oldBudget, oldToken := gemmaWorkerSelectedProfile, gemmaWorkerMaxTokens, gemmaWorkerTokenFile
	t.Cleanup(func() {
		gemmaWorkerSelectedProfile = oldProfile
		gemmaWorkerMaxTokens = oldBudget
		gemmaWorkerTokenFile = oldToken
	})
	canceled := make(chan struct{}, 1)
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{"data":[{"id":"exact"}]}`)
			return
		}
		posts.Add(1)
		var req struct{ Messages []map[string]string }
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) != 2 || req.Messages[0]["content"] == "" || !strings.Contains(req.Messages[1]["content"], "TASK: draft") {
			t.Error("identity/input changed")
		}
		<-r.Context().Done()
		canceled <- struct{}{}
	}))
	defer srv.Close()
	seconds := 1
	gemmaWorkerSelectedProfile = &provider.LocalProfile{Endpoint: srv.URL + "/v1", Model: "exact", RequestTimeoutSeconds: &seconds}
	gemmaWorkerMaxTokens = 256
	gemmaWorkerTokenFile = ""
	s, r := claimedGemmaItem()
	w := testGemmaWorker(s, r, gemmaWorkerGenerate)
	w.leaseTTL = 3 * time.Second
	start := time.Now()
	_, e := w.runOnce(context.Background())
	if e == nil || posts.Load() != 1 || s.blocked != "" || s.failed == "" || r.calls != 0 || s.renews < 1 || s.failID != s.lease.ItemID || s.failToken != s.lease.Token {
		t.Fatal("request timeout did not retain exact retryable lease linkage")
	}
	if time.Since(start) > 1800*time.Millisecond {
		t.Fatal("worker profile timeout not applied")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("worker HTTP request not canceled")
	}
}

func TestGemmaRequestContextBoundsLongerHTTPTimeout(t *testing.T) {
	canceled := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(new(map[string]any))
		<-r.Context().Done()
		canceled <- struct{}{}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, e := gemmaWarmCompleteProfile(ctx, srv.URL, "exact", "full input", 256, "", 600)
	if !errors.Is(e, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatal("HTTP request exceeded generation envelope")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("envelope did not cancel request")
	}
}

func TestGemmaWorkerPermanentContextBlocksWithoutRetryOrReply(t *testing.T) {
	s, r := claimedGemmaItem()
	calls := 0
	_, err := testGemmaWorker(s, r, func(context.Context, string) (string, error) {
		calls++
		return "", &gemmaInferenceError{Status: 400, Code: "context_length_exceeded", Message: "input context overflow", Permanent: true}
	}).runOnce(context.Background())
	if err == nil || calls != 1 || s.blocked == "" || s.failed != "" || r.calls != 0 {
		t.Fatalf("permanent context failure disposition calls=%d block=%q failure=%q replies=%d", calls, s.blocked, s.failed, r.calls)
	}
}

func TestGemmaWorkerBusyKeepsSameClaimAndReplyFence(t *testing.T) {
	s, r := claimedGemmaItem()
	calls := 0
	_, err := testGemmaWorker(s, r, func(context.Context, string) (string, error) {
		calls++
		if calls == 1 {
			return "", &gemmaInferenceError{Status: 503, Busy: true, RetryAfter: time.Millisecond}
		}
		return "draft", nil
	}).runOnce(context.Background())
	if err != nil || calls != 2 || s.failed != "" || s.blocked != "" || r.calls != 1 {
		t.Fatalf("busy handling lost lease flow: %v", err)
	}
}

func TestGemmaWorkerBudgetAndPrivateBrokerProfile(t *testing.T) {
	for profile, want := range map[string]int{"m1": 256, "m5": 1024} {
		got, e := gemmaWorkerBudget(profile, 0)
		if e != nil || got != want {
			t.Fatal("profile budget mismatch")
		}
	}
	if _, e := gemmaWorkerBudget("m1", 1024); e == nil {
		t.Fatal("oversized M1 budget accepted")
	}
	file := filepath.Join(t.TempDir(), "capability")
	if e := os.WriteFile(file, []byte("fixture-private-capability\n"), 0600); e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-private-capability" {
			t.Error("missing client auth")
		}
		var p struct {
			Model     string
			MaxTokens int `json:"max_tokens"`
			Messages  []map[string]string
		}
		if e := json.NewDecoder(r.Body).Decode(&p); e != nil {
			t.Error(e)
		}
		if p.Model != "full-exact-model-identity" || p.MaxTokens != 256 || len(p.Messages) != 2 || p.Messages[0]["content"] == "" {
			t.Error("profile altered request identity or budget")
		}
		fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"draft"}}]}`)
	}))
	defer srv.Close()
	if _, e := gemmaWarmCompleteProfile(context.Background(), srv.URL, "full-exact-model-identity", "input", 256, file); e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(file, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := gemmaBrokerBearer(file); e == nil {
		t.Fatal("public capability accepted")
	}
	if _, e := gemmaBrokerBearer(file + "-missing"); e == nil {
		t.Fatal("missing configured capability accepted")
	}
}

func TestGemmaBrokerErrorBoundedAndRedacted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"code":"context_length_exceeded","message":"fixture-private-capability overflow"}}`)
	}))
	defer srv.Close()
	r, e := http.Get(srv.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	err := gemmaBrokerError(r, "fixture-private-capability")
	if strings.Contains(err.Error(), "fixture-private-capability") || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatal("capability redaction failed")
	}
}

func TestGemmaExplicitInstalledProfileNoM5Fallback(t *testing.T) {
	if _, e := readGemmaWorkerProfile("", "m1"); e == nil {
		t.Fatal("M1 silently selected M5 defaults")
	}
	f := filepath.Join(t.TempDir(), "profile.json")
	if e := os.WriteFile(f, []byte(`{"endpoint":"http://127.0.0.1:8477","model":"installed-full-Anubis-GAE2B-NVFP4-identity","access_token_file":"~/.sirsi/gemma-server.token","context_tokens":1024,"output_tokens":256,"streaming":false}`), 0600); e != nil {
		t.Fatal(e)
	}
	p, e := readGemmaWorkerProfile(f, "m1")
	if e != nil || p.Model != "installed-full-Anubis-GAE2B-NVFP4-identity" {
		t.Fatalf("exact selected profile lost: %v", e)
	}
	if e := os.WriteFile(f, []byte(`{"endpoint":"http://127.0.0.1:8477","model":"installed"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := readGemmaWorkerProfile(f, "m1"); e == nil {
		t.Fatal("M1 capability omission accepted")
	}
}

func TestGemmaBusyBoundedByCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	calls := 0
	_, e := gemmaGenerateWhileBusy(ctx, func(context.Context, string) (string, error) {
		calls++
		return "", &gemmaInferenceError{Status: 503, Busy: true, RetryAfter: time.Minute}
	}, "same request")
	var broker *gemmaInferenceError
	if !errors.As(e, &broker) || !broker.Busy || calls != 1 {
		t.Fatal("busy cancellation classification lost")
	}
}

func TestGemmaUnknown503DoesNotBusyWaitOrPermanentBlock(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(503)
		fmt.Fprint(w, `{"error":{"code":"runtime_failure","message":"native runtime failed"}}`)
	}))
	defer srv.Close()
	s, r := claimedGemmaItem()
	_, e := testGemmaWorker(s, r, func(ctx context.Context, prompt string) (string, error) {
		return gemmaWarmCompleteProfile(ctx, srv.URL, "exact-model", prompt, 256, "")
	}).runOnce(context.Background())
	if e == nil || calls != 1 || s.blocked != "" || s.failed == "" || r.calls != 0 {
		t.Fatal("unknown503 masqueraded as busy/block/success")
	}
}

func TestGemmaHTTPFixtureIsolatedFromExistingSelectedProfile(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "selected-m5.json")
	if e := os.WriteFile(profile, []byte(`{"endpoint":"http://127.0.0.1:8477/v1","model":"mlx-community/gemma-4-12B-it-8bit","context_tokens":4096,"output_tokens":1024,"streaming":false,"request_timeout_seconds":510}`), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("SIRSI_LOCAL_PROFILE", profile)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
		fmt.Fprint(w, `{"error":{"code":"runtime_failure","message":"isolated fixture"}}`)
	}))
	defer srv.Close()
	_, e := gemmaWarmCompleteProfile(context.Background(), srv.URL, "exact-model", "full fixture identity", 256, "")
	var broker *gemmaInferenceError
	if !errors.As(e, &broker) || broker.Busy || broker.Permanent || calls.Load() != 1 {
		t.Fatal("raw classification fixture consumed host selection")
	}
	_, e = gemmaWarmCompleteContext(context.Background(), srv.URL, "exact-model", "full identity", 256)
	if e == nil || !strings.Contains(e.Error(), "differs from selected") || calls.Load() != 1 {
		t.Fatal("real selected-profile mismatch refusal was disabled")
	}
}

func TestGemmaBusyDeadlineKeepsOriginalRetryableWithExactFence(t *testing.T) {
	s, r := claimedGemmaItem()
	calls := 0
	w := testGemmaWorker(s, r, func(context.Context, string) (string, error) {
		calls++
		return "", &gemmaInferenceError{Status: 503, Code: "BUSY", Message: "temporary backpressure", Busy: true, RetryAfter: time.Minute}
	})
	w.leaseTTL = 30 * time.Millisecond
	_, e := w.runOnce(context.Background())
	if e == nil || calls != 1 || s.blocked != "" || s.failed == "" || r.calls != 0 {
		t.Fatal("busy deadline stranded original or falsely replied")
	}
	if s.failID != s.lease.ItemID || s.failToken != s.lease.Token {
		t.Fatal("busy failure lost exact item/lease fence")
	}
	if !strings.Contains(s.failed, "temporary backpressure") {
		t.Fatal("busy cause evidence lost")
	}
}
