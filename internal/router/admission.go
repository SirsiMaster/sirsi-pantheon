// Package router — admission.go
//
// Consumer admission primitives (rs-37, ADR-065 task 4). Relocated verbatim
// from wake.go so they are a shared boundary rather than RunWakeLoop-private
// state: both RunWakeLoop today and the ADR-065 informer once task 5 lands
// call the SAME consumerPIDFilePath/adoptRunningConsumer/dispatchConsumer/
// fabricDispatchQuarantined/fabricDispatchOverloaded/measurementWindowOpen/
// attendedSessionOwnsInbox — never duplicated, per-caller copies.
//
// admitConsumer below is the new piece this task adds: a shared PID filename
// alone is not atomic spawn admission (SSA correction) — two concurrent
// callers could both observe "nothing running" before either writes the
// marker. admitConsumer serializes check(adopt) → spawn → write-marker as one
// critical section under a per-agent flock, so a second caller that loses the
// race always adopts the first caller's consumer instead of duplicating it.
package router

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// consumerRun tracks ONE dispatched consumer process for its whole lifetime.
//
// The first cut of this fix used a 10-minute timer as its re-dispatch authority,
// because Setsid+Release deliberately discards the process handle — so the loop
// could not tell "still working" from "died", and a slow-but-healthy agent got a
// second consumer dispatched on top of it. A timer is not dispatch authority
// (codex-pantheon review of #389, finding 3).
//
// So the handle is KEPT and reaped: Setsid still detaches the child from the
// terminal/session, but we never Release, and a goroutine Waits. `done` closing
// is real completion, not an elapsed guess.
type consumerRun struct {
	pid  int
	done chan struct{}
	err  error
	tail *ringTail

	// depthAtDispatch is the inbox depth when this consumer started, so its exit
	// can be scored for PROGRESS rather than merely for liveness (#636 C1).
	// -1 means unknown and is scored as progress — an unreadable inbox must never
	// quarantine a healthy lane. See madeProgress in dispatchgate.go.
	depthAtDispatch int

	// startedAt is set only on a run ADOPTED from the durable marker file
	// (done == nil — this process never forked the child, so it cannot Wait
	// on it). running() polls OS-truth liveness instead, recycle-guarded by
	// this start signature.
	startedAt string

	// Stall gate for a RUNNING consumer (SHA 2026-09-14: claude-io's consumer
	// sat in `Ss` for 40+ min on open HTTPS connections, never claimed, acked
	// or closed anything, and the green wake-loop held the slot against its
	// own inbox). progressMark is the inbox fingerprint (ids + acked_at) last
	// seen changing; progressAt is when. stalled latches the one-time kill.
	progressMark string
	progressAt   time.Time
	stalled      bool
}

// wakeLoopConsumerStall bounds how long a running consumer may hold the
// dispatch slot with NO durable router action visible in its inbox — no item
// claimed/closed, none acknowledged. Liveness (#636 C1 scores only FINISHED
// consumers) cannot see a blocked `claude --print`; this can. A var so the
// test can shorten it.
//
// ponytail: the fingerprint also changes on an ARRIVAL, which resets the clock
// in the consumer's favor; replace with the ledger's last durable action per
// consumer if arrivals ever mask a real stall.
var wakeLoopConsumerStall = 30 * time.Minute

// inboxMark fingerprints the open inbox: a change means a durable router action
// landed (close removes an id, acknowledge stamps acked_at) or an item arrived.
func inboxMark(items []work.Item) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, it.ID+"@"+it.AckedAt)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

// consumerTailBytes bounds what a failing consumer can put in the log. The
// blackhole this replaces produced a 1.4 MB log carrying only "exit status 1";
// wiring the raw transcript through would trade no information for too much, so
// the last few KB — where the fatal line lives — is what gets kept.
const consumerTailBytes = 4096

// ringTail keeps only the last max bytes written to it.
type ringTail struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *ringTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

// String returns the captured tail, or a marker when nothing was captured —
// silence is itself a finding and must not read as "no output field".
func (t *ringTail) String() string {
	if t == nil {
		return "(not captured)"
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if s := strings.TrimSpace(string(t.buf)); s != "" {
		return s
	}
	return "(no output)"
}

// running reports whether the dispatched consumer is still alive.
func (r *consumerRun) running() bool {
	if r == nil {
		return false
	}
	if r.done == nil {
		return PIDStateOf(r.pid, r.startedAt) == PIDAlive
	}
	select {
	case <-r.done:
		return false
	default:
		return true
	}
}

// fabricDispatchQuarantined reports (and records, R7/G6) whether the operator
// has stood the whole fabric down via `sirsi router quarantine`. Checked right
// before every consumer dispatch so the marker holds against the loop's own
// restart, not just against the launchd-level revivers (fabricquarantine.go).
func fabricDispatchQuarantined(agentID string, depth int) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false // cannot resolve home — fail open, same stance as an unreadable load average
	}
	if !IsFabricQuarantined(home) {
		return false
	}
	log.Printf("wake-loop %s: dispatch held — fabric quarantined (inbox depth %d)", agentID, depth)
	return true
}

// fabricDispatchOverloaded reports (and records, R7/G6) whether load average
// is at or above the core count, in which case dispatch defers this pass and
// retries next tick rather than piling another lane on an already-saturated
// host (incident 2026-08-06: load average 36 on 18 cores from just 3 lanes).
func fabricDispatchOverloaded(agentID string, depth int) bool {
	hold, load, cores := shouldDeferDispatch()
	if !hold {
		return false
	}
	msg := fmt.Sprintf("wake-loop %s: dispatch deferred — CPU load %.2f of %d cores (inbox depth %d)",
		agentID, load, cores, depth)
	log.Print(msg)
	RecordHeal(msg)
	return true
}

// railsLockPath is the Ma'at measurement-window marker a benchmark lane holds
// for its run: ~/libsirsimpi/rails.lock, or $MAAT_RAILS_LOCK. The owner's
// maat-window-gate hook reads the same file, so one definition of "window open"
// governs interactive sessions and wake loops alike.
func railsLockPath() string {
	if p := strings.TrimSpace(os.Getenv("MAAT_RAILS_LOCK")); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "libsirsimpi", "rails.lock")
}

// measurementWindowOpen reports (and records) whether a hardware measurement
// window is open on this host, in which case dispatch holds: a consumer started
// mid-window loads the CPU and invalidates the run (2026-09-30: the 19:12
// cablepull quiet reservation on m1 was invalidated by a Claude shell). The
// session already running is left alone; only new dispatches wait.
func measurementWindowOpen(agentID string, depth int) bool {
	holder, open := railsLockHolder()
	if !open {
		return false
	}
	log.Printf("wake-loop %s: dispatch held — measurement window open (rails.lock held by %q; inbox depth %d)",
		agentID, holder, depth)
	return true
}

// attendedLiveFn is the injectable attended-session probe (Rule A16/A21).
var (
	attendedMu     sync.RWMutex
	attendedLiveFn = AttendedSessionLive
)

func getAttendedLiveFn() func(routerRoot, agentID string) bool {
	attendedMu.RLock()
	defer attendedMu.RUnlock()
	return attendedLiveFn
}

// setAttendedLiveFn installs a probe for tests.
func setAttendedLiveFn(fn func(routerRoot, agentID string) bool) {
	attendedMu.Lock()
	defer attendedMu.Unlock()
	if fn == nil {
		fn = AttendedSessionLive
	}
	attendedLiveFn = fn
}

// attendedSessionOwnsInbox reports whether a live, armed attended session
// (interactive claude/codex) is already consuming this lane's inbox. Dispatching
// a headless consumer on top of it makes two writers act as one lane id — the
// claude-finalwishes-m5 collision, 2026-10-02: a worker acknowledged and worked
// PR #893 while the owner's interactive session never saw it. The attended
// session wins; the worker resumes as soon as it is gone.
func attendedSessionOwnsInbox(routerRoot, agentID string, depth int) bool {
	if !getAttendedLiveFn()(routerRoot, agentID) {
		return false
	}
	log.Printf("wake-loop %s: dispatch held — an attended session is live on this lane (inbox depth %d)", agentID, depth)
	return true
}

// railsLockHolder reports, without logging, whether a measurement window is open
// and who holds it. Used by the lane-state publisher so a sender can see a hold
// without the loop spamming its log every tick.
func railsLockHolder() (string, bool) {
	p := railsLockPath()
	if p == "" {
		return "", false
	}
	holder, err := os.ReadFile(p)
	if err != nil {
		return "", false // no lock (or unreadable) — no window
	}
	return strings.TrimSpace(string(holder)), true
}

// loginShellArgv wraps argv so the consumer runs through the operator's login
// shell instead of being exec'd directly.
//
// launchd execs a program with ONLY the plist's EnvironmentVariables and no
// shell at all, so none of the shell startup files run. The consumer CLIs take
// their credentials from what those files export (~/.zshenv), so a directly
// exec'd consumer reported "Not logged in - Please run /login" and exited 1 on
// every single dispatch: 3907 of 4161 across eight lanes, while the very same
// command run by a human in a terminal succeeded every time. The login state
// was never broken; the environment carrying it simply never reached the child.
//
// argv is passed as POSITIONAL PARAMETERS and never interpolated into the
// script text. That is load-bearing, not stylistic: the consumer prompt
// contains backticks and $(...) that a naive `-lc "<command>"` would execute at
// dispatch. `exec` then replaces the shell, so the pid the caller tracks is
// still the consumer itself and the setsid detach is unchanged.
//
// Fails OPEN — an unset or unusable SHELL dispatches exactly as before, the
// same stance as an unreadable load average.
func loginShellArgv(argv []string) []string {
	if len(argv) == 0 {
		return argv
	}
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		return argv
	}
	// Executable bit included deliberately: exec.Command on a non-executable
	// SHELL fails the dispatch outright, which would be failing CLOSED — the
	// opposite of the stance this function documents.
	if info, err := os.Stat(shell); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return argv
	}
	return append([]string{shell, "-lc", `exec "$@"`, shell}, argv...)
}

// dispatchConsumer starts a resolved consumer and returns a handle that
// completes when the process exits.
//
// Setsid without Release is the deliberate combination: the child survives this
// loop's own restart (it is in its own session), while this loop keeps the
// handle it needs to know whether the work is still in flight. cmd.Wait also
// reaps the child, so repeated dispatch cannot accumulate zombies.
func dispatchConsumer(rc *ResolvedConsumer) (*consumerRun, error) {
	if rc.Resident {
		return nil, fmt.Errorf("resident consumer is external and must not be spawned")
	}
	spawn := loginShellArgv(rc.Argv)
	cmd := exec.Command(spawn[0], spawn[1:]...)
	cmd.Dir = rc.Cwd
	cmd.Env = rc.Env
	cmd.SysProcAttr = detachedSysProcAttr()

	// Capture a bounded tail of the child's output. Leaving Stdout/Stderr nil
	// makes exec.Cmd wire both to /dev/null, which destroyed the cause of 94% of
	// fleet-wide dispatch failures at the source (3843/4082 exits with nothing
	// but "exit status 1").
	//
	// An *os.File is handed to the child DIRECTLY — exec starts no copier
	// goroutine for it and cmd.Wait does not block on one. That matters here: the
	// consumer is setsid-detached and may leave grandchildren holding the fd, and
	// with an io.Writer sink Wait would hang until the LAST holder closed it,
	// pinning running() true forever and silently killing re-dispatch.
	pr, pw, perr := os.Pipe()
	if perr != nil {
		return nil, perr
	}
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return nil, err
	}
	pw.Close() // the child holds the only write end now; the reader sees EOF when it goes
	tail := &ringTail{max: consumerTailBytes}
	go func() {
		defer pr.Close()
		_, _ = io.Copy(tail, pr)
	}()

	run := &consumerRun{pid: cmd.Process.Pid, done: make(chan struct{}), tail: tail}
	go func() {
		run.err = cmd.Wait()
		close(run.done)
	}()
	return run, nil
}

// consumerPIDFilePath is the durable marker for the consumer last dispatched
// for agentID (design constraint 7: "pidfile" idempotency, stated at the top
// of wake.go but never implemented until this fix).
//
// dispatchConsumer's child is Setsid-detached without Release specifically so
// it survives THIS PROCESS restarting — but a launchctl kickstart -k (or a
// KeepAlive crash-restart) replaces the process outright, and the in-memory
// `run` variable that tracked the dispatch dies with it. The new process
// starts with run == nil and, on its first tick with depth > 0, dispatches a
// SECOND consumer on top of the still-running first one. Observed 2026-08-07:
// two live claude-finalwishes sessions, 480MB each, spawned 65s apart. This
// marker is what lets a restarted loop recognize "already running" instead of
// re-dispatching.
func consumerPIDFilePath(routerRoot, agentID string) string {
	return filepath.Join(routerRoot, "wake-consumer-"+agentID+".pid")
}

// writeConsumerPIDFile records the just-dispatched consumer's (pid, startedAt)
// so a restarted loop can find it. Best-effort: a write failure only costs the
// restart-survives-dispatch guarantee, never the dispatch itself.
func writeConsumerPIDFile(routerRoot, agentID string, pid int, startedAt string) {
	path := consumerPIDFilePath(routerRoot, agentID)
	_ = os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"+startedAt+"\n"), 0o644)
}

// clearConsumerPIDFile removes the marker once the dispatching process has
// confirmed (via Wait) that the consumer exited.
func clearConsumerPIDFile(routerRoot, agentID string) {
	_ = os.Remove(consumerPIDFilePath(routerRoot, agentID))
}

// adoptRunningConsumer reads the durable marker and, if the recorded PID is
// still OS-truth alive (recycle-guarded by its start signature), returns a
// consumerRun this loop can poll instead of blind-dispatching a duplicate. A
// stale marker (process gone) is cleaned up so it never misreports again.
func adoptRunningConsumer(routerRoot, agentID string) *consumerRun {
	data, err := os.ReadFile(consumerPIDFilePath(routerRoot, agentID))
	if err != nil {
		return nil
	}
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	pid, perr := strconv.Atoi(strings.TrimSpace(lines[0]))
	if perr != nil || pid <= 0 {
		clearConsumerPIDFile(routerRoot, agentID)
		return nil
	}
	startedAt := ""
	if len(lines) > 1 {
		startedAt = strings.TrimSpace(lines[1])
	}
	if PIDStateOf(pid, startedAt) != PIDAlive {
		clearConsumerPIDFile(routerRoot, agentID)
		return nil
	}
	return &consumerRun{pid: pid, startedAt: startedAt}
}

// consumerAdmissionLockPath is the flock file guarding one agent's
// check(adopt) → spawn → write-marker critical section. A SEPARATE file from
// consumerPIDFilePath (never removed, only ever created once) — the PID
// marker is rewritten/removed across the consumer's lifecycle, and flock
// arbitration is only coherent while every locker holds an fd on the SAME
// inode (see internal/selfupdate/lock_unix.go's inode-recycling note); a lock
// file that is always truncated-not-removed makes that true by construction
// for free.
func consumerAdmissionLockPath(routerRoot, agentID string) string {
	return consumerPIDFilePath(routerRoot, agentID) + ".lock"
}

// admitConsumer is the ADR-065 coexistence boundary (rs-37 task 4): the ONE
// entry point every dispatching caller — RunWakeLoop today, the informer once
// task 5 lands — MUST use instead of calling adoptRunningConsumer and
// dispatchConsumer separately.
//
// A shared PID filename alone is not atomic spawn admission (SSA correction):
// two concurrent callers reading "no marker, nothing alive" before either has
// written a marker would both dispatch. admitConsumer closes that window by
// holding a per-agent, blocking, exclusive flock across the whole
// check(adopt) → spawn → write-marker sequence, so a second caller always
// blocks until the first either adopts (lock released, no new marker written)
// or spawns and records (lock released, marker now present) — and then itself
// re-checks under the SAME lock before ever spawning.
//
// Returns the live consumer and adopted=true when an existing consumer
// (dispatched by this call or a concurrent one) was found rather than
// freshly spawned — callers treat an adopted run's depthAtDispatch as
// unknown, the same convention already used for a loop-restart adoption.
func admitConsumer(routerRoot, agentID string, rc *ResolvedConsumer) (run *consumerRun, adopted bool, err error) {
	unlock, lerr := lockConsumerAdmission(consumerAdmissionLockPath(routerRoot, agentID))
	if lerr != nil {
		return nil, false, fmt.Errorf("admission lock: %w", lerr)
	}
	defer unlock()

	if existing := adoptRunningConsumer(routerRoot, agentID); existing != nil {
		return existing, true, nil
	}
	run, err = dispatchConsumer(rc)
	if err != nil {
		return nil, false, err
	}
	writeConsumerPIDFile(routerRoot, agentID, run.pid, getPIDStartFn()(run.pid))
	return run, false, nil
}
