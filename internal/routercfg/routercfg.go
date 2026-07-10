// Package routercfg holds the small, dependency-free fabric switches read by
// BOTH the dispatch facade (internal/dispatch) and the watcher-spec generator
// (internal/router) without coupling them. Keeping it here avoids an import
// cycle and keeps the cutover a single, discoverable flag rather than a value
// threaded through a dozen call sites.
package routercfg

import (
	"os"
	"path/filepath"
)

// StoreWakeEnv is the environment switch that promotes the durable store to the
// SOLE dispatch + wake authority (ADR-036 cutover / ADR-037):
//
//   - consumers wake by blocking on `sirsi router wait <agent>` (the store's
//     per-agent FIFO), NOT by watching the items/ directory;
//   - Send stops writing the items/<id>.md audit view (the store row is the
//     record); Show/Close/Inbox read from the store when no file exists.
//
// Default (unset + no marker) is the legacy dual-write + items/-directory watch,
// so a binary that ships with the flag off behaves exactly as before. The flip
// is a deliberate deploy step performed AFTER the `router wait` verb is in the
// running binary and live watchers are re-armed (ADR-036: "an owner-visible
// decision, not a side effect of a build PR") — shipped as `sirsi router cutover
// enable`, which drops the marker below and re-arms the durable watchers.
const StoreWakeEnv = "SIRSI_ROUTER_STORE_WAKE"

// markerRel is the persistent cutover marker, relative to the user's home. Its
// existence promotes the store to sole authority for every process on the host
// (launchd watchers + CLI + sessions) — so the flip survives restarts without
// exporting an env var into every surface. The env var still wins, so tests and
// one-off overrides force a value regardless of the marker.
var markerRel = filepath.Join(".sirsi", "store-wake")

// MarkerPath returns the absolute cutover-marker path, or "" if $HOME is unknown.
func MarkerPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, markerRel)
}

// StoreWake reports whether the store-as-sole-authority cutover is active.
// Precedence: an explicit env value ("1" on, "0"/anything-else off) always wins;
// with the env unset, the persistent marker decides.
func StoreWake() bool {
	if v, ok := os.LookupEnv(StoreWakeEnv); ok {
		return v == "1"
	}
	if p := MarkerPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}
