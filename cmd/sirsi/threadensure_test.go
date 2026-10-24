package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

type ensureFixture struct {
	caller     attendedEnsureCaller
	rows       map[string]*router.Thread
	pid        int
	start      string
	anchorErr  error
	trace      []string
	registered *router.Thread
	heartbeats int
}

func newEnsureFixture() *ensureFixture {
	c := attendedEnsureCaller{"real-session", "codex-pantheon", "original", "own-machine"}
	return &ensureFixture{caller: c, pid: 400, start: "generation-A", rows: map[string]*router.Thread{
		"original": {ThreadID: "original", AgentID: c.agent, SessionID: c.session, MachineID: c.machine,
			Host: "own-host", Surface: "codex", Repo: "/owned/repo", Workstream: "original-task",
			Watches: []string{"codex-pantheon"}, WakeMechanism: "automation", PID: 400, StartTime: "generation-A", Status: router.ThreadStatusActive,
			AnswerRole: true, ConsumerCapable: true, CurrentItem: "owned-item"},
	}}
}

func cloneEnsureThread(t *router.Thread) *router.Thread {
	if t == nil {
		return nil
	}
	o := *t
	o.Watches = append([]string(nil), t.Watches...)
	return &o
}

func (f *ensureFixture) ops() attendedEnsureOps {
	return attendedEnsureOps{
		load: func(id string) (*router.Thread, error) {
			f.trace = append(f.trace, "load:"+id)
			return cloneEnsureThread(f.rows[id]), nil
		},
		anchor: func() (int, string, error) { f.trace = append(f.trace, "anchor"); return f.pid, f.start, f.anchorErr },
		register: func(in *router.Thread) (*router.Thread, error) {
			f.trace = append(f.trace, "register")
			f.registered = cloneEnsureThread(in)
			out := cloneEnsureThread(in)
			out.Status = router.ThreadStatusActive
			f.rows[out.ThreadID] = out
			return cloneEnsureThread(out), nil
		},
		persist: func(session, id string) error {
			f.trace = append(f.trace, "persist")
			if session != f.caller.session {
				return errors.New("foreign marker")
			}
			f.caller.thread = id
			return nil
		},
		heartbeat: func(id string, u router.HeartbeatUpdate) (*router.Thread, error) {
			f.trace = append(f.trace, "heartbeat")
			f.heartbeats++
			out := f.rows[id]
			if out == nil || out.PID != u.ExpectedPID || out.StartTime != u.ExpectedStart {
				return nil, errors.New("wrong native generation")
			}
			return cloneEnsureThread(out), nil
		},
	}
}

func TestAttendedEnsureGenerationBeforeHeartbeatAndWait(t *testing.T) {
	f := newEnsureFixture()
	if _, err := ensureAttendedHeartbeat(f.caller, "", router.HeartbeatUpdate{}, f.ops()); err != nil {
		t.Fatal(err)
	}
	if f.registered != nil || f.heartbeats != 1 {
		t.Fatal("same generation must only heartbeat")
	}
	f.trace = nil
	f.pid, f.start = 500, "generation-B"
	old := cloneEnsureThread(f.rows["original"])
	out, err := ensureAttendedHeartbeat(f.caller, "original", router.HeartbeatUpdate{}, f.ops())
	if err != nil {
		t.Fatal(err)
	}
	// The existing caller enters its store-backed wait only after success.
	f.trace = append(f.trace, "wait")
	want := []string{"load:original", "anchor", "register", "load:" + out.ThreadID, "anchor", "persist", "heartbeat", "wait"}
	if !reflect.DeepEqual(f.trace, want) {
		t.Fatalf("preflight/register must precede heartbeat/wait: %v", f.trace)
	}
	if out.PID != 500 || out.StartTime != "generation-B" || !reflect.DeepEqual(out.Watches, old.Watches) || out.Workstream != old.Workstream || out.Repo != old.Repo {
		t.Fatal("scope or generation lost")
	}
	if out.ThreadID == old.ThreadID || !reflect.DeepEqual(f.rows[old.ThreadID], old) || out.AnswerRole || out.ConsumerCapable {
		t.Fatal("predecessor changed or successor borrowed a grant")
	}
	f.trace = nil
	f.registered = nil
	if _, err := ensureAttendedHeartbeat(f.caller, "", router.HeartbeatUpdate{}, f.ops()); err != nil {
		t.Fatal(err)
	}
	if f.registered != nil {
		t.Fatal("next invocation repeated registration")
	}
}

func TestAttendedEnsureTerminalGetsNormalSuccessorNeverRevival(t *testing.T) {
	for _, status := range []router.ThreadStatus{router.ThreadStatusReaped, router.ThreadStatusClosed} {
		t.Run(string(status), func(t *testing.T) {
			f := newEnsureFixture()
			f.rows["original"].Status = status
			old := cloneEnsureThread(f.rows["original"])
			f.pid, f.start = 500, "generation-B"
			out, err := ensureAttendedHeartbeat(f.caller, "", router.HeartbeatUpdate{}, f.ops())
			if err != nil {
				t.Fatal(err)
			}
			if out.ThreadID == "original" || !reflect.DeepEqual(f.rows["original"], old) {
				t.Fatal("terminal original revived or modified")
			}
			if f.registered.AnswerRole || f.registered.ConsumerCapable || f.registered.Lane != nil {
				t.Fatal("successor borrowed capability/result")
			}
			if f.caller.thread != out.ThreadID {
				t.Fatal("returned successor was not persisted")
			}
		})
	}
}

func TestAttendedEnsureSuccessorNameIsIdempotentNotAuthority(t *testing.T) {
	f := newEnsureFixture()
	old := f.rows["original"]
	a := attendedSuccessorID(old, 500, "B")
	if a != attendedSuccessorID(cloneEnsureThread(old), 500, "B") {
		t.Fatal("overlapping ticks chose different successor IDs")
	}
	changed := cloneEnsureThread(old)
	changed.SessionID = "different"
	if attendedSuccessorID(changed, 500, "B") == a {
		t.Fatal("another session got the same successor name")
	}
	if attendedSuccessorID(old, 500, "C") == a {
		t.Fatal("new native generation got predecessor successor name")
	}
}

func TestAttendedEnsureForeignAndUnrelatedRefusedBeforeMutation(t *testing.T) {
	for _, name := range []string{"explicit-unrelated", "agent", "session", "machine", "surface", "unknown", "suspended", "empty-session", "same-generation-terminal"} {
		t.Run(name, func(t *testing.T) {
			f := newEnsureFixture()
			explicit := ""
			switch name {
			case "explicit-unrelated":
				explicit = "someone-else"
			case "agent":
				f.rows["original"].AgentID = "foreign"
			case "session":
				f.rows["original"].SessionID = "other-session"
			case "machine":
				f.rows["original"].MachineID = "foreign"
			case "surface":
				f.rows["original"].Surface = "worker"
			case "unknown":
				delete(f.rows, "original")
			case "suspended":
				f.rows["original"].Status = router.ThreadStatusSuspended
			case "empty-session":
				f.caller.session = ""
			case "same-generation-terminal":
				f.rows["original"].Status = router.ThreadStatusReaped
			}
			if _, err := ensureAttendedHeartbeat(f.caller, explicit, router.HeartbeatUpdate{}, f.ops()); err == nil {
				t.Fatal("unrelated/retired binding accepted")
			}
			if f.registered != nil || f.heartbeats != 0 {
				t.Fatal("refusal mutated or renewed a binding")
			}
		})
	}
}

func TestAttendedEnsureAmbiguousMissingAndChangingRuntimeFailClosed(t *testing.T) {
	for _, name := range []string{"ambiguous", "missing", "no-start", "changes-during-register", "marker-fails", "readback-drift"} {
		t.Run(name, func(t *testing.T) {
			f := newEnsureFixture()
			f.pid, f.start = 500, "generation-B"
			op := f.ops()
			switch name {
			case "ambiguous":
				f.anchorErr = errors.New("two code-mode hosts")
			case "missing":
				f.anchorErr = errors.New("no durable host")
			case "no-start":
				f.start = ""
			case "changes-during-register":
				base := op.register
				op.register = func(in *router.Thread) (*router.Thread, error) {
					out, err := base(in)
					f.start = "generation-C"
					return out, err
				}
			case "marker-fails":
				op.persist = func(string, string) error { return errors.New("read-only marker") }
			case "readback-drift":
				base := op.register
				op.register = func(in *router.Thread) (*router.Thread, error) {
					out, err := base(in)
					f.rows[out.ThreadID].Watches = []string{"codex-pantheon", "foreign"}
					return out, err
				}
			}
			if _, err := ensureAttendedHeartbeat(f.caller, "", router.HeartbeatUpdate{}, op); err == nil {
				t.Fatal("unproved runtime/binding accepted")
			}
			if f.heartbeats != 0 {
				t.Fatal("failure reached heartbeat/wait")
			}
		})
	}
}
