package router

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

func TestReapStrayThreadsForAgentLeavesOtherAgents(t *testing.T) {
	t.Setenv(routercfg.StoreWakeEnv, "0")
	oldMID := getMachineIDFn()
	setMachineIDFn(func() string { return "fixture-machine" })
	defer setMachineIDFn(oldMID)
	defer perPIDProbe(t, 60000, 60002)()
	root := t.TempDir()
	reg := &ThreadRegistry{Threads: map[string]*Thread{
		"a-live": {ThreadID: "a-live", AgentID: "fixture-a", Surface: "worker", PID: 60000, StartTime: "sig", MachineID: "fixture-machine", Status: ThreadStatusActive},
		"a-old":  {ThreadID: "a-old", AgentID: "fixture-a", Surface: "worker", MachineID: "fixture-machine", Status: ThreadStatusSuspended},
		"b-live": {ThreadID: "b-live", AgentID: "fixture-b", Surface: "worker", PID: 60002, StartTime: "sig", MachineID: "fixture-machine", Status: ThreadStatusActive},
		"b-old":  {ThreadID: "b-old", AgentID: "fixture-b", Surface: "worker", MachineID: "fixture-machine", Status: ThreadStatusSuspended},
	}}
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}
	retired, err := ReapStrayThreadsForAgent(root, "fixture-a")
	if err != nil || len(retired) != 1 || retired[0].ThreadID != "a-old" {
		t.Fatalf("retired=%+v err=%v", retired, err)
	}
	out, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if out.Threads["a-old"].Status != ThreadStatusClosed || out.Threads["b-old"].Status != ThreadStatusSuspended {
		t.Fatalf("scope crossed: a=%s b=%s", out.Threads["a-old"].Status, out.Threads["b-old"].Status)
	}
}
