package guard

import "testing"

func TestFindReliefTarget(t *testing.T) {
	old := getSampleFn()
	defer setSampleFn(old)

	// Highest-CPU first (sampleTopCPU returns sorted desc).
	setSampleFn(func(int) ([]ProcessInfo, error) {
		return []ProcessInfo{
			{PID: 100, Name: "WindowServer", CPUPercent: 95}, // protected — must skip
			{PID: 200, Name: "Google Chrome Helper (Renderer)", CPUPercent: 80},
			{PID: 300, Name: "Slack Helper", CPUPercent: 40},
			{PID: 400, Name: "sirsi-menubar", CPUPercent: 30}, // protected (sirsi) — skip
			{PID: 500, Name: "Finder", CPUPercent: 5},
		}, nil
	})

	// Top NON-protected hog → Chrome (WindowServer is higher but protected).
	if c, err := FindReliefTarget("", 15); err != nil || c == nil || c.PID != 200 {
		t.Fatalf("top hog: want Chrome pid 200, got %+v (err %v)", c, err)
	}
	// Name hint wins over raw CPU rank → Slack.
	if c, _ := FindReliefTarget("slack", 15); c == nil || c.PID != 300 {
		t.Errorf("hint 'slack': want pid 300, got %+v", c)
	}
	// Below min-cpu → nil (Finder at 5%).
	if c, _ := FindReliefTarget("finder", 15); c != nil {
		t.Errorf("Finder at 5%% < min-cpu 15: want nil, got %+v", c)
	}
	// Only a protected process is above the bar → nil (WindowServer 95).
	if c, _ := FindReliefTarget("", 90); c != nil {
		t.Errorf("only protected above 90%%: want nil, got %+v", c)
	}
}

// TestRelieve_RefusesProtected locks the A1 floor at the relief layer too: the
// refusal happens inside reniceByPID BEFORE any real renice, so this never
// touches a process on the host.
func TestRelieve_RefusesProtected(t *testing.T) {
	if err := Relieve(&ReliefCandidate{PID: 4242, Name: "WindowServer"}); err == nil {
		t.Error("Relieve must refuse a protected process")
	}
	if err := Relieve(nil); err == nil {
		t.Error("Relieve(nil) must error")
	}
}
