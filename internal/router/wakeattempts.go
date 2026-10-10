package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// wakeAttemptsFile is the local, file-based counterpart to routerstore's
// WakeEvent table (internal/routerstore/wakeevents.go, MaxWakeAttempts=3):
// the file-based router has no service-side store to track delivery misses
// in, so unroutableAgents' static WakeNone check had no signal for a lane
// that DECLARES a wake mechanism but whose wake keeps failing in practice
// (lane-ping-and-incoming-notice remaining (4), option b — the cheaper,
// no-new-dependency slice; ClassifyLane/wakeevents.go remain the service-side
// answer for a routerstore-backed deployment).
const wakeAttemptsFile = "wake-attempts.json"

var wakeAttemptsMu sync.Mutex

func wakeAttemptsPath(routerRoot string) string {
	return filepath.Join(routerRoot, wakeAttemptsFile)
}

func loadWakeAttemptsLocked(routerRoot string) (map[string]int, error) {
	data, err := os.ReadFile(wakeAttemptsPath(routerRoot))
	if os.IsNotExist(err) {
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	if len(data) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func saveWakeAttemptsLocked(routerRoot string, m map[string]int) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(wakeAttemptsPath(routerRoot), data, 0o644)
}

// recordWakeMiss increments agentID's consecutive-miss count. Best-effort: a
// tracking write must never fail the wake pass it is observing, so read/write
// errors are swallowed here exactly as the pass already swallows its other
// bookkeeping writes (setWake).
func recordWakeMiss(routerRoot, agentID string) {
	wakeAttemptsMu.Lock()
	defer wakeAttemptsMu.Unlock()
	m, err := loadWakeAttemptsLocked(routerRoot)
	if err != nil {
		return
	}
	m[agentID]++
	_ = saveWakeAttemptsLocked(routerRoot, m)
}

// recordWakeSuccess clears agentID's miss count. A successful invocation ends
// the failure streak — this counts CONSECUTIVE misses, mirroring
// wakeevents.go's attempt semantics, not a lifetime total.
func recordWakeSuccess(routerRoot, agentID string) {
	wakeAttemptsMu.Lock()
	defer wakeAttemptsMu.Unlock()
	m, err := loadWakeAttemptsLocked(routerRoot)
	if err != nil || m[agentID] == 0 {
		return
	}
	delete(m, agentID)
	_ = saveWakeAttemptsLocked(routerRoot, m)
}

// TerminalWakeFailures reports agentID's consecutive wake-miss count from the
// local file-based tracker (routerRoot/wake-attempts.json). A read error or
// missing entry reports 0 — unknown is treated as zero, never as failing, so a
// tracker that has never been written (every host before this change) changes
// no existing behavior.
func TerminalWakeFailures(routerRoot, agentID string) int {
	wakeAttemptsMu.Lock()
	defer wakeAttemptsMu.Unlock()
	m, err := loadWakeAttemptsLocked(routerRoot)
	if err != nil {
		return 0
	}
	return m[agentID]
}
