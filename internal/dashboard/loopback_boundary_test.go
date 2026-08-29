package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequireLoopbackHostRejectsDNSRebindingBeforeHandler(t *testing.T) {
	called := false
	handler := requireLoopbackHost(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "http://attacker.example/v1/chat/completions", nil)
	request.Host = "attacker.example"
	request.Header.Set("Origin", "http://attacker.example")
	response := httptest.NewRecorder()
	handler(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if called {
		t.Fatal("protected handler ran for a non-loopback Host")
	}
}

func TestRequireLoopbackHostAcceptsOnlyCanonicalLoopbackAuthorities(t *testing.T) {
	for _, authority := range []string{"127.0.0.1:9119", "localhost:9119", "LOCALHOST.", "[::1]:9119", "::1"} {
		t.Run(authority, func(t *testing.T) {
			called := false
			handler := requireLoopbackHost(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/sne", nil)
			request.Host = authority
			response := httptest.NewRecorder()
			handler(response, request)
			if response.Code != http.StatusNoContent || !called {
				t.Fatalf("authority %q: status=%d called=%t", authority, response.Code, called)
			}
		})
	}
}

func TestIsLoopbackRequestHostRejectsAmbiguousOrRemoteAuthorities(t *testing.T) {
	for _, authority := range []string{"", "example.com", "example.com:9119", "127.0.0.1.example.com", "127.0.0.1:bad", "::ffff:192.0.2.1"} {
		if isLoopbackRequestHost(authority) {
			t.Fatalf("authority %q was admitted", authority)
		}
	}
}

func TestSNECapabilityRejectsMissingAndInvalidCredentials(t *testing.T) {
	server := &Server{sneAccess: newSNELocalAccess("correct-horse")}
	called := false
	handler := server.requireSNECapability(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	for name, credential := range map[string]string{"missing": "", "wrong": "Bearer wrong-battery", "malformed": "Basic correct-horse"} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/chat/completions", nil)
			request.Header.Set("Authorization", credential)
			response := httptest.NewRecorder()
			handler(response, request)
			if response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden {
				t.Fatalf("status = %d", response.Code)
			}
		})
	}
	if called {
		t.Fatal("protected handler ran without the configured capability")
	}
}

func TestSNECapabilityAcceptsExactBearerAndPreflight(t *testing.T) {
	server := &Server{sneAccess: newSNELocalAccess("correct-horse")}
	called := 0
	handler := server.requireSNECapability(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/chat/completions", nil)
	request.Header.Set("Authorization", "Bearer correct-horse")
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("authorized status = %d", response.Code)
	}

	preflight := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1/v1/chat/completions", nil)
	preflightResponse := httptest.NewRecorder()
	handler(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusNoContent || called != 2 {
		t.Fatalf("preflight status=%d called=%d", preflightResponse.Code, called)
	}
}

func TestEmbeddedDashboardSessionCookieAuthorizesWithoutExposingTokenToJavaScript(t *testing.T) {
	const token = "correct-horse-battery-staple-1234567890"
	server := New(Config{SNELocalAccessToken: token})

	overview := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	overview.Host = "127.0.0.1:9119"
	overviewResponse := httptest.NewRecorder()
	server.handler.ServeHTTP(overviewResponse, overview)
	response := overviewResponse.Result()
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count=%d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != sneLocalSessionCookie || cookie.Value != token || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatalf("unsafe embedded session cookie: %#v", cookie)
	}

	called := false
	handler := requireLoopbackHost(server.requireSNECapability(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/sne/stop", nil)
	request.Host = "127.0.0.1:9119"
	request.AddCookie(cookie)
	result := httptest.NewRecorder()
	handler(result, request)
	if result.Code != http.StatusNoContent || !called {
		t.Fatalf("cookie authorization status=%d called=%t", result.Code, called)
	}
}

func TestExplicitInvalidBearerCannotFallBackToEmbeddedSessionCookie(t *testing.T) {
	server := &Server{sneAccess: newSNELocalAccess("correct-horse")}
	handler := server.requireSNECapability(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/sne/stop", nil)
	request.Header.Set("Authorization", "Bearer wrong-battery")
	request.AddCookie(&http.Cookie{Name: sneLocalSessionCookie, Value: "correct-horse"})
	result := httptest.NewRecorder()
	handler(result, request)
	if result.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", result.Code)
	}
}

func TestRegisteredSensitiveRoutesRejectDNSRebinding(t *testing.T) {
	server := New(Config{SNELocalAccessToken: "abcdefghijklmnopqrstuvwxyz123456"})
	for _, path := range []string{"/api/sne", "/api/sne/start", "/api/recovery/restart", "/v1/models", "/v1/chat/completions"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://attacker.example"+path, nil)
			request.Host = "attacker.example"
			request.Header.Set("Origin", "http://attacker.example")
			response := httptest.NewRecorder()
			server.handler.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
		})
	}
}

func TestRegisteredInferenceRouteRequiresConfiguredCapability(t *testing.T) {
	const token = "abcdefghijklmnopqrstuvwxyz123456"
	server := New(Config{SNELocalAccessToken: token})

	missing := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9119/v1/chat/completions", nil)
	missing.Host = "127.0.0.1:9119"
	missingResponse := httptest.NewRecorder()
	server.handler.ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusUnauthorized {
		t.Fatalf("missing credential status = %d, want %d", missingResponse.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(missingResponse.Body.String(), "sirsi sne open") || !strings.Contains(missingResponse.Body.String(), `"no_fallback":true`) {
		t.Fatalf("missing credential omitted safe recovery: %s", missingResponse.Body.String())
	}

	authorized := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9119/v1/chat/completions", nil)
	authorized.Host = "127.0.0.1:9119"
	authorized.Header.Set("Authorization", "Bearer "+token)
	authorizedResponse := httptest.NewRecorder()
	server.handler.ServeHTTP(authorizedResponse, authorized)
	if authorizedResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("authorized request status = %d, want configured-handler %d", authorizedResponse.Code, http.StatusServiceUnavailable)
	}
}

func TestRegisteredCapabilityRotationImmediatelyRevokesPriorToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), sneLocalAccessTokenFilename)
	initial, err := LoadOrCreateSNELocalAccessToken(path)
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{SNELocalAccessToken: initial, SNELocalAccessTokenPath: path})
	rotate := func(token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9119/api/sne/access/rotate", nil)
		request.Host = "127.0.0.1:9119"
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		server.handler.ServeHTTP(response, request)
		return response
	}

	firstResponse := rotate(initial)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("rotation status = %d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	rotated := payload["access_token"]
	if rotated == "" || rotated == initial {
		t.Fatal("rotation did not return a replacement capability")
	}
	if response := rotate(initial); response.Code != http.StatusForbidden {
		t.Fatalf("revoked token status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if response := rotate(rotated); response.Code != http.StatusOK {
		t.Fatalf("replacement token status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestRegisteredInferenceAuthorizationOrdersHostOriginAndCapability(t *testing.T) {
	const token = "abcdefghijklmnopqrstuvwxyz123456"
	server := New(Config{SNELocalAccessToken: token})

	knownMissing := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9119/v1/chat/completions", nil)
	knownMissing.Host = "127.0.0.1:9119"
	knownMissing.Header.Set("Origin", "https://sirsi.ai")
	knownMissingResponse := httptest.NewRecorder()
	server.handler.ServeHTTP(knownMissingResponse, knownMissing)
	if knownMissingResponse.Code != http.StatusUnauthorized || knownMissingResponse.Header().Get("Access-Control-Allow-Origin") != "https://sirsi.ai" {
		t.Fatalf("known-origin missing capability status=%d allow-origin=%q", knownMissingResponse.Code, knownMissingResponse.Header().Get("Access-Control-Allow-Origin"))
	}

	hostileAuthorized := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9119/v1/chat/completions", nil)
	hostileAuthorized.Host = "127.0.0.1:9119"
	hostileAuthorized.Header.Set("Origin", "https://attacker.example")
	hostileAuthorized.Header.Set("Authorization", "Bearer "+token)
	hostileResponse := httptest.NewRecorder()
	server.handler.ServeHTTP(hostileResponse, hostileAuthorized)
	if hostileResponse.Code != http.StatusForbidden || hostileResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("hostile-origin status=%d allow-origin=%q", hostileResponse.Code, hostileResponse.Header().Get("Access-Control-Allow-Origin"))
	}
}
