// Package router — prune.go
//
// Retention sweep for the router fabric's byproduct artifacts. The router core
// is daemonless by design, so nothing ever owned the lifecycle of the logs,
// incident dumps, and completed-work records it leaves behind — they grow
// without bound (a 54 MB triage log, a 17 MB wake log, a 45 MB one-time
// quarantine dump). Prune applies two orthogonal caps:
//
//   - Age cap: anything older than the cutoff (default 90 days) is removed,
//     except closed router items, whose payloads are compacted into tombstones.
//   - Size cap: active append-only logs are tail-capped so a single recent
//     file cannot balloon indefinitely.
//
// Every destructive action is gated behind a dry-run that reports the exact
// bytes it would reclaim (PANTHEON_RULES Rule A1 — no deletion without a
// dry-run path). Open work items are never candidates, and closed work item IDs
// are never removed — that is enforced one layer down in work.PruneItems.
package router

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// DefaultRetentionDays is the owner-set retention window: logging beyond this
// is wasteful (owner directive, 2026-07-10).
const DefaultRetentionDays = 90

// maxLogTailBytes bounds a single active append-only log. When a log younger
// than the cutoff exceeds this, its head is dropped and only the most recent
// tail is kept — recent lines carry the diagnostic value, old ones do not.
const maxLogTailBytes = 4 << 20 // 4 MiB

// snapshotCapBytes bounds a regenerated point-in-time snapshot (processes.json).
// Above this it is removed; its writer recreates a fresh, small one on the next
// tick, so removal is non-destructive.
const snapshotCapBytes = 8 << 20 // 8 MiB

// PruneAction is one artifact acted on (or that would be, under dry-run).
type PruneAction struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"` // item | quarantine | log-deleted | log-capped | workqueue | snapshot
	Before int64  `json:"before_bytes"`
	After  int64  `json:"after_bytes"`
}

// Reclaimed is the bytes freed by this action.
func (a PruneAction) Reclaimed() int64 { return a.Before - a.After }

// PruneReport aggregates every action from a sweep.
type PruneReport struct {
	Cutoff  time.Time     `json:"cutoff"`
	DryRun  bool          `json:"dry_run"`
	Actions []PruneAction `json:"actions"`
}

// Reclaimed is the total bytes freed across all actions.
func (r PruneReport) Reclaimed() int64 {
	var n int64
	for _, a := range r.Actions {
		n += a.Reclaimed()
	}
	return n
}

// add appends an action to the report.
func (r *PruneReport) add(a PruneAction) { r.Actions = append(r.Actions, a) }

// PruneArtifacts sweeps the router root: closed item payloads, dated quarantine
// dumps, logs (age-delete + size tail-cap), the work queue's terminal records,
// and the oversized process snapshot. cutoff is the retention boundary; dryRun
// reports without mutating.
func PruneArtifacts(routerRoot string, cutoff time.Time, dryRun bool) (PruneReport, error) {
	rep := PruneReport{Cutoff: cutoff, DryRun: dryRun}

	// 1. Closed items past the cutoff are tombstoned; open items are never touched.
	items, err := work.PruneItems(routerRoot, cutoff, dryRun)
	if err != nil {
		return rep, err
	}
	for _, it := range items {
		rep.add(PruneAction{
			Path:   filepath.Join("items", it.ID+".md"),
			Kind:   "item",
			Before: it.Bytes,
			After:  it.After,
		})
	}

	// 2. Dated quarantine / incident dumps (quarantine-YYYYMMDD-*).
	if err := pruneDatedDirs(routerRoot, "quarantine-", cutoff, dryRun, &rep); err != nil {
		return rep, err
	}

	// 3. Logs under logs/ — age-delete stale, tail-cap oversized-but-recent.
	if err := pruneLogDir(filepath.Join(routerRoot, "logs"), cutoff, dryRun, &rep); err != nil {
		return rep, err
	}

	// 4. Work queue: drop terminal records older than the cutoff.
	if err := pruneWorkQueue(routerRoot, cutoff, dryRun, &rep); err != nil {
		return rep, err
	}

	// 5. Oversized regenerated snapshot.
	pruneSnapshot(filepath.Join(routerRoot, "processes.json"), dryRun, &rep)

	return rep, nil
}

// PruneHomeLogs sweeps the runtime home (~/.sirsi): worker, triage, and wake
// logs written by shell tooling outside the router binary. Same age + size
// policy as the router log dir.
func PruneHomeLogs(homeDir string, cutoff time.Time, dryRun bool) (PruneReport, error) {
	rep := PruneReport{Cutoff: cutoff, DryRun: dryRun}
	if err := pruneLogDir(homeDir, cutoff, dryRun, &rep); err != nil {
		return rep, err
	}
	// Nested logs/ dir too.
	if err := pruneLogDir(filepath.Join(homeDir, "logs"), cutoff, dryRun, &rep); err != nil {
		return rep, err
	}
	return rep, nil
}

// PruneLogDirExported applies the log retention policy to a single directory.
// Exported for the `router prune --logs-only` path and for tooling that sweeps
// a specific log dir.
func PruneLogDirExported(dir string, cutoff time.Time, dryRun bool, rep *PruneReport) error {
	return pruneLogDir(dir, cutoff, dryRun, rep)
}

// pruneDatedDirs removes immediate subdirectories named "<prefix>YYYYMMDD-*"
// whose embedded date precedes cutoff.
func pruneDatedDirs(root, prefix string, cutoff time.Time, dryRun bool, rep *PruneReport) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		datePart := strings.TrimPrefix(e.Name(), prefix)
		if len(datePart) < 8 {
			continue
		}
		day, perr := time.Parse("20060102", datePart[:8])
		if perr != nil {
			continue
		}
		if !day.Before(cutoff) {
			continue
		}
		full := filepath.Join(root, e.Name())
		size := dirSize(full)
		if !dryRun {
			if err := os.RemoveAll(full); err != nil {
				return err
			}
		}
		rep.add(PruneAction{Path: e.Name(), Kind: "quarantine", Before: size})
	}
	return nil
}

// isLogName reports whether a filename is a disposable log artifact.
func isLogName(name string) bool {
	for _, suf := range []string{".log", ".err", ".out"} {
		if strings.HasSuffix(name, suf) {
			return true
		}
	}
	return false
}

// pruneLogDir applies the log policy to every log file directly under dir:
// delete if mtime precedes cutoff, else tail-cap if it exceeds maxLogTailBytes.
func pruneLogDir(dir string, cutoff time.Time, dryRun bool, rep *PruneReport) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !isLogName(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if fi.ModTime().Before(cutoff) {
			if !dryRun {
				if err := os.Remove(full); err != nil {
					return err
				}
			}
			rep.add(PruneAction{Path: full, Kind: "log-deleted", Before: fi.Size()})
			continue
		}
		if fi.Size() > maxLogTailBytes {
			if !dryRun {
				if err := tailCapFile(full, maxLogTailBytes); err != nil {
					return err
				}
			}
			rep.add(PruneAction{Path: full, Kind: "log-capped", Before: fi.Size(), After: maxLogTailBytes})
		}
	}
	return nil
}

// tailCapFile shrinks path to its last keep bytes (aligned to the next line
// boundary) using copy-truncate: it preserves the file's inode so a launchd
// daemon holding an open append fd keeps writing to the same file instead of
// an unlinked one. This is the same strategy as logrotate's `copytruncate`.
// A concurrent appender may lose the handful of lines written between the tail
// read and the truncate — acceptable for logs, and far better than the
// silent write-to-unlinked-inode loss that renaming would cause.
func tailCapFile(path string, keep int64) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if fi.Size() <= keep {
		return nil
	}
	if _, err = f.Seek(fi.Size()-keep, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	// Align to the next newline so we never begin mid-line.
	if idx := indexByte(data, '\n'); idx >= 0 && idx+1 < len(data) {
		data = data[idx+1:]
	}
	// Truncate in place (preserving the inode) and rewrite the tail at offset 0.
	if err = f.Truncate(0); err != nil {
		return err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

// indexByte is bytes.IndexByte inlined to keep the import set minimal.
func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// pruneWorkQueue drops WorkItems in a terminal state (completed/failed/blocked)
// whose completion precedes cutoff, rewriting work-queue.json.
func pruneWorkQueue(routerRoot string, cutoff time.Time, dryRun bool, rep *PruneReport) error {
	path := filepath.Join(routerRoot, "work-queue.json")
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	wq, err := LoadWorkQueue(routerRoot)
	if err != nil {
		return err
	}
	kept := make([]WorkItem, 0, len(wq.Items))
	var dropped int
	for _, it := range wq.Items {
		if isTerminal(it.Status) && !it.CompletedAt.IsZero() && it.CompletedAt.Before(cutoff) {
			dropped++
			continue
		}
		kept = append(kept, it)
	}
	if dropped == 0 {
		return nil
	}
	before := fi.Size()
	var after int64
	if dryRun {
		if data, mErr := json.MarshalIndent(&WorkQueue{Items: kept}, "", "  "); mErr == nil {
			after = int64(len(data))
		}
	} else {
		wq.Items = kept
		if err := wq.Save(); err != nil {
			return err
		}
		if nfi, sErr := os.Stat(path); sErr == nil {
			after = nfi.Size()
		}
	}
	rep.add(PruneAction{Path: path, Kind: "workqueue", Before: before, After: after})
	return nil
}

// isTerminal reports whether a work status is a settled end state.
func isTerminal(s WorkStatus) bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusBlocked
}

// pruneSnapshot removes a regenerated snapshot file that has grown past the
// size cap. Its writer recreates a fresh, small one, so this is non-destructive.
func pruneSnapshot(path string, dryRun bool, rep *PruneReport) {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() <= snapshotCapBytes {
		return
	}
	if !dryRun {
		if err := os.Remove(path); err != nil {
			return
		}
	}
	rep.add(PruneAction{Path: path, Kind: "snapshot", Before: fi.Size()})
}

// dirSize returns the total bytes of a directory tree (best effort).
func dirSize(root string) int64 {
	var n int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}
