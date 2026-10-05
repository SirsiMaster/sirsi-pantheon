package main

import (
	"reflect"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// Both directions: conformant ids are not flagged, bare/short ids are, and the
// migration map only names conformant targets.
func TestNonconformantNames(t *testing.T) {
	reg := &router.Registry{Agents: map[string]router.AgentConfig{
		"ra-router-m1": {}, "claude-finalwishes-m1": {}, "claude-finalwishes-m1-fw-r02": {},
		"ra": {}, "mercury-m5": {}, "Claude-Home-M1": {},
	}}
	got := nonconformantNames(reg)
	want := []string{"Claude-Home-M1", "mercury-m5", "ra"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nonconformant = %v, want %v", got, want)
	}
	for id, to := range nameMigrationMap {
		if len(bad2(to)) != 0 {
			t.Errorf("migration target %q for %q is itself not conformant", to, id)
		}
	}
}

func bad2(id string) []string {
	return nonconformantNames(&router.Registry{Agents: map[string]router.AgentConfig{id: {}}})
}
