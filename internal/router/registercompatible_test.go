package router

import (
	"encoding/json"
	"testing"
)

func TestRegisterCompatibleOccupiedAndRacedTargetsRefuse(t *testing.T) {
	for _, mode := range []string{"session", "workstream", "terminal", "suspended", "raced-session", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			intent := &Thread{EnsureCompatible: true, ThreadID: "computed", AgentID: "own", SessionID: "session", Host: "host", MachineID: "machine", Surface: "codex", PID: 400, StartTime: "B", Repo: "repo", Workstream: "work", Watches: []string{"own"}, WakeMechanism: "automation"}
			occupied := *intent
			occupied.EnsureCompatible = false
			occupied.Status = ThreadStatusActive
			switch mode {
			case "session", "raced-session":
				occupied.SessionID = "unrelated"
			case "workstream":
				occupied.Workstream = "other"
			case "terminal":
				occupied.Status = ThreadStatusClosed
			case "suspended":
				occupied.Status = ThreadStatusSuspended
			}
			state := &ThreadRegistry{Threads: map[string]*Thread{}}
			if mode != "raced-session" {
				state.Threads["computed"] = &occupied
			}
			before, _ := json.Marshal(&occupied)
			saves, loads := 0, 0
			load := func(string) (*ThreadRegistry, error) {
				loads++
				b, _ := json.Marshal(state)
				var out ThreadRegistry
				if err := json.Unmarshal(b, &out); err != nil {
					return nil, err
				}
				return &out, nil
			}
			save := func(_ string, r *ThreadRegistry) error {
				saves++
				if !r.createOnly["computed"] {
					t.Fatal("missing actual insert-only CAS intent")
				}
				// Models the store's INSERT ON CONFLICT DO NOTHING returning false.
				state.Threads["computed"] = &occupied
				return ErrLostLifecycleFence
			}
			out, err := registerThreadRetry("", intent, load, save)
			if mode == "duplicate" {
				if err != nil || out == nil || saves != 0 {
					t.Fatalf("duplicate wrote: %v %d", err, saves)
				}
			} else if err == nil {
				t.Fatal("occupied target accepted")
			}
			after, _ := json.Marshal(state.Threads["computed"])
			if string(before) != string(after) {
				t.Fatal("occupied target changed")
			}
			if mode == "raced-session" && (saves != 1 || loads != 2) {
				t.Fatalf("race was not rederived: saves%d loads%d", saves, loads)
			}
			if mode != "raced-session" && saves != 0 {
				t.Fatal("preexisting conflict reached save")
			}
		})
	}
}
