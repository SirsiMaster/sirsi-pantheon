// Package reaper — reclaim RAM from leaked claude-desktop task-runner sessions.
//
// WHY: every Claude scheduled-task/wakeup run leaves a resident
// `claude-desktop` session that never self-exits. They pile up (~8/hr from
// */15 tasks), eat GBs of RSS, and drive swap-death that wedges local gemma
// (memory reference_scheduled_task_session_leak; 40 accumulated → 6.8 GB →
// swap 82% → gemma wedged, 2026-07-17). The leaked runners are
// process-IDENTICAL to the owner's live interactive session (same argv/env/tty),
// so there is NO safe blind reaper.
//
// SAFETY CONTRACT (non-negotiable — this is why the verb exists):
//   - NEVER kill anything in the CALLER'S process-ancestry chain. Running this
//     from your live Claude conversation cannot close that conversation, because
//     the conversation's PID is an ancestor of this process and is protected.
//   - Only `claude-code/<ver>/claude.app/Contents/MacOS/claude` procs are ever
//     candidates — never /Applications/*.app (the main app), never the gemma
//     server, never a non-claude process.
//   - Default is dry-run (Rule A1): selection mutates nothing. SIGTERM only on
//     Apply; SIGKILL survivors only with Force.
//   - Age + RSS floors so a just-started, actively-running task is not reaped
//     mid-write.
//
// The selection (selectCandidates) is a PURE function of the process table and
// the protected set, unit-tested to ALWAYS exclude the ancestry — the regression
// guard on the safety contract.
package reaper

import (
	"regexp"
	"sort"
)

// Defaults for the age/RSS floors (mirror the reference shell reaper).
const (
	DefaultMinAgeSec = 600 // a live task run is < 10 min; older = leaked
	DefaultMinRSSMB  = 120 // skip tiny stubs; real leaked sessions hold > 120 MB
)

// ClaudeRe matches ONLY a claude-code desktop session executable — never the
// main /Applications app bundle, never any other process.
var ClaudeRe = regexp.MustCompile(`claude-code/[0-9.]+/claude\.app/Contents/MacOS/claude`)

// Proc is one row of the process table the reaper reasons over.
type Proc struct {
	PID     int
	PPID    int
	AgeSec  int
	RSSMB   int
	Command string
}

// Options tune the reap.
type Options struct {
	MinAgeSec int
	MinRSSMB  int
	Apply     bool // SIGTERM the candidates
	Force     bool // SIGKILL survivors after SIGTERM
}

func (o Options) withDefaults() Options {
	if o.MinAgeSec <= 0 {
		o.MinAgeSec = DefaultMinAgeSec
	}
	if o.MinRSSMB <= 0 {
		o.MinRSSMB = DefaultMinRSSMB
	}
	return o
}

// Result is the outcome, JSON-safe for the menubar/doctor surfaces.
type Result struct {
	Candidates        []int `json:"candidates"`         // PIDs selected for reaping
	ReclaimMBEst      int   `json:"reclaim_mb_est"`     // summed RSS of candidates
	ProtectedAncestry int   `json:"protected_ancestry"` // ancestry PIDs skipped
	Apply             bool  `json:"apply"`
	Force             bool  `json:"force"`
	Reaped            []int `json:"reaped,omitempty"`    // PIDs no longer alive after apply
	Survivors         []int `json:"survivors,omitempty"` // PIDs still alive after apply(+force)
}

// Deps are the injectable side-effect seams (Rule A16): everything that touches
// the real OS is a function, so the safety-critical logic is tested without a
// live process table or a real kill.
type Deps struct {
	// Procs enumerates the full process table.
	Procs func() ([]Proc, error)
	// SelfPID is this process's PID (os.Getpid on the real path).
	SelfPID int
	// Kill sends signal sig to pid (syscall.Kill on the real path).
	Kill func(pid, sig int) error
	// Alive reports whether pid is still running (for survivor re-check).
	Alive func(pid int) bool
}

// ancestrySet walks the caller's PID up the ppid chain to 1, returning every
// PID in the chain — the protected set. A pid→ppid map from the process table
// makes this a pure lookup (no /proc, no ps re-shell).
func ancestrySet(self int, procs []Proc) map[int]bool {
	ppidOf := make(map[int]int, len(procs))
	for _, p := range procs {
		ppidOf[p.PID] = p.PPID
	}
	protected := map[int]bool{}
	for cur := self; cur > 1; {
		protected[cur] = true
		next, ok := ppidOf[cur]
		if !ok || next == cur { // unknown or self-parent → stop (never loop)
			break
		}
		cur = next
	}
	return protected
}

// selectCandidates is the PURE, safety-critical core: from the process table and
// the protected ancestry set, return the leaked-session PIDs to reap (sorted)
// and their summed RSS. A protected PID can NEVER appear in the result — that is
// the invariant the regression test pins.
func selectCandidates(procs []Proc, protected map[int]bool, opts Options) (kill []int, reclaimMB int) {
	opts = opts.withDefaults()
	for _, p := range procs {
		if !ClaudeRe.MatchString(p.Command) {
			continue // only claude-code desktop sessions are ever candidates
		}
		if protected[p.PID] {
			continue // NEVER the caller's ancestry
		}
		if p.AgeSec < opts.MinAgeSec || p.RSSMB < opts.MinRSSMB {
			continue // floors: don't reap a fresh, actively-running task
		}
		kill = append(kill, p.PID)
		reclaimMB += p.RSSMB
	}
	sort.Ints(kill)
	return kill, reclaimMB
}

// Plan performs the read-only selection (never kills), for dry-run + the doctor
// pileup check. It resolves the protected ancestry from the SAME table it
// selects over, so it can never count its own session.
func Plan(opts Options, deps Deps) (Result, error) {
	procs, err := deps.Procs()
	if err != nil {
		return Result{}, err
	}
	protected := ancestrySet(deps.SelfPID, procs)
	kill, mb := selectCandidates(procs, protected, opts)
	return Result{
		Candidates:        kill,
		ReclaimMBEst:      mb,
		ProtectedAncestry: len(protected),
		Apply:             opts.Apply,
		Force:             opts.Force,
	}, nil
}

// Run selects and, when opts.Apply is set, SIGTERMs the candidates (SIGKILL
// survivors only with Force). Dry-run (Apply=false) is identical to Plan.
func Run(opts Options, deps Deps) (Result, error) {
	res, err := Plan(opts, deps)
	if err != nil || !opts.Apply || len(res.Candidates) == 0 {
		return res, err
	}
	const sigTERM, sigKILL = 15, 9
	for _, pid := range res.Candidates {
		_ = deps.Kill(pid, sigTERM)
	}
	survivors := aliveSubset(res.Candidates, deps)
	if len(survivors) > 0 && opts.Force {
		for _, pid := range survivors {
			_ = deps.Kill(pid, sigKILL)
		}
		survivors = aliveSubset(res.Candidates, deps)
	}
	res.Survivors = survivors
	res.Reaped = difference(res.Candidates, survivors)
	return res, nil
}

func aliveSubset(pids []int, deps Deps) []int {
	if deps.Alive == nil {
		return nil
	}
	var alive []int
	for _, pid := range pids {
		if deps.Alive(pid) {
			alive = append(alive, pid)
		}
	}
	return alive
}

func difference(all, remove []int) []int {
	drop := make(map[int]bool, len(remove))
	for _, p := range remove {
		drop[p] = true
	}
	var out []int
	for _, p := range all {
		if !drop[p] {
			out = append(out, p)
		}
	}
	return out
}
