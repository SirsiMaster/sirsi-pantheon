package dispatch

import (
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActorOnlySendPreservesLegacy(t *testing.T) {
	f := testFacade(t)
	p := filepath.Join(f.root, "agents.json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(b), `"supervisor": {"id":"supervisor","type":"service"`, `"supervisor": {"delivery_contract":{"version":1,"mode":"actor-only"},"id":"supervisor","type":"service"`, 1)
	if err := os.WriteFile(p, []byte(s), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Send("a", "supervisor", "unreachable", "review", "partial"); err == nil {
		t.Fatal("actor-only accepted")
	}
	rows, err := f.store.Inbox("supervisor")
	if err != nil || len(rows) != 0 {
		t.Fatalf("side effect: %v %v", rows, err)
	}
	if _, err := f.Send("a", "b", "legacy", "review", "still works"); err != nil {
		t.Fatal(err)
	}
	if err := f.ValidateAgent("acting agent", "supervisor"); err != nil {
		t.Fatalf("registration/acting identity changed: %v", err)
	}
}

// New client guard remains effective against a server with legacy options.
func TestActorOnlyNewClientLegacyServer(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // fixture-only synthetic session cache; never real home
	f := testFacade(t)
	p := filepath.Join(f.root, "agents.json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	b = []byte(strings.Replace(string(b), `"supervisor": {"id":"supervisor"`, `"supervisor": {"delivery_contract":{"version":1,"mode":"actor-only"},"id":"supervisor"`, 1))
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	h, err := routerstore.Handler(f.store, routerstore.ServerOptions{Token: "test", RuleOfRa: "off"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	rs := routerstore.NewRemoteStore(srv.URL, "test")
	client := New(f.root, rs)
	if _, err := client.Send("a", "supervisor", "blocked", "review", "x"); err == nil {
		t.Fatal("new client sent actor-only mail to old server")
	}
	rows, err := f.store.Inbox("supervisor")
	if err != nil || len(rows) != 0 {
		t.Fatalf("inbox side effects: %v %v", rows, err)
	}
	if _, err := client.Send("a", "b", "legacy", "review", "x"); err != nil {
		t.Fatal(err)
	}
}
