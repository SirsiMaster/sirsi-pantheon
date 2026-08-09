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
	"path/filepath"
	"strings"
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

// restoreWait is the pause after `sirsi gemma serve` before the post-restore
// re-probe: long enough for the broker to bind its port, short enough not to
// block the supervisor tick. Var (not const) so tests can zero it out.
var restoreWait = 5 * time.Second

// restoreFailTitle is the router item title deduped against the open user inbox
// so a persistent RAM gap does not flood every 2-minute duty tick.
const restoreFailTitle = "gemma-liveness: broker restore failed — free memory"

// weightsAbsentTitle is the router item title for the "weights likely absent"
// class: the HF model cache was deleted and a restart cannot restore it. Distinct
// from restoreFailTitle so the two conditions have independent dedup tracks.
const weightsAbsentTitle = "gemma-liveness: model weights absent — re-download required"

// QuarantineMarkerPath is the file whose presence tells every gemma-liveness
// restorer to STAND DOWN. Its absence has always meant "restore me"; there was
// no way to say "I stopped this on purpose."
//
// Live incident, 2026-08-06: the operator stopped the broker deliberately and
// moved its LaunchAgent plists to ~/.sirsi/quarantined-wake-plists/ — which
// defeats ai.sirsi.liveness-watch and sirsi-fabric-watchdog.sh (both
// launchd-based, both need a loaded label to act on). It did NOT defeat THIS
// duty, because RunGemmaLivenessDuty is a Go tick inside the resident router
// process, not a launchd consumer — plist quarantine is invisible to it by
// construction. It restarted the broker within one 2-minute tick, twice,
// silently overriding an explicit operator instruction each time.
//
// So there were three independent restorers, and moving plists only silenced
// two of them. The fix is a signal all three can observe without launchd: a
// plain marker file, checked first, before any probe result is acted on.
// `sirsi gemma quarantine` / `sirsi gemma unquarantine` are the sanctioned way
// to set or clear it — never hand-edit the file.
func QuarantineMarkerPath(home string) string {
	return filepath.Join(home, ".sirsi", "gemma-quarantine")
}

// isQuarantinedFn is the injected existence check (A16/A21) — a plain os.Stat
// by default, swappable in tests without touching the real filesystem.
var isQuarantinedFn = func(home string) bool {
	_, err := os.Stat(QuarantineMarkerPath(home))
	return err == nil
}

func setIsQuarantinedFn(fn func(home string) bool) {
	gemmaLifeMu.Lock()
	defer gemmaLifeMu.Unlock()
	isQuarantinedFn = fn
}

func getIsQuarantinedFn() func(home string) bool {
	gemmaLifeMu.RLock()
	defer gemmaLifeMu.RUnlock()
	return isQuarantinedFn
}

// RunGemmaLivenessDuty is the supervisor duty: probe the broker and restore it.
// Signature matches the SupervisorDuty GoRun contract (routerRoot, repoRoot).
// routerRoot is used to route an owner alert when a restore does not stick.
func RunGemmaLivenessDuty(routerRoot, _ string) error {
	home, _ := os.UserHomeDir()
	// Checked BEFORE the probe result is acted on — deliberately before the
	// switch below, so a quarantined broker being GONE (the intended state)
	// never reaches the GemmaDown branch that would restart it. Strikes reset
	// so a later un-quarantine starts the wedge-confirmation counter clean.
	if getIsQuarantinedFn()(home) {
		gemmaWedgeStrikes = 0
		return nil
	}
	status, detail := getGemmaProbeFn()(home)
	switch status {
	case liveness.GemmaHealthy, liveness.GemmaBusy:
		gemmaWedgeStrikes = 0
		// A healthy tick is the common case and the only place a launchd-owned
		// respawn (KeepAlive crash-relaunch, or a broker that was never started
		// through `sirsi gemma serve`) ever gets its pidfile corrected — the
		// restart branches below already get this for free via the `sirsi gemma
		// serve` subprocess they shell out to. Without it, gemma-server.pid can
		// sit stale for hours while the broker itself is fine (found via router
		// item 20260809-060146).
		liveness.SyncGemmaPidFile(home)
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
		// "Weights likely absent" class: RSS below the weight floor means the HF
		// model cache was deleted — a restart will NOT fix it (the process comes
		// back up but still can't load weights from a missing cache). Skip the
		// restart entirely; route a re-download item to the owner instead.
		if strings.Contains(detail, liveness.WeightsAbsentSentinel) {
			gemmaWedgeStrikes = 0
			gemmaRouteWeightsAbsent(routerRoot, detail)
			return nil
		}
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
		// Post-restore re-probe: wait briefly for the port to bind, then verify.
		// If the broker is still wedged (e.g. RAM won't fit) the restart silently
		// failed — route once so the owner sees "restore did not stick."
		getRestoreWait()()
		if postStatus, postDetail := getGemmaProbeFn()(home); postStatus == liveness.GemmaWedged || postStatus == liveness.GemmaDown {
			statusStr := map[liveness.GemmaStatus]string{liveness.GemmaWedged: "wedged", liveness.GemmaDown: "down"}[postStatus]
			gemmaRouteRestoreFail(routerRoot, "broker still "+statusStr+" after graceful restart — "+postDetail)
			return nil
		}
		RecordHeal("local AI stopped answering — gracefully restarted (bounded)")
		return nil
	}
	return nil
}

// restoreWaitFn is the injectable sleep before the post-restore re-probe (A16/A21).
var (
	restoreWaitMu sync.RWMutex
	restoreWaitFn = func() { time.Sleep(restoreWait) }
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

	// Dedup: skip if the owner already has an open item with this title.
	if items, listErr := f.Inbox("owner"); listErr == nil {
		for _, it := range items {
			if it.Title == restoreFailTitle {
				fmt.Fprintf(os.Stderr, "gemma-liveness: restore-fail item already open, skipping route\n")
				return
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

// gemmaRouteWeightsAbsent routes ONE non-duplicate "re-download weights" item to
// the owner when the broker is running but has no model weights (RSS < 1 GB).
// A restart cannot fix this — it sends the same broker back up with an empty HF
// cache. Only the owner can re-download the model, so we alert immediately rather
// than wasting two ticks on a futile restart.
func gemmaRouteWeightsAbsent(routerRoot, detail string) {
	if routerRoot == "" {
		return
	}
	f, err := dispatch.OpenRoot(routerRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gemma-liveness: route weights-absent: open dispatch: %v\n", err)
		return
	}
	defer func() { _ = f.Close() }()

	// Dedup: skip if the owner already has an open item with this title.
	if items, listErr := f.Inbox("owner"); listErr == nil {
		for _, it := range items {
			if it.Title == weightsAbsentTitle {
				fmt.Fprintf(os.Stderr, "gemma-liveness: weights-absent item already open, skipping route\n")
				return
			}
		}
	}
	body := "The gemma broker process is running but its RSS is below the 1 GB weight floor — " +
		"the model weights are absent from the HuggingFace cache. Detail: " + detail + ". " +
		"A restart will NOT fix this: the broker comes back up but still cannot load weights from an empty cache. " +
		"Re-download the weights: `huggingface-cli download $(cat ~/.sirsi/gemma-model.conf)`, " +
		"then run `sirsi gemma serve` to reload."
	if _, sendErr := f.Send("horus", "owner", weightsAbsentTitle, "decision", body); sendErr != nil {
		fmt.Fprintf(os.Stderr, "gemma-liveness: route weights-absent: %v\n", sendErr)
	}
}
