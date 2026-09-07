package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestControlEndpointURLCanonicalizesHostOnlyEndpoint(t *testing.T) {
	got, err := controlEndpointURL("https://m5.example.test:8734/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://m5.example.test:8734/api/control" {
		t.Fatalf("endpoint = %q", got)
	}
	if _, err := controlEndpointURL("file:///tmp/control"); err == nil {
		t.Fatal("accepted non-HTTP control endpoint")
	}
	if _, err := controlEndpointURL("https://m5.example.test/other"); err == nil {
		t.Fatal("accepted non-control endpoint path")
	}
}

func TestControlActionRequestAndRemoteSubmissionUseClosedEndpoint(t *testing.T) {
	requestBody := []byte(`{"verb":"delegate","agent":"codex","task_id":"t-1","subject":"ship"}`)
	requestFile := t.TempDir() + "/request.json"
	if err := os.WriteFile(requestFile, requestBody, 0o600); err != nil {
		t.Fatal(err)
	}
	gotBody, err := readControlActionRequest(requestFile)
	if err != nil || !bytes.Equal(gotBody, requestBody) {
		t.Fatalf("request body = %s, err=%v", gotBody, err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/control/action" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("headers = %#v", r.Header)
		}
		received, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(received, requestBody) {
			t.Fatalf("received body = %s, err=%v", received, err)
		}
		_, _ = w.Write([]byte(`{"schema":"pantheon.worker-control/v1","verb":"delegate","task_id":"t-1"}`))
	}))
	defer server.Close()
	response, err := sendRemoteControlAction(context.Background(), server.URL, "test-token", requestBody)
	if err != nil || !strings.Contains(string(response), `"task_id":"t-1"`) {
		t.Fatalf("response = %s, err=%v", response, err)
	}
}

func TestFetchRemoteControlUsesBearerAndRejectsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/control" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema":"pantheon.worker-control/v1","revision":7}`))
	}))
	defer server.Close()

	body, err := fetchRemoteControl(context.Background(), server.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"revision":7`) {
		t.Fatalf("body = %s", body)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer bad.Close()
	if _, err := fetchRemoteControl(context.Background(), bad.URL, ""); err == nil {
		t.Fatal("accepted invalid remote control JSON")
	}
}
