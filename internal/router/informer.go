// Package router — informer.go
//
// ADR-065 (Router-Owned Watcher/Informer) Decision 3a: the environment's
// delivery quirks live in one table, in the router — not copied into every
// lane's own config. This file is that table, for Phase 1 (single host):
// given a registered agent's declared Type and Wake.SessionMode, it returns
// the one delivery strategy the informer (task 5) will use to reach that
// lane. Task 3 only; no subscriber loop, no push, no admission logic lives
// here yet.
package router

import (
	"fmt"
	"strings"
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
