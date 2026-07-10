// Package routercfg holds the small, dependency-free fabric switches read by
// BOTH the dispatch facade (internal/dispatch) and the watcher-spec generator
// (internal/router) without coupling them. Keeping it here avoids an import
// cycle and keeps the cutover a single, discoverable flag rather than a value
// threaded through a dozen call sites.
package routercfg

import "os"

// StoreWakeEnv is the environment switch that promotes the durable store to the
// SOLE dispatch + wake authority (ADR-036 cutover / ADR-037):
//
//   - consumers wake by blocking on `sirsi router wait <agent>` (the store's
//     per-agent FIFO), NOT by watching the items/ directory;
//   - Send stops writing the items/<id>.md audit view (the store row is the
//     record); Show/Close/Inbox read from the store when no file exists.
//
// Default (unset/"0") is the legacy dual-write + items/-directory watch, so a
// binary that ships with the flag off behaves exactly as before. The flip is a
// deliberate deploy step performed AFTER the `router wait` verb is in the
// running binary and live watchers are re-armed (ADR-036: "an owner-visible
// decision, not a side effect of a build PR").
const StoreWakeEnv = "SIRSI_ROUTER_STORE_WAKE"

// StoreWake reports whether the store-as-sole-authority cutover is active.
func StoreWake() bool {
	return os.Getenv(StoreWakeEnv) == "1"
}
