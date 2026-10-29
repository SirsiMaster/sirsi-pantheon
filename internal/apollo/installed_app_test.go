package apollo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type appRoundTrip func(*http.Request) (*http.Response, error)

func (f appRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func appRequestFixture() InstalledAppRequest {
	return InstalledAppRequest{Model: "qwen3.5-9b", Mode: "plain", Precision: InstalledAppPrecision, MaxOutputTokens: 64, Messages: []ChatMessage{{Role: "user", Content: "fixture only"}}}
}

func appResponseFixture(tier string) string {
	drafter := `"Apollo head apollo-qwen3.5-9b-mtp-clean-s416-smart4"`
	if tier == "plain" {
		drafter = "null"
	}
	return `{"model":"` + InstalledAppModel + `","apollo":{"model":"` + InstalledAppModel + `","engine":"apollo-qwen","tier":"` + tier + `","drafter":` + drafter + `},"choices":[{"index":0,"message":{"role":"assistant","content":"fixture answer"},"finish_reason":"stop"}]}`
}

func TestInstalledAppModesWireAndResponse(t *testing.T) {
	for _, tc := range []struct{ mode, tier string }{{"plain", "plain"}, {"flash", "flash-max"}, {"mlx-certified", "mlx-certified"}} {
		t.Run(tc.mode, func(t *testing.T) {
			in := appRequestFixture()
			in.Mode = tc.mode
			calls := 0
			client := &http.Client{Transport: appRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != installedAppEndpoint+"/chat/completions" || r.Method != "POST" || r.Header.Get("Authorization") != "" {
					t.Fatalf("unbound request: %v", r)
				}
				var wire installedAppWireRequest
				if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
					t.Fatal(err)
				}
				if wire.Model != InstalledAppRoute || wire.Apollo.Mode != tc.tier || wire.Stream || wire.Temperature != 0 || wire.Template.EnableThinking || wire.MaxTokens != 64 {
					t.Fatalf("wire = %+v", wire)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(appResponseFixture(tc.tier))), Header: make(http.Header)}, nil
			})}
			got, err := chatInstalledApp(context.Background(), client, in)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || got.Tier != tc.tier || got.Mode != tc.mode || got.Precision != InstalledAppPrecision || got.Text != "fixture answer" || got.AdmissionOwner != "installed Apollo service" {
				t.Fatalf("calls=%d result=%+v", calls, got)
			}
		})
	}
}

func TestInstalledAppRequestRefusalsBeforeHTTP(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*InstalledAppRequest)
	}{
		{"Gemma", func(r *InstalledAppRequest) { r.Model = "mlx-community/gemma-4-12B-it-8bit" }},
		{"foreign model", func(r *InstalledAppRequest) { r.Model = "qwen3.8-27b" }},
		{"wrong precision", func(r *InstalledAppRequest) { r.Precision = "Q8" }},
		{"empty precision", func(r *InstalledAppRequest) { r.Precision = "" }},
		{"omitted mode", func(r *InstalledAppRequest) { r.Mode = "" }},
		{"wire tier not CLI mode", func(r *InstalledAppRequest) { r.Mode = "flash-max" }},
		{"unsupported mode", func(r *InstalledAppRequest) { r.Mode = "auto" }},
		{"zero output", func(r *InstalledAppRequest) { r.MaxOutputTokens = 0 }},
		{"excess output", func(r *InstalledAppRequest) { r.MaxOutputTokens = 4097 }},
		{"no message", func(r *InstalledAppRequest) { r.Messages = nil }},
		{"wrong role", func(r *InstalledAppRequest) { r.Messages[0].Role = "tool" }},
		{"oversize prompt", func(r *InstalledAppRequest) { r.Messages[0].Content = strings.Repeat("x", 32769) }},
		{"invalid UTF8", func(r *InstalledAppRequest) { r.Messages[0].Content = string([]byte{255}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := appRequestFixture()
			tc.mutate(&in)
			client := &http.Client{Transport: appRoundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("invalid request reached HTTP"); return nil, nil })}
			if _, err := chatInstalledApp(context.Background(), client, in); err == nil {
				t.Fatal("accepted unsupported request")
			}
		})
	}
}

func TestInstalledAppResponseRefusals(t *testing.T) {
	base := appResponseFixture("plain")
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"model drift", strings.Replace(base, InstalledAppModel, "foreign", 1), 200},
		{"tier drift", strings.Replace(base, `"tier":"plain"`, `"tier":"flash-max"`, 1), 200},
		{"plain drafter", strings.Replace(base, `"drafter":null`, `"drafter":"head"`, 1), 200},
		{"missing drafter", strings.Replace(base, `,"drafter":null`, "", 1), 200},
		{"engine drift", strings.Replace(base, "apollo-qwen", "mlx-lm", 1), 200},
		{"missing content", strings.Replace(base, `"content":"fixture answer"`, `"other":"fixture answer"`, 1), 200},
		{"incomplete", strings.Replace(base, `"finish_reason":"stop"`, `"finish_reason":null`, 1), 200},
		{"tool output", strings.Replace(base, `"content":"fixture answer"`, `"content":"fixture answer","tool_calls":[{}]`, 1), 200},
		{"choice drift", strings.Replace(base, `"index":0`, `"index":1`, 1), 200},
		{"trailing JSON", base + `{}`, 200},
		{"malformed", `{`, 200},
		{"oversize", strings.Repeat("x", 4*1024*1024+1), 200},
		{"refusal", base, 503},
		{"redirect", base, 307},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: appRoundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			if _, err := chatInstalledApp(context.Background(), client, appRequestFixture()); err == nil {
				t.Fatal("accepted bad response")
			}
			if calls != 1 {
				t.Fatalf("retried %d times", calls)
			}
		})
	}
}

func TestInstalledAppTransportFailureNotRetried(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: appRoundTrip(func(r *http.Request) (*http.Response, error) { calls++; return nil, errors.New("fixture disconnect") })}
	if _, err := chatInstalledApp(context.Background(), client, appRequestFixture()); err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestInstalledAppCatalogPreservesGemma(t *testing.T) {
	gemma := Engine{ID: "apollo-local-sne", MachineID: "this-mac", Provider: "SNE", ResidentModel: "mlx-community/gemma-4-12B-it-8bit", State: "configured"}
	got := withInstalledAppEngine([]Engine{gemma}, installedAppProfile())
	if len(got) != 2 || got[0] != gemma {
		t.Fatalf("legacy engine changed: %+v", got)
	}
	profile := got[1].InstalledProfile
	if got[1].ResidentModel != "" || profile == nil || profile.Model != InstalledAppModel || profile.Route != InstalledAppRoute || profile.Precision != InstalledAppPrecision || got[1].Endpoint != installedAppEndpoint {
		t.Fatalf("unbound app profile %+v", got[1])
	}
}

func TestInstalledAppHTTPTransportCannotLeaveLoopback(t *testing.T) {
	client := installedAppHTTPClient()
	defer client.CloseIdleConnections()
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || client.Timeout != 120*time.Second {
		t.Fatal("transport is not fixed, proxy-free and bounded")
	}
	if client.CheckRedirect(&http.Request{}, nil) != http.ErrUseLastResponse {
		t.Fatal("redirects enabled")
	}
	for _, addr := range []string{"example.com:8520", "127.0.0.1:8621", "127.0.0.2:8520", "[::1]:8520"} {
		if conn, err := transport.DialContext(context.Background(), "tcp", addr); err == nil {
			if conn != nil {
				_ = conn.Close()
			}
			t.Fatalf("allowed alternate address %s", addr)
		}
	}
}

func TestInstalledAppCancellationIsNotRetried(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	client := &http.Client{Transport: appRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		cancel()
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	if _, err := chatInstalledApp(ctx, client, appRequestFixture()); err == nil || !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestInstalledAppArtifactBinding(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Contents", "Helpers", "router")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("fixture"))
	files := map[string]string{"Helpers/router": hex.EncodeToString(sum[:])}
	if err := verifyInstalledApp(root, files); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("drift"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyInstalledApp(root, files); err == nil {
		t.Fatal("accepted changed installed bytes")
	}
	if err := verifyInstalledApp(root, map[string]string{"Helpers/missing": files["Helpers/router"]}); err == nil {
		t.Fatal("accepted missing installed artifact")
	}
	if err := os.Symlink(path, path+"-link"); err != nil {
		t.Fatal(err)
	}
	if err := verifyInstalledApp(root, map[string]string{"Helpers/router-link": files["Helpers/router"]}); err == nil {
		t.Fatal("accepted final symlink")
	}
	if IsInstalledAppModel("mlx-community/gemma-4-12B-it-8bit") {
		t.Fatal("Gemma stolen from native route")
	}
	for _, id := range []string{"qwen3.5-9b", InstalledAppModel, InstalledAppRoute} {
		if !IsInstalledAppModel(id) {
			t.Fatalf("missing explicit alias %s", id)
		}
	}
}
