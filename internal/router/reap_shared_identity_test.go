package router

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

func TestSharedReaperRequiresKnownMatchingMachine(t *testing.T) {
	for _, tc := range []struct {
		name, owner, observer string
		wantReap              bool
	}{
		{"same host", "studio", "studio", true},
		{"foreign host", "studio", "m5", false},
		{"unknown observer", "studio", "", false},
		{"unknown owner", "", "m5", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("SIRSI_ROUTER_DB", filepath.Join(root, "router.db"))
			t.Setenv(routercfg.StoreWakeEnv, "0")
			reg := &ThreadRegistry{Threads: map[string]*Thread{"fixture": {
				ThreadID: "fixture", AgentID: "fixture-agent", Surface: "fixture",
				PID: 4001, MachineID: tc.owner, Status: ThreadStatusActive, LastSeenAt: time.Now().UTC(),
			}}}
			if err := SaveThreadRegistry(root, reg); err != nil {
				t.Fatal(err)
			}
			t.Setenv(routercfg.StoreWakeEnv, "1")
			oldMachine, oldPID := getMachineIDFn(), getPIDStateFn()
			setMachineIDFn(func() string { return tc.observer })
			probes := 0
			setPIDStateFn(func(int) PIDState { probes++; return PIDGone })
			t.Cleanup(func() { setMachineIDFn(oldMachine); setPIDStateFn(oldPID) })
			reaped, err := ReapDeadThreads(root)
			if err != nil {
				t.Fatal(err)
			}
			if (len(reaped) == 1) != tc.wantReap {
				t.Fatalf("reaped=%v, wantReap=%v", reaped, tc.wantReap)
			}
			if !tc.wantReap && probes != 0 {
				t.Fatalf("foreign or unknown host PID was probed %d times", probes)
			}
			got, err := LoadThreadRegistry(root)
			if err != nil {
				t.Fatal(err)
			}
			want := ThreadStatusActive
			if tc.wantReap {
				want = ThreadStatusReaped
			}
			if got.Threads["fixture"].Status != want {
				t.Fatalf("status=%s want %s", got.Threads["fixture"].Status, want)
			}
		})
	}
}
