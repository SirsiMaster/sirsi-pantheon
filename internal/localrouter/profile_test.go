package localrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

func TestCompleteLocalUsesSelectedAuthenticatedNonstreamProfile(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".sirsi"), 0700)
	capability := filepath.Join(home, "capability")
	os.WriteFile(capability, []byte("fixture-capability"), 0600)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-capability" {
			t.Error("auth lost in localrouter")
		}
		var p struct {
			Model     string
			MaxTokens int `json:"max_tokens"`
			Stream    bool
			Messages  []map[string]string
		}
		json.NewDecoder(r.Body).Decode(&p)
		if p.Model != "exact-Anubis" || p.MaxTokens != 256 || p.Stream || len(p.Messages) != 2 || p.Messages[0]["content"] != SystemPrompt() {
			t.Error("localrouter identity/profile mismatch")
		}
		fmt.Fprint(w, `{"model":"exact-Anubis","choices":[{"finish_reason":"stop","message":{"content":"draft"}}]}`)
	}))
	defer srv.Close()
	no := false
	b, _ := json.Marshal(provider.LocalProfile{Endpoint: srv.URL + "/v1", Model: "exact-Anubis", ContextTokens: 1024, OutputTokens: 256, Streaming: &no, AccessTokenFile: capability})
	profile := filepath.Join(home, "profile.json")
	os.WriteFile(profile, b, 0600)
	os.WriteFile(filepath.Join(home, ".sirsi", "orchestrator.conf"), []byte("provider=sne-native-v2\nlocal_profile="+profile), 0600)
	if _, e := CompleteLocal(context.Background(), home, "full user input", 400); e != nil {
		t.Fatal(e)
	}
}
