package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// The question and grounding must never follow a redirect (it could leave the machine).
func TestAskLocalEngineRefusesRedirect(t *testing.T) {
	var hit atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit.Store(true) }))
	defer target.Close()
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/elsewhere", http.StatusTemporaryRedirect)
	}))
	defer engine.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GEMMA_MODEL", "fixture-model")
	u, _ := url.Parse(engine.URL)
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-server.port"), []byte(u.Port()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := askLocalEngine(context.Background(), "q", "g"); err == nil {
		t.Fatal("redirect response accepted")
	}
	if hit.Load() {
		t.Fatal("request followed the redirect")
	}
}
