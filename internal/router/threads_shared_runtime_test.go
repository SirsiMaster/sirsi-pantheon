package router

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestCodexRegistrationsIsolateThreadsSharingRootAndRuntime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(SessionIDEnv, "")
	t.Setenv("CODEX_SESSION_ID", "shared-root-session")
	if err := WriteSessionAgentMarker("shared-root-session", "legacy-root-agent"); err != nil {
		t.Fatal(err)
	}
	if err := WriteSessionThreadMarker("shared-root-session", "legacy-root-thread"); err != nil {
		t.Fatal(err)
	}
	if err := WriteSessionAgentMarker("sibling-execution", "sibling-agent"); err != nil {
		t.Fatal(err)
	}
	if err := WriteSessionThreadMarker("sibling-execution", "sibling-record"); err != nil {
		t.Fatal(err)
	}

	// Exercise the production retry/registration path with injected memory-only
	// load/save functions; no router store or host/process identity probe is used.
	state := &ThreadRegistry{Threads: map[string]*Thread{}}
	clone := func(reg *ThreadRegistry) *ThreadRegistry {
		data, err := json.Marshal(reg)
		if err != nil {
			t.Fatal(err)
		}
		var copied ThreadRegistry
		if err := json.Unmarshal(data, &copied); err != nil {
			t.Fatal(err)
		}
		if copied.Threads == nil {
			copied.Threads = map[string]*Thread{}
		}
		return &copied
	}
	load := func(string) (*ThreadRegistry, error) { return clone(state), nil }
	save := func(_ string, next *ThreadRegistry) error {
		state = clone(next)
		return nil
	}
	register := func(threadID string) *Thread {
		t.Helper()
		t.Setenv("CODEX_THREAD_ID", threadID)
		sessionKey, err := CheckedCurrentSessionID()
		if err != nil || sessionKey != threadID {
			t.Fatalf("Codex marker key = %q, %v; want current thread %q", sessionKey, err, threadID)
		}
		intent := &Thread{
			AgentID: "codex-fixture", Surface: "codex", SessionID: sessionKey,
			Host: "same-host", MachineID: "same-machine", PID: 4242, StartTime: "same-source-anchor",
		}
		out, err := registerThreadRetry("inert-router", intent, load, save)
		if err != nil {
			t.Fatalf("inert registration for %s: %v", threadID, err)
		}
		return out
	}
	writeCurrentMarkers := func(threadID string, record *Thread) {
		t.Helper()
		if err := WriteSessionAgentMarker(threadID, record.AgentID); err != nil {
			t.Fatal(err)
		}
		if err := WriteSessionThreadMarker(threadID, record.ThreadID); err != nil {
			t.Fatal(err)
		}
	}
	assertMarkers := func(key, agent, thread string) {
		t.Helper()
		if got := ReadSessionAgentMarker(key); got != agent {
			t.Fatalf("agent marker %q = %q, want %q", key, got, agent)
		}
		if got := ReadSessionThreadMarker(key); got != thread {
			t.Fatalf("thread marker %q = %q, want %q", key, got, thread)
		}
	}

	parent := register("parent-execution")
	if parent.SessionID != "parent-execution" || parent.Host != "same-host" || parent.PID != 4242 || parent.StartTime != "same-source-anchor" {
		t.Fatalf("parent registration did not retain current-thread/runtime identity: %+v", parent)
	}
	writeCurrentMarkers("parent-execution", parent)
	parentBefore := clone(state).Threads[parent.ThreadID]

	child := register("child-execution")
	if child.ThreadID == parent.ThreadID || child.SessionID != "child-execution" || len(state.Threads) != 2 {
		t.Fatalf("shared root/runtime did not create distinct thread records: parent=%+v child=%+v count=%d", parent, child, len(state.Threads))
	}
	if child.Host != parent.Host || child.PID != parent.PID || child.StartTime != parent.StartTime {
		t.Fatalf("fixture executions did not retain the same host/PID/source anchor: parent=%+v child=%+v", parent, child)
	}
	writeCurrentMarkers("child-execution", child)
	childAgentMarker, childThreadMarker := ReadSessionAgentMarker("child-execution"), ReadSessionThreadMarker("child-execution")

	childAgain := register("child-execution")
	if childAgain.ThreadID != child.ThreadID || childAgain.SessionID != child.SessionID || len(state.Threads) != 2 {
		t.Fatalf("child re-registration did not reuse its own record: first=%+v again=%+v count=%d", child, childAgain, len(state.Threads))
	}
	if !reflect.DeepEqual(state.Threads[parent.ThreadID], parentBefore) {
		t.Fatalf("child registration changed parent record: before=%+v after=%+v", parentBefore, state.Threads[parent.ThreadID])
	}
	assertMarkers("shared-root-session", "legacy-root-agent", "legacy-root-thread")
	assertMarkers("sibling-execution", "sibling-agent", "sibling-record")
	assertMarkers("parent-execution", "codex-fixture", parent.ThreadID)
	assertMarkers("child-execution", childAgentMarker, childThreadMarker)
}

func TestRegistrationSharedRuntimeDoesNotAdoptDifferentSubject(t *testing.T) {
	for _, tc := range []struct{ name, oldSession, newSession, oldSurface, newSurface string }{
		{"distinct sessions", "parent", "helper", "codex", "codex"},
		{"missing incoming session", "parent", "", "codex", "codex"},
		{"new session cannot adopt legacy", "", "helper", "codex", "codex"},
		{"distinct surfaces", "parent", "parent", "codex", "worker"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := &Thread{ThreadID: "parent-thread", AgentID: "same-agent", SessionID: tc.oldSession,
				Surface: tc.oldSurface, PID: 4242, StartTime: "same-native-start", Host: "same-host", MachineID: "same-machine",
				Repo: "/parent-repo", Workstream: "parent-work", CurrentItem: "parent-task", Watches: []string{"same-agent"},
				Status: ThreadStatusActive, LastSeenAt: time.Now().UTC(), StartedAt: time.Now().UTC()}
			before := *parent
			reg := &ThreadRegistry{Threads: map[string]*Thread{parent.ThreadID: parent}}
			intent := &Thread{AgentID: "same-agent", SessionID: tc.newSession, Surface: tc.newSurface, PID: parent.PID,
				StartTime: parent.StartTime, Host: parent.Host, MachineID: parent.MachineID,
				Repo: "/helper-repo", Workstream: "helper-work", CurrentItem: "helper-task"}
			out, err := registerThreadAttempt("unused", intent, reg, "new-helper-thread", "", func(string, *ThreadRegistry) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			if out.ThreadID != "new-helper-thread" || len(reg.Threads) != 2 {
				t.Fatalf("shared native PID adopted other subject: got %s, records%d", out.ThreadID, len(reg.Threads))
			}
			if !reflect.DeepEqual(*parent, before) {
				t.Fatal("registration changed parent metadata or heartbeat")
			}
			if out.SessionID != tc.newSession || out.Surface != tc.newSurface {
				t.Fatal("new subject identity changed")
			}
		})
	}
}

func TestRegistrationSharedRuntimeSameSubjectStillReuses(t *testing.T) {
	for _, session := range []string{"", "same-session"} {
		t.Run("session="+session, func(t *testing.T) {
			parent := &Thread{ThreadID: "original", AgentID: "same-agent", SessionID: session, Surface: "codex", PID: 4242,
				StartTime: "same-start", Host: "same-host", MachineID: "same-machine", Status: ThreadStatusActive, LastSeenAt: time.Now().UTC()}
			reg := &ThreadRegistry{Threads: map[string]*Thread{"original": parent}}
			intent := *parent
			intent.ThreadID = ""
			intent.CurrentItem = "resumed-own-task"
			out, err := registerThreadAttempt("unused", &intent, reg, "must-not-mint", "", func(string, *ThreadRegistry) error { return nil })
			if err != nil || out.ThreadID != "original" || len(reg.Threads) != 1 || out.CurrentItem != "resumed-own-task" {
				t.Fatalf("same subject did not reuse: %+v %v", out, err)
			}
		})
	}
}

func TestRegistrationPinnedThreadCannotRebindSessionOrSurface(t *testing.T) {
	for _, tc := range []struct{ name, session, surface string }{
		{"different session", "helper", "codex"}, {"missing session", "", "codex"}, {"different surface", "parent", "worker"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := &Thread{ThreadID: "pinned", AgentID: "same-agent", SessionID: "parent", Surface: "codex", PID: 4242,
				StartTime: "same-start", Host: "same-host", MachineID: "same-machine", Status: ThreadStatusActive, LastSeenAt: time.Now().UTC()}
			before := *parent
			reg := &ThreadRegistry{Threads: map[string]*Thread{"pinned": parent}}
			intent := *parent
			intent.SessionID, intent.Surface, intent.Repo = tc.session, tc.surface, "/foreign-repo"
			saved := false
			_, err := registerThreadAttempt("unused", &intent, reg, "unused", "", func(string, *ThreadRegistry) error { saved = true; return nil })
			if err == nil || saved || !reflect.DeepEqual(*reg.Threads["pinned"], before) {
				t.Fatal("pinned subject was rebound or saved")
			}
		})
	}
}
