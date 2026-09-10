package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// selfMarkerThread resolves the current session's existing thread on the skip
// path (SSA #731): it matches the single live thread on this host by PID, and
// declines to guess when there is none or more than one.
func TestSelfMarkerThreadResolvesExistingThreadByPID(t *testing.T) {
	host := "h1"
	tr := &router.ThreadRegistry{Threads: map[string]*router.Thread{
		"thr-self":   {ThreadID: "thr-self", AgentID: "ra", PID: 4242, Host: "h1", Status: router.ThreadStatusActive},
		"thr-other":  {ThreadID: "thr-other", AgentID: "claude-io", PID: 9999, Host: "h1", Status: router.ThreadStatusActive},
		"thr-remote": {ThreadID: "thr-remote", AgentID: "ra", PID: 4242, Host: "h2", Status: router.ThreadStatusActive},
		"thr-dead":   {ThreadID: "thr-dead", AgentID: "ra", PID: 4242, Host: "h1", Status: router.ThreadStatusReaped},
	}}
	self := []router.DiscoverAction{{Proc: router.DiscoveredProc{PID: 4242}, Outcome: router.OutcomeSkip, AgentID: "ra"}}

	a, th := selfMarkerThread(tr, self, host)
	if a != "ra" || th != "thr-self" {
		t.Fatalf("must resolve the live local thread by pid: agent=%q thread=%q", a, th)
	}
	// no matching live thread → no guess.
	if a, th := selfMarkerThread(tr, []router.DiscoverAction{{Proc: router.DiscoveredProc{PID: 1}, Outcome: router.OutcomeSkip}}, host); a != "" || th != "" {
		t.Fatalf("no match must yield empty: %q %q", a, th)
	}
	// multi-proc discover (len != 1) never writes the caller's marker.
	if a, th := selfMarkerThread(tr, append(self, self[0]), host); a != "" || th != "" {
		t.Fatalf("multi-action must yield empty: %q %q", a, th)
	}
	// two live threads for the same pid → ambiguous, no guess.
	tr.Threads["thr-dup"] = &router.Thread{ThreadID: "thr-dup", AgentID: "ra", PID: 4242, Host: "h1", Status: router.ThreadStatusActive}
	if a, th := selfMarkerThread(tr, self, host); a != "" || th != "" {
		t.Fatalf("ambiguous pid must yield empty: %q %q", a, th)
	}
}
