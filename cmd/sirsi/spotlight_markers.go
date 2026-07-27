package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Durable half of the Spotlight remediation.
//
// `spotlight-exclude` detects the storm and guides the user to the Privacy
// pane, because that list is SIP-protected and genuinely user-only. But the
// Privacy pane is not the only lever, and it is not the durable one: a
// `.metadata_never_index` marker at the root of a directory tells Spotlight to
// skip that tree, needs no privilege, is version-stable, survives reboots, and
// is reversed by deleting one file.
//
// It is also the ONLY indexing lever available at user privilege. Measured on
// this class of host: `mds` runs as root and `mds_stores` as `_mds_stores`, so
// both `renice` and `taskpolicy -b` are refused with EPERM. Bounding the
// indexer's CPU/IO — the thing that would actually cap indexing pressure —
// requires root and therefore an owner action, not an agent one.
//
// The churn set below is deliberately narrow: caches, module stores, transient
// worktrees and agent transcript trees. Source trees are NOT marked — code
// search is the one Spotlight result a developer actually wants.

// markerFile is the sentinel macOS honors to skip a directory tree.
const markerFile = ".metadata_never_index"

// indexChurnPaths returns the fixed, high-write, zero-search-value trees.
// Relative to home so the set is portable across machines.
func indexChurnPaths(home string) []string {
	return []string{
		filepath.Join(home, "Library", "Caches"),
		filepath.Join(home, "go", "pkg", "mod"),
		filepath.Join(home, ".claude", "projects"),
		filepath.Join(home, "Library", "Application Support", "Claude"),
		"/private/tmp",
	}
}

// buildDirNames are per-project output trees: regenerated constantly, never
// worth searching, and the largest single source of reindex churn in a dev tree.
var buildDirNames = map[string]bool{
	"node_modules": true,
	".build":       true,
	"target":       true,
	"dist":         true,
	".next":        true,
}

// discoverBuildDirs finds build-output trees under root, bounded to maxDepth so
// a deep monorepo cannot turn this into a full filesystem walk. It does not
// descend INTO a matched directory — marking node_modules covers everything
// beneath it, and walking a 40k-file tree to find nested copies is waste.
func discoverBuildDirs(root string, maxDepth int) []string {
	var found []string
	rootDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil //nolint:nilerr // an unreadable dir is skipped, not fatal
		}
		if strings.Count(path, string(os.PathSeparator))-rootDepth > maxDepth {
			return filepath.SkipDir
		}
		if buildDirNames[d.Name()] {
			found = append(found, path)
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

// markerState is what one candidate directory looks like right now.
type markerState struct {
	Path     string
	Exists   bool // the directory exists
	Marked   bool // already carries the marker
	WroteNow bool
}

// planIndexMarkers resolves every candidate to its current state. Pure
// inspection — nothing is written here, so the preview and the apply share one
// code path and cannot disagree about what would change (Rule A1).
func planIndexMarkers(home, devDir string) []markerState {
	cands := indexChurnPaths(home)
	cands = append(cands, discoverBuildDirs(devDir, 3)...)

	seen := map[string]bool{}
	out := make([]markerState, 0, len(cands))
	for _, p := range cands {
		if seen[p] {
			continue
		}
		seen[p] = true
		st := markerState{Path: p}
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			st.Exists = true
			if _, err := os.Stat(filepath.Join(p, markerFile)); err == nil {
				st.Marked = true
			}
		}
		out = append(out, st)
	}
	return out
}

// applyIndexMarkers writes the marker into every unmarked, existing candidate.
// Idempotent: an already-marked tree is left alone and reported as such, so a
// second run is a no-op rather than a lie about having changed something.
func applyIndexMarkers(plan []markerState) ([]markerState, error) {
	var firstErr error
	for i := range plan {
		if !plan[i].Exists || plan[i].Marked {
			continue
		}
		f, err := os.OpenFile(filepath.Join(plan[i].Path, markerFile),
			os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("mark %s: %w", plan[i].Path, err)
			}
			continue
		}
		_ = f.Close()
		plan[i].Marked = true
		plan[i].WroteNow = true
	}
	return plan, firstErr
}

// rootOnlyLevers are the indexing controls that exist but need privileges this
// process does not have. Surfaced verbatim so the owner can run them, rather
// than silently omitted — an incomplete remedy presented as complete is the
// failure mode this whole subsystem keeps hitting.
func rootOnlyLevers() []string {
	return []string{
		"sudo mdutil -i off /                     # stop indexing the boot volume entirely",
		"sudo mdutil -E /                         # erase + rebuild the index (expect a reindex storm first)",
		"sudo renice +20 -p $(pgrep -x mds)       # deprioritise the indexer (resets on respawn)",
	}
}
