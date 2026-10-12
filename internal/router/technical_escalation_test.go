package router

import (
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/supervision"
)

func TestSSATechnicalLaneEscalationDoesNotPageOwner(t *testing.T) {
	root := t.TempDir()
	writeGemmaRouteRegistry(t, root)
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "router.db"))
	e := supervision.Escalation{Agent: "example", OpenItems: 2, Why: "no automatic consumer"}
	for range 2 {
		if _, err := RouteLaneEscalations(root, []supervision.Escalation{e}); err != nil {
			t.Fatal(err)
		}
	}
	f, err := dispatch.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	items, err := f.Inbox(technicalEscalationAgent)
	if err != nil || len(items) != 1 || items[0].Type != "proposal" {
		t.Fatalf("technical incident must be delivered once: %+v, %v", items, err)
	}
	owner, err := f.Inbox("owner")
	if err != nil || len(owner) != 0 {
		t.Fatalf("routine lane repair must not demand owner intervention: %+v, %v", owner, err)
	}
}
