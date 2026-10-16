package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestServiceActorsAreNotUnstaffedModelSeats(t *testing.T) {
	reg := &router.Registry{Agents: map[string]router.AgentConfig{
		"horus":        {ID: "horus", Type: "service"},
		"typed-runner": {ID: "typed-runner", Type: "service"},
		"owner":        {ID: "owner", Type: "human"},
		"worker":       {ID: "worker", Type: "codex"},
	}}
	for _, id := range []string{"horus", "typed-runner", "owner"} {
		if needsModelStaffing(reg, id) {
			t.Fatalf("actor %s incorrectly treated as an empty model seat", id)
		}
	}
	for _, id := range []string{"worker", "missing"} {
		if !needsModelStaffing(reg, id) {
			t.Fatalf("model or unknown lane %s silently suppressed", id)
		}
	}
	if !needsModelStaffing(nil, "missing") {
		t.Fatal("unknown registry silently credited")
	}
}
