package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalProfileRequestTimeoutValidationAndLegacyDefaults(t *testing.T) {
	no := false
	profile := LocalProfile{Endpoint: "http://127.0.0.1:8477/v1", Model: "exact", ContextTokens: 4096, OutputTokens: 1024, Streaming: &no}
	file := filepath.Join(t.TempDir(), "profile.json")
	for _, n := range []int{0, -1, 601} {
		profile.RequestTimeoutSeconds = &n
		b, _ := json.Marshal(profile)
		os.WriteFile(file, b, 0600)
		if _, e := ReadLocalProfile(file); e == nil {
			t.Fatalf("invalid timeout %d accepted", n)
		}
	}
	profile.RequestTimeoutSeconds = nil
	b, _ := json.Marshal(profile)
	os.WriteFile(file, b, 0600)
	p, e := ReadLocalProfile(file)
	if e != nil || ProfileRequestTimeoutSeconds(p) != 0 {
		t.Fatal("missing timeout did not preserve default")
	}
	if (&OpenAICompat{}).client().Timeout != 180*time.Second {
		t.Fatal("nonworker legacy timeout changed")
	}
	custom := &http.Client{Timeout: 7 * time.Second}
	if (&OpenAICompat{HTTP: custom}).client() != custom {
		t.Fatal("custom legacy client changed")
	}
	overridden := (&OpenAICompat{HTTP: custom, RequestTimeoutSeconds: 1}).client()
	if overridden.Timeout != time.Second || custom.Timeout != 7*time.Second {
		t.Fatal("profile override mutated original client")
	}
}

func TestSelectedProviderRequestTimeoutActuallyCancelsHTTP(t *testing.T) {
	canceled := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(new(map[string]any))
		<-r.Context().Done()
		canceled <- struct{}{}
	}))
	defer srv.Close()
	home := t.TempDir()
	profile := filepath.Join(home, "profile.json")
	no := false
	seconds := 1
	b, _ := json.Marshal(LocalProfile{Endpoint: srv.URL + "/v1", Model: "exact", ContextTokens: 1024, OutputTokens: 256, Streaming: &no, RequestTimeoutSeconds: &seconds})
	os.WriteFile(profile, b, 0600)
	p := Local(home, Conf{LocalProfile: profile})
	if p.RequestTimeoutSeconds != 1 {
		t.Fatal("Local dropped request timeout")
	}
	start := time.Now()
	_, e := p.Complete(context.Background(), Request{System: "full identity", Prompt: "full input", MaxTokens: 256})
	var timeout interface{ Timeout() bool }
	if !errors.As(e, &timeout) || !timeout.Timeout() {
		t.Fatalf("want HTTP timeout, got %v", e)
	}
	if time.Since(start) > 1800*time.Millisecond {
		t.Fatal("configured HTTP deadline not applied")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("server request not canceled")
	}
}
