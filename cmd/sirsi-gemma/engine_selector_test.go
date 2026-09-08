package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestEffectiveEngine(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"default", Config{}, "mlx"},
		{"legacy-sne-url", Config{SNEURL: "http://sne/v1"}, "sne"},
		{"omlx-url", Config{OMLXURL: "http://omlx/v1"}, "omlx"},
		{"explicit-mlx", Config{Engine: "mlx", SNEURL: "http://sne/v1"}, "mlx"},
		{"explicit-sne", Config{Engine: "sne"}, "sne"},
		{"explicit-omlx", Config{Engine: "omlx"}, "omlx"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.cfg.EffectiveEngine(); got != test.want {
				t.Fatalf("EffectiveEngine() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLoadConfigEngineValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gemma.toml")
	if err := os.WriteFile(path, []byte("engine = omlx\nomlx_url = http://127.0.0.1:8000/v1\nomlx_model = gemma-4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EffectiveEngine() != "omlx" || cfg.OMLXURL != "http://127.0.0.1:8000/v1" || cfg.OMLXModel != "gemma-4" {
		t.Fatalf("unexpected OMLX config: %+v", cfg)
	}
	if err := cfg.set("engine", "unknown"); err == nil {
		t.Fatal("invalid engine was accepted")
	}
}

func TestOpenAICompatibleEnginesShareWorkflow(t *testing.T) {
	var paths []string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Error(err)
		}
		var request sneRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Error(err)
		}
		if len(request.Messages) != 1 || request.Messages[0].Content != "hello" {
			t.Errorf("unexpected request: %+v", request)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"same answer"}}]}`)),
		}, nil
	})}

	for _, runner := range []*SNERunner{
		NewSNERunner("http://sne.test/v1", "gemma-4"),
		NewOMLXRunner("http://omlx.test/v1", "gemma-4"),
	} {
		runner.client = client
		got, err := runner.Generate(context.Background(), "hello", 8, 0.1)
		if err != nil {
			t.Fatal(err)
		}
		if got != "same answer" {
			t.Fatalf("Generate() = %q", got)
		}
	}
	if len(paths) != 2 || paths[0] != "/v1/chat/completions" || paths[1] != paths[0] {
		t.Fatalf("engine workflows diverged: %v", paths)
	}
}
