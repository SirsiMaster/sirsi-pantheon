package guard

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// Mock command runner for throttle tests
type mockThrottleCmdRunner struct {
	calls  []string
	errors map[string]error
}

func (m *mockThrottleCmdRunner) run(name string, args ...string) (string, error) {
	call := name + " " + strings.Join(args, " ")
	m.calls = append(m.calls, call)
	if err, ok := m.errors[call]; ok {
		return "", err
	}
	return "", nil
}

func newMockThrottler() (*Throttler, *mockThrottleCmdRunner) {
	mock := &mockThrottleCmdRunner{errors: make(map[string]error)}
	t := NewThrottler()
	t.cmdRunner = mock.run
	return t, mock
}

// ── Throttle Tests ──────────────────────────────────────────────────────

func TestThrottle_Basic(t *testing.T) {
	throttler, mock := newMockThrottler()

	err := throttler.Throttle(12345, "language_server", 15.5, ThrottleMedium)
	if err != nil {
		t.Fatalf("Throttle failed: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("Expected 1 renice call, got %d", len(mock.calls))
	}
	if mock.calls[0] != "renice +10 -p 12345" {
		t.Errorf("Expected 'renice +10 -p 12345', got %q", mock.calls[0])
	}

	if !throttler.IsThrottled(12345) {
		t.Error("Process should be marked as throttled")
	}
	if throttler.ThrottledCount() != 1 {
		t.Errorf("ThrottledCount = %d, want 1", throttler.ThrottledCount())
	}
}

func TestThrottle_MildLevel(t *testing.T) {
	throttler, mock := newMockThrottler()

	err := throttler.Throttle(100, "test_proc", 10.0, ThrottleMild)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mock.calls[0], "+5") {
		t.Errorf("ThrottleMild should use +5, got %q", mock.calls[0])
	}
}

func TestThrottle_HardLevel(t *testing.T) {
	throttler, mock := newMockThrottler()

	err := throttler.Throttle(100, "test_proc", 50.0, ThrottleHard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mock.calls[0], "+15") {
		t.Errorf("ThrottleHard should use +15, got %q", mock.calls[0])
	}
}

func TestThrottle_RefusesPID1(t *testing.T) {
	throttler, _ := newMockThrottler()

	err := throttler.Throttle(1, "launchd", 100.0, ThrottleMedium)
	if err == nil {
		t.Error("Should refuse to throttle PID 1")
	}
	if !strings.Contains(err.Error(), "system process") {
		t.Errorf("Error = %q, should mention 'system process'", err.Error())
	}
}

func TestThrottle_RefusesSelf(t *testing.T) {
	throttler, _ := newMockThrottler()

	err := throttler.Throttle(os.Getpid(), "test", 100.0, ThrottleMedium)
	if err == nil {
		t.Error("Should refuse to throttle self")
	}
	if !strings.Contains(err.Error(), "self") {
		t.Errorf("Error = %q, should mention 'self'", err.Error())
	}
}

func TestThrottle_RefusesProtected(t *testing.T) {
	throttler, _ := newMockThrottler()

	protectedNames := []string{"WindowServer", "kernel_task", "loginwindow", "Dock", "Finder", "mds_stores"}
	for _, name := range protectedNames {
		err := throttler.Throttle(999, name, 100.0, ThrottleMedium)
		if err == nil {
			t.Errorf("Should refuse to throttle protected process: %s", name)
		}
	}
}

func TestThrottle_ReniceError(t *testing.T) {
	throttler, mock := newMockThrottler()
	mock.errors["renice +10 -p 999"] = fmt.Errorf("operation not permitted")

	err := throttler.Throttle(999, "test_proc", 50.0, ThrottleMedium)
	if err == nil {
		t.Error("Should return error when renice fails")
	}
	if !strings.Contains(err.Error(), "not permitted") {
		t.Errorf("Error = %q", err.Error())
	}
}

// ── Unthrottle Tests ────────────────────────────────────────────────────

func TestUnthrottle(t *testing.T) {
	throttler, mock := newMockThrottler()

	_ = throttler.Throttle(12345, "test_proc", 50.0, ThrottleMedium)
	err := throttler.Unthrottle(12345)
	if err != nil {
		t.Fatalf("Unthrottle failed: %v", err)
	}

	if throttler.IsThrottled(12345) {
		t.Error("Process should no longer be throttled")
	}
	// Should have called renice 0
	lastCall := mock.calls[len(mock.calls)-1]
	if lastCall != "renice 0 -p 12345" {
		t.Errorf("Expected 'renice 0 -p 12345', got %q", lastCall)
	}
}

func TestUnthrottleAll(t *testing.T) {
	throttler, _ := newMockThrottler()

	_ = throttler.Throttle(100, "proc_a", 20.0, ThrottleMedium)
	_ = throttler.Throttle(200, "proc_b", 30.0, ThrottleMedium)
	_ = throttler.Throttle(300, "proc_c", 40.0, ThrottleMedium)

	if throttler.ThrottledCount() != 3 {
		t.Fatalf("Expected 3 throttled, got %d", throttler.ThrottledCount())
	}

	result := throttler.UnthrottleAll()
	if result.Throttled != 3 {
		t.Errorf("UnthrottleAll should restore 3, got %d", result.Throttled)
	}
	if throttler.ThrottledCount() != 0 {
		t.Errorf("ThrottledCount should be 0 after UnthrottleAll, got %d", throttler.ThrottledCount())
	}
}

// ── AutoThrottleFromAlert Tests ─────────────────────────────────────────

func TestAutoThrottle_IgnoresLowCPU(t *testing.T) {
	throttler, mock := newMockThrottler()

	alert := WatchAlert{
		Process:    ProcessInfo{PID: 12345, Name: "test_proc"},
		CPUPercent: 10.0, // Under 15% threshold
	}

	err := throttler.AutoThrottleFromAlert(alert)
	if err != nil {
		t.Fatal(err)
	}
	if len(mock.calls) != 0 {
		t.Error("Should not renice processes under 15% CPU")
	}
}

func TestAutoThrottle_ThrottlesHighCPU(t *testing.T) {
	throttler, mock := newMockThrottler()

	alert := WatchAlert{
		Process:    ProcessInfo{PID: 12345, Name: "language_server"},
		CPUPercent: 25.0,
	}

	err := throttler.AutoThrottleFromAlert(alert)
	if err != nil {
		t.Fatal(err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("Expected 1 renice call, got %d", len(mock.calls))
	}
	if !strings.Contains(mock.calls[0], "+10") {
		t.Errorf("Should apply ThrottleMedium (+10), got %q", mock.calls[0])
	}
}

func TestAutoThrottle_EscalatesToHard(t *testing.T) {
	throttler, mock := newMockThrottler()

	alert := WatchAlert{
		Process:    ProcessInfo{PID: 12345, Name: "runaway_proc"},
		CPUPercent: 50.0,
	}

	// First throttle → medium
	_ = throttler.AutoThrottleFromAlert(alert)
	if !strings.Contains(mock.calls[0], "+10") {
		t.Errorf("First throttle should be +10, got %q", mock.calls[0])
	}

	// Second throttle → escalate to hard
	_ = throttler.AutoThrottleFromAlert(alert)
	lastCall := mock.calls[len(mock.calls)-1]
	if !strings.Contains(lastCall, "+15") {
		t.Errorf("Second throttle should escalate to +15, got %q", lastCall)
	}
}

func TestAutoThrottle_CapsAtHard(t *testing.T) {
	throttler, mock := newMockThrottler()

	alert := WatchAlert{
		Process:    ProcessInfo{PID: 12345, Name: "maxed_proc"},
		CPUPercent: 90.0,
	}

	// Throttle twice to reach hard
	_ = throttler.AutoThrottleFromAlert(alert)
	_ = throttler.AutoThrottleFromAlert(alert)
	callsBefore := len(mock.calls)

	// Third time should be a no-op
	_ = throttler.AutoThrottleFromAlert(alert)
	if len(mock.calls) != callsBefore {
		t.Error("Should not escalate past ThrottleHard")
	}
}

// ── Prune Tests ────────────────────────────────────────────────────────

func TestPrune_RemovesDeadPIDs(t *testing.T) {
	throttler, mock := newMockThrottler()

	// Throttle three processes
	_ = throttler.Throttle(100, "alive_proc", 20.0, ThrottleMedium)
	_ = throttler.Throttle(200, "dead_proc", 30.0, ThrottleMedium)
	_ = throttler.Throttle(300, "also_dead", 40.0, ThrottleMedium)

	// Make kill -0 fail for PIDs 200 and 300 (they're dead)
	mock.errors["kill -0 200"] = fmt.Errorf("no such process")
	mock.errors["kill -0 300"] = fmt.Errorf("no such process")

	pruned := throttler.Prune()

	if pruned != 2 {
		t.Errorf("Prune() = %d, want 2", pruned)
	}
	if throttler.ThrottledCount() != 1 {
		t.Errorf("ThrottledCount = %d, want 1", throttler.ThrottledCount())
	}
	if !throttler.IsThrottled(100) {
		t.Error("PID 100 should still be throttled (alive)")
	}
	if throttler.IsThrottled(200) {
		t.Error("PID 200 should be pruned (dead)")
	}
	if throttler.IsThrottled(300) {
		t.Error("PID 300 should be pruned (dead)")
	}
}

func TestPrune_NothingToPrune(t *testing.T) {
	throttler, _ := newMockThrottler()

	_ = throttler.Throttle(100, "alive_proc", 20.0, ThrottleMedium)

	// kill -0 succeeds by default (mock returns nil), so PID is alive
	pruned := throttler.Prune()

	if pruned != 0 {
		t.Errorf("Prune() = %d, want 0", pruned)
	}
	if throttler.ThrottledCount() != 1 {
		t.Errorf("ThrottledCount = %d, want 1", throttler.ThrottledCount())
	}
}

func TestPrune_EmptyMap(t *testing.T) {
	throttler, _ := newMockThrottler()

	pruned := throttler.Prune()
	if pruned != 0 {
		t.Errorf("Prune() on empty map = %d, want 0", pruned)
	}
}

// ── ThrottledPIDs Tests ─────────────────────────────────────────────────

func TestThrottledPIDs(t *testing.T) {
	throttler, _ := newMockThrottler()

	_ = throttler.Throttle(100, "a", 20.0, ThrottleMedium)
	_ = throttler.Throttle(200, "b", 30.0, ThrottleMedium)

	pids := throttler.ThrottledPIDs()
	if len(pids) != 2 {
		t.Errorf("Expected 2 PIDs, got %d", len(pids))
	}
}

// ── FormatThrottleReport Tests ──────────────────────────────────────────

func TestFormatThrottleReport_Empty(t *testing.T) {
	throttler, _ := newMockThrottler()

	report := throttler.FormatThrottleReport()
	if !strings.Contains(report, "No processes deprioritized") {
		t.Errorf("Empty report = %q", report)
	}
}

func TestFormatThrottleReport_WithThrottled(t *testing.T) {
	throttler, _ := newMockThrottler()

	_ = throttler.Throttle(12345, "language_server", 25.0, ThrottleMedium)

	report := throttler.FormatThrottleReport()
	if !strings.Contains(report, "12345") {
		t.Error("Report should contain PID")
	}
	if !strings.Contains(report, "language_server") {
		t.Error("Report should contain process name")
	}
	if !strings.Contains(report, "Isis") {
		t.Error("Report should contain Isis branding")
	}
}

// ── isProtectedName Tests ───────────────────────────────────────────────

func TestIsProtectedName(t *testing.T) {
	protected := []string{"kernel_task", "WindowServer", "mds_stores", "Dock", "Finder"}
	for _, name := range protected {
		if !isProtectedName(name) {
			t.Errorf("%s should be protected", name)
		}
	}

	notProtected := []string{"language_server", "node", "electron", "chrome", "python"}
	for _, name := range notProtected {
		if isProtectedName(name) {
			t.Errorf("%s should NOT be protected", name)
		}
	}
}

// ── ThrottleLevel Constants ─────────────────────────────────────────────

func TestThrottleLevels(t *testing.T) {
	if ThrottleMild != 5 {
		t.Errorf("ThrottleMild = %d, want 5", ThrottleMild)
	}
	if ThrottleMedium != 10 {
		t.Errorf("ThrottleMedium = %d, want 10", ThrottleMedium)
	}
	if ThrottleHard != 15 {
		t.Errorf("ThrottleHard = %d, want 15", ThrottleHard)
	}
}
