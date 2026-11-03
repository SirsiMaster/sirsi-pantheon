package dashboard

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type askRoundTripFunc func(*http.Request) (*http.Response, error)

func (f askRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Inert transport: no listener, external network, broker or model is used.
func TestLocalAskClientRejectsRedirects(t *testing.T) {
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client := localAskClient()
			client.Transport = askRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://example.invalid/collect"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
			})
			req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8477/v1/chat/completions", strings.NewReader("private diagnostics"))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if calls != 1 || resp.StatusCode != status {
				t.Fatalf("calls=%d status=%d", calls, resp.StatusCode)
			}
		})
	}
}

// Exercise the actual Ask path using an in-memory transport. This must not be
// parallel: it temporarily replaces Go's process-wide default transport.
func TestAskLocalEngineRedirectAndSuccess(t *testing.T) {
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	t.Setenv("HOME", t.TempDir())
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			http.DefaultTransport = askRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				body := `{"model":"inert","choices":[{"message":{"content":"{}"}}]}`
				return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: http.Header{"Location": []string{"https://example.invalid/collect"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})
			_, model, err := askLocalEngine(context.Background(), "private question", "private grounding")
			if status == http.StatusOK && (err != nil || model != "inert") {
				t.Fatalf("200 control: model=%q err=%v", model, err)
			}
			if status != http.StatusOK && err == nil {
				t.Fatal("redirect unexpectedly succeeded")
			}
			if calls != 1 {
				t.Fatalf("transport calls=%d; want 1", calls)
			}
		})
	}
}
