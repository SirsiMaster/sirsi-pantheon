package seshat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureGoogleTokenRefreshesAndPersistsPrivately(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 22, 5, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if request.Form.Get("grant_type") != "refresh_token" || request.Form.Get("refresh_token") != "durable-refresh" {
			t.Fatalf("unexpected refresh request: %v", request.Form)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"access_token":"renewed-access","token_type":"Bearer","expires_in":3600}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	credentials := filepath.Join(dir, "credentials.json")
	tokenPath := filepath.Join(dir, "token.json")
	writeGoogleFixture(t, credentials, map[string]any{"installed": map[string]any{
		"client_id": "client", "client_secret": "secret", "token_uri": server.URL,
	}})
	writeGoogleFixture(t, tokenPath, googleToken{
		AccessToken: "expired", RefreshToken: "durable-refresh", TokenType: "Bearer",
		Expiry: now.Add(-time.Minute).Format(time.RFC3339Nano),
	})

	token, err := ensureGoogleToken(context.Background(), server.Client(), now, credentials, tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "renewed-access" || token.RefreshToken != "durable-refresh" {
		t.Fatalf("unexpected refreshed token: %#v", token)
	}
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("token permissions = %o, want 600", info.Mode().Perm())
	}
	persisted, err := readGoogleToken(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.AccessToken != "renewed-access" || persisted.RefreshToken != "durable-refresh" {
		t.Fatalf("unexpected persisted token: %#v", persisted)
	}
}

func TestInspectGoogleWorkspaceAuthReturnsOnlyState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	credentials := filepath.Join(dir, "credentials.json")
	tokenPath := filepath.Join(dir, "token.json")
	writeGoogleFixture(t, credentials, map[string]any{"installed": map[string]any{
		"client_id": "client", "token_uri": "https://oauth2.googleapis.com/token",
	}})
	writeGoogleFixture(t, tokenPath, googleToken{AccessToken: "secret-access", RefreshToken: "secret-refresh"})
	status, err := InspectGoogleWorkspaceAuth(credentials, tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || !status.Authorized || !status.Refreshable {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func writeGoogleFixture(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
