package guard

import "testing"

// TestReniceByPID_RefusesProtected locks the A1 safety floor: an auto/one-click
// renice must NEVER deprioritize a critical system process or sirsi itself —
// doing so would starve the machine and make a freeze worse, not better.
func TestReniceByPID_RefusesProtected(t *testing.T) {
	protected := []string{
		"WindowServer", "kernel_task", "launchd", "loginwindow",
		"SystemUIServer", "WindowManager", "coreaudiod",
		"sirsi", "sirsi-agent", "sirsi-menubar", "/usr/sbin/WindowServer",
	}
	for _, name := range protected {
		called := false
		err := reniceByPIDWith(4242, name,
			func(int, int) error { called = true; return nil },
			func(int) error { return nil })
		if err == nil {
			t.Errorf("%q: expected refusal, got nil error", name)
		}
		if called {
			t.Errorf("%q: reniceFn MUST NOT be called for a protected process", name)
		}
	}
}

// TestReniceByPID_AllowsNormal confirms a normal background process is still
// deprioritized (+10) so relief actually works for legitimate CPU hogs.
func TestReniceByPID_AllowsNormal(t *testing.T) {
	gotPID, gotNice, tpCalled := 0, 0, false
	err := reniceByPIDWith(5151, "SomeApp Helper (Renderer)",
		func(pid, nice int) error { gotPID, gotNice = pid, nice; return nil },
		func(int) error { tpCalled = true; return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPID != 5151 || gotNice != 10 {
		t.Errorf("expected renice(5151, 10), got renice(%d, %d)", gotPID, gotNice)
	}
	if !tpCalled {
		t.Error("expected taskpolicy (background QoS) to be applied")
	}
}

// TestReniceByPID_RefusesLowPID keeps the original pid<=1 guard.
func TestReniceByPID_RefusesLowPID(t *testing.T) {
	for _, pid := range []int{0, 1} {
		if err := reniceByPIDWith(pid, "anything",
			func(int, int) error { t.Fatalf("must not renice pid %d", pid); return nil },
			func(int) error { return nil }); err == nil {
			t.Errorf("pid %d: expected refusal", pid)
		}
	}
}
