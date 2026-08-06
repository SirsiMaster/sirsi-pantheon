package router

// gemmaliveness.go — the self-healing gemma-liveness supervisor duty (A32).
//
// Owner directive (2026-07-17): "Gemma's liveness is absolutely critical — even
// more than [the IDE's]. The local LLM has to survive based on PANTHEON's
// survival, not the IDE." The gap: the `ai.sirsi.gemma` LaunchAgent is
// RunAtLoad + KeepAlive=false — a one-shot launcher that starts the broker at
// boot and exits, so if the broker later dies (crash, OOM, wedge) NOTHING
// restores it until the next reboot. Its survival was tied to boot events, not
// to Pantheon's always-on infrastructure.
//
// This wires gemma liveness into the ROUTER: a duty on the resident Horus
// supervisor (a KeepAlive LaunchAgent — always-on, IDE-independent) that each
// tick probes the broker and RESTORES it. Because it rides the supervisor,
// gemma survives on Pantheon's survival, not on any Claude/IDE session.
//
// Load-bearing safety (A32/ADR-040): restore is `sirsi gemma serve` — which is
// idempotent (no-ops when already warm) and RAM-gated (refuses rather than OOM).
// A confirmed-wedged broker is restored with a GRACEFUL `serve --stop` (SIGTERM
// to the process group, the broker consenting to stop) then `serve` — NEVER a
// SIGKILL, and only after the wedge is confirmed across consecutive ticks so a
// single transient hiccup never bounces a working broker.

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/liveness"
)

// gemmaWedgeThreshold is how many consecutive wedged observations must occur
// before a graceful restart — one transient timeout never bounces the broker.
const gemmaWedgeThreshold = 2

// gemmaWedgeStrikes counts consecutive wedged observations. Accessed only from
// the supervisor tick (duties run sequentially), reset on any non-wedged state.
var gemmaWedgeStrikes int

// Injected side-effect seams (Rule A16/A21): the probe and the (re)start action,
// behind mutex-guarded accessors so tests can swap them without a data race.
// gemmaLivenessFn is the duty-level seam (mirrors auto-heal): it defaults to a
// no-op so the supervisor-duty framework and library consumers never run a real
// probe/restore; cmd/sirsi injects the real RunGemmaLivenessDuty at init so it is
// ALWAYS active in production (not gated on autonomous — gemma must always live).
var (
	gemmaLifeMu     sync.RWMutex
	gemmaLivenessFn = func(_, _ string) error { return nil }
	gemmaProbeFn    = liveness.ProbeGemmaState
	gemmaServeFn    = defaultGemmaServe
)

func getGemmaLivenessFn() func(string, string) error {
	gemmaLifeMu.RLock()
	defer gemmaLifeMu.RUnlock()
	return gemmaLivenessFn
}

// SetGemmaLivenessFn installs the real gemma-liveness pass the supervisor's
// gemma-liveness duty runs (wired from cmd/sirsi at init).
func SetGemmaLivenessFn(fn func(routerRoot, repoRoot string) error) {
	gemmaLifeMu.Lock()
	defer gemmaLifeMu.Unlock()
	if fn != nil {
		gemmaLivenessFn = fn
	}
}

func getGemmaProbeFn() func(string) (liveness.GemmaStatus, string) {
	gemmaLifeMu.RLock()
	defer gemmaLifeMu.RUnlock()
	return gemmaProbeFn
}

func setGemmaProbeFn(fn func(string) (liveness.GemmaStatus, string)) {
	gemmaLifeMu.Lock()
	defer gemmaLifeMu.Unlock()
	gemmaProbeFn = fn
}

func getGemmaServeFn() func(restart bool) error {
	gemmaLifeMu.RLock()
	defer gemmaLifeMu.RUnlock()
	return gemmaServeFn
}

func setGemmaServeFn(fn func(restart bool) error) {
	gemmaLifeMu.Lock()
	defer gemmaLifeMu.Unlock()
	gemmaServeFn = fn
}

// defaultGemmaServe (re)starts the warm broker via the SAME `sirsi gemma serve`
// path the LaunchAgent uses — idempotent + RAM-gated (ADR-031), so a start when
// already warm no-ops and a start that won't fit refuses rather than OOMs. On a
// confirmed wedge it first gracefully stops (SIGTERM the process group; A32 —
// never SIGKILL). `serve` forks a detached child and returns, so this never
// blocks the supervisor tick.
func defaultGemmaServe(restart bool) error {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "sirsi" // fall back to PATH (the supervisor plist exports it)
	}
	if restart {
		_ = exec.Command(exe, "gemma", "serve", "--stop").Run() // graceful; ignore if already down
	}
	return exec.Command(exe, "gemma", "serve").Run()
}

// restorePollInterval is the sleep between successive post-restore re-probes.
// A large model (gemma-4-12B) can take 60-120s to mmap weights onto the GPU;
// a 5s flat sleep fires before the broker is ready and reports false failure.
// var so tests can zero it for instant termination.
var restorePollInterval = 10 * time.Second

// restoreDeadlineDur is the maximum wall time to poll for the broker to recover
// before routing a failure. Only route when this expires — never on the first
// post-restart probe. var so tests can set to 0 (first failed probe routes).
var restoreDeadlineDur = 2 * time.Minute

// restoreFailTitle is the router item title deduped against the open user inbox
// so a persistent RAM gap does not flood every 2-minute duty tick.
const restoreFailTitle = "gemma-liveness: broker restore failed — free memory"

// RunGemmaLivenessDuty is the supervisor duty: probe the broker and restore it.
// Signature matches the SupervisorDuty GoRun contract (routerRoot, repoRoot).
// routerRoot is used to route an owner alert when a restore does not stick.
func RunGemmaLivenessDuty(routerRoot, _ string) error {
	home, _ := os.UserHomeDir()
	status, detail := getGemmaProbeFn()(home)
	switch status {
	case liveness.GemmaHealthy, liveness.GemmaBusy:
		gemmaWedgeStrikes = 0
		return nil
	case liveness.GemmaDown:
		gemmaWedgeStrikes = 0
		// Nothing serving — start it (RAM-gated inside serve; non-forcing).
		fmt.Fprintf(os.Stderr, "gemma-liveness: broker down (%s) — starting\n", detail)
		if err := getGemmaServeFn()(false); err != nil {
			// serve refused (RAM won't fit, install missing, etc.) — route once.
			gemmaRouteRestoreFail(routerRoot, err.Error())
			return err
		}
		RecordHeal("local AI was down — restarted (bounded)")
		return nil
	case liveness.GemmaWedged:
		gemmaWedgeStrikes++
		if gemmaWedgeStrikes < gemmaWedgeThreshold {
			// Confirm across ticks — a single transient never bounces the broker.
			fmt.Fprintf(os.Stderr, "gemma-liveness: broker wedged (%s) — strike %d/%d, waiting to confirm\n",
				detail, gemmaWedgeStrikes, gemmaWedgeThreshold)
			return nil
		}
		gemmaWedgeStrikes = 0
		fmt.Fprintf(os.Stderr, "gemma-liveness: broker wedged confirmed (%s) — graceful restart\n", detail)
		if err := getGemmaServeFn()(true); err != nil { // stop + start; never SIGKILL (A32/ADR-040)
			gemmaRouteRestoreFail(routerRoot, err.Error())
			return err
		}
		// Post-restore: poll until healthy or deadline expires. A large model
		// takes 60-120s to mmap weights; a single short probe almost always
		// fires during that window and reports a false failure. Poll every
		// restorePollInterval up to restoreDeadlineDur; only route failure
		// when the deadline expires so a genuine success is never mislabelled.
		// ponytail: global deadline, per-model tuning if load times diverge.
		deadline := getRestoreDeadline()
		restoreStart := time.Now()
		for {
			getRestoreWait()()
			postStatus, postDetail := getGemmaProbeFn()(home)
			if postStatus == liveness.GemmaHealthy || postStatus == liveness.GemmaBusy {
				break
			}
			if time.Since(restoreStart) >= deadline {
				statusStr := map[liveness.GemmaStatus]string{liveness.GemmaWedged: "wedged", liveness.GemmaDown: "down"}[postStatus]
				gemmaRouteRestoreFail(routerRoot, "broker still "+statusStr+" after "+deadline.Round(time.Second).String()+" — "+postDetail)
				return nil
			}
		}
		RecordHeal("local AI stopped answering — gracefully restarted (bounded)")
		return nil
	}
	return nil
}

// restoreWaitFn is the injectable per-iteration sleep inside the post-restore
// poll loop (A16/A21). Tests set this to a no-op for instant termination.
var (
	restoreWaitMu sync.RWMutex
	restoreWaitFn = func() { time.Sleep(restorePollInterval) }
)

func getRestoreWait() func() {
	restoreWaitMu.RLock()
	defer restoreWaitMu.RUnlock()
	return restoreWaitFn
}

func setRestoreWaitFn(fn func()) {
	restoreWaitMu.Lock()
	defer restoreWaitMu.Unlock()
	restoreWaitFn = fn
}

func getRestoreDeadline() time.Duration {
	restoreWaitMu.RLock()
	defer restoreWaitMu.RUnlock()
	return restoreDeadlineDur
}

func setRestoreDeadlineDur(d time.Duration) {
	restoreWaitMu.Lock()
	defer restoreWaitMu.Unlock()
	restoreDeadlineDur = d
}

// gemmaRouteRestoreFail sends ONE non-duplicate "restore did not stick" item
// to the owner via the dispatch facade (not work.Send directly) so the alert
// is written to BOTH the router store AND the audit file — without the store
// row the wake path (router wait) never delivers the alert. Silently skips
// when routerRoot is "" (test scaffolds, library consumers without a real
// router).
func gemmaRouteRestoreFail(routerRoot, reason string) {
	if routerRoot == "" {
		return
	}
	f, err := dispatch.OpenRoot(routerRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gemma-liveness: route restore-fail: open dispatch: %v\n", err)
		return
	}
	defer func() { _ = f.Close() }()

	// Dedup across both owner-surface recipients: the earlier 20260806-005954
	// alert was addressed to "user" (from the liveness-watch path) while this
	// path sends to "owner" — each inbox-scoped check missed the other, letting
	// the same alarm appear twice. Check both to honor the ONE-open guarantee.
	for _, rcpt := range []string{"owner", "user"} {
		if items, listErr := f.Inbox(rcpt); listErr == nil {
			for _, it := range items {
				if it.Title == restoreFailTitle {
					fmt.Fprintf(os.Stderr, "gemma-liveness: restore-fail item already open in %s inbox, skipping route\n", rcpt)
					return
				}
			}
		}
	}
	body := "The gemma-liveness supervisor duty attempted to restore the warm broker " +
		"(load-bearing Tier-0 substrate for router/reconcile/gemma-builder) but the " +
		"broker did not recover. Reason: " + reason + ". " +
		"Most likely cause: RAM won't fit the model. " +
		"Fix: free memory (run `sirsi reap-sessions --apply` to reclaim leaked sessions, " +
		"or right-size the model in ~/.sirsi/gemma-model.conf to a smaller variant), " +
		"then run `sirsi gemma serve` to restart manually."
	if _, sendErr := f.Send("horus", "owner", restoreFailTitle, "decision", body); sendErr != nil {
		fmt.Fprintf(os.Stderr, "gemma-liveness: route restore-fail: %v\n", sendErr)
	}
}
