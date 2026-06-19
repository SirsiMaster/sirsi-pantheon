//go:build darwin && cgo

// Package guard — pressure_darwin.go
//
// The macOS memory-pressure watcher: a tiny libdispatch shim over
// DISPATCH_SOURCE_TYPE_MEMORYPRESSURE (codex-pantheon SME verdict, ADR-031-B #4).
// The C side OWNS the serial queue + source and forwards the coalesced event
// bitmask to Go through an exported callback with PRIMITIVE values only — no Go
// pointers are ever retained in C (the cgo gotcha). The handler does only a state
// update + non-blocking enqueue; it never traverses/purges caches.
//
// This file holds the C DEFINITION of the start function, so it must NOT use
// //export (cgo forbids definitions in an //export file's preamble). The exported
// goPressureCallback lives in pressure_darwin_callback.go.
package guard

/*
#include <dispatch/dispatch.h>

// Implemented in Go (pressure_darwin_callback.go) via //export.
extern void goPressureCallback(unsigned long flags);

static dispatch_source_t sirsi_pressure_source = NULL;

// sirsi_pressure_start creates (once) the memory-pressure dispatch source on a
// dedicated serial queue, registers a handler that forwards the coalesced event
// bitmask to Go, and resumes it. Idempotent: a second call is a no-op.
static void sirsi_pressure_start(void) {
	if (sirsi_pressure_source != NULL) {
		return;
	}
	dispatch_queue_t q = dispatch_queue_create("ai.sirsi.hapi.pressure", DISPATCH_QUEUE_SERIAL);
	unsigned long mask = DISPATCH_MEMORYPRESSURE_NORMAL | DISPATCH_MEMORYPRESSURE_WARN | DISPATCH_MEMORYPRESSURE_CRITICAL;
	dispatch_source_t src = dispatch_source_create(DISPATCH_SOURCE_TYPE_MEMORYPRESSURE, 0, mask, q);
	if (src == NULL) {
		return;
	}
	dispatch_source_set_event_handler(src, ^{
		unsigned long flags = dispatch_source_get_data(src);
		goPressureCallback(flags);
	});
	sirsi_pressure_source = src;
	dispatch_resume(src); // must follow handler registration
}
*/
import "C"

import (
	"context"
	"sync"
)

// darwinPressureWatcher wraps the process-wide libdispatch source. The source is
// started once; Subscribe just registers a fan-out channel.
type darwinPressureWatcher struct{}

// NewPressureWatcher returns the libdispatch-backed watcher on darwin+cgo.
func NewPressureWatcher() PressureWatcher { return darwinPressureWatcher{} }

var (
	pressureSubMu sync.Mutex
	pressureSubs  []chan<- PressureEvent
	pressureStart sync.Once
)

// Subscribe registers ch and starts the dispatch source on first use. The source
// stays resident for the life of the process (cheap); only the channel
// registration is torn down when ctx ends.
func (darwinPressureWatcher) Subscribe(ctx context.Context, ch chan<- PressureEvent) error {
	pressureSubMu.Lock()
	pressureSubs = append(pressureSubs, ch)
	pressureSubMu.Unlock()

	pressureStart.Do(func() { C.sirsi_pressure_start() })

	go func() {
		<-ctx.Done()
		pressureSubMu.Lock()
		for i, c := range pressureSubs {
			if c == ch {
				pressureSubs = append(pressureSubs[:i], pressureSubs[i+1:]...)
				break
			}
		}
		pressureSubMu.Unlock()
	}()
	return nil
}

// fanoutPressure records the level and non-blocking-sends to every subscriber.
// Called from the exported callback (the dispatch handler thread).
func fanoutPressure(level PressureLevel) {
	if level == PressureUnknown {
		return
	}
	recordPressure(level) // tiny state update only — no cache traversal in the handler
	pressureSubMu.Lock()
	subs := pressureSubs
	pressureSubMu.Unlock()
	for _, c := range subs {
		select {
		case c <- PressureEvent{Level: level}:
		default: // a slow consumer never stalls the kernel handler
		}
	}
}
