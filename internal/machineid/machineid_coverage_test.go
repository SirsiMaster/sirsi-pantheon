package machineid

import "testing"

func TestMachineIDProbeInjectionAndSameMachine(t *testing.T) {
	old := GetProbe()
	defer SetProbe(old)
	SetProbe(func() string { return "injected-machine-id" })
	if got := MachineID(); got != "injected-machine-id" {
		t.Fatalf("MachineID() = %q", got)
	}
	if got := GetProbe()(); got != "injected-machine-id" {
		t.Fatalf("GetProbe()() = %q", got)
	}

	for _, tc := range []struct {
		record, host string
		want         bool
	}{
		{"same", "same", true},
		{"foreign", "local", false},
		{"", "local", true},
		{"foreign", "", true},
		{"", "", true},
	} {
		if got := SameMachine(tc.record, tc.host); got != tc.want {
			t.Errorf("SameMachine(%q, %q) = %v, want %v", tc.record, tc.host, got, tc.want)
		}
	}
}

func TestMachineIDProbeAndCache(t *testing.T) {
	// Exercise the platform probe and the once-per-process cache without
	// replacing the production function used by the other tests.
	machineIDCacheMu.Lock()
	oldID, oldDone := machineIDCache, machineIDDone
	machineIDCache, machineIDDone = "", false
	machineIDCacheMu.Unlock()
	defer func() {
		machineIDCacheMu.Lock()
		machineIDCache, machineIDDone = oldID, oldDone
		machineIDCacheMu.Unlock()
	}()
	first := defaultMachineID()
	second := defaultMachineID()
	if first != second {
		t.Fatalf("cached machine id changed: %q -> %q", first, second)
	}
	if first != "" && !LooksLikeMachineID(first) {
		t.Fatalf("probe returned an invalid machine id shape: %q", first)
	}
}
