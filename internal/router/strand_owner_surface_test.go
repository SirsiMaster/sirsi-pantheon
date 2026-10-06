package router

import "testing"

func TestOwnerSurfaceLanesAreNeverStranded(t *testing.T) {
	reg := &Registry{Agents: map[string]AgentConfig{
		"owner": {ID: "owner", Wake: WakeConfig{Mechanism: WakeOwnerSurface}},
		"alice": {ID: "alice", Wake: WakeConfig{Mechanism: WakeLaunchAgent}},
	}}
	got := computeStranded(t.TempDir(), map[string][]string{"owner": {"a"}, "alice": {"b"}}, nil, noWakeAgents(reg))
	if len(got) != 1 || got[0].AgentID != "alice" {
		t.Fatalf("want only alice stranded, got %+v", got)
	}
}
