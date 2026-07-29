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

// RunGemmaLivenessDuty is the supervisor duty: probe the broker and restore it.
// Signature matches the SupervisorDuty GoRun contract (routerRoot, repoRoot).
func RunGemmaLivenessDuty(_, _ string) error {
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
			return err
		}
		RecordHeal("local AI stopped answering — gracefully restarted (bounded)")
		return nil
	}
	return nil
}
