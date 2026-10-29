package router

import (
	"reflect"
	"testing"
	"time"
)

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
