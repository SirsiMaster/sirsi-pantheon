// Package router — wake.go
//
// Wake-or-declare-unavailable (PR#2 router hardening). The honest-liveness work
// (#79/#80) and the stranded-inbox surface (#85) tell us WHICH agents have work
// waiting and whether they are armed. This file closes the loop: on a SUPERVISOR
// or DOCTOR tick (never on `router send`), it walks every open inbox item and for
// each one either (a) confirms the target is armed and leaves it, (b) wakes the
// target via an explicitly-configured, reachable adapter exactly once, or (c)
// records wake_status:wake-unavailable on the item itself — so a stranded item is
// never silent and never blind-spawned.
//
// Design constraints (claude-home bind 2026-06-19, codex SME):
//  1. Wake fires from a supervisor/doctor pass, not inside router send.
//  2. LaunchAgent pull-loops are pull-loop watchers, armed by HEARTBEAT freshness
//     — never the loop-monitor pgrep gate (#79/#80 stays Claude-only).
//  3. Interactive claude-* are never daemonized/blind-spawned — if unarmed they
//     are routed via the claude-home conduit or marked wake-unavailable.
//  4. cli-spawn is EXPLICIT-only: a legacy Command array (WakeMechanism()'s
//     cli-spawn default) must NOT trigger a wake.
//  5. The mechanism enum carries launchagent + none (wakemechanism.go).
//  6. Delivery truth is additive item frontmatter, not a sidecar (work.SetWake).
//  7. Idempotency: item wake_attempted_at + (for LaunchAgent) label/pidfile/flock.
package router

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// Wake status values recorded on an item's wake_status frontmatter field.
const (
	WakeStatusPending     = "pending"
	WakeStatusAttempted   = "wake-attempted"
	WakeStatusUnavailable = "wake-unavailable"
	WakeStatusArmed       = "armed"
)

// DefaultWakeRetryAfter bounds re-invocation: once an item records a
// wake_attempted_at, the pass will not invoke its adapter again until this long
// has elapsed. Keeps repeated supervisor ticks from spawning a worker per tick.
const DefaultWakeRetryAfter = 10 * time.Minute

// inboxUnion lists open items for agent (all agents if agent==""), unioning the
// store with the files when the ADR-036/037 cutover is active so store-only
// items are visible to the internal/router surfaces (node-status, supervisor,
// wake pass). Default-off it is exactly work.ListInbox — byte-identical — and if
// the store cannot open it degrades to files rather than failing the surface.
func inboxUnion(routerRoot, agent string) ([]work.Item, error) {
	if !routercfg.StoreWake() {
		return work.ListInbox(routerRoot, agent)
	}
	f, err := dispatch.OpenRoot(routerRoot)
	if err != nil {
		// FAIL CLOSED. Post-cutover the store is the authority and the files are a
		// frozen legacy copy; degrading to them on a store-open failure resurrects
		// closed work through pull/wake/plan/board — reproducing the exact P0 this
		// path was fixed for, precisely when the store is unavailable. The facade's
		// own read methods fail closed for the same reason; this must match them.
		return nil, fmt.Errorf("router: store unavailable and the cutover makes it the sole authority (refusing to read frozen legacy files): %w", err)
	}
	defer func() { _ = f.Close() }()
	return f.Inbox(agent)
}

// inboxUnionAll reads EVERY open item once and partitions by recipient.
//
// The supervisor used to call inboxUnion(routerRoot, id) inside its per-agent
// loop, and each call is a full dispatch.OpenRoot → Inbox → Close. With 22
// registered agents that is 22 SQLite open/close cycles PER TICK, every tick,
// each one dirtying WAL and journal pages — measured at ~2.1 GB/day of disk
// writes, enough that the kernel filed a disk-writes report (claude-home,
// router item 20260731-105700).
//
// The work is identical: one open, one read, partition in memory. Agents with
// no items get an empty slice rather than being absent, so callers keep their
// existing "every registered agent appears" contract.
func inboxUnionAll(routerRoot string, agentIDs []string) (map[string][]work.Item, error) {
	all, err := inboxUnion(routerRoot, "")
	if err != nil {
		return nil, err
	}
	byAgent := make(map[string][]work.Item, len(agentIDs))
	for _, id := range agentIDs {
		byAgent[id] = nil
	}
	for _, it := range all {
		if _, tracked := byAgent[it.To]; tracked {
			byAgent[it.To] = append(byAgent[it.To], it)
		}
	}
	return byAgent, nil
}

// OpenItems is inboxUnion for callers outside this package (the CLI's ctr,
// conduit-tick, and plan surfaces). Every reader of open work must go through
// ONE cutover-aware entry point — a surface still calling work.ListInbox
// directly reads the frozen legacy files and reports closed items as open.
func OpenItems(routerRoot, agent string) ([]work.Item, error) {
	return inboxUnion(routerRoot, agent)
}

// AllItems is OpenItems for whole-corpus readers (the work board's pace and
// turnaround stats), which need closed items too.
func AllItems(routerRoot string) ([]work.Item, error) {
	if !routercfg.StoreWake() {
		return work.ListAll(routerRoot)
	}
	f, err := dispatch.OpenRoot(routerRoot)
	if err != nil {
		return nil, fmt.Errorf("router: store unavailable and the cutover makes it the sole authority (refusing to read frozen legacy files): %w", err)
	}
	defer func() { _ = f.Close() }()
	return f.ListAll()
}

// AgentWakeHealth (the readiness verdict) is defined in nodestatus.go — it is the
// per-agent wake-readiness type the node-status surface already exposes. The wake
// pass reuses it so the surfaced readiness and the acted-on readiness are one type.

// ExplicitWakeMechanism returns ONLY the explicitly-configured wake mechanism.
// Unlike AgentConfig.WakeMechanism(), it never infers cli-spawn from a bare
// Command array: a legacy command agent has declared no wake intent and must
// never be blind-spawned on a tick (constraint 4). Returns "" for such agents.
func ExplicitWakeMechanism(cfg AgentConfig) string {
	return strings.TrimSpace(cfg.Wake.Mechanism)
}

// isInteractiveSpawn reports a delivery capability, not a model vendor. New
// registrations declare wake.session_mode. The Claude fallback preserves the
// fail-closed behavior of legacy records until they are migrated.
func isInteractiveSpawn(cfg AgentConfig) bool {
	switch strings.TrimSpace(cfg.Wake.SessionMode) {
	case "interactive":
		return true
	case "headless":
		return false
	default:
		return strings.EqualFold(strings.TrimSpace(cfg.Type), "claude")
	}
}

// ProbeWakeReadiness probes whether cfg can be woken right now without
// blind-spawning. It is the HONEST, explicit-only readiness the wake pass acts
// on (and node-status can surface): unlike the legacy permissive view, a bare
// command array is NOT treated as wakeable.
func ProbeWakeReadiness(cfg AgentConfig) AgentWakeHealth {
	mech := ExplicitWakeMechanism(cfg)
	h := AgentWakeHealth{AgentID: cfg.ID, Mechanism: mech}
	switch mech {
	case "":
		h.Detail = "no explicit wake mechanism — legacy command agents are never blind-spawned (set wake.mechanism to enable)"
	case WakeNone:
		h.Detail = "wake disabled (mechanism: none)"
	case WakeCLISpawn:
		switch {
		case isInteractiveSpawn(cfg):
			h.Detail = fmt.Sprintf("interactive %s agent — not blind-spawned; arm its /loop or route via claude-home conduit", cfg.Type)
		case len(cfg.Command) == 0:
			h.Detail = "cli-spawn configured but command array is empty"
		default:
			if _, err := exec.LookPath(cfg.Command[0]); err != nil {
				h.Detail = fmt.Sprintf("cli-spawn command %q not found in PATH", cfg.Command[0])
			} else {
				h.Ready, h.Adapter, h.Detail = true, WakeCLISpawn, "explicit cli-spawn ready"
			}
		}
	case WakeAPICall:
		if strings.TrimSpace(cfg.Wake.Endpoint) == "" {
			h.Detail = "api-call configured but wake.endpoint is empty"
		} else {
			h.Ready, h.Adapter, h.Detail = true, WakeAPICall, cfg.Wake.Endpoint
		}
	case WakeMCPNotification:
		// MCP wake is not yet wired in the pull-model CLI — there is no invoker
		// that can deliver an MCP notification, so readiness MUST report not-ready.
		// The doctor's report view and the acted-on pass must agree; claiming
		// "ready" here while the invoker always fails is dishonest readiness
		// (codex SME #89, finding 3). Wire an adapter before flipping this.
		h.Detail = "mcp-notification wake is not yet wired in the pull-model CLI — use launchagent or api-call"
	case WakeLaunchAgent:
		label := WakeLaunchAgentLabel(cfg.ID)
		if !launchAgentInstalled(label) {
			h.Detail = fmt.Sprintf("launchagent %s not installed — run `sirsi router wake-install %s`", label, cfg.ID)
		} else {
			h.Ready, h.Adapter, h.Detail = true, WakeLaunchAgent, label
		}
	default:
		h.Detail = fmt.Sprintf("unsupported wake mechanism %q", mech)
	}
	return h
}

// WakeInvoke performs the side effect of waking an agent via the named adapter.
// Injectable (Rule A16) + mutex-guarded (Rule A21) so the wake pass is testable
// without spawning real processes or making network calls.
type WakeInvoke func(cfg AgentConfig, adapter string) error

var (
	wakeInvokeMu sync.RWMutex
	wakeInvokeFn WakeInvoke = defaultWakeInvoke
)

func getWakeInvoke() WakeInvoke {
	wakeInvokeMu.RLock()
	defer wakeInvokeMu.RUnlock()
	return wakeInvokeFn
}

func setWakeInvoke(fn WakeInvoke) {
	wakeInvokeMu.Lock()
	defer wakeInvokeMu.Unlock()
	if fn == nil {
		fn = defaultWakeInvoke
	}
	wakeInvokeFn = fn
}

// launchctlTimeout bounds every launchctl subprocess this package spawns.
//
// `launchctl kickstart -k` against a label sitting at `state = spawn scheduled`
// never returns: -k means kill-then-start, and with nothing running it blocks
// waiting for a spawn that will not come. That is the RESTING state of an
// interval-driven wake agent, not a corrupt one, so bootout+bootstrap buys a
// single iteration and the next pass hangs again. Observed 2026-07-29: one
// `sirsi router doctor --fix` stuck 18m15s in wait4 on this exact call, leaving
// orphaned launchctl clients behind that blocked forever after their parent died.
//
// A doctor that never returns is strictly worse than one that reports a label it
// could not reach, so the deadline is short and the failure is explicit.
// A var, not a const, purely so the deadline test can shorten it — the timeout
// path is only real if a test can drive a child that actually outlives it.
var launchctlTimeout = 15 * time.Second

// runLaunchctl runs launchctl under a hard deadline. On expiry CommandContext
// kills the child, which is what keeps hung clients from accumulating.
func runLaunchctl(args ...string) error {
	return runBounded("launchctl", args...)
}

// runBounded runs name under launchctlTimeout and wraps context.DeadlineExceeded
// on expiry so callers (and tests) can tell a timeout from a real exit status.
// Run returning at all means the child was killed and reaped by CommandContext.
//
// Split out from runLaunchctl so the deadline is exercisable with a command that
// genuinely blocks; any launchctl invocation the test could reach either returns
// immediately or hangs the test suite for the full production bound.
func runBounded(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), launchctlTimeout)
	defer cancel()
	err := exec.CommandContext(ctx, name, args...).Run()
	if ctx.Err() == context.DeadlineExceeded {
		// Deliberately says nothing about launchd state. This helper also carries
		// print, bootout, and the checker probes, and a timeout there tells us
		// only that launchctl did not answer — asserting a `spawn scheduled` label
		// would be inventing an unobserved state, which is the same class of
		// mistake as the "parked" diagnosis this change removed. Callers that
		// actually know the operation add their own context.
		return fmt.Errorf("%s %s: no response in %s: %w",
			name, strings.Join(args, " "), launchctlTimeout, context.DeadlineExceeded)
	}
	return err
}

func defaultWakeInvoke(cfg AgentConfig, adapter string) error {
	switch adapter {
	case WakeCLISpawn:
		if len(cfg.Command) == 0 {
			return fmt.Errorf("cli-spawn: empty command")
		}
		cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
		cmd.Dir = cfg.Cwd
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		// Detach (Unix Setsid) + Release so the nudged worker survives this
		// short-lived doctor tick and leaves no zombie — the established
		// cmd/sirsi router-event spawn pattern (codex SME #89, finding 2).
		cmd.SysProcAttr = detachedSysProcAttr()
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	case WakeAPICall:
		req, err := http.NewRequest(http.MethodPost, cfg.Wake.Endpoint, nil)
		if err != nil {
			return err
		}
		if cfg.Wake.Auth != "" {
			req.Header.Set("Authorization", cfg.Wake.Auth)
		}
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("api-call: wake endpoint returned %s", resp.Status)
		}
		return nil
	case WakeLaunchAgent:
		// The pull-loop is resident; kick it to poll immediately.
		label := WakeLaunchAgentLabel(cfg.ID)
		target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
		if err := runLaunchctl("kickstart", "-k", target); err != nil {
			// Only here do we know the operation was a kickstart -k, which is the
			// one that blocks when the label has a spawn scheduled but nothing
			// running. runBounded stays silent about launchd state precisely so
			// this inference lives where it is actually warranted.
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("wake %s: launchctl kickstart -k %s did not return in %s — "+
					"the label likely has a spawn scheduled with nothing running yet: %w",
					cfg.ID, target, launchctlTimeout, err)
			}
			return err
		}
		return nil
	default:
		// Includes mcp-notification: ProbeWakeReadiness never yields it as a
		// ready adapter (not yet wired), so this is the honest failure if reached.
		return fmt.Errorf("no invoker for adapter %q", adapter)
	}
}

// WakeOutcome is one item's result in a wake pass.
type WakeOutcome struct {
	ItemID  string `json:"item_id"`
	AgentID string `json:"agent_id"`
	Adapter string `json:"adapter,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// WakePassReport summarizes one wake-or-declare-unavailable pass.
type WakePassReport struct {
	Armed       []WakeOutcome `json:"armed,omitempty"`
	Attempted   []WakeOutcome `json:"attempted,omitempty"`
	Unavailable []WakeOutcome `json:"unavailable,omitempty"`
}

// WakePass runs one wake-or-declare-unavailable pass over every OPEN inbox item.
// This is the supervisor/doctor-tick wake (constraint 1): it is never called from
// `router send`. For each item, by its target agent:
//   - armed (a non-terminal, heartbeat-FRESH thread exists) → annotate armed, no spawn.
//   - unarmed + WakeHealth.Ready → invoke the adapter ONCE (idempotent via the
//     item's wake_attempted_at), annotate wake-attempted.
//   - unarmed + !Ready → annotate wake-unavailable + wake_error (never blind-spawn).
//
// "Armed" is heartbeat freshness only (constraint 2) — it deliberately does NOT
// consult the loop-monitor pgrep gate, so it never widens #79/#80 to pull-loops.
func WakePass(routerRoot string, now time.Time) (WakePassReport, error) {
	return WakePassFiltered(routerRoot, now, nil)
}

// WakePassFiltered is WakePass restricted to the items `allow` accepts. A nil
// filter is allow-all, so WakePass is byte-identical. The continuous work loop
// (ADR-039 P3) passes a filter of exactly the gate-cleared, dispatch-authorized
// item ids, so a wake pass can never touch an owner-gated item.
func WakePassFiltered(routerRoot string, now time.Time, allow func(work.Item) bool) (WakePassReport, error) {
	var rep WakePassReport
	if now.IsZero() {
		now = time.Now().UTC()
	}

	reg, err := LoadRegistry(routerRoot)
	if err != nil {
		return rep, fmt.Errorf("load agents: %w", err)
	}
	threadReg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return rep, fmt.Errorf("load threads: %w", err)
	}

	// "Armed" means SOMETHING WILL DRAIN THIS INBOX — not merely that some
	// process for this agent is alive.
	//
	// This used to be heartbeat freshness alone, over any thread of any surface.
	// A wake loop with nothing to dispatch heartbeats identically to one that
	// works the queue, so a watch-only loop credited its own lane as armed and
	// this pass then SKIPPED it — the loop manufactured the evidence that
	// suppressed its own rescue, and claude-deck sat at 5 open items behind a
	// green heartbeat while doctor reported "already-armed" (review finding 4).
	//
	// IsInboxConsumer is the honest predicate: agent-session surfaces count (they
	// are the consumer), observer/worker surfaces must have resolved a declared
	// consumer capability.
	armed := map[string]bool{}
	thisMachine := MachineID()
	for _, t := range threadReg.SortedThreads() {
		if t.Status.IsTerminal() || t.IsStale(now, DefaultThreadStaleAfter) {
			continue
		}
		// Two independent ways a thread can be fresh-looking and still not be
		// draining its inbox. Both must demote, so this is a UNION of the two
		// fixes that landed either side of this merge — taking one would have
		// silently dropped the other's guarantee.

		// (1) A watcher that never dispatches is not armed. RunWakeLoop used to
		// heartbeat beside an inbox forever without consuming it, and that
		// heartbeat marked the agent armed — the loop manufactured the evidence
		// that suppressed its own rescue.
		if !t.IsInboxConsumer() {
			continue
		}

		// (2) HEARTBEAT FRESHNESS IS NOT LIVENESS when the process is PROVABLY
		// gone. A record keeps its last heartbeat for the whole stale window, so
		// a worker that died mid-window still reads "armed" and its inbox is
		// never woken. Observed live 2026-08-03: the claude-pantheon worker died
		// at 21:21Z, 24 launchd jobs sat down ~15 hours, and CTR reported the
		// lane heartbeat-fresh the entire time.
		//
		// Same asymmetry ReapDeadThreads and ReconcileExits already use: ONLY a
		// gone/defunct pid demotes. PIDUnknown (unreadable table, EPERM) and
		// foreign-machine records keep the old heartbeat behavior rather than
		// stranding a lane on a probe failure, and a record with no pid at all —
		// the normal shape for an app-hosted session — is untouched.
		if t.PID > 0 && SameMachine(t.MachineID, thisMachine) &&
			getPIDStateFn()(t.PID) == PIDGone {
			continue // provably dead: not armed, so its inbox gets woken
		}
		armed[t.AgentID] = true
	}

	// Post-cutover (ADR-036/037) open items live only as store rows, and a
	// store-only item has no file to annotate — so both the READ (which items are
	// waiting) and the WRITE (idempotent wake_status) must go through the facade,
	// which unions files+store and routes SetWake to whichever holds the item.
	// Default-off stays byte-identical: f == nil → the exact legacy file calls.
	var f *dispatch.Facade
	if routercfg.StoreWake() {
		repoRoot := filepath.Dir(filepath.Dir(routerRoot)) // <repo> from <repo>/.agents/idea-router
		if fac, ferr := dispatch.Open(repoRoot); ferr == nil {
			f = fac
			defer func() { _ = f.Close() }()
		}
	}
	setWake := func(id string, ann work.WakeAnnotation) {
		if f != nil {
			_ = f.SetWake(id, ann)
			return
		}
		_ = work.SetWake(routerRoot, id, ann)
	}

	var items []work.Item
	if f != nil {
		items, err = f.Inbox("") // all open items (store ∪ files)
	} else {
		items, err = work.ListInbox(routerRoot, "") // all open items
	}
	if err != nil {
		return rep, fmt.Errorf("list inbox: %w", err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })

	invoke := getWakeInvoke()
	// One wake per agent per pass. The adapter nudges an agent, not an item —
	// once its pull-loop is kicked it drains its whole inbox — so invoking per
	// item made an agent with N stranded items cost N identical adapter calls.
	// With a blocking launchctl that turned 16 items into 16 sequential hangs.
	// Items still each get their own annotation below; only the side effect is
	// shared, so a failed wake still marks every one of that agent's items.
	invoked := map[string]error{}
	for _, item := range items {
		agentID := item.To
		if agentID == "" {
			continue
		}
		// The continuous loop's gate/authorization filter: skip any item not
		// explicitly cleared for dispatch (owner-gated items never pass).
		if allow != nil && !allow(item) {
			continue
		}

		if armed[agentID] {
			// Idempotent: only WRITE when the status actually changes to armed.
			// Re-writing an already-armed item every pass bumps its mtime, and
			// when items/ is a launchd WatchPath (the conduit mesh) that would
			// self-trigger an endless tick loop. A steady state produces no writes.
			if item.WakeStatus != WakeStatusArmed {
				setWake(item.ID, work.WakeAnnotation{Status: WakeStatusArmed})
			}
			rep.Armed = append(rep.Armed, WakeOutcome{ItemID: item.ID, AgentID: agentID})
			continue
		}

		var health AgentWakeHealth
		if cfg, lerr := reg.Lookup(agentID); lerr != nil {
			health = AgentWakeHealth{Detail: fmt.Sprintf("agent %q not registered", agentID)}
		} else {
			health = ProbeWakeReadiness(*cfg)
		}

		if !health.Ready {
			// Idempotent (same reason as the armed branch): only write on change.
			if item.WakeStatus != WakeStatusUnavailable {
				setWake(item.ID, work.WakeAnnotation{Status: WakeStatusUnavailable, Error: health.Detail})
			}
			rep.Unavailable = append(rep.Unavailable, WakeOutcome{ItemID: item.ID, AgentID: agentID, Detail: health.Detail})
			continue
		}

		// Idempotency: don't re-invoke while a prior attempt is still fresh.
		if item.WakeStatus == WakeStatusAttempted && item.WakeAttemptedAt != "" {
			if at, perr := time.Parse(time.RFC3339, item.WakeAttemptedAt); perr == nil && now.Sub(at) < DefaultWakeRetryAfter {
				rep.Attempted = append(rep.Attempted, WakeOutcome{ItemID: item.ID, AgentID: agentID, Adapter: health.Adapter, Detail: "already attempted (idempotent skip)"})
				continue
			}
		}

		cfg, _ := reg.Lookup(agentID) // Ready implies registered
		ann := work.WakeAnnotation{Status: WakeStatusAttempted, AttemptedAt: now.Format(time.RFC3339), Adapter: health.Adapter}
		ierr, alreadyInvoked := invoked[agentID]
		if !alreadyInvoked {
			ierr = invoke(*cfg, health.Adapter)
			invoked[agentID] = ierr
		}
		if ierr != nil {
			ann = work.WakeAnnotation{Status: WakeStatusUnavailable, Error: fmt.Sprintf("%s adapter failed: %v", health.Adapter, ierr)}
			setWake(item.ID, ann)
			rep.Unavailable = append(rep.Unavailable, WakeOutcome{ItemID: item.ID, AgentID: agentID, Adapter: health.Adapter, Detail: ann.Error})
			continue
		}
		setWake(item.ID, ann)
		rep.Attempted = append(rep.Attempted, WakeOutcome{ItemID: item.ID, AgentID: agentID, Adapter: health.Adapter})
	}
	return rep, nil
}

// DefaultWakeLoopInterval is the pull-loop heartbeat cadence (A27 bounded ≥60s).
const DefaultWakeLoopInterval = 60 * time.Second

// RunWakeLoop is the foreground bounded pull-loop for a worker/headless agent
// (A27: "a bounded headless pull loop over items/ plus sirsi thread heartbeat").
// It registers a concrete pull-loop thread (surface=worker, bound to THIS
// process's pid, so it stays armed by heartbeat freshness and is not OS-reaped
// between heartbeats), then each interval pulls the agent's inbox and heartbeats.
// It runs until ctx is canceled (SIGTERM/SIGINT from launchd), then closes the
// thread. It is meant to be RUN BY the wake LaunchAgent — external automation
// (A26 Automation Boundary) — and does NOT self-daemonize: it is a plain
// foreground loop, not a reintroduced daemon verb.
// wakeLoopHeartbeatLog bounds how long a running loop may stay silent. Chosen
// so a reader of the log can distinguish "quiet" from "dead" within one
// coffee-length gap, without turning the file into a tick log.
const wakeLoopHeartbeatLog = 15 * time.Minute

// dispatchFailureBackoff holds the dispatch slot when a consumer could not even
// START. There is no process to observe in that case, so a bounded pause is the
// only available instrument — the tight-error-loop half of the PR #199 lesson.
var dispatchFailureBackoff = 30 * time.Second

// closedChan is an already-closed channel, so a failed dispatch records a run
// that is complete on arrival.
func closedChan() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

// setThreadConsumerCapable persists whether a thread can drain its agent's
// inbox. Written once at loop start rather than every heartbeat: it is a
// property of the loop's configuration, not of a tick.
func setThreadConsumerCapable(routerRoot, threadID string, capable bool) error {
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return err
	}
	for _, t := range reg.Threads {
		if t != nil && t.ThreadID == threadID {
			t.ConsumerCapable = capable
			return SaveThreadRegistry(routerRoot, reg)
		}
	}
	return fmt.Errorf("thread %s not found", threadID)
}

// consumerRun tracks ONE dispatched consumer process for its whole lifetime.
//
// The first cut of this fix used a 10-minute timer as its re-dispatch authority,
// because Setsid+Release deliberately discards the process handle — so the loop
// could not tell "still working" from "died", and a slow-but-healthy agent got a
// second consumer dispatched on top of it. A timer is not dispatch authority
// (codex-pantheon review of #389, finding 3).
//
// So the handle is KEPT and reaped: Setsid still detaches the child from the
// terminal/session, but we never Release, and a goroutine Waits. `done` closing
// is real completion, not an elapsed guess.
type consumerRun struct {
	pid  int
	done chan struct{}
	err  error
}

// running reports whether the dispatched consumer is still alive.
func (r *consumerRun) running() bool {
	if r == nil {
		return false
	}
	select {
	case <-r.done:
		return false
	default:
		return true
	}
}

// dispatchConsumer starts a resolved consumer and returns a handle that
// completes when the process exits.
//
// Setsid without Release is the deliberate combination: the child survives this
// loop's own restart (it is in its own session), while this loop keeps the
// handle it needs to know whether the work is still in flight. cmd.Wait also
// reaps the child, so repeated dispatch cannot accumulate zombies.
func dispatchConsumer(rc *ResolvedConsumer) (*consumerRun, error) {
	if rc.Resident {
		return nil, fmt.Errorf("resident consumer is external and must not be spawned")
	}
	cmd := exec.Command(rc.Argv[0], rc.Argv[1:]...)
	cmd.Dir = rc.Cwd
	cmd.Env = rc.Env
	cmd.SysProcAttr = detachedSysProcAttr()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	run := &consumerRun{pid: cmd.Process.Pid, done: make(chan struct{})}
	go func() {
		run.err = cmd.Wait()
		close(run.done)
	}()
	return run, nil
}

func RunWakeLoop(ctx context.Context, routerRoot, agentID string, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultWakeLoopInterval
	}
	// Keyed singleton on (agent, surface=worker, this machine) — P-heartbeat-owner,
	// #178 pattern. Every launchd bootout/bootstrap gives this loop a new PID;
	// registering by PID minted a NEW worker thread each restart and left the old
	// record active until a reap pass, so agents accumulated duplicate workers
	// (claude-finalwishes carried two). Adopt the existing worker record instead —
	// the thread id stays stable across restarts — and retire any other OS-dead
	// worker duplicate for this agent while we're here.
	thr, err := adoptWorkerThread(routerRoot, agentID, os.Getpid())
	if err != nil {
		return fmt.Errorf("adopt wake-loop thread: %w", err)
	}
	if thr == nil {
		host, _ := os.Hostname()
		thr, err = RegisterThread(routerRoot, &Thread{
			AgentID:       agentID,
			Surface:       surfaceWorker,
			WakeMechanism: WakeLaunchAgent,
			PID:           os.Getpid(),
			Host:          host,
			Status:        ThreadStatusActive,
		})
		if err != nil {
			return fmt.Errorf("register wake-loop thread: %w", err)
		}
	}
	defer func() { _, _ = CloseThread(routerRoot, thr.ThreadID) }()

	// Resolve the DECLARED consumer once. A loop that cannot dispatch still runs:
	// its heartbeat is the only liveness signal the fabric has for this agent, and
	// dropping that would trade a silent inbox for a silent agent. But it now says
	// so out loud AND records it on its thread record, because "loop running,
	// inbox never drains" was previously indistinguishable from a healthy steady
	// state — and worse, its heartbeat marked the lane armed.
	var consumer *ResolvedConsumer
	if reg, rerr := LoadRegistry(routerRoot); rerr != nil {
		log.Printf("wake-loop %s: registry unreadable, running WATCH-ONLY: %v", agentID, rerr)
	} else if cfg, lerr := reg.Lookup(agentID); lerr != nil {
		log.Printf("wake-loop %s: agent not registered, running WATCH-ONLY: %v", agentID, lerr)
	} else if rc, why := ResolveConsumer(*cfg, routerRoot); rc == nil {
		log.Printf("wake-loop %s: WATCH-ONLY — this lane has NO consumer: %s", agentID, why)
	} else {
		consumer = rc
	}

	// Publish the capability on the thread record. This is what stops a watch-only
	// loop from crediting its own lane as armed: the armed predicate consults
	// IsInboxConsumer, not heartbeat freshness alone.
	//
	// A RESIDENT consumer never arms this watcher, even when its health check
	// passes. The check runs ONCE at startup and proves only that a binary could
	// print something — `gemma-pantheon`'s is literally
	// `sirsi-gemma-worker.sh --version`, which proves a file exists, not that an
	// external inbox consumer is alive. Crediting the watcher on that basis
	// means: external worker dies at any point after startup, watcher keeps
	// heartbeating as ConsumerCapable, WakePass skips the lane, inbox strands.
	// That is precisely the false-green this change exists to remove, so a
	// resident lane stays WATCH-ONLY here (codex-pantheon, PR #389).
	//
	// The real fix is the inverse: a resident must PUBLISH AND HEARTBEAT ITS OWN
	// consumer-capable thread. Liveness you did not observe is not liveness you
	// can borrow. That is the immediate follow-up, not this PR.
	capable := consumer != nil && !consumer.Resident
	if thr.ConsumerCapable != capable {
		thr.ConsumerCapable = capable
		if err := setThreadConsumerCapable(routerRoot, thr.ThreadID, capable); err != nil {
			log.Printf("wake-loop %s: could not record consumer capability: %v", agentID, err)
		}
	}

	tick := time.NewTicker(interval)
	defer tick.Stop()
	log.Printf("wake-loop %s: started (thread=%s pid=%d interval=%s consumer=%v)",
		agentID, thr.ThreadID, os.Getpid(), interval, consumer != nil)
	lastDepth := -1
	lastBeat := time.Now()
	// The in-flight consumer, or nil. Completion — not elapsed time — is what
	// authorizes the next dispatch.
	var run *consumerRun
	for {
		// Surface the inbox depth into the heartbeat status; the worker surface's
		// own logic processes the items — this loop only proves liveness + watches.
		//
		// Reads go through OpenItems, NOT work.ListInbox. Post-cutover the legacy
		// files are frozen, so reading them here derived the heartbeat status from
		// stale data — a surface reporting on a source nothing writes any more.
		// This was a call site #315 missed while claiming every observer had been
		// routed through the cutover-aware entry point.
		status := ThreadStatusIdle
		depth := 0
		items, lerr := OpenItems(routerRoot, agentID)
		if lerr != nil {
			// Fail loud in the log rather than silently reporting idle: an unreadable
			// inbox is not an empty one, and this loop is the only liveness signal
			// the fabric has for this agent.
			log.Printf("wake-loop %s: inbox read FAILED: %v", agentID, lerr)
		} else if depth = len(items); depth > 0 {
			status = ThreadStatusActive
		}

		// Log on CHANGE, and periodically regardless.
		//
		// Change-only was the first design and it is insufficient: at a constant
		// depth the loop emits one start line and one transition, then can stay
		// silent forever — so a healthy loop at depth 32 and a loop wedged
		// immediately after observing depth 32 leave IDENTICAL forensic records.
		// That is the exact ambiguity this logging exists to remove (codex review
		// of #327). A liveness line is not noise; it is the evidence that the
		// silence means "nothing changed" rather than "nothing is running".
		now := time.Now()
		switch {
		case depth != lastDepth:
			log.Printf("wake-loop %s: inbox depth %d -> %d (status=%s)",
				agentID, lastDepth, depth, status)
			lastDepth = depth
			lastBeat = now
		case now.Sub(lastBeat) >= wakeLoopHeartbeatLog:
			log.Printf("wake-loop %s: alive, inbox depth %d unchanged (status=%s)",
				agentID, depth, status)
			lastBeat = now
		}

		// Dispatch a consumer — EDGE-TRIGGERED, never per tick.
		//
		// `depth > 0` is a LEVEL, not an edge: it stays true for the whole time an
		// agent takes to work its inbox, so "if depth > 0 { dispatch }" would spawn
		// a fresh agent every interval on top of the one already draining. That is
		// the router-cutover fork-storm (PR #199) in slow motion, and each fork here
		// is a whole agent session rather than a `router wait`, so it is far more
		// expensive per iteration.
		//
		// So the gate is the CONSUMER'S OWN LIVENESS, not a clock: dispatch only
		// when nothing is in flight. A slow-but-healthy agent holds the slot for as
		// long as it genuinely needs, and a dead one frees it the instant it exits —
		// neither of which a 10-minute timer could distinguish (review finding 3).
		//
		// A read error is NOT a drain: lerr leaves depth 0, and dispatching on it
		// would treat an unreadable inbox as an empty one.
		if consumer != nil && !consumer.Resident && lerr == nil && depth > 0 && !run.running() {
			// Report the previous run's exit before starting another, so a consumer
			// that is failing fast leaves a trail rather than looking like progress.
			if run != nil && run.err != nil {
				log.Printf("wake-loop %s: previous consumer (pid %d) exited with error: %v",
					agentID, run.pid, run.err)
			}
			next, derr := dispatchConsumer(consumer)
			if derr != nil {
				// A command that fails to START will fail identically next tick, so
				// hold the slot for one retry window rather than tight-looping. This
				// is the only place a timer is still the right instrument: there is no
				// process to observe.
				run = &consumerRun{done: closedChan(), err: derr}
				log.Printf("wake-loop %s: dispatch FAILED (depth %d): %v", agentID, depth, derr)
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(dispatchFailureBackoff):
				}
			} else {
				run = next
				log.Printf("wake-loop %s: dispatched consumer pid %d %v (depth %d)",
					agentID, run.pid, consumer.Argv, depth)
			}
		}

		_, _ = Heartbeat(routerRoot, thr.ThreadID, HeartbeatUpdate{Status: status})

		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// adoptWorkerThread resolves the keyed-singleton worker thread for an agent on
// THIS machine: it re-points the newest live (non-terminal, non-suspended)
// surface=worker record at the calling process and retires any other worker
// record for the agent that is dead by OS truth (ADR-022 — live and suspended
// records are never touched). Returns nil when no record exists to adopt; the
// caller then registers a fresh one.
func adoptWorkerThread(routerRoot, agentID string, pid int) (*Thread, error) {
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil, err
	}
	thisMachine := MachineID()
	now := time.Now().UTC()

	var adopted *Thread
	for _, t := range reg.Threads {
		if t == nil || t.AgentID != agentID || t.Surface != surfaceWorker {
			continue
		}
		if t.Status.IsTerminal() || t.Status == ThreadStatusSuspended {
			continue
		}
		if !SameMachine(t.MachineID, thisMachine) {
			continue
		}
		if adopted == nil || t.LastSeenAt.After(adopted.LastSeenAt) {
			adopted = t
		}
	}
	if adopted == nil {
		return nil, nil
	}

	// Retire OS-dead duplicates — every live candidate EXCEPT the one adopted.
	for _, t := range reg.Threads {
		if t == nil || t == adopted || t.AgentID != agentID || t.Surface != surfaceWorker {
			continue
		}
		if t.Status.IsTerminal() || t.Status == ThreadStatusSuspended {
			continue
		}
		if !SameMachine(t.MachineID, thisMachine) {
			continue
		}
		if t.PID >= minAgentPID && !DeadByOSTruth(PIDStateOfThread(t)) {
			continue // genuinely alive — never touch it
		}
		t.Status = ThreadStatusReaped
		t.LastSeenAt = now
		t.LastError = fmt.Sprintf("reaped: duplicate worker superseded by %s at %s", adopted.ThreadID, now.Format(time.RFC3339))
	}

	adopted.PID = pid
	adopted.StartTime = PIDStartTimeOf(pid)
	adopted.Status = ThreadStatusActive
	adopted.WakeMechanism = WakeLaunchAgent
	adopted.LastSeenAt = now
	adopted.LastError = ""
	if adopted.MachineID == "" {
		adopted.MachineID = thisMachine
	}
	if err := SaveThreadRegistry(routerRoot, reg); err != nil {
		return nil, err
	}
	return adopted, nil
}

// ── LaunchAgent pull-loop install (constraint 2 + 7) ─────────────────────────

const wakeLaunchAgentPrefix = "ai.sirsi.router.wake."

var labelUnsafe = regexp.MustCompile(`[^a-zA-Z0-9.-]+`)

// launchAgentsDirOverride lets tests point the installer at a temp dir instead of
// the real ~/Library/LaunchAgents. Empty in production.
var launchAgentsDirOverride string

func launchAgentsDir() string {
	if launchAgentsDirOverride != "" {
		return launchAgentsDirOverride
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents")
}

// WakeLaunchAgentLabel returns the launchd label for an agent's wake pull-loop.
func WakeLaunchAgentLabel(agentID string) string {
	return wakeLaunchAgentPrefix + labelUnsafe.ReplaceAllString(strings.TrimSpace(agentID), "-")
}

func launchAgentInstalled(label string) bool {
	_, err := os.Stat(filepath.Join(launchAgentsDir(), label+".plist"))
	return err == nil
}

// wakeLaunchAgentPlist renders the per-agent pull-loop plist. The loop is the
// LaunchAgent's StartInterval: every interval it pulls the agent's inbox and
// heartbeats — a pull-loop watcher armed by heartbeat freshness, never a
// loop-monitor (constraint 2). Deterministic for a given (label, agent, bin) so
// InstallWakeLaunchAgent is idempotent by content comparison.
//
// ProgramArguments is a DIRECT argv (no `/bin/sh -c`): the loop is the real
// `sirsi router wake-loop <agent>` verb, so there is no shell to break on a path
// with spaces or to inject via a metacharacter-bearing agent id (codex SME #89,
// finding 4 — and finding 1: the prior `thread heartbeat --agent` flag did not
// exist). KeepAlive (not StartInterval) keeps ONE long-lived process whose stable
// pid is not OS-reaped between heartbeats; launchd restarts it if it exits. Both
// values are XML-escaped for the plist text.
func wakeLaunchAgentPlist(label string, cfg AgentConfig, sirsiBin string) string {
	// Observability + crash-loop bounds, learned the hard way (2026-07-07:
	// nine loops dead-respawning ~15k times with NO log file to say why):
	//   - StandardOut/ErrorPath: every start/exit leaves evidence under
	//     ~/.sirsi/logs/wake-<agent>.log.
	//   - ThrottleInterval 60: a crashing loop respawns once a minute, not
	//     at launchd's default burn rate.
	//   - The binary path is absolute (InstallWakeLaunchAgent enforces it).
	//   - EnvironmentVariables/PATH: launchd hands a job the minimal
	//     /usr/bin:/bin:/usr/sbin:/sbin. The loop resolves its DECLARED consumer
	//     with exec.LookPath, so without this every agent whose CLI lives
	//     outside those four dirs resolves to "no consumer" and the loop
	//     degrades to WATCH-ONLY — it heartbeats forever and never dispatches.
	//     That is not a theoretical gap: it is why all 11 codex lanes sat at
	//     wake.mechanism "none" while `codex` (a symlink in ~/.local/bin) was
	//     installed and working. Measured 2026-08-06, codex-pantheon:
	//     `WATCH-ONLY — this lane has NO consumer: consumer command "codex"
	//     not found in PATH`, with 11 items open on the lane.
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, ".sirsi", "logs", "wake-"+slugifyLabelPart(cfg.ID)+".log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>%s</string>
  </dict>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>router</string>
    <string>wake-loop</string>
    <string>%s</string>
  </array>
  <key>KeepAlive</key>
  <true/>
  <key>RunAtLoad</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>60</integer>
  <key>ProcessType</key>
  <string>Background</string>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, escapeXML(label), escapeXML(LaunchAgentPATH(sirsiBin)), escapeXML(sirsiBin),
		escapeXML(cfg.ID), escapeXML(logPath), escapeXML(logPath))
}

// LaunchAgentPATH builds the PATH a Sirsi LaunchAgent must export so that
// exec.LookPath can resolve agent CLIs under launchd's minimal default
// environment (/usr/bin:/bin:/usr/sbin:/sbin).
//
// It leads with the sirsi binary's own directory — agent CLIs are typically
// installed alongside it (`codex` is a symlink in ~/.local/bin next to `sirsi`)
// — then ~/.local/bin, common package-manager prefixes, and the system dirs,
// de-duplicated in order.
//
// This is the single definition; internal/setup's supervisor plist delegates
// here. Two copies of this list is how one surface gets the fix and the other
// keeps the bug, which is exactly what happened: the supervisor plist learned
// the minimal-PATH lesson in its own file and the wake plist never did.
func LaunchAgentPATH(binPath string) string {
	dirs := []string{filepath.Dir(binPath)}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	dirs = append(dirs,
		"/opt/homebrew/bin", "/usr/local/bin",
		"/usr/bin", "/bin", "/usr/sbin", "/sbin",
	)
	seen := make(map[string]bool, len(dirs))
	uniq := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d == "" || d == "." || seen[d] {
			continue
		}
		seen[d] = true
		uniq = append(uniq, d)
	}
	return strings.Join(uniq, ":")
}

// slugifyLabelPart mirrors WakeLaunchAgentLabel's id sanitization for log
// filenames, so the label and its log always correspond.
func slugifyLabelPart(id string) string {
	return labelUnsafe.ReplaceAllString(strings.TrimSpace(id), "-")
}

// escapeXML escapes the five XML predefined entities so a sirsi path or agent id
// containing &, <, >, ", or ' cannot break (or inject into) the plist text.
func escapeXML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(s)
}

// WakeLaunchAgentInstalled reports whether the per-agent pull-loop LaunchAgent
// plist for agentID is present on disk. Exported so thread-scoped callers can show
// install status without re-deriving the label/dir convention.
func WakeLaunchAgentInstalled(agentID string) bool {
	return launchAgentInstalled(WakeLaunchAgentLabel(agentID))
}

// UninstallWakeLaunchAgent removes the per-agent pull-loop LaunchAgent installed by
// InstallWakeLaunchAgent — the clean-off path (A27: install AND clean uninstall).
// It is the exact inverse of Install: it best-effort boots out the running launchd
// job (so the resident wake-loop stops immediately) and then removes the plist.
// Idempotent: removed=false when there is nothing installed, so re-running is a
// no-op. The bootout is best-effort — an unloaded-but-present plist still gets its
// file removed, and a missing plist never errors.
func UninstallWakeLaunchAgent(cfg AgentConfig) (removed bool, path string, err error) {
	label := WakeLaunchAgentLabel(cfg.ID)
	path = filepath.Join(launchAgentsDir(), label+".plist")
	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, path, nil // nothing installed — clean no-op
		}
		return false, path, fmt.Errorf("stat wake LaunchAgent: %w", statErr)
	}
	// Best-effort: stop the resident loop before deleting its definition. Ignore
	// the error — the job may already be unloaded, and file removal is what makes
	// the uninstall durable across a relaunch. Guarded so tests that redirect
	// launchAgentsDirOverride to a temp dir don't touch the real launchd domain.
	if launchAgentsDirOverride == "" {
		target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
		_ = runLaunchctl("bootout", target)
	}
	if rmErr := os.Remove(path); rmErr != nil {
		return false, path, fmt.Errorf("remove wake LaunchAgent plist: %w", rmErr)
	}
	return true, path, nil
}

// InstallWakeLaunchAgent writes (idempotently) the per-agent pull-loop LaunchAgent
// for a worker/headless agent. Returns changed=false when the plist already exists
// with identical content — re-running setup is a no-op (constraint 7 idempotency
// at the install layer; flock/pidfile keep the loop single-instance at runtime).
func InstallWakeLaunchAgent(cfg AgentConfig, sirsiBin string) (changed bool, path string, err error) {
	if strings.TrimSpace(sirsiBin) == "" {
		// Resolution order: the RUNNING binary first (always absolute, always
		// exists — and it is exactly the code the caller just exercised),
		// then PATH. A bare name in ProgramArguments[0] is unspawnable under
		// launchd (its PATH has no ~/.local/bin): the job "runs", dies
		// instantly, and KeepAlive respawns it forever — 15,189 silent
		// restarts per loop before anyone noticed (2026-07-07). PATH lookup
		// alone is also the ADR-023 D3 drift vector. Refuse loudly rather
		// than write an unspawnable plist.
		if self, serr := os.Executable(); serr == nil && strings.Contains(filepath.Base(self), "sirsi") {
			sirsiBin = self
		} else if p, lerr := exec.LookPath("sirsi"); lerr == nil {
			sirsiBin = p
		} else {
			return false, "", fmt.Errorf("cannot resolve an absolute sirsi binary for the LaunchAgent (launchd cannot spawn a bare name): %w", lerr)
		}
	}
	if !filepath.IsAbs(sirsiBin) {
		if abs, aerr := filepath.Abs(sirsiBin); aerr == nil {
			sirsiBin = abs
		}
	}
	label := WakeLaunchAgentLabel(cfg.ID)
	dir := launchAgentsDir()
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return false, "", fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	// launchd creates the log FILES but not their parent directory — a
	// missing ~/.sirsi/logs would turn StandardErrorPath into another
	// silent-death mode.
	if home, herr := os.UserHomeDir(); herr == nil {
		if err = os.MkdirAll(filepath.Join(home, ".sirsi", "logs"), 0o755); err != nil {
			return false, "", fmt.Errorf("create wake log dir: %w", err)
		}
	}
	path = filepath.Join(dir, label+".plist")
	content := wakeLaunchAgentPlist(label, cfg, sirsiBin)
	if existing, rerr := os.ReadFile(path); rerr == nil && string(existing) == content {
		return false, path, nil
	}
	if err = os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, path, fmt.Errorf("write plist: %w", err)
	}
	return true, path, nil
}
