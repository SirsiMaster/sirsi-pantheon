package main

// ccd reap — reap completed CCD scheduled-task session leaks and archive
// stale session records. Go port of the former scripts/ccd-reap-completed.py
// (owner directive 2026-07-23: Go shop, no Python where Go does the job).
//
// Completion-based, NOT age-based. A resident claude-desktop runner is
// reaped IFF its CCD session:
//  1. has a scheduledTaskId (interactive/named sessions have none — never touched)
//  2. is NOT the newest instance for that task (the running one has outstanding work)
//  3. last did work > grace ago (a just-finished turn gets a grace period)
//
// Archive pass (store-level, reversible from the app's Archived list):
//  a. scheduled-task runs that are NOT the newest instance for their task
//  b. untagged sessions idle > staleDays with no live process
//
// Why SIGKILL: the `disclaimer` wrapper ignores SIGTERM. These are completed
// sessions with no outstanding work — not load-bearing model servers (those
// live under ~/.sirsi/*.pid and carry no scheduledTaskId).

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const (
	ccdGraceMin    = 10  // ponytail: fixed 10-min grace; widen if a run legitimately idles longer mid-task
	ccdMatchWinSec = 180 // pid start must be within this of session createdAt to attribute
	ccdStaleDays   = 7   // ponytail: fixed window; tune if interactive sessions idle longer
)

type ccdSession struct {
	path    string
	sid     string
	title   string
	sched   string
	created float64 // unix seconds, 0 = unknown
	last    float64
}

func ccdEpoch(v any) float64 {
	switch t := v.(type) {
	case float64:
		if t > 1e12 {
			return t / 1000.0
		}
		return t
	case string:
		if ts, err := time.Parse(time.RFC3339, t); err == nil {
			return float64(ts.UnixNano()) / 1e9
		}
	}
	return 0
}

func ccdLoadSessions(base string) []ccdSession {
	var out []ccdSession
	_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasPrefix(d.Name(), "local_") || !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			return nil
		}
		if b, _ := m["isArchived"].(bool); b {
			return nil
		}
		s := ccdSession{path: p, created: ccdEpoch(m["createdAt"]), last: ccdEpoch(m["lastActivityAt"])}
		s.sid, _ = m["sessionId"].(string)
		s.title, _ = m["title"].(string)
		s.sched, _ = m["scheduledTaskId"].(string)
		out = append(out, s)
		return nil
	})
	return out
}

// ccdPidStart returns the unix start time of a pid via ps lstart.
func ccdPidStart(pid int) (float64, bool) {
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, false
	}
	ts, err := time.ParseInLocation("Mon Jan _2 15:04:05 2006", strings.TrimSpace(string(out)), time.Local)
	if err != nil {
		return 0, false
	}
	return float64(ts.Unix()), true
}

func ccdPgrep(args ...string) []int {
	out, _ := exec.Command("pgrep", args...).Output()
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		if p, err := strconv.Atoi(f); err == nil {
			pids = append(pids, p)
		}
	}
	return pids
}

var ccdCmd = &cobra.Command{
	Use:   "ccd",
	Short: "Claude desktop (CCD) session hygiene",
}

var ccdReapApply bool

var ccdReapCmd = &cobra.Command{
	Use:   "reap",
	Short: "Reap completed scheduled-task session leaks + archive stale session records (dry-run by default)",
	RunE: func(cmd *cobra.Command, args []string) error {
		now := float64(time.Now().Unix())
		base := filepath.Join(os.Getenv("HOME"), "Library/Application Support/Claude/claude-code-sessions")
		sessions := ccdLoadSessions(base)

		// newest instance per scheduled task = the running one, protected
		newest := map[string]float64{}
		for _, s := range sessions {
			if s.sched != "" && s.last > newest[s.sched] {
				newest[s.sched] = s.last
			}
		}

		// resident MAIN runners → attribute to a session by start-time proximity
		me := os.Getpid()
		type reapT struct {
			pid  int
			s    ccdSession
			idle int
		}
		var reap []reapT
		// Sessions with an attributed live runner. The archive pass consults this
		// so a session that is still ALIVE is never archived out of the app's
		// history — the same protection the kill path gives, extended to the
		// archive path (it holds for untagged/interactive sessions too, which the
		// kill path skips outright and would otherwise be archived on age alone).
		liveSIDs := map[string]bool{}
		for _, pid := range ccdPgrep("-f", "claude.app/Contents/MacOS/claude ") {
			if pid == me {
				continue
			}
			st, ok := ccdPidStart(pid)
			if !ok {
				continue
			}
			var best *ccdSession
			bd := 1e18
			for i := range sessions {
				if sessions[i].created == 0 {
					continue
				}
				if d := absF(sessions[i].created - st); d < bd {
					bd, best = d, &sessions[i]
				}
			}
			if best == nil || bd > ccdMatchWinSec { // unattributable → never kill
				continue
			}
			liveSIDs[best.sid] = true // attributed and running — archive must skip it
			if best.sched == "" {     // interactive/named → protect
				continue
			}
			if best.last != 0 && best.last == newest[best.sched] { // running instance → protect
				continue
			}
			idleMin := 1e18
			if best.last != 0 {
				idleMin = (now - best.last) / 60
			}
			if idleMin < ccdGraceMin { // just finished → grace
				continue
			}
			reap = append(reap, reapT{pid, *best, int(idleMin)})
		}

		killed := 0
		verb := "WOULD-REAP"
		if ccdReapApply {
			verb = "KILL"
		}
		for _, r := range reap {
			targets := append([]int{r.pid}, ccdPgrep("-P", strconv.Itoa(r.pid))...)
			if ccdReapApply {
				for _, t := range targets {
					if syscall.Kill(t, syscall.SIGKILL) == nil { // TERM is ignored by disclaimer
						killed++
					}
				}
			}
			fmt.Printf("%s pid=%d task=%s idle=%dmin\n", verb, r.pid, r.s.sched, r.idle)
		}
		fmt.Printf("reaped %d completed-leak session(s) (%d procs killed); grace=%dmin apply=%v\n",
			len(reap), killed, ccdGraceMin, ccdReapApply)

		// archive pass (owner directive 2026-07-23: single continuous history, no manual archiving)
		archived := 0
		verb = "WOULD-ARCHIVE"
		if ccdReapApply {
			verb = "ARCHIVE"
		}
		for _, s := range sessions {
			if !ccdShouldArchive(s, newest, liveSIDs, now) {
				continue
			}
			idleMin := 1e18
			if s.last != 0 {
				idleMin = (now - s.last) / 60
			}
			if ccdReapApply {
				if err := ccdArchiveRecord(s.path); err != nil {
					fmt.Fprintf(os.Stderr, "  archive-fail %s: %v\n", s.sid, err)
					continue
				}
			}
			archived++
			fmt.Printf("%s %s task=%s title=%q idle=%.0fmin\n", verb, s.sid, orDash(s.sched), s.title, idleMin)
		}
		fmt.Printf("archived %d session record(s); stale-window=%dd apply=%v\n", archived, ccdStaleDays, ccdReapApply)
		return nil
	},
}

// ccdShouldArchive is the archive-pass predicate, extracted so the live-session
// protection is unit-testable without shelling ps/pgrep.
//
// Never archive a session with an attributed LIVE runner — that would hide an
// active session (including an untagged interactive one, which the kill path
// skips outright and would otherwise be archived on age alone). Otherwise:
// a scheduled-task run is archivable once it is not the newest instance for its
// task and has idled past the grace window; an untagged session only after
// ccdStaleDays.
func ccdShouldArchive(s ccdSession, newest map[string]float64, liveSIDs map[string]bool, now float64) bool {
	if liveSIDs[s.sid] {
		return false
	}
	idleMin := 1e18
	if s.last != 0 {
		idleMin = (now - s.last) / 60
	}
	if s.sched != "" {
		if s.last != 0 && s.last == newest[s.sched] {
			return false // the running instance for that task
		}
		return idleMin >= ccdGraceMin
	}
	return idleMin >= ccdStaleDays*1440
}

func ccdArchiveRecord(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var m map[string]any
	if uerr := json.Unmarshal(raw, &m); uerr != nil {
		return uerr
	}
	m["isArchived"] = true
	out, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func absF(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func init() {
	ccdReapCmd.Flags().BoolVar(&ccdReapApply, "apply", false, "actually kill and archive (default: dry-run)")
	ccdCmd.AddCommand(ccdReapCmd)
}
