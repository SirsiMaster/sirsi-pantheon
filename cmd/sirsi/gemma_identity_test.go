package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/localrouter"
)

// The warm broker takes a real system message, so the Sirsi identity must reach
// it EXACTLY once — as the system role. PR #382 originally enveloped the prompt
// in runGemma before the warm call as well, which shipped the whole
// SystemPrompt() block a second time inside the user turn.
//
// This drives runGemma itself against a fake broker rather than calling
// gemmaWarmComplete directly. That distinction is the whole point: the defect
// was never in gemmaWarmComplete (which correctly sends one system message) but
// in its CALLER, so a test that pokes the callee passes while the bug ships.
func TestRunGemmaSendsIdentityExactlyOnceOnWarmPath(t *testing.T) {
	var got struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/v1/models") {
			io.WriteString(w, `{"data":[]}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &got); err != nil {
			t.Errorf("broker got unparsable body: %v", err)
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	// runGemma finds the broker through $HOME, so a temp home with a port file
	// pointing at the fake broker puts the real warm path under test.
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	if err := os.WriteFile(gemmaPortPath(home), []byte(port), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("GEMMA_MODEL", "test-model")

	if err := runGemma(gemmaCmd, []string{"what is my router state?"}); err != nil {
		t.Fatalf("runGemma: %v", err)
	}
	if len(got.Messages) == 0 {
		t.Fatal("fake broker was never called — the warm path did not run, so this test proved nothing")
	}

	// A stable, distinctive line from the identity block. Matching a sentinel
	// rather than the whole prompt keeps the test alive when the copy is edited.
	const sentinel = "Your operating identity is Sirsi."
	if !strings.Contains(localrouter.SystemPrompt(), sentinel) {
		t.Fatalf("sentinel %q no longer appears in SystemPrompt() — update the sentinel, not the assertion", sentinel)
	}

	total := 0
	for _, m := range got.Messages {
		n := strings.Count(m.Content, sentinel)
		total += n
		if m.Role == "user" && n > 0 {
			t.Errorf("identity block leaked into the USER turn %d time(s) — the warm path must not be pre-enveloped", n)
		}
		if m.Role == "system" && n != 1 {
			t.Errorf("system message carries the identity %d time(s), want exactly 1", n)
		}
	}
	if total != 1 {
		t.Errorf("identity block sent %d time(s) across all messages, want exactly 1", total)
	}
}
