package router

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Progress-gated dispatch (#636 C1) + hard spawn-rate ceiling (#636 C3).
//
// THE BUG THIS CLOSES. RunWakeLoop's gate was `depth > 0 && !run.running()`. Its
// comment anticipated fork-storms and deliberately gated on consumer liveness
// rather than a timer — correct for two consumer outcomes: works-long-and-drains,
// or dies-visibly. It does not cover the third: EXITS QUICKLY HAVING MADE NO
// PROGRESS. Then `depth > 0` stays true forever (nothing drained) and
// `!run.running()` becomes true every tick (the consumer exited), so the
// edge-trigger degenerates into a level-trigger — precisely what the comment set
// out to prevent.
//
// Measured 2026-08-07 on claude-finalwishes: 1,221 dispatches in a day, 1,197
// sessions, 99% of them achieving nothing (median 17 transcript lines), ~59.2M
// tokens against a metered account. There was no progress requirement anywhere:
// a consumer that drained five items and one that drained zero were
// indistinguishable to the gate.

const (
	// wakeLoopFruitlessQuarantine is how many consecutive no-progress dispatches a
	// lane gets before the loop stops dispatching entirely. A lane that cannot make
	// progress is a bug to REPORT, not a spawn to retry.
	wakeLoopFruitlessQuarantine = 10

	// wakeLoopMaxSpawnsPerHour is a hard ceiling enforced independently of the
	// progress gate. It is the backstop that would have capped the 2026-08-07
	// incident at ~288 sessions/day instead of 1,197 EVEN IF the progress gate were
	// broken. A ceiling that depends on other logic being correct is not a ceiling.
	wakeLoopMaxSpawnsPerHour = 12

	// wakeLoopBackoffCap bounds the backoff so a stuck lane costs O(log n) spawns
	// per day rather than O(ticks), without ever going fully silent before the
	// quarantine threshold decides.
	wakeLoopBackoffCap = time.Hour
)

// wakeLoopBackoff returns the delay before a lane may dispatch again after
// `fruitless` consecutive no-progress exits: 1x, 2x, 4x, 8x… interval, capped.
//
// Backoff rather than immediate block is deliberate. Inbox depth is the cheapest
// progress proxy and it is already read every tick, but it is not perfect — an
// agent may legitimately work an item without closing it inside one dispatch. So
// a slow-but-real lane pays one doubled interval, while a no-op lane decays to
// one spawn/hour and is then quarantined.
func wakeLoopBackoff(fruitless int, interval time.Duration) time.Duration {
	if fruitless <= 0 {
		return 0
	}
	d := interval
	for i := 1; i < fruitless; i++ {
		d *= 2
		if d >= wakeLoopBackoffCap {
			return wakeLoopBackoffCap
		}
	}
	if d > wakeLoopBackoffCap {
		return wakeLoopBackoffCap
	}
	return d
}

// dispatchLedgerPath is the per-agent spawn ledger. A plain append-only file of
// unix timestamps, deliberately NOT the router store: the ceiling must hold
// across a wake-loop restart (the LaunchAgents are KeepAlive=true and 151
// restarts were logged during the incident), and it must not require a schema
// migration to land. In-process counters reset on restart — that is the hole
// this closes.
func dispatchLedgerPath(agentID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sirsi", "dispatch", agentID+".log")
}

// recordDispatch appends one dispatch timestamp. Best-effort: a ledger write
// failure must never block the loop, but it does forfeit rate protection, so it
// is the caller's job to log it.
func recordDispatch(agentID string, now time.Time) error {
	p := dispatchLedgerPath(agentID)
	if p == "" {
		return fmt.Errorf("no home dir")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%d\n", now.Unix())
	return err
}

// recentDispatchCount counts ledger entries inside the window and rewrites the
// file with only those, so it cannot grow without bound.
func recentDispatchCount(agentID string, now time.Time, window time.Duration) int {
	p := dispatchLedgerPath(agentID)
	if p == "" {
		return 0
	}
	f, err := os.Open(p)
	if err != nil {
		return 0
	}
	cutoff := now.Add(-window).Unix()
	var kept []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		ts, err := strconv.ParseInt(sc.Text(), 10, 64)
		if err != nil {
			continue
		}
		if ts >= cutoff {
			kept = append(kept, sc.Text())
		}
	}
	f.Close()

	// Compact only when it would actually shrink the file — avoids rewriting on
	// every tick of a quiet lane.
	if len(kept) > 0 {
		if fi, err := os.Stat(p); err == nil && fi.Size() > int64(len(kept)*12) {
			var buf []byte
			for _, k := range kept {
				buf = append(buf, k...)
				buf = append(buf, '\n')
			}
			_ = os.WriteFile(p, buf, 0o644)
		}
	} else {
		_ = os.Remove(p)
	}
	return len(kept)
}

// spawnCeilingReached reports whether this agent has hit its hourly ceiling.
func spawnCeilingReached(agentID string, now time.Time) (bool, int) {
	n := recentDispatchCount(agentID, now, time.Hour)
	return n >= wakeLoopMaxSpawnsPerHour, n
}

// madeProgress reports whether a completed consumer reduced the inbox.
//
// depthAtDispatch < 0 means "unknown" (the consumer predates this accounting or
// the inbox read failed) and is treated as progress: an unreadable inbox must
// never be scored as a fruitless run, or a transient store error would quarantine
// a healthy lane.
func madeProgress(depthAtDispatch, depthNow int) bool {
	if depthAtDispatch < 0 || depthNow < 0 {
		return true
	}
	return depthNow < depthAtDispatch
}
