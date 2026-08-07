// Package router — spotlightmarkers.go
//
// Wires the durable Spotlight-marker mechanism (internal/guard) into the
// resident supervisor as a duty, closing the gap the 2026-08-07 reopened
// directive named: the `.metadata_never_index` marker mechanism existed and
// had been applied by hand once, but nothing kept it current — a new build
// dir appearing after that one run stayed unmarked until someone remembered
// to re-run the CLI. Injected from cmd/sirsi (Rule A16): internal/guard
// depends on this package already (thread_registry_split.go), so this
// package cannot import internal/guard without a cycle — the real
// implementation is wired in from main instead, same seam shape as
// gemma-liveness and auto-heal.
package router

import "sync"

var (
	spotlightMarkersMu sync.RWMutex
	spotlightMarkersFn = func(_, _ string) error { return nil }
)

func getSpotlightMarkersFn() func(string, string) error {
	spotlightMarkersMu.RLock()
	defer spotlightMarkersMu.RUnlock()
	return spotlightMarkersFn
}

// SetSpotlightMarkersFn installs the real marker-apply pass the supervisor's
// spotlight-markers duty runs (wired from cmd/sirsi at init).
func SetSpotlightMarkersFn(fn func(routerRoot, repoRoot string) error) {
	spotlightMarkersMu.Lock()
	defer spotlightMarkersMu.Unlock()
	if fn != nil {
		spotlightMarkersFn = fn
	}
}
