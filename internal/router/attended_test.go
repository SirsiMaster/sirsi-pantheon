package router

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeThreads persists a registry the way the CLI would, so these tests
// exercise the real load path rather than an in-memory shortcut.
func writeThreads(t *testing.T, threads ...*Thread) string {
	t.Helper()
	root := t.TempDir()
	m := make(map[string]*Thread, len(threads))
	for _, thr := range threads {
		m[thr.ThreadID] = thr
	}
	if err := SaveThreadRegistry(root, &ThreadRegistry{Threads: m}); err != nil {
		t.Fatal(err)
	}
	return root
}

func attendedThread(agent, surface string, age time.Duration) *Thread {
	return &Thread{
		ThreadID:   "thr-" + agent + "-" + surface,
		AgentID:    agent,
		Surface:    surface,
		Status:     ThreadStatusActive,
		LastSeenAt: time.Now().UTC().Add(-age),
	}
}

// THE deadlock case, and the reason this predicate is not AgentArmed.
//
// surfaceWorker is non-resident and watches its inbox, so every
// capability-shaped check counts the headless backstop worker as a live
// consumer. If the worker gated its own deferral on that, it would defer to
// itself forever and the item would never be processed at all — strictly worse
// than the unconditional wait being replaced.
func TestAttendedSessionLive_WorkerDoesNotCountAsItsOwnAttendedSession(t *testing.T) {
	root := writeThreads(t, attendedThread("claude-home", surfaceWorker, 10*time.Second))

	if AttendedSessionLive(root, "claude-home") {
		t.Fatal("the backstop worker counted itself as the attended session — it would defer to itself forever and never process the item")
	}
	// Sanity: the capability-shaped check really does count it, which is
	// exactly why AttendedSessionLive cannot be built on it.
	if !AgentArmed(root, "claude-home") {
		t.Fatal("AgentArmed no longer counts a live worker; this test's premise needs revisiting")
	}
}

// stubWatcher installs both loop-process probes (thread-id and agent-id). Tests
// must never pgrep the real host — the answer depends on whichever agents are live.
func stubWatcher(t *testing.T, alive bool) {
	t.Helper()
	setWatcherAliveFn(func(string) bool { return alive })
	setWatcherAliveByAgentFn(func(string) bool { return alive })
	t.Cleanup(func() {
		setWatcherAliveFn(nil)
		setWatcherAliveByAgentFn(nil)
	})
}

func TestAttendedSessionLive_LiveClaudeSessionCounts(t *testing.T) {
	stubWatcher(t, true)
	root := writeThreads(t, attendedThread("claude-home", surfaceClaude, 10*time.Second))

	if !AttendedSessionLive(root, "claude-home") {
		t.Error("a claude session with a live /loop must count — the worker must keep deferring to it")
	}
}

// ADR-079 D1 reverses the earlier loop-evidence rule. ADR-065 retired the watch
// loop, so a fresh claude session with no loop process is the normal attended
// state, not a stall. Requiring the loop made every Claude lane read as
// unattended, and a headless worker answered beside the live session (Ra,
// 2026-10-26). Owner: the interactive session is the lane; a quiet lane is
// surfaced as needs-owner, never raced by a second consumer.
func TestAttendedSessionLive_FreshClaudeSessionWithoutLoopCounts(t *testing.T) {
	stubWatcher(t, false)
	root := writeThreads(t, attendedThread("ra", surfaceClaude, 10*time.Second))

	if !AttendedSessionLive(root, "ra") {
		t.Error("a heartbeat-fresh claude session read as unattended because no watch loop exists — the headless worker would answer beside it")
	}
}

// The heartbeat is now the whole proof for claude too, so staleness must end it.
func TestAttendedSessionLive_StaleClaudeSessionDoesNotCount(t *testing.T) {
	stubWatcher(t, true)
	root := writeThreads(t, attendedThread("ra", surfaceClaude, 2*time.Hour))

	if AttendedSessionLive(root, "ra") {
		t.Error("a stale claude record counted as attended — dispatch would be suppressed for a session that is gone")
	}
}

func TestAttendedSessionLive_CodexSessionCounts(t *testing.T) {
	root := writeThreads(t, attendedThread("codex-pantheon", surfaceCodex, 10*time.Second))

	if !AttendedSessionLive(root, "codex-pantheon") {
		t.Error("a fresh attended codex session must count")
	}
}

// A stale attended session is not attending: the session exited, its record
// lingered, and the worker would go on deferring to a thread that is never
// coming back.
func TestAttendedSessionLive_StaleSessionDoesNotCount(t *testing.T) {
	root := writeThreads(t, attendedThread("codex-pantheon", surfaceCodex, 2*time.Hour))

	if AttendedSessionLive(root, "codex-pantheon") {
		t.Error("a stale attended record counted as live — the worker would keep deferring to a dead session")
	}
}

func TestAttendedSessionLive_SuspendedAndTerminalDoNotCount(t *testing.T) {
	for _, st := range []ThreadStatus{ThreadStatusSuspended, ThreadStatusClosed} {
		thr := attendedThread("claude-home", surfaceClaude, 10*time.Second)
		thr.Status = st
		if AttendedSessionLive(writeThreads(t, thr), "claude-home") {
			t.Errorf("status %q counted as an attended session", st)
		}
	}
}

// Resident UI surfaces heartbeat but do not consume (codex-pantheon, PR #389),
// so their liveness is not evidence that anything will claim the item.
func TestAttendedSessionLive_ResidentSurfacesDoNotCount(t *testing.T) {
	for _, s := range []string{surfaceMenubar, surfaceTUI, surfaceMacApp} {
		if AttendedSessionLive(writeThreads(t, attendedThread("claude-home", s, 10*time.Second)), "claude-home") {
			t.Errorf("resident surface %q counted as an attended session", s)
		}
	}
}

func TestAttendedSessionLive_OtherAgentsSessionDoesNotCount(t *testing.T) {
	root := writeThreads(t, attendedThread("claude-pantheon", surfaceClaude, 10*time.Second))

	if AttendedSessionLive(root, "claude-home") {
		t.Error("another agent's live session counted — deferral must be per-agent")
	}
}

func TestAttendedSessionLive_NoThreadsAtAllMeansNobodyIsAttending(t *testing.T) {
	if AttendedSessionLive(t.TempDir(), "claude-home") {
		t.Error("an empty registry reported an attended session; the worker would never collapse its wait")
	}
}

// Fail direction (A36/ADR-057). An unreadable registry must report NOT
// attended, so the worker takes the item.
//
// This test previously asserted the opposite, on the reasoning that returning
// true "degrades to the previous behavior" of waiting. That was backwards. The
// registry is the only authority that can say whether an attended session
// exists; when it cannot answer, returning true fabricates an attended consumer
// out of an authority failure and re-creates the fixed idle window this whole
// function exists to remove. The item then sits unclaimed while every surface
// reports someone is handling it — the silent-strand failure mode, not a safe
// default.
//
// Taking the item is the recoverable direction: worst case an operator sees
// duplicated work, which is visible and bounded. The other direction produces
// work nobody picks up and no signal that anything is wrong.
func TestAttendedSessionLive_UnreadableRegistryReportsNotAttended(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "threads.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if AttendedSessionLive(root, "claude-home") {
		t.Error("an unreadable registry reported ATTENDED — an authority failure must surface as absence, never as a fabricated live consumer")
	}
}
