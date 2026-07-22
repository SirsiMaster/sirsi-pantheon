// Package router — sessionreaper.go
//
// Supervisor duty: reap superseded interactive-session process leaks.
//
// Incident (2026-07-08, claude-home RCA item 20260708-154507): a
// ScheduleWakeup-driven session loop leaked one full claude CLI process per
// wakeup — 26 live processes (13 parent/child pairs) for ONE logical session,
// inflating the thread registry to 37 "live" threads and likely contributing
// to a session crash. The harness bug (old process not terminated when a
// wakeup spawns its successor) is reported upstream, but the deterministic
// layer must not depend on that fix: same lesson as ADR-031-C and ADR-035 —
// scaffolding guards beat model-remembered habits, which fail exactly when
// sessions crash (i.e., exactly when the leak happens).
//
// The duty (monitor → identify → FIX, ADR-033 shape):
//  1. Group live claude CLI processes by their `--resume <session-uuid>` arg.
//  2. More than one process GROUP (a root + its descendants) for the same
//     session uuid means every group but the NEWEST is a superseded leak —
//     the wakeup respawned the session and the old process should be gone.
//     Reap the older groups (SIGTERM).
//  3. Reap `sirsi thread watch-router` children whose parent is gone.
//  4. Reap pre-2026-07-08 legacy sidecar watchers (/tmp/watcher-thr-* bash
//     loops) — superseded by in-band heartbeats.
//  5. Registry truth: after any reap, ReapDeadThreads folds the affected
//     thread records via the existing OS-truth path (ADR-022) — no bespoke
//     marking here.
//
// Ambiguity is NOT killed: a high total claude-process count is Sekhmet's
// "Runaway Executor" doctor finding (host-level, with the quarantine lever);
// this duty only ever touches processes it can PROVE superseded — same-uuid
// older groups, dead-parent watchers, and the known legacy sidecar pattern.
package router

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ReapedLeak records one process this duty terminated and why.
type ReapedLeak struct {
	PID  int    `json:"pid"`
	Kind string `json:"kind"` // superseded-session | orphaned-watcher | legacy-sidecar
	Why  string `json:"why"`
}

// SessionReapReport summarizes one reaper pass.
type SessionReapReport struct {
	Reaped []ReapedLeak `json:"reaped,omitempty"`
	// KeptNewest maps session uuid → the root PID of the group kept alive.
	KeptNewest map[string]int `json:"kept_newest,omitempty"`
}

type sessionProc struct {
	pid, ppid int
	started   time.Time
	args      string
}

var resumeUUIDRe = regexp.MustCompile(`--resume[= ]([0-9a-fA-F-]{8,})`)

// psLstartLayout parses `ps -axo lstart` ("Mon Jul  7 10:11:12 2026") after
// whitespace collapsing.
const psLstartLayout = "Mon Jan 2 15:04:05 2006"

// parsePSLines parses `ps -axo pid,ppid,lstart,args` output. Unparseable
// lines are skipped — the reaper must prove, never guess.
func parsePSLines(out string) []sessionProc {
	var procs []sessionProc
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 8 { // pid ppid + 5 lstart fields + at least 1 arg
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		started, err := time.Parse(psLstartLayout, strings.Join(fields[2:7], " "))
		if err != nil {
			continue
		}
		procs = append(procs, sessionProc{pid: pid, ppid: ppid, started: started, args: strings.Join(fields[7:], " ")})
	}
	return procs
}

// isClaudeCLI matches the interactive claude CLI (the leaking binary), not
// arbitrary processes with "claude" in an argument.
func isClaudeCLI(args string) bool {
	first := strings.Fields(args)
	if len(first) == 0 {
		return false
	}
	base := first[0]
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	return base == "claude" || strings.Contains(first[0], "claude.app/Contents/MacOS/claude")
}

// ReapSupersededSessionsWith is the pure core (Rule A16): callers inject the
// ps snapshot and the kill primitive. selfPID/selfParent are NEVER touched,
// nor is any group containing them (the directive's own-session guard).
func ReapSupersededSessionsWith(selfPID, selfParent int, psOut string, kill func(pid int) error) (*SessionReapReport, error) {
	procs := parsePSLines(psOut)
	byPID := make(map[int]sessionProc, len(procs))
	for _, p := range procs {
		byPID[p.pid] = p
	}

	// Session groups: root = a claude CLI proc whose parent is not itself a
	// claude CLI proc with the same uuid; descendants collapse into the root.
	type group struct {
		rootPID int
		started time.Time
		pids    []int
	}
	groupsByUUID := map[string][]group{}
	rootOf := func(p sessionProc, uuid string) sessionProc {
		cur := p
		for {
			parent, ok := byPID[cur.ppid]
			if !ok || !isClaudeCLI(parent.args) {
				return cur
			}
			if m := resumeUUIDRe.FindStringSubmatch(parent.args); m == nil || m[1] != uuid {
				return cur
			}
			cur = parent
		}
	}
	memberPIDs := map[string]map[int][]int{} // uuid → rootPID → member pids
	for _, p := range procs {
		if !isClaudeCLI(p.args) {
			continue
		}
		m := resumeUUIDRe.FindStringSubmatch(p.args)
		if m == nil {
			continue // no-resume interactive session: never ours to judge
		}
		uuid := m[1]
		root := rootOf(p, uuid)
		if memberPIDs[uuid] == nil {
			memberPIDs[uuid] = map[int][]int{}
		}
		memberPIDs[uuid][root.pid] = append(memberPIDs[uuid][root.pid], p.pid)
	}
	for uuid, roots := range memberPIDs {
		for rootPID, pids := range roots {
			groupsByUUID[uuid] = append(groupsByUUID[uuid], group{rootPID: rootPID, started: byPID[rootPID].started, pids: pids})
		}
	}

	report := &SessionReapReport{KeptNewest: map[string]int{}}
	containsSelf := func(g group) bool {
		for _, pid := range g.pids {
			if pid == selfPID || pid == selfParent {
				return true
			}
		}
		return false
	}
	for uuid, groups := range groupsByUUID {
		if len(groups) < 2 {
			if len(groups) == 1 {
				report.KeptNewest[uuid] = groups[0].rootPID
			}
			continue
		}
		// Keep the newest root; every older group is a superseded leak.
		newest := groups[0]
		for _, g := range groups[1:] {
			if g.started.After(newest.started) {
				newest = g
			}
		}
		report.KeptNewest[uuid] = newest.rootPID
		for _, g := range groups {
			if g.rootPID == newest.rootPID || containsSelf(g) {
				continue
			}
			for _, pid := range g.pids {
				if err := kill(pid); err == nil {
					report.Reaped = append(report.Reaped, ReapedLeak{
						PID:  pid,
						Kind: "superseded-session",
						Why:  fmt.Sprintf("session %s respawned %s after this group started — wakeup leak", uuid[:8], newest.started.Sub(g.started).Round(time.Minute)),
					})
				}
			}
		}
	}

	// Orphaned watch-router children: parent gone or no longer a claude proc.
	for _, p := range procs {
		if !strings.Contains(p.args, "thread watch-router") {
			continue
		}
		if p.pid == selfPID || p.pid == selfParent {
			continue
		}
		parent, ok := byPID[p.ppid]
		if ok && (isClaudeCLI(parent.args) || strings.Contains(parent.args, "horus supervise")) {
			continue // parented and supervised — healthy
		}
		if err := kill(p.pid); err == nil {
			report.Reaped = append(report.Reaped, ReapedLeak{PID: p.pid, Kind: "orphaned-watcher", Why: fmt.Sprintf("parent %d gone or not a session process", p.ppid)})
		}
	}

	// Legacy sidecar watchers (pre-2026-07-08 hand-rolled bash loops):
	// superseded by in-band heartbeats, safe to reap by the RCA directive.
	for _, p := range procs {
		if p.pid == selfPID || p.pid == selfParent {
			continue
		}
		if strings.Contains(p.args, "/tmp/watcher-thr-") {
			if err := kill(p.pid); err == nil {
				report.Reaped = append(report.Reaped, ReapedLeak{PID: p.pid, Kind: "legacy-sidecar", Why: "hand-rolled sidecar watcher — deprecated by in-band heartbeats (2026-07-08)"})
			}
		}
	}

	return report, nil
}

// sessionReaperRunFn indirects the duty body so tests can stub it (Rule A16 +
// A21 accessors): the registry wires the ACCESSOR, never the real function —
// a unit test that runs the duty table must not shell `ps` and SIGTERM real
// processes on the dev machine (the #151 test-side-effects class; this
// exact mistake fired real reaper code inside the first test run of this file).
var (
	sessionReaperMu   sync.RWMutex
	sessionReaperImpl = sessionReaperDuty
)

func getSessionReaperImpl() func(routerRoot, repoRoot string) error {
	sessionReaperMu.RLock()
	defer sessionReaperMu.RUnlock()
	return sessionReaperImpl
}

func setSessionReaperImpl(fn func(routerRoot, repoRoot string) error) {
	sessionReaperMu.Lock()
	defer sessionReaperMu.Unlock()
	sessionReaperImpl = fn
}

func runSessionReaperDuty(routerRoot, repoRoot string) error {
	return getSessionReaperImpl()(routerRoot, repoRoot)
}

// sessionReaperDuty is the supervisor GoRun wrapper: real ps, real SIGTERM,
// then the existing OS-truth registry reap folds affected thread records.
func sessionReaperDuty(routerRoot, repoRoot string) error {
	out, err := exec.Command("ps", "-axo", "pid,ppid,lstart,args").Output()
	if err != nil {
		return fmt.Errorf("session-reaper: ps: %w", err)
	}
	report, err := ReapSupersededSessionsWith(os.Getpid(), os.Getppid(), string(out), func(pid int) error {
		return syscall.Kill(pid, syscall.SIGTERM)
	})
	if err != nil {
		return err
	}
	if len(report.Reaped) == 0 {
		return nil
	}
	_, _ = ReapDeadThreads(routerRoot) // fold registry records via OS truth (ADR-022)
	for _, r := range report.Reaped {
		appendSupervisorLog(routerRoot, fmt.Sprintf("session-reaper: SIGTERM %d (%s: %s)", r.PID, r.Kind, r.Why))
	}
	return nil
}

// appendSupervisorLog best-effort appends one line to the router log dir.
func appendSupervisorLog(routerRoot, line string) {
	f, err := os.OpenFile(routerRoot+"/logs/session-reaper.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "[%s] %s\n", time.Now().Format(time.RFC3339), line)
}
