// Package guard — pressure.go
//
// Memory-pressure watcher (ADR-031-B #4, codex-pantheon SME). macOS computes
// memory pressure RELATIVE to the machine, so subscribing to the kernel's level is
// node-proportional "for free" — it becomes NodeCapacity's AUTHORITATIVE pressure
// source, replacing the bootstrap free-% seed (which stays only as the fallback
// when no watcher has reported).
//
// Signal vs. policy (the architectural contract): the watcher does ONLY a tiny
// state update — it records the last-observed kernel level. ALL policy (suspend/
// kill within the consent invariant, dynamic cap, concurrency) runs in Hapi's
// existing bounded loop, which combines (kernel level) + (governed RSS) +
// (current concurrency) + (NodeCapacity). The hard MLX cap remains the instant
// backstop for a fast balloon between ticks.
//
// Cross-process bridge: dispatch pressure is TRANSITION-based and per-process. The
// Hapi daemon runs the watcher; one-shot consumers (`sirsi gemma serve`'s pre-
// launch gate, `sirsi hapi status`, the menubar) live in other processes, so the
// daemon re-stamps a tiny cache file every tick. Freshness there means "a watcher
// is alive and this is the current level" — not "pressure just changed" — so a
// stable NORMAL never looks stale. A missing/old cache → no live source → the
// reader falls back to the bootstrap snapshot.
package guard

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// PressureEvent is one observed kernel memory-pressure transition.
type PressureEvent struct {
	Level PressureLevel `json:"level"`
}

// PressureWatcher subscribes to kernel memory-pressure transitions. The
// darwin+cgo implementation wraps DISPATCH_SOURCE_TYPE_MEMORYPRESSURE; every other
// build is a no-op (the reader falls back to the bootstrap snapshot — never a
// fabricated threshold, Rule A3 keeps the agent binary CGO_ENABLED=0).
type PressureWatcher interface {
	// Subscribe registers ch to receive transitions until ctx is canceled. The
	// handler enqueues non-blocking, so a slow consumer never stalls the kernel
	// source. Implementations must NOT assume an initial event is delivered.
	Subscribe(ctx context.Context, ch chan<- PressureEvent) error
}

// ── process-local last-observed level (set by the watcher's handler) ──────────
var (
	lastPressureLevel atomic.Int32 // PressureLevel; 0 == PressureUnknown
	lastPressureAuth  atomic.Bool  // a kernel-dispatch level has been observed in THIS process
)

// observedPressure returns the kernel level observed in THIS process, if any.
// (The level is recorded by the dispatch handler in fanoutPressure — colocated
// with its only caller in the darwin+cgo build so the !cgo build doesn't see a
// dead writer.)
func observedPressure() (PressureLevel, bool) {
	if !lastPressureAuth.Load() {
		return PressureUnknown, false
	}
	return PressureLevel(lastPressureLevel.Load()), true
}

// mostSevere picks the highest pressure level present in a coalesced dispatch
// bitmask (events coalesce while the handler is pending — pick the worst flag).
// Pure, so the severity logic is unit-tested without cgo.
func mostSevere(normal, warn, critical bool) PressureLevel {
	switch {
	case critical:
		return PressureCritical
	case warn:
		return PressureWarn
	case normal:
		return PressureNormal
	default:
		return PressureUnknown
	}
}

// ── cross-process cache (the daemon stamps it; one-shot readers consume it) ────

// pressureCacheStale bounds how long a stamp is trusted. The daemon re-stamps on
// every tick (~3s), so ~10 ticks of silence means no live watcher → fall back.
const pressureCacheStale = 30 * time.Second

type pressureCacheEntry struct {
	Level  PressureLevel `json:"level"`
	Source string        `json:"source"`
	Unix   int64         `json:"unix"`
}

func pressureCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sirsi", "pressure.json")
}

// writePressureCache stamps the current level for cross-process readers. Fail-soft:
// any error simply means readers fall back to the bootstrap snapshot.
func writePressureCache(l PressureLevel) {
	e := pressureCacheEntry{Level: l, Source: "kernel-dispatch", Unix: time.Now().Unix()}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(pressureCachePath()), 0o755)
	_ = os.WriteFile(pressureCachePath(), b, 0o644)
}

// readPressureCache returns the cached level + source if a live daemon stamped it
// within pressureCacheStale; otherwise (Unknown, "", false).
func readPressureCache() (PressureLevel, string, bool) {
	b, err := os.ReadFile(pressureCachePath())
	if err != nil {
		return PressureUnknown, "", false
	}
	var e pressureCacheEntry
	if json.Unmarshal(b, &e) != nil {
		return PressureUnknown, "", false
	}
	if time.Since(time.Unix(e.Unix, 0)) > pressureCacheStale {
		return PressureUnknown, "", false
	}
	return e.Level, e.Source, true
}

// effectivePressure resolves the authoritative pressure for a NodeCapacity sample,
// preferring (1) the kernel level observed in THIS process (the Hapi daemon),
// (2) a fresh cross-process cache stamped by the daemon's watcher, then
// (3) the bootstrap free-% snapshot, and (4) Unknown when there is nothing to go
// on. This is what REPLACES the hardcoded free-% seed as the primary source: the
// seed is now only the no-watcher fallback (ADR-031-B #4).
func effectivePressure(freeRAM, totalRAM int64) (PressureLevel, string) {
	if l, ok := observedPressure(); ok {
		return l, "kernel-dispatch"
	}
	if l, src, ok := readPressureCache(); ok {
		return l, src
	}
	if totalRAM > 0 {
		return bootstrapPressure(float64(freeRAM) / float64(totalRAM) * 100), "bootstrap-snapshot"
	}
	return PressureUnknown, "unknown"
}

// EffectivePressure exposes the resolved pressure level + source for callers
// that already hold a memory sample (the `sirsi vitals` fast path, gap V1) —
// the same resolution SampleNodeCapacity uses, without re-running the full
// hardware/agent sampling it performs.
func EffectivePressure(freeRAM, totalRAM int64) (PressureLevel, string) {
	return effectivePressure(freeRAM, totalRAM)
}
