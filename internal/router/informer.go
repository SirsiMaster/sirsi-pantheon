// Package router — informer.go
//
// ADR-065 (Router-Owned Watcher/Informer) Decision 3a: the environment's
// delivery quirks live in one table, in the router — not copied into every
// lane's own config. This file is that table (task 3), plus the single-host
// subscriber loop it feeds (task 5): one goroutine per lane, blocked on the
// store's ListenNotify/Wait instead of a per-lane ticker, dispatching through
// the exact same admission boundary (admission.go, task 4) RunWakeLoop uses —
// coexistence, not replacement (sprint /goal item 2 and item 6).
package router

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// Delivery strategies, per ADR-065 Decision 3a's table. These name HOW the
// informer reaches a lane, not WHERE (that is AgentConfig.Delivery, task 2).
const (
	StrategyFilesystemPush = "filesystem-push" // codex: no DNS, filesystem-only reach (rs-22a/e)
	StrategySessionMessage = "session-message" // claude, interactive: existing session only, never blind-spawned
	StrategyCLISpawn       = "cli-spawn"       // claude, headless: process wake permitted
	StrategyResidentNotify = "resident-notify" // gemma/qwen: resident process, never a second spawn
)

// strategyAllowedTarget names the one AgentConfig.Delivery.Target value a
// strategy may be paired with. Strategies absent from this map accept no
// explicit Target at all — they have no address to narrow (session-message,
// cli-spawn, resident-notify all reach the lane by mechanism, not address).
var strategyAllowedTarget = map[string]string{
	StrategyFilesystemPush: "spool-dir",
}

// DeliveryStrategyFor returns the ADR-065 Decision 3a delivery strategy for
// cfg, derived from its declared Type and Wake.SessionMode via the single
// lookup table below — never re-derived ad hoc per lane. Reuses the existing
// isInteractiveSpawn helper (wake.go) for the claude headless/interactive
// split so the legacy-default fallback it already encodes is not duplicated
// (Rule 0).
//
// If cfg.Delivery declares an explicit Target, it must narrow — never
// widen — the type-mandated strategy (Decision 3a: "a codex lane cannot opt
// into HTTP"). A Target outside the strategy's allowed set is refused.
func DeliveryStrategyFor(cfg AgentConfig) (string, error) {
	var mandated string
	switch strings.TrimSpace(cfg.Type) {
	case "codex":
		mandated = StrategyFilesystemPush
	case "claude":
		if isInteractiveSpawn(cfg) {
			mandated = StrategySessionMessage
		} else {
			mandated = StrategyCLISpawn
		}
	case "gemma", "qwen":
		mandated = StrategyResidentNotify
	default:
		return "", fmt.Errorf("router: no ADR-065 delivery strategy for agent %q (type %q) — add a row to the Decision 3a table before registering this type", cfg.ID, cfg.Type)
	}

	if cfg.Delivery == nil || strings.TrimSpace(cfg.Delivery.Target) == "" {
		return mandated, nil
	}
	target := strings.TrimSpace(cfg.Delivery.Target)
	if allowed, ok := strategyAllowedTarget[mandated]; !ok || target != allowed {
		return "", fmt.Errorf("router: agent %q (type %q) declared delivery target %q, which widens its mandated strategy %q — a lane may narrow but never widen its type's strategy (ADR-065 Decision 3a)", cfg.ID, cfg.Type, target, mandated)
	}
	return mandated, nil
}

// RunInformer is the ADR-065 single-host subscriber (task 5): one lane
// goroutine per registered, dispatchable agent, each blocked on
// store.ListenNotify instead of polling a ticker. It runs ALONGSIDE every
// lane's existing RunWakeLoop, never instead of it — Phase 1 proves
// coexistence before anything is asked to depend on the informer exclusively
// (sprint /goal item 2; no retirement code in this sprint).
//
// A lane is skipped here, left entirely to its own RunWakeLoop/watch-only
// path, when it has no dispatchable consumer (resident, or none declared) or
// when its mandated strategy is session-message — wake.go design constraint 3
// (interactive claude is never blind-spawned) holds for the informer exactly
// as it holds for the per-lane loop; session-message delivery is built in
// task 6, not here.
func RunInformer(ctx context.Context, routerRoot string) error {
	reg, err := LoadRegistry(routerRoot)
	if err != nil {
		return fmt.Errorf("router: informer: load registry: %w", err)
	}
	store, err := routerstore.Resolve()
	if err != nil {
		return fmt.Errorf("router: informer: resolve store: %w", err)
	}

	var wg sync.WaitGroup
	for id, cfg := range reg.Agents {
		rc, why := ResolveConsumer(cfg, routerRoot)
		if rc == nil || rc.Resident {
			log.Printf("informer %s: no dispatchable consumer, not subscribing: %s", id, why)
			continue
		}
		strategy, serr := DeliveryStrategyFor(cfg)
		if serr != nil {
			log.Printf("informer %s: %v", id, serr)
			continue
		}
		if strategy == StrategySessionMessage || strategy == StrategyFilesystemPush {
			// session-message: never blind-spawn an interactive lane (wake.go
			// design constraint 3). filesystem-push: the actual spool-dir
			// transport is ADR-065 Decision 4, explicitly deferred past Phase 1
			// (SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md "Out of scope") — until
			// that lands, admitting a generic process-spawn for a sandboxed
			// codex lane would be wrong, not merely unimplemented, so this
			// strategy is held here rather than silently falling through to
			// the cli-spawn dispatch path below.
			log.Printf("informer %s: strategy %q has no admitted transport yet, not subscribing", id, strategy)
			continue
		}
		events, lerr := store.ListenNotify(ctx, id)
		if lerr != nil {
			log.Printf("informer %s: ListenNotify: %v", id, lerr)
			continue
		}
		wg.Add(1)
		go func(agentID string, consumer *ResolvedConsumer, events <-chan struct{}) {
			defer wg.Done()
			runInformerLane(ctx, routerRoot, agentID, consumer, events)
		}(id, rc, events)
	}
	wg.Wait()
	return nil
}

// runInformerLane blocks on events (one store notification per wake poke)
// and, on each, attempts admission through the SAME shared primitives
// RunWakeLoop's own dispatch call site checks (/goal item 6) before calling
// admitConsumer — a quarantined or overloaded host, an open measurement
// window, a live attended session, a full host consumer slot table, or an
// hourly spawn ceiling already tripped holds the informer's hand exactly as
// it holds the per-lane loop's (codex-pantheon review 20261008-174407,
// finding 1: the informer must not have its own, narrower admission gate).
// admitConsumer's per-agent flock (task 4) is what makes a simultaneous edge
// seen by both loops produce exactly one spawn — caller identity is
// irrelevant to that boundary, which is the point.
func runInformerLane(ctx context.Context, routerRoot, agentID string, consumer *ResolvedConsumer, events <-chan struct{}) {
	for range events {
		if ctx.Err() != nil {
			return
		}
		if fabricDispatchQuarantined(agentID, -1) || fabricDispatchOverloaded(agentID, -1) ||
			measurementWindowOpen(agentID, -1) || attendedSessionOwnsInbox(routerRoot, agentID, -1) ||
			hostConsumerSlotsFull(routerRoot, agentID, -1) {
			continue
		}
		if over, n := spawnCeilingReached(agentID, time.Now()); over {
			log.Printf("informer %s: SPAWN CEILING reached (%d dispatches in the last hour) — refusing to dispatch", agentID, n)
			continue
		}

		// Borrow the lane's own RunWakeLoop worker-thread identity rather than
		// minting one (finding 2: a dispatch with no thread binding has no
		// real worker identity). Read-only — the informer is a different
		// process from the loop that owns this thread and must never
		// re-point its PID (that is adoptWorkerThread's job, called only by
		// the loop itself). No thread yet (no RunWakeLoop running this lane)
		// means no identity to hand the consumer, so the informer declines
		// rather than inventing one.
		thr := currentWorkerThread(routerRoot, agentID)
		if thr == nil {
			log.Printf("informer %s: no live worker thread registered for this lane, declining to dispatch", agentID)
			continue
		}
		if thr.Lane != nil && thr.Lane.Hold == HoldQuarantine {
			log.Printf("informer %s: lane quarantined by its own wake loop, holding dispatch too", agentID)
			continue
		}
		bound := bindConsumerThread(consumer, thr.ThreadID)

		run, adopted, derr := admitConsumer(routerRoot, agentID, bound)
		if derr != nil {
			log.Printf("informer %s: admission failed: %v", agentID, derr)
			continue
		}
		if !adopted {
			if err := recordDispatch(agentID, time.Now()); err != nil {
				log.Printf("informer %s: WARNING could not record dispatch in the rate ledger: %v", agentID, err)
			}
			log.Printf("informer %s: dispatched consumer pid %d", agentID, run.pid)
		}
		markDeliveryAttempted(routerRoot, agentID)
	}
}

// currentWorkerThread returns the lane's own live RunWakeLoop worker thread
// for agentID on this machine, without mutating it — read-only, unlike
// adoptWorkerThread which re-points PID at the caller. The informer borrows
// this identity rather than minting a second worker thread for the same
// lane, which would fragment "who is the worker" across two live records.
func currentWorkerThread(routerRoot, agentID string) *Thread {
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil
	}
	return findWorkerThread(reg, agentID, MachineID())
}

// markDeliveryAttempted is the ADR-065 Decision 2 correction (task 6): the
// informer's push records a DELIVERY-ATTEMPT marker only, reusing the exact
// wake_status/wake_attempted_at frontmatter WakePass already writes
// (dispatch.Facade.SetWake — no new ack primitive invented, per sprint
// scope). It never touches acked_at/read_at; the lane's own `router
// acknowledge` call (SetAckedAt) is the ONLY writer of that field. A marking
// failure is logged and swallowed — the dispatch already happened, and losing
// the attempt record must not be treated as losing the dispatch.
func markDeliveryAttempted(routerRoot, agentID string) {
	items, err := OpenItems(routerRoot, agentID)
	if err != nil {
		log.Printf("informer %s: delivery-attempt mark: inbox read failed: %v", agentID, err)
		return
	}
	if len(items) == 0 {
		return
	}
	f, ferr := dispatch.OpenRoot(routerRoot)
	if ferr != nil {
		log.Printf("informer %s: delivery-attempt mark: open store: %v", agentID, ferr)
		return
	}
	defer func() { _ = f.Close() }()
	ann := work.WakeAnnotation{Status: WakeStatusAttempted, AttemptedAt: time.Now().UTC().Format(time.RFC3339), Adapter: "informer"}
	for _, item := range items {
		if werr := f.SetWake(item.ID, ann); werr != nil {
			log.Printf("informer %s: delivery-attempt mark failed for %s: %v", agentID, item.ID, werr)
		}
	}
}
