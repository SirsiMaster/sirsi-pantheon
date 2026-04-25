// Package guard — throttle.go
//
// Isis Throttle: Non-destructive CPU pressure relief.
//
// Instead of killing processes, Isis can "renice" them — lower their
// scheduling priority so they yield CPU to interactive apps (like your IDE).
//
// This is the gentle alternative to slaying:
//
//	Kill:     Process dies. Data may be lost. User is disrupted.
//	Renice:   Process runs slower. No data loss. User barely notices.
//	Suspend:  Process paused entirely (SIGSTOP). Reversible (SIGCONT).
//
// Safety:
//   - Only renices processes owned by the current user
//   - Never touches root/system processes
//   - Reniced processes can be restored with UnthrottleAll
//   - Throttle events are logged for transparency
//
// Architecture:
//
//	Watchdog Alert ──▶ Throttle(pid) ──▶ renice +10 pid
//	                       │
//	                  throttledPIDs map ──▶ Unthrottle(pid) ──▶ renice 0 pid
package guard

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ThrottleLevel defines how aggressively to deprioritize a process.
type ThrottleLevel int

const (
	ThrottleMild   ThrottleLevel = 5  // Slightly lower priority
	ThrottleMedium ThrottleLevel = 10 // Noticeably lower — default
	ThrottleHard   ThrottleLevel = 15 // Very low priority — last resort before kill
)

// ThrottleEvent records a renice operation.
type ThrottleEvent struct {
	PID         int           `json:"pid"`
	ProcessName string        `json:"process_name"`
	CPUBefore   float64       `json:"cpu_before"`
	Level       ThrottleLevel `json:"level"`
	Timestamp   time.Time     `json:"timestamp"`
	Action      string        `json:"action"` // "throttled" or "unthrottled"
}

// ThrottleResult summarizes a throttle operation.
type ThrottleResult struct {
	Throttled int             `json:"throttled"`
	Failed    int             `json:"failed"`
	Events    []ThrottleEvent `json:"events"`
	Errors    []error         `json:"errors,omitempty"`
}

// Throttler manages process priority adjustments.
type Throttler struct {
	mu        sync.RWMutex
	throttled map[int]ThrottleEvent // PID → original throttle event
	cmdRunner func(name string, args ...string) (string, error)
}

// NewThrottler creates a Throttler instance.
func NewThrottler() *Throttler {
	return &Throttler{
		throttled: make(map[int]ThrottleEvent),
		cmdRunner: defaultCmdRunner,
	}
}

func defaultCmdRunner(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Throttle reduces the priority of a process using renice.
// Returns nil if the process was successfully reniced.
func (t *Throttler) Throttle(pid int, processName string, cpuBefore float64, level ThrottleLevel) error {
	// Safety checks
	if pid <= 1 {
		return fmt.Errorf("refusing to throttle PID %d (system process)", pid)
	}
	if pid == os.Getpid() {
		return fmt.Errorf("refusing to throttle self (PID %d)", pid)
	}
	if isProtectedName(processName) {
		return fmt.Errorf("refusing to throttle protected process: %s", processName)
	}

	// Perform renice
	niceValue := strconv.Itoa(int(level))
	pidStr := strconv.Itoa(pid)
	_, err := t.cmdRunner("renice", "+"+niceValue, "-p", pidStr)
	if err != nil {
		return fmt.Errorf("renice failed for PID %d (%s): %w", pid, processName, err)
	}

	// Record event
	event := ThrottleEvent{
		PID:         pid,
		ProcessName: processName,
		CPUBefore:   cpuBefore,
		Level:       level,
		Timestamp:   time.Now(),
		Action:      "throttled",
	}

	t.mu.Lock()
	t.throttled[pid] = event
	t.mu.Unlock()

	return nil
}

// Unthrottle restores a process to normal priority.
func (t *Throttler) Unthrottle(pid int) error {
	pidStr := strconv.Itoa(pid)
	_, err := t.cmdRunner("renice", "0", "-p", pidStr)
	if err != nil {
		return fmt.Errorf("unthrottle failed for PID %d: %w", pid, err)
	}

	t.mu.Lock()
	delete(t.throttled, pid)
	t.mu.Unlock()

	return nil
}

// UnthrottleAll restores all throttled processes to normal priority.
func (t *Throttler) UnthrottleAll() *ThrottleResult {
	t.mu.RLock()
	pids := make([]int, 0, len(t.throttled))
	for pid := range t.throttled {
		pids = append(pids, pid)
	}
	t.mu.RUnlock()

	result := &ThrottleResult{}
	for _, pid := range pids {
		if err := t.Unthrottle(pid); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, err)
		} else {
			result.Throttled++
			result.Events = append(result.Events, ThrottleEvent{
				PID:       pid,
				Timestamp: time.Now(),
				Action:    "unthrottled",
			})
		}
	}

	return result
}

// ThrottledCount returns the number of currently throttled processes.
func (t *Throttler) ThrottledCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.throttled)
}

// ThrottledPIDs returns the list of currently throttled PIDs.
func (t *Throttler) ThrottledPIDs() []int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	pids := make([]int, 0, len(t.throttled))
	for pid := range t.throttled {
		pids = append(pids, pid)
	}
	return pids
}

// Prune removes entries from the throttled map whose PIDs are no longer alive.
// Returns the number of pruned (dead) entries.
func (t *Throttler) Prune() int {
	t.mu.RLock()
	pids := make([]int, 0, len(t.throttled))
	for pid := range t.throttled {
		pids = append(pids, pid)
	}
	t.mu.RUnlock()

	var dead []int
	for _, pid := range pids {
		pidStr := strconv.Itoa(pid)
		if _, err := t.cmdRunner("kill", "-0", pidStr); err != nil {
			dead = append(dead, pid)
		}
	}

	if len(dead) > 0 {
		t.mu.Lock()
		for _, pid := range dead {
			delete(t.throttled, pid)
		}
		t.mu.Unlock()
	}

	return len(dead)
}

// IsThrottled returns true if the given PID is currently throttled.
func (t *Throttler) IsThrottled(pid int) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, ok := t.throttled[pid]
	return ok
}

// AutoThrottleFromAlert processes a WatchAlert and applies throttle if appropriate.
// This is designed to be called from the Watchdog alert consumer loop.
// Rules:
//   - Only throttle if CPU > 15% sustained
//   - Only throttle non-foreground, non-system processes
//   - Apply ThrottleMedium (+10) by default
//   - Escalate to ThrottleHard (+15) if already throttled and still hot
func (t *Throttler) AutoThrottleFromAlert(alert WatchAlert) error {
	// Don't throttle low CPU — only act on serious hogs
	if alert.CPUPercent < 15.0 {
		return nil
	}

	// Don't throttle if already throttled at hard level
	t.mu.RLock()
	existing, alreadyThrottled := t.throttled[alert.Process.PID]
	t.mu.RUnlock()

	level := ThrottleMedium
	if alreadyThrottled {
		if existing.Level >= ThrottleHard {
			return nil // Already at max throttle
		}
		level = ThrottleHard // Escalate
	}

	return t.Throttle(alert.Process.PID, alert.Process.Name, alert.CPUPercent, level)
}

// FormatThrottleReport returns a human-readable summary of throttled processes.
func (t *Throttler) FormatThrottleReport() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.throttled) == 0 {
		return "𓁵 Isis: No processes deprioritized"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("𓁵 Isis: %d process(es) deprioritized (safe, reversible)\n", len(t.throttled)))
	for _, event := range t.throttled {
		sb.WriteString(fmt.Sprintf("  PID %d (%s): priority lowered +%d (was %.0f%% CPU at %s)\n",
			event.PID, event.ProcessName, event.Level,
			event.CPUBefore, event.Timestamp.Format("15:04:05")))
	}
	return sb.String()
}

// isProtectedName returns true for processes that should never be throttled.
func isProtectedName(name string) bool {
	nameLower := strings.ToLower(name)
	protected := []string{
		"kernel_task", "launchd", "windowserver", "loginwindow",
		"coreaudiod", "dock", "finder", "systemuiserver",
		"cfprefsd", "distnoted", "syslogd", "notifyd",
		"securityd", "trustd", "tccd", "locationd",
		"spotlight", "mds", "mds_stores",
	}
	for _, p := range protected {
		if nameLower == p || strings.Contains(nameLower, p) {
			return true
		}
	}
	return false
}
