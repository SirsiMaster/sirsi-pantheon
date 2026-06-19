//go:build !darwin || !cgo

// Package guard — pressure_stub.go
//
// Non-darwin / no-cgo fallback: there is no unprivileged kernel pressure LEVEL to
// subscribe to, so the watcher is a no-op. It never emits events, so NodeCapacity
// falls back to the bootstrap free-% snapshot — never a fabricated threshold
// (ADR-031-B #4). The cross-platform agent binary (CGO_ENABLED=0, Rule A3) takes
// this path.
package guard

import "context"

// NewPressureWatcher returns a no-op watcher on builds without the libdispatch
// source available.
func NewPressureWatcher() PressureWatcher { return noopPressureWatcher{} }

type noopPressureWatcher struct{}

func (noopPressureWatcher) Subscribe(context.Context, chan<- PressureEvent) error { return nil }
