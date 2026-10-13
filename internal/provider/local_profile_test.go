package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectedLocalProfileActualRequestAuthBudgetStreamAndError(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".sirsi"), 0700)
	token := filepath.Join(home, "capability")
	os.WriteFile(token, []byte("fixture-capability"), 0600)
	status := 200
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer fixture-capability" {
			t.Error("profile auth absent")
		}
		if r.Method == "GET" {
			fmt.Fprint(w, `{"data":[{"id":"exact-Anubis-identity"}]}`)
			return
		}
		var req ccRequest
		if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
			t.Error(e)
		}
		if req.Model != "exact-Anubis-identity" || req.MaxTokens > 256 || req.Stream || req.Messages[0].Content != "full system identity" {
			t.Error("actual request did not preserve selected profile")
		}
		if status != 200 {
			w.WriteHeader(status)
			fmt.Fprint(w, `{"error":{"code":"context_length_exceeded","message":"input exceeds context"}}`)
			return
		}
		fmt.Fprint(w, `{"model":"exact-Anubis-identity","choices":[{"finish_reason":"stop","message":{"content":"draft"}}]}`)
	}))
	defer srv.Close()
	profile := filepath.Join(home, "profile.json")
	b, _ := json.Marshal(LocalProfile{Endpoint: srv.URL + "/v1", Model: "exact-Anubis-identity", ContextTokens: 1024, OutputTokens: 256, Streaming: boolProfile(false), AccessTokenFile: token})
	os.WriteFile(profile, b, 0600)
	os.WriteFile(filepath.Join(home, ".sirsi", "orchestrator.conf"), []byte("provider=sne-native-v2\nlocal_profile="+profile+"\n"), 0600)
	p := Local(home, LoadConf(home))
	if p == nil || p.Caps().ContextTokens != 1024 || p.Caps().Streaming {
		t.Fatal("profile metadata not bound")
	}
	req := Request{System: "full system identity", Prompt: "full input without truncation", MaxTokens: 400}
	if _, e := p.Complete(context.Background(), req); e != nil {
		t.Fatal(e)
	}
	before := calls
	if _, e := p.Stream(context.Background(), req); e == nil || calls != before {
		t.Fatal("nonstream profile attempted SSE")
	}
	status = 400
	_, e := p.Complete(context.Background(), req)
	var broker *InferenceError
	if !errors.As(e, &broker) || !broker.Permanent || broker.Code != "context_length_exceeded" {
		t.Fatal("permanent context error lost across provider")
	}
	os.Remove(token)
	before = calls
	if _, e := p.Complete(context.Background(), req); e == nil || calls != before {
		t.Fatal("missing configured auth reached server")
	}
}
func boolProfile(b bool) *bool { return &b }
func TestSelectedProfileUnknownCapabilitiesFailLoud(t *testing.T) {
	f := filepath.Join(t.TempDir(), "unknown.json")
	os.WriteFile(f, []byte(`{"endpoint":"http://127.0.0.1:8477/v1","model":"exact"}`), 0600)
	p := Local(t.TempDir(), Conf{LocalProfile: f})
	if p == nil || p.ConfigurationError == nil {
		t.Fatal("unknown capabilities silently defaulted")
	}
	if _, e := p.Complete(context.Background(), Request{Prompt: "x"}); e == nil {
		t.Fatal("unknown profile executed")
	}
}
