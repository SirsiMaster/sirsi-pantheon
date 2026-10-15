package router

import "testing"

func TestDiscoveryHomeCoordinatorDoesNotCaptureOtherProjects(t *testing.T) {
	reg := testRegistry()
	reg.Agents["software"] = AgentConfig{ID: "software", Type: "codex", Cwd: "/Users/x", Repo: "/Users/x"}
	for _, cwd := range []string{"/Users/x", "/Users/x/Development/unregistered-project", "/Users/x/Documents/old-session"} {
		actions := ReconcileDiscovery(reg, &ThreadRegistry{}, []DiscoveredProc{{PID: 123, Surface: "codex", Cwd: cwd}}, "host1", "/Users/x")
		if len(actions) != 1 || actions[0].Outcome != OutcomeUnmappable {
			t.Fatalf("broad home captured %s: %+v", cwd, actions)
		}
	}
	// The narrower real project still resolves, and the actual explicitly
	// registered home coordinator remains registered rather than being relabeled.
	actions := ReconcileDiscovery(reg, &ThreadRegistry{}, []DiscoveredProc{{PID: 124, Surface: "codex", Cwd: "/Users/x/Development/sirsi-pantheon"}}, "host1", "/Users/x")
	if len(actions) != 1 || actions[0].AgentID != "codex-pantheon" {
		t.Fatalf("specific project lost: %+v", actions)
	}
	threads := &ThreadRegistry{Threads: map[string]*Thread{"own": {ThreadID: "own", AgentID: "software", Host: "host1", PID: 125, Status: ThreadStatusActive}}}
	actions = ReconcileDiscovery(reg, threads, []DiscoveredProc{{PID: 125, Surface: "codex", Cwd: "/Users/x"}}, "host1", "/Users/x")
	if len(actions) != 1 || actions[0].Outcome != OutcomeSkip || actions[0].AgentID != "software" {
		t.Fatalf("explicit identity lost: %+v", actions)
	}
}
