package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// NEGATIVE CONTROL for the whole fix: make madeProgress always report true (i.e.
// remove the progress requirement) and TestFruitlessRunsAccumulate MUST fail.
// A gate that passes without the guard covers nothing.

func TestMadeProgressRequiresTheInboxToShrink(t *testing.T) {
	cases := []struct {
		name            string
		atDispatch, now int
		want            bool
	}{
		{"drained two", 19, 17, true},
		{"drained to empty", 3, 0, true},
		{"unchanged — THE BUG", 19, 19, false},
		{"grew", 19, 21, false},
		// Unknown depth must score as progress: an unreadable inbox is a transient
		// store error, and treating it as fruitless would quarantine a healthy lane.
		{"unknown at dispatch", -1, 19, true},
		{"unknown now", 19, -1, true},
	}
	for _, c := range cases {
		if got := madeProgress(c.atDispatch, c.now); got != c.want {
			t.Errorf("%s: madeProgress(%d,%d) = %v, want %v", c.name, c.atDispatch, c.now, got, c.want)
		}
	}
}

// The 2026-08-07 shape: depth pinned at 19 while consumers exit instantly. Ten
// such exits must quarantine the lane rather than spawn forever.
func TestFruitlessRunsAccumulate(t *testing.T) {
	depth := 19
	fruitless := 0
	for i := 0; i < wakeLoopFruitlessQuarantine; i++ {
		if madeProgress(depth, depth) {
			fruitless = 0
		} else {
			fruitless++
		}
	}
	if fruitless < wakeLoopFruitlessQuarantine {
		t.Fatalf("fruitless = %d after %d no-progress exits, want >= %d — the lane would "+
			"keep spawning forever (this is the #636 incident)", fruitless, wakeLoopFruitlessQuarantine,
			wakeLoopFruitlessQuarantine)
	}
	// One real drain resets it — a slow-but-healthy lane must not be punished.
	if !madeProgress(19, 17) {
		t.Error("a consumer that drained 19->17 was scored as no progress")
	}
}

func TestBackoffIsMonotonicAndBounded(t *testing.T) {
	iv := 3 * time.Minute
	if d := wakeLoopBackoff(0, iv); d != 0 {
		t.Errorf("backoff(0) = %s, want 0 — a healthy lane must not wait", d)
	}
	prev := time.Duration(0)
	for i := 1; i <= 40; i++ {
		d := wakeLoopBackoff(i, iv)
		if d < prev {
			t.Fatalf("backoff(%d) = %s < backoff(%d) = %s — not monotonic", i, d, i-1, prev)
		}
		if d > wakeLoopBackoffCap {
			t.Fatalf("backoff(%d) = %s exceeds cap %s", i, d, wakeLoopBackoffCap)
		}
		prev = d
	}
	if wakeLoopBackoff(40, iv) != wakeLoopBackoffCap {
		t.Errorf("backoff never reaches the cap; a stuck lane would keep spawning")
	}
}

// The ceiling must hold across a wake-loop RESTART. The LaunchAgents are
// KeepAlive=true and 151 restarts were logged during the incident, so an
// in-process counter is exactly the hole this closes — hence a file ledger.
func TestSpawnCeilingSurvivesProcessRestart(t *testing.T) {
	// Uses the SetDispatchDir seam rather than a HOME override. The HOME trick
	// worked only while the ledger path was derived from the home dir on every
	// call; TestMain now pins the dir package-wide so no test can write the
	// operator's real ~/.sirsi/dispatch, and that pin correctly wins over HOME.
	// Setting the seam is also what this test actually means — it asserts the
	// ledger is a FILE that outlives the process, not where home resolves to.
	dir := t.TempDir()
	SetDispatchDir(dir)
	t.Cleanup(func() { SetDispatchDir("") })
	agent := "test-lane"
	now := time.Now()

	for i := 0; i < wakeLoopMaxSpawnsPerHour; i++ {
		if err := recordDispatch(agent, now.Add(-time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	// Simulating a restart = simply reading again; nothing is held in memory.
	over, n := spawnCeilingReached(agent, now)
	if !over {
		t.Fatalf("ceiling not reached with %d dispatches in the hour (max %d)", n, wakeLoopMaxSpawnsPerHour)
	}
	if _, err := os.Stat(filepath.Join(dir, agent+".log")); err != nil {
		t.Errorf("ledger not persisted: %v", err)
	}
}

func TestSpawnCeilingIgnoresEntriesOlderThanAnHour(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	agent := "old-lane"
	now := time.Now()
	for i := 0; i < 50; i++ {
		_ = recordDispatch(agent, now.Add(-2*time.Hour))
	}
	if over, n := spawnCeilingReached(agent, now); over {
		t.Errorf("stale entries counted: over=%v n=%d — the window is not rolling", over, n)
	}
}

func TestSpawnCeilingAllowsAHealthyLane(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	agent := "healthy"
	now := time.Now()
	for i := 0; i < wakeLoopMaxSpawnsPerHour-1; i++ {
		_ = recordDispatch(agent, now)
	}
	if over, _ := spawnCeilingReached(agent, now); over {
		t.Error("ceiling tripped below the limit — a healthy lane would be starved")
	}
}

// Source guard, matching this package's existing idiom (see
// TestWakeLoopReadsThroughCutoverEntryPoint). The gate's correctness lives in a
// long loop body that a unit test cannot reach, and the ORIGINAL bug was exactly
// a missing clause in that condition — so assert the clauses are present.
func TestWakeLoopDispatchGateChecksProgressAndCeiling(t *testing.T) {
	src, err := os.ReadFile("wake.go")
	if err != nil {
		t.Fatalf("read wake.go: %v", err)
	}
	body := string(src)
	start := strings.Index(body, "func RunWakeLoop(")
	if start < 0 {
		t.Fatal("RunWakeLoop not found")
	}
	fn := body[start:]

	for _, want := range []struct{ frag, why string }{
		{"madeProgress(", "the gate no longer scores consumers for PROGRESS — a consumer that exits " +
			"instantly having drained nothing is again indistinguishable from one that worked (#636)"},
		{"spawnCeilingReached(", "the hard hourly spawn ceiling is gone — the backstop that bounds a " +
			"runaway even when the progress gate is wrong"},
		{"nextDispatchAllowed", "the backoff is not consulted before dispatching"},
		{"wakeLoopFruitlessQuarantine", "a lane that never makes progress is no longer quarantined"},
		{"recordDispatch(", "dispatches are not recorded, so the rate ceiling can never trip"},
	} {
		if !strings.Contains(fn, want.frag) {
			t.Errorf("RunWakeLoop is missing %q: %s", want.frag, want.why)
		}
	}
}
