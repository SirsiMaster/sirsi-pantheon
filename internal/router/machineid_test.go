package router

import "testing"

func TestSameMachine(t *testing.T) {
	cases := []struct {
		name    string
		record  string
		current string
		want    bool
	}{
		{"same id → local", "M1", "M1", true},
		{"different id → foreign", "M1", "M2", false},
		{"legacy record (no id) → local", "", "M1", true},
		{"this machine has no probe → treat as local", "M1", "", true},
		{"both unknown → local", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SameMachine(c.record, c.current); got != c.want {
				t.Errorf("SameMachine(%q,%q)=%v want %v", c.record, c.current, got, c.want)
			}
		})
	}
}

func TestMachineID_Injectable(t *testing.T) {
	old := getMachineIDFn()
	setMachineIDFn(func() string { return "INJECTED" })
	defer setMachineIDFn(old)
	if got := MachineID(); got != "INJECTED" {
		t.Errorf("MachineID()=%q want INJECTED", got)
	}
}

// TestRegisterThread_StampsMachineID proves a fresh registration captures the
// machine identity the reaper later scopes on.
func TestRegisterThread_StampsMachineID(t *testing.T) {
	tmp := t.TempDir()
	old := getMachineIDFn()
	setMachineIDFn(func() string { return "STAMP-ME" })
	defer setMachineIDFn(old)

	thr, err := RegisterThread(tmp, &Thread{AgentID: "claude-pantheon", Surface: "worker", PID: 7001})
	if err != nil {
		t.Fatalf("RegisterThread: %v", err)
	}
	if thr.MachineID != "STAMP-ME" {
		t.Errorf("MachineID=%q want STAMP-ME", thr.MachineID)
	}
}
