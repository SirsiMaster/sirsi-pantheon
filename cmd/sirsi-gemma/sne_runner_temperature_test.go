package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSNERunnerGenerateSerializesNumericZeroTemperature(t *testing.T) {
	var request map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	if _, err := NewSNERunner(server.URL+"/v1", "test-model").Generate(context.Background(), "hello", 8, 0); err != nil {
		t.Fatal(err)
	}

	raw, ok := request["temperature"]
	if !ok {
		t.Fatal("temperature was omitted")
	}
	if string(raw) != "0" {
		t.Fatalf("temperature wire value = %s, want numeric 0", raw)
	}
}

func TestSNERunnerHealthUsesZeroAndOMLXHealthKeepsPointZeroOne(t *testing.T) {
	temperatures := make([]json.RawMessage, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		temperatures = append(temperatures, request["temperature"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	if err := NewSNERunner(server.URL+"/v1", "sne-model").Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := NewOMLXRunner(server.URL+"/v1", "omlx-model").Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(temperatures) != 2 {
		t.Fatalf("captured %d temperatures, want 2", len(temperatures))
	}
	if string(temperatures[0]) != "0" {
		t.Fatalf("SNE health temperature = %s, want numeric 0", temperatures[0])
	}
	if string(temperatures[1]) != "0.01" {
		t.Fatalf("oMLX health temperature = %s, want numeric 0.01", temperatures[1])
	}
}
