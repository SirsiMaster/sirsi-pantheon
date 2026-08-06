package router

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/liveness"
)

func writeGemmaRouteRegistry(t *testing.T, root string) {
	t.Helper()
	body := `{"agents":{"horus":{"id":"horus","type":"service","repo":"/tmp","workstream":"pantheon","wake":{"mechanism":"none"}},"owner":{"id":"owner","type":"human","repo":"/tmp","workstream":"portfolio","wake":{"mechanism":"owner-surface"}}}}`
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// zeroRestoreWait stubs the post-restore poll sleep to nothing and sets the
// deadline to zero so the first failed post-restore probe routes immediately,
// keeping tests fast without changing the observable routing outcome.
func zeroRestoreWait(t *testing.T) {
	t.Helper()
	oldFn := getRestoreWait()
	oldDur := getRestoreDeadline()
	setRestoreWaitFn(func() {})
	setRestoreDeadlineDur(0)
	t.Cleanup(func() {
		setRestoreWaitFn(oldFn)
		setRestoreDeadlineDur(oldDur)
	})
}

// installGemmaFakes swaps the probe + serve seams and returns a pointer to the
// recorded serve calls ("start" or "restart"). The probe always returns status
// for every invocation (initial + post-restore); use routerRoot="" so the route
// path is a no-op for tests that don't need an on-disk items dir.
func installGemmaFakes(t *testing.T, status liveness.GemmaStatus) *[]string {
	t.Helper()
	zeroRestoreWait(t)
	oldProbe := getGemmaProbeFn()
	oldServe := getGemmaServeFn()
	var calls []string
	setGemmaProbeFn(func(string) (liveness.GemmaStatus, string) { return status, "test" })
	setGemmaServeFn(func(restart bool) error {
		if restart {
			calls = append(calls, "restart")
		} else {
			calls = append(calls, "start")
		}
		return nil
	})
	gemmaWedgeStrikes = 0
	t.Cleanup(func() {
		setGemmaProbeFn(oldProbe)
		setGemmaServeFn(oldServe)
		gemmaWedgeStrikes = 0
	})
	return &calls
}

// TestGemmaLiveness_HealthyDoesNothing: a healthy broker is never touched.
func TestGemmaLiveness_HealthyDoesNothing(t *testing.T) {
	calls := installGemmaFakes(t, liveness.GemmaHealthy)
	if err := RunGemmaLivenessDuty("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Errorf("healthy broker triggered serve calls %v, want none", *calls)
	}
}

// TestGemmaLiveness_DownStartsImmediately: a down broker is started at once (no
// process to protect — the common post-crash gap).
func TestGemmaLiveness_DownStartsImmediately(t *testing.T) {
	calls := installGemmaFakes(t, liveness.GemmaDown)
	if err := RunGemmaLivenessDuty("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0] != "start" {
		t.Errorf("down broker → calls %v, want [start]", *calls)
	}
}

// TestGemmaLiveness_WedgedRestartsOnlyAfterConfirmation is the anti-thrash
// guard: a wedged broker is NOT restarted on the first observation (could be a
// transient), only after the strike threshold — and the restart is graceful.
func TestGemmaLiveness_WedgedRestartsOnlyAfterConfirmation(t *testing.T) {
	calls := installGemmaFakes(t, liveness.GemmaWedged)

	// First tick: one strike, no action.
	if err := RunGemmaLivenessDuty("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Fatalf("first wedged tick restarted the broker (%v) — must wait for confirmation", *calls)
	}
	// Second tick: threshold reached → graceful restart.
	// routerRoot="" so the post-restore route path is a no-op.
	if err := RunGemmaLivenessDuty("", ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0] != "restart" {
		t.Errorf("confirmed wedge → calls %v, want [restart] (graceful stop+start, never SIGKILL)", *calls)
	}
}

// TestGemmaLiveness_TransientWedgeResets: a wedge that recovers before the
// threshold never triggers a restart (the strike counter resets on healthy).
func TestGemmaLiveness_TransientWedgeResets(t *testing.T) {
	zeroRestoreWait(t)
	oldProbe := getGemmaProbeFn()
	oldServe := getGemmaServeFn()
	var calls []string
	state := liveness.GemmaWedged
	setGemmaProbeFn(func(string) (liveness.GemmaStatus, string) { return state, "test" })
	setGemmaServeFn(func(restart bool) error { calls = append(calls, "call"); return nil })
	gemmaWedgeStrikes = 0
	t.Cleanup(func() { setGemmaProbeFn(oldProbe); setGemmaServeFn(oldServe); gemmaWedgeStrikes = 0 })

	_ = RunGemmaLivenessDuty("", "") // strike 1
	state = liveness.GemmaHealthy
	_ = RunGemmaLivenessDuty("", "") // recovered → reset
	state = liveness.GemmaWedged
	_ = RunGemmaLivenessDuty("", "") // strike 1 again, not 2

	if len(calls) != 0 {
		t.Errorf("a transient wedge that recovered triggered %v, want no restart", calls)
	}
}

// TestGemmaLiveness_RestoreFailRoutesToUser: when serve itself errors (e.g. RAM
// won't fit), the duty routes ONE non-duplicate item to `user` via the dispatch
// facade (store + file, not file-only).
func TestGemmaLiveness_RestoreFailRoutesToUser(t *testing.T) {
	zeroRestoreWait(t)
	root := t.TempDir()
	writeGemmaRouteRegistry(t, root)
	// Isolate from the live store so test sends never reach the user's DB.
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "router.db"))

	oldProbe := getGemmaProbeFn()
	oldServe := getGemmaServeFn()
	t.Cleanup(func() {
		setGemmaProbeFn(oldProbe)
		setGemmaServeFn(oldServe)
		gemmaWedgeStrikes = 0
	})
	setGemmaProbeFn(func(string) (liveness.GemmaStatus, string) { return liveness.GemmaDown, "no port" })
	setGemmaServeFn(func(bool) error { return errors.New("RAM won't fit — 14 GB model > 8 GB free") })
	gemmaWedgeStrikes = 0

	err := RunGemmaLivenessDuty(root, "")
	if err == nil {
		t.Fatal("expected error from failed serve, got nil")
	}

	// Verify a restore-fail item landed in the store (dispatch facade reads store).
	f, openErr := dispatch.OpenRoot(root)
	if openErr != nil {
		t.Fatalf("open dispatch: %v", openErr)
	}
	defer func() { _ = f.Close() }()

	items, listErr := f.Inbox("owner")
	if listErr != nil {
		t.Fatalf("read user inbox: %v", listErr)
	}
	if len(items) == 0 {
		t.Error("no router item written after restore failure; expected one item in user inbox")
	}
	found := false
	for _, it := range items {
		if it.Title == restoreFailTitle {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("restore-fail item not found in user inbox; got %d items", len(items))
	}

	// Second call: item already exists — must not duplicate.
	_ = RunGemmaLivenessDuty(root, "")
	items2, _ := f.Inbox("owner")
	if len(items2) != len(items) {
		t.Errorf("second call wrote a duplicate item (got %d items, want %d)", len(items2), len(items))
	}
}

// TestGemmaLiveness_PostRestorePollingSucceeds is the regression test for the
// 2026-07-17T17:09Z false alarm: a broker that is still mmap'ing weights on the
// first post-restore probe (GemmaWedged) must NOT route a failure alert. The poll
// loop must iterate and only succeed once the broker reports GemmaHealthy. This
// is the behavior that justifies the poll loop's existence. With deadline=0 every
// other test collapses the loop to a single shot (time.Since(start)>=0 is always
// true) and cannot detect a regression where the loop is deleted or reverted to a
// flat sleep — this test catches exactly that.
func TestGemmaLiveness_PostRestorePollingSucceeds(t *testing.T) {
	// No-op wait but a positive deadline so the loop can iterate past the first
	// failed probe without sleeping. deadline=0 would route on iter 1.
	oldWait := getRestoreWait()
	oldDur := getRestoreDeadline()
	setRestoreWaitFn(func() {})
	setRestoreDeadlineDur(5 * time.Second)
	t.Cleanup(func() {
		setRestoreWaitFn(oldWait)
		setRestoreDeadlineDur(oldDur)
	})

	oldProbe := getGemmaProbeFn()
	oldServe := getGemmaServeFn()
	t.Cleanup(func() {
		setGemmaProbeFn(oldProbe)
		setGemmaServeFn(oldServe)
		gemmaWedgeStrikes = 0
	})

	// Probe sequence (indexed by call count):
	//   call 0 — tick 1 initial probe: Wedged → strike 1, no restart
	//   call 1 — tick 2 initial probe: Wedged → strike 2 → graceful restart
	//   call 2 — post-restore poll iter 1: Wedged → still loading, loop again
	//   call 3 — post-restore poll iter 2: Healthy → break → RecordHeal
	seq := []liveness.GemmaStatus{
		liveness.GemmaWedged,
		liveness.GemmaWedged,
		liveness.GemmaWedged,
		liveness.GemmaHealthy,
	}
	callCount := 0
	setGemmaProbeFn(func(string) (liveness.GemmaStatus, string) {
		s := seq[callCount]
		callCount++
		return s, "test"
	})

	var serveCalls []string
	setGemmaServeFn(func(restart bool) error {
		if restart {
			serveCalls = append(serveCalls, "restart")
		} else {
			serveCalls = append(serveCalls, "start")
		}
		return nil
	})
	gemmaWedgeStrikes = 0
	_ = drainHeals() // clear any stale heals from prior tests

	// Tick 1: strike 1, no action yet.
	if err := RunGemmaLivenessDuty("", ""); err != nil {
		t.Fatal(err)
	}
	if len(serveCalls) != 0 {
		t.Fatalf("tick 1 must not serve; got %v", serveCalls)
	}

	// Tick 2: strike 2 → graceful restart → poll loop exits on Healthy → RecordHeal.
	if err := RunGemmaLivenessDuty("", ""); err != nil {
		t.Fatalf("poll-then-recover path must not error: %v", err)
	}
	if len(serveCalls) != 1 || serveCalls[0] != "restart" {
		t.Errorf("serve calls %v, want [restart]", serveCalls)
	}
	// RecordHeal is the primary assertion: if the poll loop were deleted and
	// replaced with a flat sleep, the probe sequence would never reach Healthy
	// and RecordHeal would not fire, failing this test.
	if heals := drainHeals(); len(heals) == 0 {
		t.Error("RecordHeal not called — poll loop did not reach GemmaHealthy; " +
			"a reverted flat-sleep would pass all other tests but break this one")
	}
	if callCount != 4 {
		t.Errorf("probe called %d times, want 4 (2 initial + 2 poll iterations)", callCount)
	}
}

// TestGemmaLiveness_PostRestoreStillWedgedRoutesToUser: serve succeeds but the
// broker is still wedged after the restart — route to user via dispatch, no error.
func TestGemmaLiveness_PostRestoreStillWedgedRoutesToUser(t *testing.T) {
	zeroRestoreWait(t)
	root := t.TempDir()
	writeGemmaRouteRegistry(t, root)
	// Isolate from the live store so test sends never reach the user's DB.
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "router.db"))

	oldProbe := getGemmaProbeFn()
	oldServe := getGemmaServeFn()
	t.Cleanup(func() {
		setGemmaProbeFn(oldProbe)
		setGemmaServeFn(oldServe)
		gemmaWedgeStrikes = 0
	})

	var calls []string
	// Probe always returns wedged (so post-restore re-probe also returns wedged).
	setGemmaProbeFn(func(string) (liveness.GemmaStatus, string) { return liveness.GemmaWedged, "still wedged" })
	setGemmaServeFn(func(restart bool) error {
		if restart {
			calls = append(calls, "restart")
		} else {
			calls = append(calls, "start")
		}
		return nil
	})
	gemmaWedgeStrikes = 0

	// Strike 1 (no restart yet).
	_ = RunGemmaLivenessDuty(root, "")
	// Strike 2 → restart → post-probe still wedged → route to user.
	err := RunGemmaLivenessDuty(root, "")
	if err != nil {
		t.Fatalf("post-restore still-wedged path should not error, got %v", err)
	}
	if len(calls) != 1 || calls[0] != "restart" {
		t.Errorf("serve calls %v, want [restart]", calls)
	}

	// A restore-fail item must exist in the store (dispatch facade reads store).
	f, openErr := dispatch.OpenRoot(root)
	if openErr != nil {
		t.Fatalf("open dispatch: %v", openErr)
	}
	defer func() { _ = f.Close() }()

	items, _ := f.Inbox("owner")
	found := false
	for _, it := range items {
		if it.Title == restoreFailTitle {
			found = true
			break
		}
	}
	if !found {
		t.Error("post-restore wedge did not route an item to the owner via dispatch")
	}
}
