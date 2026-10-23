package router

import (
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"os"
	"testing"
)

func TestSessionReuseProcessBinding(t *testing.T) {
	for _, tc := range []struct {
		name               string
		pid                int
		start, probe, want string
	}{{"new-process", 301, "new-start", "", "new-start"}, {"unknown-new-start", 301, "", "", ""}, {"os-probed-start", 301, "", "kernel-start", "kernel-start"}, {"same-pid-unknown", 201, "", "", "old-start"}, {"no-pid-evidence", 0, "", "", "old-start"}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(routercfg.StoreWakeEnv, "0")
			t.Setenv("HOME", t.TempDir())
			t.Setenv("SIRSI_ROUTER_URL", "")
			root := t.TempDir()
			old := getPIDStartFn()
			setPIDStartFn(func(int) string { return tc.probe })
			t.Cleanup(func() { setPIDStartFn(old) })
			first, e := RegisterThread(root, &Thread{AgentID: "binding-lane", Surface: "codex", SessionID: "same-session", Host: "host-a", MachineID: "machine-a", PID: 201, StartTime: "old-start"})
			if e != nil {
				t.Fatal(e)
			}
			second, e := RegisterThread(root, &Thread{AgentID: "binding-lane", Surface: "codex", SessionID: "same-session", Host: "host-a", MachineID: "machine-a", PID: tc.pid, StartTime: tc.start})
			if e != nil {
				t.Fatal(e)
			}
			wantPID := tc.pid
			if wantPID == 0 {
				wantPID = 201
			}
			if second.ThreadID != first.ThreadID || second.PID != wantPID || second.StartTime != tc.want {
				t.Fatalf("binding got id=%s pid=%d start=%q want id=%s pid=%d start=%q", second.ThreadID, second.PID, second.StartTime, first.ThreadID, wantPID, tc.want)
			}
			reg, e := LoadThreadRegistry(root)
			if e != nil {
				t.Fatal(e)
			}
			stored := reg.Threads[first.ThreadID]
			if stored.PID != wantPID || stored.StartTime != tc.want {
				t.Fatal("stored process pair differs")
			}
		})
	}
}
func TestSessionReusePreservesIdentityGuards(t *testing.T) {
	for _, tc := range []struct {
		name, agent, host, machine string
		status                     ThreadStatus
		reuse                      bool
	}{{"live", "lane-a", "host-a", "machine-a", ThreadStatusActive, true}, {"idle", "lane-a", "host-a", "machine-a", ThreadStatusIdle, true}, {"blocked", "lane-a", "host-a", "machine-a", ThreadStatusBlocked, true}, {"suspended", "lane-a", "host-a", "machine-a", ThreadStatusSuspended, false}, {"closed", "lane-a", "host-a", "machine-a", ThreadStatusClosed, false}, {"cross-lane", "lane-b", "host-a", "machine-a", ThreadStatusActive, false}, {"cross-host", "lane-a", "host-b", "machine-a", ThreadStatusActive, false}, {"cross-machine", "lane-a", "host-a", "machine-b", ThreadStatusActive, false}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(routercfg.StoreWakeEnv, "0")
			t.Setenv("HOME", t.TempDir())
			t.Setenv("SIRSI_ROUTER_URL", "")
			root := t.TempDir()
			first, e := RegisterThread(root, &Thread{AgentID: "lane-a", Surface: "codex", SessionID: "same-session", Host: "host-a", MachineID: "machine-a", PID: 201, StartTime: "same-start", Status: tc.status})
			if e != nil {
				t.Fatal(e)
			}
			// Same PID forces the later PID-keyed fallback to respect owner scope too.
			second, e := RegisterThread(root, &Thread{AgentID: tc.agent, Surface: "codex", SessionID: "same-session", Host: tc.host, MachineID: tc.machine, PID: 201, StartTime: "same-start"})
			if e != nil {
				t.Fatal(e)
			}
			if (second.ThreadID == first.ThreadID) != tc.reuse {
				t.Fatalf("reuse=%v want%v", second.ThreadID == first.ThreadID, tc.reuse)
			}
			reg, e := LoadThreadRegistry(root)
			if e != nil {
				t.Fatal(e)
			}
			if reg.Threads[first.ThreadID].Host != "host-a" || reg.Threads[first.ThreadID].MachineID != "machine-a" || reg.Threads[first.ThreadID].AgentID != "lane-a" || reg.Threads[first.ThreadID].Status != tc.status {
				t.Fatal("original identity/state changed")
			}
		})
	}
	_ = os.Getpid()
}
