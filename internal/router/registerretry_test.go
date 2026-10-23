package router

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestRegisterRetryFreshDerivation(t *testing.T) {
	for _, mode := range []string{"heartbeat", "closed", "suspended", "identity", "host", "machine", "session", "start", "agent", "missing", "exhausted", "other-error", "mint"} {
		t.Run(mode, func(t *testing.T) {
			original := &Thread{ThreadID: "own", AgentID: "ra", Host: "host", MachineID: "machine", Surface: "codex", SessionID: "session", PID: 9000, StartTime: "start", Status: ThreadStatusActive, StartedAt: time.Now().UTC()}
			state := &ThreadRegistry{Threads: map[string]*Thread{"own": original, "foreign": {ThreadID: "foreign", AgentID: "other", Host: "elsewhere", Status: ThreadStatusActive, Workstream: "untouched"}}}
			input := *original
			input.ThreadID = ""
			input.Workstream = "requested"
			if mode == "mint" {
				delete(state.Threads, "own")
			}
			loads, saves := 0, 0
			var minted string
			load := func(string) (*ThreadRegistry, error) {
				loads++
				b, _ := json.Marshal(state)
				var r ThreadRegistry
				_ = json.Unmarshal(b, &r)
				return &r, nil
			}
			save := func(_ string, r *ThreadRegistry) error {
				saves++
				if saves == 1 || mode == "exhausted" {
					if mode == "other-error" {
						return errors.New("denied")
					}
					if mode == "mint" {
						for id := range r.Threads {
							if id != "foreign" {
								if minted != "" && minted != id {
									t.Fatal("mint identity drift")
								}
								minted = id
							}
						}
					} else {
						state.Threads["own"].LastError = "competing heartbeat"
						switch mode {
						case "closed":
							state.Threads["own"].Status = ThreadStatusClosed
						case "suspended":
							state.Threads["own"].Status = ThreadStatusSuspended
						case "identity":
							state.Threads["own"].PID++
						case "host":
							state.Threads["own"].Host = "foreign"
						case "machine":
							state.Threads["own"].MachineID = "foreign"
						case "session":
							state.Threads["own"].SessionID = "foreign"
						case "start":
							state.Threads["own"].StartTime = "replaced"
						case "agent":
							state.Threads["own"].AgentID = "foreign"
						case "missing":
							delete(state.Threads, "own")
						}
					}
					state.Threads["foreign"].Workstream = "concurrent foreign update"
					return ErrLostLifecycleFence
				}
				if r.Threads["foreign"].Workstream != "concurrent foreign update" {
					t.Fatal("foreign update overwritten")
				}
				if mode != "mint" && r.Threads["own"].LastError != "competing heartbeat" {
					t.Fatal("stale mutation replayed")
				}
				state = r
				return nil
			}
			out, err := registerThreadRetry("", &input, load, save)
			switch mode {
			case "heartbeat", "mint":
				if err != nil || out == nil || saves != 2 || loads != 2 {
					t.Fatalf("out=%+v err=%v loads=%d saves=%d", out, err, loads, saves)
				}
				if mode == "mint" && out.ThreadID != minted {
					t.Fatal("new mint drift")
				}
			case "exhausted":
				if !errors.Is(err, ErrLostLifecycleFence) || saves != 3 {
					t.Fatalf("budget err=%v saves=%d", err, saves)
				}
			case "other-error":
				if err == nil || saves != 1 || loads != 1 {
					t.Fatal("non-fence retried")
				}
			default:
				if err == nil || saves != 1 {
					t.Fatalf("retired/replaced binding overwritten err=%v saves=%d", err, saves)
				}
			}
		})
	}
}
