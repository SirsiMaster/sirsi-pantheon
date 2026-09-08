package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
)

func TestRequiredBoardServeControlTokenFailsClosed(t *testing.T) {
	t.Setenv("SIRSI_CONTROL_TOKEN", "")
	if _, err := requiredBoardServeControlToken(); err == nil || !strings.Contains(err.Error(), "SIRSI_CONTROL_TOKEN is required") {
		t.Fatalf("missing board token was accepted or had the wrong error: %v", err)
	}
}

func TestBoardServeControlTokenProtectsControlEndpoint(t *testing.T) {
	t.Setenv("SIRSI_CONTROL_TOKEN", "board-token")
	token, err := requiredBoardServeControlToken()
	if err != nil {
		t.Fatal(err)
	}
	h := routerboard.NewHandlerWithControlAuth(routerboard.New("/bin/false", "", "test-build"), t.TempDir(), token, true)
	mux := http.NewServeMux()
	h.Register(mux)

	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/control", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated board control status = %d, want 401", unauthorized.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer "+token)
	authorized := httptest.NewRecorder()
	mux.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code == http.StatusUnauthorized {
		t.Fatalf("configured board token did not authorize control request: %s", authorized.Body.String())
	}
}
