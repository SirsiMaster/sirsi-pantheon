package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSNERunnerTokenFileAddsBearerAndTrimsNewline(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "sne-token")
	if err := os.WriteFile(tokenFile, []byte("private-token\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	if _, err := NewSNERunnerWithTokenFile(server.URL+"/v1", "sne-model", tokenFile).
		Generate(context.Background(), "hello", 8, 0); err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer private-token" {
		t.Fatalf("Authorization = %q, want bearer token", authorization)
	}
}

func TestSNERunnerRejectsMissingOrEmptyTokenFileBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		path func(t *testing.T) string
	}{
		{
			name: "missing",
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing-token")
			},
		},
		{
			name: "empty",
			path: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "empty-token")
				if err := os.WriteFile(path, []byte("\r\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			_, err := NewSNERunnerWithTokenFile(server.URL+"/v1", "sne-model", tt.path(t)).
				Generate(context.Background(), "hello", 8, 0)
			if err == nil {
				t.Fatal("Generate() succeeded with invalid token file")
			}
			if requests != 0 {
				t.Fatalf("server received %d requests, want 0", requests)
			}
		})
	}
}

func TestOMLXRunnerDoesNotSendSNEAuthorization(t *testing.T) {
	authorizationSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizationSeen = r.Header.Get("Authorization") != ""
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	if _, err := NewOMLXRunner(server.URL+"/v1", "omlx-model").
		Generate(context.Background(), "hello", 8, 0); err != nil {
		t.Fatal(err)
	}
	if authorizationSeen {
		t.Fatal("oMLX request unexpectedly included SNE Authorization")
	}
}
