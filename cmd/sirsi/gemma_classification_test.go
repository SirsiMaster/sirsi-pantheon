package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
			_, err := gemmaWarmCompleteContext(context.Background(), srv.URL, "exact-model", "input", 256)
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
