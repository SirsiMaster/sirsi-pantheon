package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
)

const cliTestToken = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ"

func TestBuildNexusCapabilityURLUsesFragmentOnly(t *testing.T) {
	result, err := dashboard.BuildNexusCapabilityURL(dashboard.NexusLocalAIURL, cliTestToken)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(result)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/local-ai" || parsed.RawQuery != "" || parsed.Fragment != "sne_capability="+cliTestToken {
		t.Fatalf("unsafe launch URL: %s", result)
	}
	if _, err := dashboard.BuildNexusCapabilityURL("https://attacker.example/", cliTestToken); err == nil {
		t.Fatal("non-Sirsi launch origin was admitted")
	}
}

func TestRotateSNELocalCapabilityUsesBearerAndValidatesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sne/access/rotate" || r.Header.Get("Authorization") != "Bearer "+cliTestToken {
			t.Fatalf("unexpected rotation request: %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": cliTestToken, "token_type": "Bearer"})
	}))
	defer server.Close()
	rotated, err := rotateSNELocalCapability(server.URL, cliTestToken, server.Client())
	if err != nil || rotated != cliTestToken {
		t.Fatalf("rotated=%q err=%v", rotated, err)
	}
	if _, err := rotateSNELocalCapability("https://attacker.example", cliTestToken, server.Client()); err == nil {
		t.Fatal("remote rotation endpoint was admitted")
	}
}
