package routerstore

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func deliveryRegistry(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "agents.json")
	b := `{"agents":{
 "legacy":{"type":"claude","wake":{"mechanism":"none"}},
 "wakeable":{"type":"codex","wake":{"mechanism":"launchagent"}},
 "service":{"type":"service","wake":{"mechanism":"none"}},
 "horus":{"type":"service","wake":{"mechanism":"none"},"description":"Actor-only local supervision identity; never route inbox work here"},
 "typed":{"delivery_contract":{"version":1,"mode":"actor-only"}},
 "inbox":{"delivery_contract":{"version":1,"mode":"inbox"}},
 "future":{"delivery_contract":{"version":2,"mode":"inbox"}}
 }}`
	if err := os.WriteFile(p, []byte(b), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestRecipientDeliveryCompatibility(t *testing.T) {
	p := deliveryRegistry(t)
	for _, id := range []string{"legacy", "wakeable", "service", "inbox"} {
		if err := ValidateRecipientDelivery(p, id); err != nil {
			t.Fatalf("legacy/typed inbox %s refused: %v", id, err)
		}
	}
	for _, id := range []string{"horus", "typed", "future", "unknown"} {
		if err := ValidateRecipientDelivery(p, id); err == nil {
			t.Fatalf("%s accepted", id)
		}
	}
	if err := ValidateRecipientDelivery("", "legacy"); err != nil {
		t.Fatal(err)
	}
}
func TestRecipientDeliveryMixedClientServer(t *testing.T) {
	backend := newDst(t)
	p := deliveryRegistry(t)
	h, err := Handler(backend, ServerOptions{Token: "test", RuleOfRa: "off", RecipientRegistry: p})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	rs := NewRemoteStore(srv.URL, "test")
	rs.sessionDir = ""
	// Legacy wire shape has no delivery-contract field. Both old direct Send and
	// current SendGuarded must be refused before a row appears.
	source, err := backend.Send("horus", "legacy", "original", "review", "unfinished")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"horus", "typed"} {
		if _, err := rs.Send("legacy", id, "oldclient", "review", "x"); err == nil || !strings.Contains(err.Error(), "actor-only") {
			t.Fatalf("direct Send %s: %v", id, err)
		}
		if _, _, err := rs.SendGuarded(SendReq{From: "legacy", To: id, Title: "response", Type: "review", Instructions: "partial", SourceItem: source}); err == nil {
			t.Fatalf("SendGuarded %s accepted", id)
		}
		rows, err := backend.Inbox(id)
		if err != nil || len(rows) != 0 {
			t.Fatalf("refusal wrote inbox %s: %v %v", id, rows, err)
		}
	}
	got, err := backend.Get(source)
	if err != nil || got.Status != "open" {
		t.Fatalf("original changed: %+v %v", got, err)
	}
	for _, id := range []string{"legacy", "wakeable", "service", "inbox"} {
		if _, _, err := rs.SendGuarded(SendReq{From: "legacy", To: id, Title: "compatible", Type: "review", Instructions: "x"}); err != nil {
			t.Fatalf("legacy destination %s: %v", id, err)
		}
	}
	if err := rs.AckItem(source); err != nil {
		t.Fatal(err)
	}
	// Registry guard never applies to registration, reads or acknowledgement.
	if err := rs.RegisterAgent("legacy", 4321); err != nil {
		t.Fatal(err)
	}
	if _, err := rs.Inbox("legacy"); err != nil {
		t.Fatal(err)
	}
}
