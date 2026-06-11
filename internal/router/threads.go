// Package router — threads.go
//
// CTR thread registry. Tracks live agent threads/sessions (not just
// registered agents). Every open conversation, worker, or session that
// touches the router should register a thread, heartbeat while alive,
// and close when done. Horus reads this for the local-node live view.
//
// Schema is model-neutral: claude, codex, gemini, gemma, qwen, mcp,
// api, webhook, and future surfaces share the same shape.
package router

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	cryptorand "crypto/rand"
	"encoding/hex"
)

// DefaultThreadStaleAfter is the grace window before a thread without
// a recent heartbeat is considered stale.
const DefaultThreadStaleAfter = 5 * time.Minute

// TerminalRetention is how long a terminal (closed/reaped) record is kept
// before opportunistic register-time compaction GCs it. PR #25 introduced
// this drain; codex's PID-identity refactor accidentally elided the const
// declaration, restored here.
const TerminalRetention = 3 * 24 * time.Hour

// ThreadStatus enumerates a thread's reported state.
type ThreadStatus string

const (
	ThreadStatusActive  ThreadStatus = "active"
	ThreadStatusIdle    ThreadStatus = "idle"
	ThreadStatusBlocked ThreadStatus = "blocked"
	ThreadStatusClosed  ThreadStatus = "closed"
	// ThreadStatusReaped is terminal: the recorded PID was confirmed gone or
	// defunct (Z) against the live OS process table. A reaped record MUST NOT
	// be revived by a late heartbeat — the only way back is re-registration.
	ThreadStatusReaped ThreadStatus = "reaped"
	// ThreadStatusStale marks a thread whose PID is alive but whose heartbeat
	// loop has gone quiet past the stale window — live-but-silent, not dead.
	ThreadStatusStale ThreadStatus = "stale-heartbeat"
	// ThreadStatusSuspended is a resumable, NON-terminal resting state (ADR-025):
	// the session ended cleanly (quit / compact / reconcile) with its memory
	// synced and continuation state snapshotted into SuspendPayload. It is
	// non-prunable (never removed by prune) and non-live (Heartbeat rejects it;
	// RegisterThread bypasses the live fast-path and routes through resume). The
	// only way back to active is `sirsi thread resume`.
	ThreadStatusSuspended ThreadStatus = "suspended"
)

// IsTerminal reports whether a status is a final resting state that a heartbeat
// must never resurrect. Closed (operator/agent ended it) and Reaped (OS truth
// says the PID is gone/defunct) are both terminal; everything else is live.
func (s ThreadStatus) IsTerminal() bool {
	return s == ThreadStatusClosed || s == ThreadStatusReaped
}

// Thread is one live registration of an agent session.
type Thread struct {
	ThreadID      string       `json:"thread_id"`
	AgentID       string       `json:"agent_id"`
	Surface       string       `json:"surface"`
	Repo          string       `json:"repo,omitempty"`
	Workstream    string       `json:"workstream,omitempty"`
	StartedAt     time.Time    `json:"started_at"`
	LastSeenAt    time.Time    `json:"last_seen_at"`
	Status        ThreadStatus `json:"status"`
	Watches       []string     `json:"watches,omitempty"`
	WakeMechanism string       `json:"wake_mechanism,omitempty"`
	CurrentItem   string       `json:"current_item,omitempty"`
	LastError     string       `json:"last_error,omitempty"`
	PID           int          `json:"pid,omitempty"`
	// StartTime is the OS start signature of PID captured at registration
	// (ADR-024 Amendment 1) — the generation half of the (pid, start_time)
	// composite identity that makes reaping resistant to PID reuse. Empty on
	// legacy records and platforms without a start-time probe (bare-PID fallback).
	StartTime string `json:"start_time,omitempty"`
	Host      string `json:"host,omitempty"`
	// SuspendPayload carries resumable continuation state while Status is
	// suspended (ADR-025). Nil for active/terminal threads.
	SuspendPayload *SuspendPayload `json:"suspend_payload,omitempty"`
}

// SuspendPayload is the resumable snapshot captured when a thread is suspended
// (ADR-025). It is what makes a clean exit recoverable: where memory was synced
// (ThothRef), what inbox work the agent still owns, and a one-line continuation.
type SuspendPayload struct {
	ThothRef       string    `json:"thoth_ref,omitempty"`        // Stele ledger id / commit xref where memory was synced
	OwnedOpenItems []string  `json:"owned_open_items,omitempty"` // router item ids still addressed to this agent
	ResumePrompt   string    `json:"resume_prompt,omitempty"`    // one-line continuation (e.g. NOTEBOOKS resume name)
	SuspendedAt    time.Time `json:"suspended_at"`               // when the suspend happened (UTC)
	ReapedFrom     string    `json:"reaped_from,omitempty"`      // set when this is a successor minted for a reaped record
}

// ThreadRegistry is the on-disk record of live threads.
type ThreadRegistry struct {
	Threads map[string]*Thread `json:"threads"`
}

const threadsFilename = "threads.json"

func threadsPath(routerRoot string) string {
	return filepath.Join(routerRoot, threadsFilename)
}

// LoadThreadRegistry reads threads.json. Missing file → empty registry.
func LoadThreadRegistry(routerRoot string) (*ThreadRegistry, error) {
	data, err := os.ReadFile(threadsPath(routerRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return &ThreadRegistry{Threads: map[string]*Thread{}}, nil
		}
		return nil, fmt.Errorf("read threads.json: %w", err)
	}
	var reg ThreadRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse threads.json: %w", err)
	}
	if reg.Threads == nil {
		reg.Threads = map[string]*Thread{}
	}
	for id, t := range reg.Threads {
		if t == nil {
			continue
		}
		if t.ThreadID == "" {
			t.ThreadID = id
		}
	}
	return &reg, nil
}

// SaveThreadRegistry writes threads.json atomically.
func SaveThreadRegistry(routerRoot string, reg *ThreadRegistry) error {
	if reg.Threads == nil {
		reg.Threads = map[string]*Thread{}
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal threads.json: %w", err)
	}
	tmp, err := os.CreateTemp(routerRoot, ".threads.json-*")
	if err != nil {
		return fmt.Errorf("create temp threads.json: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp threads.json: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp threads.json: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp threads.json: %w", err)
	}
	if err := os.Rename(tmpPath, threadsPath(routerRoot)); err != nil {
		return fmt.Errorf("replace threads.json: %w", err)
	}
	return nil
}

// NewThreadID returns a short opaque thread identifier.
func NewThreadID() string {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		// Fallback to time-based ID if entropy is unavailable.
		return fmt.Sprintf("thr-%d", time.Now().UnixNano())
	}
	return "thr-" + hex.EncodeToString(b[:])
}

// RegisterThread upserts a thread record. If t.ThreadID is empty, a new ID
// is generated and stored on t before saving.
func RegisterThread(routerRoot string, t *Thread) (*Thread, error) {
	if t == nil {
		return nil, fmt.Errorf("thread is nil")
	}
	if t.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if t.Surface == "" {
		return nil, fmt.Errorf("surface is required")
	}
	now := time.Now().UTC()

	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil, err
	}

	// Opportunistic compaction (PR #25, A28 residue): GC terminal
	// (closed/reaped) records older than TerminalRetention so the registry
	// self-cleans on every register — the durable drain for terminal-record
	// accretion. Safe by construction (only terminal records, never active
	// or suspended per ADR-025). Codex's PID-identity refactor accidentally
	// elided this call; restored here so the compaction test stays green.
	reg.PruneClosed(now, TerminalRetention)

	// Idempotent registration: if the caller did not pin a ThreadID but this
	// (agent_id, pid) already has a LIVE (non-terminal) record, reuse it instead
	// of minting a new thread + heartbeat loop. Without this, every register/
	// discover call for the same session spawned a duplicate record and a
	// duplicate caffeinate loop — 150+ loops for ~10 live PIDs, all waking each
	// minute. One live session → one thread.
	// Capture the composite-identity discriminator once (ADR-024 Amendment 1):
	// the OS start signature of this PID. Empty on legacy callers / unsupported
	// platforms, which keeps the bare-PID behavior.
	newStart := t.StartTime
	if newStart == "" && t.PID >= minAgentPID {
		newStart = PIDStartTimeOf(t.PID)
	}

	if t.ThreadID == "" && t.PID >= minAgentPID {
		for id, existing := range reg.Threads {
			if existing == nil {
				continue
			}
			// ADR-025: a suspended record must NOT be revived by the live
			// fast-path — resuming is an explicit transition (`thread resume`)
			// that restores the payload + re-arms the watcher. Skip it here so
			// register mints a fresh thread rather than silently reactivating a
			// suspended one without restoring its continuation state.
			if existing.AgentID == t.AgentID && existing.PID == t.PID &&
				!existing.Status.IsTerminal() && existing.Status != ThreadStatusSuspended {
				// (pid, start_time) composite (ADR-024 Amendment 1): only reuse
				// when the start signatures agree (or either is unknown). A
				// mismatch means the OS recycled this PID onto a DIFFERENT
				// process — adopting the stale record would resurrect a dead
				// thread, so mint a fresh one instead.
				if existing.StartTime != "" && newStart != "" && existing.StartTime != newStart {
					continue
				}
				existing.LastSeenAt = now
				if t.CurrentItem != "" {
					existing.CurrentItem = t.CurrentItem
				}
				if err := SaveThreadRegistry(routerRoot, reg); err != nil {
					return nil, err
				}
				_ = id
				return existing, nil
			}
		}
	}

	if t.ThreadID == "" {
		t.ThreadID = NewThreadID()
	}
	if t.StartTime == "" {
		t.StartTime = newStart
	}
	if t.StartedAt.IsZero() {
		t.StartedAt = now
	}
	t.LastSeenAt = now
	if t.Status == "" {
		t.Status = ThreadStatusActive
	}
	if len(t.Watches) == 0 {
		t.Watches = []string{t.AgentID}
	}

	reg.Threads[t.ThreadID] = t
	if err := SaveThreadRegistry(routerRoot, reg); err != nil {
		return nil, err
	}
	return t, nil
}

// HeartbeatThread updates LastSeenAt and optionally status/current_item/last_error.
type HeartbeatUpdate struct {
	Status      ThreadStatus
	CurrentItem *string
	LastError   *string
}

// Heartbeat updates a thread's last_seen_at and optional fields.
// Returns the updated thread, or an error if the thread is unknown.
func Heartbeat(routerRoot, threadID string, upd HeartbeatUpdate) (*Thread, error) {
	if threadID == "" {
		return nil, fmt.Errorf("thread_id is required")
	}
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil, err
	}
	t, ok := reg.Threads[threadID]
	if !ok {
		return nil, fmt.Errorf("thread %q not registered", threadID)
	}
	// reaped-is-terminal: a closed/reaped record must never be revived by a
	// late heartbeat. Refusing the write here is what stops a dead PID from
	// reappearing as `active` with a fresh last_seen_at while still carrying
	// `last_error: reaped`. Reopening requires a new registration (new ID).
	if t.Status.IsTerminal() {
		return nil, fmt.Errorf("thread %q is %s and cannot be revived by heartbeat (last_error=%q); register a new thread to resume", threadID, t.Status, t.LastError)
	}
	// ADR-025: suspended is resumable but NOT live. A heartbeat must not revive
	// it or refresh last_seen_at — that would mask a session that has actually
	// ended. Restoring requires the explicit resume transition.
	if t.Status == ThreadStatusSuspended {
		return nil, fmt.Errorf("thread %q is suspended and cannot heartbeat; run `sirsi thread resume --thread %s` to restore it", threadID, threadID)
	}
	t.LastSeenAt = time.Now().UTC()
	if upd.Status != "" {
		t.Status = upd.Status
	}
	if upd.CurrentItem != nil {
		t.CurrentItem = *upd.CurrentItem
	}
	if upd.LastError != nil {
		t.LastError = *upd.LastError
	}
	if err := SaveThreadRegistry(routerRoot, reg); err != nil {
		return nil, err
	}
	return t, nil
}

// CloseThread marks a thread closed (does not delete it; callers may prune).
func CloseThread(routerRoot, threadID string) (*Thread, error) {
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil, err
	}
	t, ok := reg.Threads[threadID]
	if !ok {
		return nil, fmt.Errorf("thread %q not registered", threadID)
	}
	t.Status = ThreadStatusClosed
	t.LastSeenAt = time.Now().UTC()
	if err := SaveThreadRegistry(routerRoot, reg); err != nil {
		return nil, err
	}
	return t, nil
}

// SuspendThread transitions a thread to the resumable suspended state (ADR-025),
// snapshotting the supplied continuation payload. It is idempotent: a thread
// already suspended is returned unchanged. A terminal (closed/reaped) thread
// cannot be suspended — terminal is final.
func SuspendThread(routerRoot, threadID string, payload *SuspendPayload) (*Thread, error) {
	if threadID == "" {
		return nil, fmt.Errorf("thread_id is required")
	}
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil, err
	}
	t, ok := reg.Threads[threadID]
	if !ok {
		return nil, fmt.Errorf("thread %q not registered", threadID)
	}
	if t.Status == ThreadStatusSuspended {
		return t, nil // idempotent
	}
	if t.Status.IsTerminal() {
		return nil, fmt.Errorf("thread %q is %s (terminal) and cannot be suspended", threadID, t.Status)
	}
	if payload == nil {
		payload = &SuspendPayload{}
	}
	if payload.SuspendedAt.IsZero() {
		payload.SuspendedAt = time.Now().UTC()
	}
	t.Status = ThreadStatusSuspended
	t.SuspendPayload = payload
	t.LastSeenAt = time.Now().UTC()
	if err := SaveThreadRegistry(routerRoot, reg); err != nil {
		return nil, err
	}
	return t, nil
}

// ResumeThread transitions a suspended thread back to active (ADR-025), clearing
// the stored payload. The returned thread RETAINS the payload in memory so the
// caller can re-surface owned items and print the resume prompt; the persisted
// record has it cleared. Errors if the thread is not suspended.
func ResumeThread(routerRoot, threadID string) (*Thread, error) {
	if threadID == "" {
		return nil, fmt.Errorf("thread_id is required")
	}
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil, err
	}
	t, ok := reg.Threads[threadID]
	if !ok {
		return nil, fmt.Errorf("thread %q not registered", threadID)
	}
	if t.Status != ThreadStatusSuspended {
		return nil, fmt.Errorf("thread %q is %s, not suspended; nothing to resume", threadID, t.Status)
	}
	payload := t.SuspendPayload
	t.Status = ThreadStatusActive
	t.LastSeenAt = time.Now().UTC()
	t.SuspendPayload = nil
	if err := SaveThreadRegistry(routerRoot, reg); err != nil {
		return nil, err
	}
	t.SuspendPayload = payload // re-attach for the caller (not persisted)
	return t, nil
}

// ReconcileReapedLookback bounds how far back a reaped record is still eligible
// for successor-minting / unrecoverable-warning during SessionStart reconciliation.
// Reaped records older than this are presumed already handled (or about to be
// pruned) and are left alone — this keeps reconciliation from re-warning forever
// about ancient post-reboot reaps.
const ReconcileReapedLookback = 24 * time.Hour

// RetroSyncFn retroactively captures memory for a thread that exited without
// syncing it, returning the resumable payload (with a fresh ThothRef) and whether
// the session transcript was still recoverable. It is injected so reconciliation
// stays pure and host-independent (Rule A16): the CLI wires it to `sirsi thoth
// sync` + an on-disk transcript check; tests stub it. For the stale-active heal
// the bool is ignored (the transcript is the live session's, always present); for
// a reaped record it gates whether a successor can be minted at all.
type RetroSyncFn func(t *Thread) (payload *SuspendPayload, transcriptAvailable bool)

// ReconcileAction names the healing transition reconciliation performed for one
// dirty-exit record.
type ReconcileAction string

const (
	// ReconcileSuspendedStale: a stale active record (the /clear / soft-exit case)
	// was healed in place — retro-synced then transitioned active→suspended.
	ReconcileSuspendedStale ReconcileAction = "suspended-stale"
	// ReconcileMintedSuccessor: a reaped (terminal) record got a NEW suspended
	// successor carrying reaped_from; the reaped record stays reaped (ADR-022).
	ReconcileMintedSuccessor ReconcileAction = "minted-successor"
	// ReconcileUnrecoverable: a reaped record had no recoverable transcript, so no
	// successor could be minted. The caller MUST surface this visibly — memory was
	// lost, never silently.
	ReconcileUnrecoverable ReconcileAction = "warn-unrecoverable"
)

// ReconcileOutcome is one action ReconcileExits took, in declaration order.
type ReconcileOutcome struct {
	ThreadID    string          `json:"thread_id"`              // the dirty record acted on
	AgentID     string          `json:"agent_id"`               // its agent
	Action      ReconcileAction `json:"action"`                 // what healing happened
	SuccessorID string          `json:"successor_id,omitempty"` // minted suspended thread (minted-successor only)
}

// hasSuccessorFor reports whether a suspended successor already exists for the
// given reaped thread id — the idempotency guard that stops every SessionStart
// from minting a fresh successor for the same reaped record.
func (r *ThreadRegistry) hasSuccessorFor(reapedID string) bool {
	for _, t := range r.Threads {
		if t == nil || t.SuspendPayload == nil {
			continue
		}
		if t.SuspendPayload.ReapedFrom == reapedID {
			return true
		}
	}
	return false
}

// ReconcileExits heals the two dirty-exit shapes ADR-025 §4 defines, on this host
// (and optionally scoped to one agent — each surface heals its own lineage at its
// own SessionStart, rather than one start sweeping every agent). It is the
// authoritative gate: SessionEnd is best-effort, but this always runs at start.
//
//   - Stale active record (heartbeat quiet, never transitioned): healed IN PLACE
//     to suspended after a retro sync. It was never terminal, so this is legal —
//     ADR-022's terminal invariant is untouched.
//   - Reaped record (terminal, hard-kill case): NEVER revived. If the transcript
//     is recoverable, a new suspended SUCCESSOR is minted carrying reaped_from;
//     otherwise an unrecoverable warning is recorded for the caller to surface.
//     Idempotent via hasSuccessorFor + a recency lookback.
//
// reg is mutated in place; the caller saves it. Outcomes are returned in a
// deterministic (sorted-id) order for stable output and tests.
func ReconcileExits(reg *ThreadRegistry, host, agentFilter string, now time.Time, staleAfter time.Duration, retro RetroSyncFn) []ReconcileOutcome {
	if reg == nil || reg.Threads == nil {
		return nil
	}
	if staleAfter <= 0 {
		staleAfter = DefaultThreadStaleAfter
	}
	if retro == nil {
		retro = func(*Thread) (*SuspendPayload, bool) { return &SuspendPayload{}, false }
	}
	// Snapshot ids: we mint successors into reg.Threads while iterating.
	ids := make([]string, 0, len(reg.Threads))
	for id := range reg.Threads {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var outcomes []ReconcileOutcome
	for _, id := range ids {
		t := reg.Threads[id]
		if t == nil {
			continue
		}
		if host != "" && t.Host != "" && t.Host != host {
			continue // another machine's process table is unobservable here
		}
		if agentFilter != "" && t.AgentID != agentFilter {
			continue
		}

		switch {
		case t.Status == ThreadStatusReaped:
			if now.Sub(t.LastSeenAt) > ReconcileReapedLookback || reg.hasSuccessorFor(t.ThreadID) {
				continue // too old, or already healed — idempotent
			}
			payload, ok := retro(t)
			if !ok {
				outcomes = append(outcomes, ReconcileOutcome{ThreadID: t.ThreadID, AgentID: t.AgentID, Action: ReconcileUnrecoverable})
				continue
			}
			if payload == nil {
				payload = &SuspendPayload{}
			}
			if payload.SuspendedAt.IsZero() {
				payload.SuspendedAt = now
			}
			payload.ReapedFrom = t.ThreadID
			succ := &Thread{
				ThreadID:       NewThreadID(),
				AgentID:        t.AgentID,
				Surface:        t.Surface,
				Repo:           t.Repo,
				Workstream:     t.Workstream,
				Host:           t.Host,
				StartedAt:      now,
				LastSeenAt:     now,
				Status:         ThreadStatusSuspended,
				SuspendPayload: payload,
			}
			reg.Threads[succ.ThreadID] = succ
			outcomes = append(outcomes, ReconcileOutcome{ThreadID: t.ThreadID, AgentID: t.AgentID, Action: ReconcileMintedSuccessor, SuccessorID: succ.ThreadID})

		case t.Status == ThreadStatusSuspended || t.Status.IsTerminal():
			continue // parked or cleanly closed — nothing to heal

		case t.IsStale(now, staleAfter):
			payload, _ := retro(t) // transcript is the live session's; always present
			if payload == nil {
				payload = &SuspendPayload{}
			}
			if payload.SuspendedAt.IsZero() {
				payload.SuspendedAt = now
			}
			t.Status = ThreadStatusSuspended
			t.SuspendPayload = payload
			t.LastSeenAt = now
			outcomes = append(outcomes, ReconcileOutcome{ThreadID: t.ThreadID, AgentID: t.AgentID, Action: ReconcileSuspendedStale})
		}
	}
	return outcomes
}

// ReapedThread records one thread the reaper retired against OS truth.
type ReapedThread struct {
	ThreadID string
	AgentID  string
	PID      int
	State    PIDState // gone | defunct
}

// ReapDeadThreads retires non-terminal threads whose recorded PID is confirmed
// dead by OS truth — gone, or defunct (zombie Z). Such records are set to
// ThreadStatusReaped with a descriptive last_error; the Heartbeat guard then
// refuses to revive them. This is what stops a dead PID from re-presenting as
// `active` after a late heartbeat.
//
// host scopes the sweep to threads on THIS machine: a thread whose Host differs
// (or is empty) is left untouched, because we cannot observe another host's
// process table. Threads without a PID are also skipped (unverifiable).
//
// Returns the reaped records (empty if none). The registry is saved only when
// at least one thread was reaped.
func ReapDeadThreads(routerRoot, host string) ([]ReapedThread, error) {
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil, err
	}
	var reaped []ReapedThread
	now := time.Now().UTC()
	for _, t := range reg.Threads {
		if t == nil || t.Status.IsTerminal() {
			continue
		}
		// ADR-025: suspended is resumable, not dead. Its PID is EXPECTED to be
		// gone (the session ended cleanly), so the reaper must NOT retire it to
		// terminal `reaped` — that would destroy the recoverable continuation
		// state. Suspended leaves the OS-truth sweep untouched.
		if t.Status == ThreadStatusSuspended {
			continue
		}
		if host != "" && t.Host != host {
			continue // a different host's process table
		}
		// Phantom PID (<minAgentPID, i.e. 0/launchd-1) is "unverifiable" by
		// the cmdline-identity check, but PR #29 established: a stale phantom
		// heartbeat is dead — reap it. A pid-less surface (e.g. MCP server)
		// that is freshly heartbeating stays alive; only stale phantoms retire.
		if t.PID < minAgentPID {
			if now.Sub(t.LastSeenAt) > DefaultThreadStaleAfter {
				t.Status = ThreadStatusReaped
				t.LastSeenAt = now
				t.LastError = fmt.Sprintf("reaped: phantom PID %d stale > %s at %s", t.PID, DefaultThreadStaleAfter, now.Format(time.RFC3339))
				reaped = append(reaped, ReapedThread{ThreadID: t.ThreadID, AgentID: t.AgentID, PID: t.PID, State: PIDUnknown})
			}
			continue
		}
		state := PIDStateOfThread(t)
		if !DeadByOSTruth(state) {
			continue
		}
		t.Status = ThreadStatusReaped
		t.LastSeenAt = now
		t.LastError = fmt.Sprintf("reaped: PID %d %s per OS truth at %s", t.PID, state, now.Format(time.RFC3339))
		reaped = append(reaped, ReapedThread{ThreadID: t.ThreadID, AgentID: t.AgentID, PID: t.PID, State: state})
	}
	if len(reaped) > 0 {
		if err := SaveThreadRegistry(routerRoot, reg); err != nil {
			return reaped, err
		}
	}
	return reaped, nil
}

// IsStale reports whether a thread should be considered stale given now and
// the configured stale-after window. Closed threads are not stale.
func (t *Thread) IsStale(now time.Time, staleAfter time.Duration) bool {
	if t == nil || t.Status.IsTerminal() {
		return false
	}
	// Suspended threads are intentionally parked, not stale (ADR-025).
	if t.Status == ThreadStatusSuspended {
		return false
	}
	if staleAfter <= 0 {
		staleAfter = DefaultThreadStaleAfter
	}
	return now.Sub(t.LastSeenAt) > staleAfter
}

// SortedThreads returns thread records sorted by LastSeenAt descending.
func (r *ThreadRegistry) SortedThreads() []*Thread {
	out := make([]*Thread, 0, len(r.Threads))
	for _, t := range r.Threads {
		if t == nil {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	return out
}

// PruneClosed removes terminal threads (closed or reaped) older than maxAge.
// Returns the count removed.
func (r *ThreadRegistry) PruneClosed(now time.Time, maxAge time.Duration) int {
	if maxAge <= 0 {
		return 0
	}
	removed := 0
	for id, t := range r.Threads {
		if t == nil {
			delete(r.Threads, id)
			continue
		}
		if t.Status.IsTerminal() && now.Sub(t.LastSeenAt) > maxAge {
			delete(r.Threads, id)
			removed++
		}
	}
	return removed
}

// SuspendedRetention is the default window a suspended record is preserved before
// PruneStaleSuspended treats it as abandoned. Generous enough for the multi-day
// NOTEBOOKS "resume name" pattern, bounded enough that suspended records — which
// the reaper AND PruneClosed both intentionally skip (ADR-025) — cannot accrete
// unbounded in threads.json (the A27 write-amplification → Spotlight mds_stores
// class, dogfooded 2026-06-02: 7 orphaned pid=0 suspends from one churny session).
const SuspendedRetention = 7 * 24 * time.Hour

// PruneStaleSuspended removes suspended records (ADR-025) whose suspend time is
// older than retention — abandoned pauses that were never resumed. It is the
// retention bound that keeps ADR-025's deliberately non-prunable `suspended`
// state from accreting forever. OPT-IN by design: callers pass an explicit
// retention, so the default prune path (PruneClosed) still never touches
// suspended — the resume-later guarantee holds for any *recent* suspend. Suspend
// time is SuspendPayload.SuspendedAt when present, else LastSeenAt (the
// transition timestamp). retention<=0 is a no-op (never a blanket wipe). Returns
// the count removed.
func (r *ThreadRegistry) PruneStaleSuspended(now time.Time, retention time.Duration) int {
	if retention <= 0 {
		return 0
	}
	removed := 0
	for id, t := range r.Threads {
		if t == nil || t.Status != ThreadStatusSuspended {
			continue
		}
		ts := t.LastSeenAt
		if t.SuspendPayload != nil && !t.SuspendPayload.SuspendedAt.IsZero() {
			ts = t.SuspendPayload.SuspendedAt
		}
		if now.Sub(ts) > retention {
			delete(r.Threads, id)
			removed++
		}
	}
	return removed
}
