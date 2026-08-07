package guard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

// routerRootForGuard resolves the LIVE router root through the same helper the
// router itself uses, so this check cannot end up inspecting a per-agent
// worktree's stale git-snapshot copy of .agents/idea-router (ADR-029). Returns
// "" when no router is reachable — the caller reports that as unknown, never OK.
// Resolved WITHOUT importing internal/router: guard -> router -> liveness ->
// guard is an import cycle. This is the same walk-up router.FindRepoRoot uses,
// minus the git-common-dir preference — a read-only presence check does not need
// worktree-exact resolution, and if the walk-up lands in a per-agent worktree
// copy the finding is still true of THAT tree.
func routerRootForGuard() string {
	dir, err := os.Getwd()
	if err == nil {
		for {
			candidate := filepath.Join(dir, ".agents", "idea-router")
			if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
				return candidate
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	// Fallback: the pinned repo the router CLI uses when cwd is unrelated.
	if home, herr := os.UserHomeDir(); herr == nil && home != "" {
		candidate := filepath.Join(home, "Development", "sirsi-pantheon", ".agents", "idea-router")
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

// Thread-registry split detection (ADR-036/037 cutover integrity).
//
// After the store-wake cutover the SQLite store is the SOLE thread authority:
// SaveThreadRegistry returns before the legacy JSON write, and
// LoadThreadRegistry reads the store. The legacy .agents/idea-router/threads.json
// survives only as a seed for an EMPTY store.
//
// That leaves a silent failure mode. If anything writes the legacy file while
// the store is populated, the host carries TWO thread registries that no code
// path reconciles, and every surface reads whichever one its call path happens
// to reach. Observed 2026-08-07: the store held 182 thread records and the
// legacy file held 137 — with ZERO ids in common, and the file carrying records
// written that same morning. Two disjoint universes, neither one wrong on its
// own terms, and nothing anywhere reporting the divergence.
//
// The consequence is not cosmetic. `sirsi thread list` reads the store while a
// reader that falls through to the file sees a different fleet — which is how a
// lane can read "no watcher" on one surface and "idle, heartbeating" on another
// at the same instant.
//
// This check exists because that divergence is invisible by construction: both
// files parse, both look internally consistent, and neither is stale in a way a
// timestamp reveals.

// legacyThreadsRel is the pre-cutover registry, relative to the router root.
const legacyThreadsRel = "threads.json"

// staleLegacyAfter is how long a legacy file may lag before its mere presence is
// worth reporting at all. A file touched seconds ago during a legitimate
// one-shot seed is not yet evidence of a second writer.
const staleLegacyAfter = 10 * time.Minute

// legacyThreadCount reports how many thread records the legacy file holds.
// Returns (0, false) when the file is absent — the healthy post-cutover shape.
func legacyThreadCount(path string) (int, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	var reg struct {
		Threads map[string]json.RawMessage `json:"threads"`
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return 0, true, fmt.Errorf("parse legacy threads.json: %w", err)
	}
	return len(reg.Threads), true, nil
}

// checkThreadRegistrySplit reports whether a legacy thread registry is present
// alongside the cutover store.
//
// Deliberately does NOT open the store. The guard package must stay free of a
// routerstore dependency, and presence of the legacy file under an active
// cutover is already the actionable signal: post-cutover nothing should be
// writing it. Counting store rows would sharpen the message but not the verdict.
func checkThreadRegistrySplit(p platform.Platform, report *DoctorReport) {
	const name = "Thread Registry Split"

	// Only meaningful once the store is the declared authority. Before the
	// cutover the legacy file IS the registry, and flagging it would be noise.
	if !routercfg.StoreWake() {
		report.Findings = append(report.Findings, DiagnosticFinding{
			Check:    name,
			Severity: SeverityOK,
			Message:  "Store-wake cutover not active — the legacy thread registry is the authority here, as designed",
		})
		return
	}

	root := routerRootForGuard()
	if root == "" {
		// Unknown location is UNKNOWN, never healthy: reporting OK here would
		// assert an absence this check never observed.
		report.Findings = append(report.Findings, DiagnosticFinding{
			Check:    name,
			Severity: SeverityWarn,
			Message:  "Cannot locate the router root — thread-registry split cannot be determined",
		})
		return
	}

	path := filepath.Join(root, legacyThreadsRel)
	count, present, err := legacyThreadCount(path)
	if err != nil {
		report.Findings = append(report.Findings, DiagnosticFinding{
			Check:    name,
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("Legacy thread registry present at %s but unreadable: %v — cannot confirm whether a second registry is live", path, err),
		})
		return
	}
	if !present {
		report.Findings = append(report.Findings, DiagnosticFinding{
			Check:    name,
			Severity: SeverityOK,
			Message:  "No legacy thread registry — the store is the only thread authority",
		})
		return
	}

	info, statErr := os.Stat(path)
	age := time.Duration(0)
	if statErr == nil {
		age = time.Since(info.ModTime())
	}

	// A recently-written legacy file under an active cutover means something is
	// STILL WRITING IT. That is the live-second-registry case and the reason
	// this check exists.
	if age < staleLegacyAfter {
		report.Findings = append(report.Findings, DiagnosticFinding{
			Check:    name,
			Severity: SeverityCritical,
			Message: fmt.Sprintf(
				"SECOND thread registry is LIVE: %s holds %d records and was written %s ago while the store-wake cutover is active. Two registries no code path reconciles — surfaces will disagree about which threads exist.",
				path, count, age.Round(time.Second)),
		})
		return
	}

	report.Findings = append(report.Findings, DiagnosticFinding{
		Check:    name,
		Severity: SeverityWarn,
		Message: fmt.Sprintf(
			"Legacy thread registry left behind: %s holds %d records, last written %s ago. The store is authoritative; this file is a stale second source that readers falling through to it will misread. Compare the id sets before removing it, then archive rather than delete: `mv %s %s.retired-$(date +%%Y%%m%%d)`.",
			path, count, age.Round(time.Minute), path, path),
	})
}
