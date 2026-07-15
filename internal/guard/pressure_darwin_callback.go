//go:build darwin && cgo

// Package guard — pressure_darwin_callback.go
//
// The exported C→Go callback for the memory-pressure dispatch source. It lives in
// its own file because a file that uses //export may only have DECLARATIONS in its
// cgo preamble (the include below supplies the DISPATCH_MEMORYPRESSURE_* constants);
// the source's C definition is in pressure_darwin.go.
package guard

/*
#include <dispatch/dispatch.h>
*/
import "C"

// goPressureCallback receives the coalesced event bitmask from the dispatch
// handler (primitives only) and forwards the most-severe level to the fan-out.
// Runs on the dispatch source's serial queue — must stay tiny and non-blocking.
//
//export goPressureCallback
func goPressureCallback(flags C.ulong) {
	f := uint64(flags)
	fanoutPressure(mostSevere(
		f&C.DISPATCH_MEMORYPRESSURE_NORMAL != 0,
		f&C.DISPATCH_MEMORYPRESSURE_WARN != 0,
		f&C.DISPATCH_MEMORYPRESSURE_CRITICAL != 0,
	))
}
