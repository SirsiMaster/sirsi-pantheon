package router

import (
	"testing"
)

// testRegistry mirrors the real agents.json shape, including the genuine
// ambiguity present in production (two codex agents share one repo cwd).
func testRegistry() *Registry {
	return &Registry{Agents: map[string]AgentConfig{
		"claude-pantheon":     {ID: "claude-pantheon", Type: "claude", Cwd: "/Users/x/Development/sirsi-pantheon"},
		"codex-pantheon":      {ID: "codex-pantheon", Type: "codex", Cwd: "/Users/x/Development/sirsi-pantheon"},
		"claude-finalwishes":  {ID: "claude-finalwishes", Type: "claude", Cwd: "/Users/x/Development/FinalWishes"},
		"claude-fw-web":       {ID: "claude-fw-web", Type: "claude", Cwd: "/Users/x/Development/FinalWishes/web"},
		"codex-homebrew":      {ID: "codex-homebrew", Type: "codex", Cwd: "/Users/x/Development/homebrew-tools"},
		"codex-homebrewtools": {ID: "codex-homebrewtools", Type: "codex", Cwd: "/Users/x/Development/homebrew-tools"},
	}}
}

func actionFor(actions []DiscoverAction, pid int) (DiscoverAction, bool) {
	for _, a := range actions {
		if a.Proc.PID == pid {
			return a, true
		}
	}
	return DiscoverAction{}, false
}

func TestReconcileDiscovery_RepoMappedSession(t *testing.T) {
	procs := []DiscoveredProc{{PID: 100, Surface: "claude", Cwd: "/Users/x/Development/sirsi-pantheon"}}
	actions := ReconcileDiscovery(testRegistry(), &ThreadRegistry{}, procs, "host1")

	a, ok := actionFor(actions, 100)
	if !ok {
		t.Fatalf("no action for pid 100")
	}
	if a.Outcome != OutcomeRegister {
		t.Fatalf("want register, got %s (%s)", a.Outcome, a.Reason)
	}
	if a.AgentID != "claude-pantheon" {
		t.Fatalf("want claude-pantheon, got %q", a.AgentID)
	}
}

func TestReconcileDiscovery_NestedRepoPrefersMostSpecific(t *testing.T) {
	// A session inside FinalWishes/web must map to claude-fw-web, not the
	// parent claude-finalwishes — longest ancestor wins.
	procs := []DiscoveredProc{{PID: 101, Surface: "claude", Cwd: "/Users/x/Development/FinalWishes/web/src"}}
	actions := ReconcileDiscovery(testRegistry(), &ThreadRegistry{}, procs, "host1")

	a, _ := actionFor(actions, 101)
	if a.Outcome != OutcomeRegister || a.AgentID != "claude-fw-web" {
		t.Fatalf("want register claude-fw-web, got %s/%s", a.Outcome, a.AgentID)
	}
}

func TestReconcileDiscovery_HomeLaunchedIsUnmappable(t *testing.T) {
	// The exact failure that motivated this feature: a session in $HOME.
	procs := []DiscoveredProc{{PID: 102, Surface: "claude", Cwd: "/Users/x"}}
	actions := ReconcileDiscovery(testRegistry(), &ThreadRegistry{}, procs, "host1")

	a, _ := actionFor(actions, 102)
	if a.Outcome != OutcomeUnmappable {
		t.Fatalf("want unmappable for home session, got %s", a.Outcome)
	}
	if a.AgentID != "" {
		t.Fatalf("unmappable session must not be assigned an agent, got %q", a.AgentID)
	}
}

func TestReconcileDiscovery_UnresolvedCwdIsUnmappable(t *testing.T) {
	procs := []DiscoveredProc{{PID: 103, Surface: "claude", Cwd: ""}}
	actions := ReconcileDiscovery(testRegistry(), &ThreadRegistry{}, procs, "host1")

	a, _ := actionFor(actions, 103)
	if a.Outcome != OutcomeUnmappable {
		t.Fatalf("want unmappable for empty cwd, got %s", a.Outcome)
	}
}

func TestReconcileDiscovery_AmbiguousIsNotGuessed(t *testing.T) {
	// Two codex agents share homebrew-tools; reconcile must refuse to guess.
	procs := []DiscoveredProc{{PID: 104, Surface: "codex", Cwd: "/Users/x/Development/homebrew-tools"}}
	actions := ReconcileDiscovery(testRegistry(), &ThreadRegistry{}, procs, "host1")

	a, _ := actionFor(actions, 104)
	if a.Outcome != OutcomeAmbiguous {
		t.Fatalf("want ambiguous, got %s", a.Outcome)
	}
	if a.AgentID != "" {
		t.Fatalf("ambiguous session must not be assigned an agent, got %q", a.AgentID)
	}
}

func TestReconcileDiscovery_SurfaceMustMatch(t *testing.T) {
	// A codex process in the pantheon repo must not map to claude-pantheon.
	procs := []DiscoveredProc{{PID: 105, Surface: "codex", Cwd: "/Users/x/Development/sirsi-pantheon"}}
	actions := ReconcileDiscovery(testRegistry(), &ThreadRegistry{}, procs, "host1")

	a, _ := actionFor(actions, 105)
	if a.Outcome != OutcomeRegister || a.AgentID != "codex-pantheon" {
		t.Fatalf("want register codex-pantheon, got %s/%s", a.Outcome, a.AgentID)
	}
}

func TestReconcileDiscovery_AlreadyRegisteredIsSkipped(t *testing.T) {
	threads := &ThreadRegistry{Threads: map[string]*Thread{
		"thr-live": {ThreadID: "thr-live", AgentID: "claude-pantheon", Status: ThreadStatusActive, PID: 200, Host: "host1"},
	}}
	procs := []DiscoveredProc{{PID: 200, Surface: "claude", Cwd: "/Users/x/Development/sirsi-pantheon"}}
	actions := ReconcileDiscovery(testRegistry(), threads, procs, "host1")

	a, _ := actionFor(actions, 200)
	if a.Outcome != OutcomeSkip {
		t.Fatalf("want skip for already-registered PID, got %s", a.Outcome)
	}
}

func TestReconcileDiscovery_ClosedThreadDoesNotShadowLivePID(t *testing.T) {
	// Dead-anchor cleanup interplay: a reaped (closed) thread for a PID that
	// the OS has since recycled to a NEW live agent must not be treated as
	// "already registered" — the live process gets a fresh registration.
	threads := &ThreadRegistry{Threads: map[string]*Thread{
		"thr-dead": {ThreadID: "thr-dead", AgentID: "claude-pantheon", Status: ThreadStatusClosed, PID: 300, Host: "host1"},
	}}
	procs := []DiscoveredProc{{PID: 300, Surface: "claude", Cwd: "/Users/x/Development/sirsi-pantheon"}}
	actions := ReconcileDiscovery(testRegistry(), threads, procs, "host1")

	a, _ := actionFor(actions, 300)
	if a.Outcome != OutcomeRegister {
		t.Fatalf("closed thread must not shadow a live PID; want register, got %s", a.Outcome)
	}
}

func TestReconcileDiscovery_RemoteHostThreadDoesNotShadow(t *testing.T) {
	// An active thread on another host with the same PID number must not be
	// mistaken for this host's process.
	threads := &ThreadRegistry{Threads: map[string]*Thread{
		"thr-remote": {ThreadID: "thr-remote", AgentID: "claude-pantheon", Status: ThreadStatusActive, PID: 400, Host: "other-host"},
	}}
	procs := []DiscoveredProc{{PID: 400, Surface: "claude", Cwd: "/Users/x/Development/sirsi-pantheon"}}
	actions := ReconcileDiscovery(testRegistry(), threads, procs, "host1")

	a, _ := actionFor(actions, 400)
	if a.Outcome != OutcomeRegister {
		t.Fatalf("remote-host thread must not shadow local PID; want register, got %s", a.Outcome)
	}
}

// A Mac's hostname follows DHCP/mDNS and is not stable: this host had written
// records under `Mac.fios-router.home`, `MacBook-Pro-2.local` and `Mac`, all
// the same machine. Indexing "already registered" by hostname made every
// record written under a previous hostname invisible, so a live session read
// as unregistered and discovery minted it a new thread each pass — the churn
// clock behind "my thread cycles every 1-2 minutes" (claude-home run
// 20260726-2013). Machine id is authoritative when the record carries one.
func TestReconcileDiscovery_StaleHostnameDoesNotRemint(t *testing.T) {
	me := MachineID()
	if me == "" {
		t.Skip("machine id unresolvable on this host")
	}
	threads := &ThreadRegistry{Threads: map[string]*Thread{
		"thr-oldname": {
			ThreadID: "thr-oldname", AgentID: "claude-pantheon", Status: ThreadStatusActive,
			PID: 400, Host: "Mac.fios-router.home", MachineID: me,
		},
	}}
	procs := []DiscoveredProc{{PID: 400, Surface: "claude", Cwd: "/Users/x/Development/sirsi-pantheon"}}
	actions := ReconcileDiscovery(testRegistry(), threads, procs, "MacBook-Pro-2.local")

	a, _ := actionFor(actions, 400)
	if a.Outcome != OutcomeSkip {
		t.Fatalf("same machine under an older hostname must be recognized; want skip, got %s (%s)", a.Outcome, a.Reason)
	}
}

// The id-less counterpart of the above: with no machine id there is nothing
// but the hostname to go on, and a foreign record must still not shadow a
// local PID. This is why the machine-id check is conditional rather than a
// bare SameMachine (which reads a missing id as "mine").
func TestReconcileDiscovery_IDLessRemoteRecordStillDoesNotShadow(t *testing.T) {
	threads := &ThreadRegistry{Threads: map[string]*Thread{
		"thr-remote": {
			ThreadID: "thr-remote", AgentID: "claude-pantheon", Status: ThreadStatusActive,
			PID: 400, Host: "other-host", // no MachineID
		},
	}}
	procs := []DiscoveredProc{{PID: 400, Surface: "claude", Cwd: "/Users/x/Development/sirsi-pantheon"}}
	actions := ReconcileDiscovery(testRegistry(), threads, procs, "host1")

	a, _ := actionFor(actions, 400)
	if a.Outcome != OutcomeRegister {
		t.Fatalf("id-less remote record must not shadow a local PID; want register, got %s", a.Outcome)
	}
}
