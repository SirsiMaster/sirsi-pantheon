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
	"strings"
	"time"

	cryptorand "crypto/rand"
	"encoding/hex"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// DefaultThreadStaleAfter is the grace window before a thread without
// a recent heartbeat is considered stale.
const DefaultThreadStaleAfter = 5 * time.Minute

// LeaseSessionTTL is the renewal window for session-keyed (app-hosted) thread
// records — those that carry a SessionID but no durable PID. Derived from the
// observed inter-heartbeat gap distribution: the canonical /loop heartbeat
// interval is 60 s, but app-hosted turns (e.g. Claude Code in the desktop app)
// can sit idle between turns for tens of minutes. Live registry sample showed
// active claude-surface sessions with last_seen gaps of 10–27 min; conduit runs
// are ~15–30 min. 2 h = 120× the 60 s heartbeat cadence, covers realistic
// idle-between-turns pauses without pinning a genuinely-dead session alive. A
// session-keyed record that misses renewal for 2 h reads expired in ReapDeadThreads
// (the lease expiry path), not via OS-truth reaping.
const LeaseSessionTTL = 2 * time.Hour

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

// StaleActiveSupervisors returns supervisory registrations that still claim
// active after missing the heartbeat contract. These records must never be
// treated as healthy merely because a shared host PID is alive: automation and
// resident loops own their own renewal duty. Interactive Codex/Claude sessions
// are intentionally excluded because their task/session lease is a different
// liveness contract.
func StaleActiveSupervisors(reg *ThreadRegistry, now time.Time, window time.Duration) []*Thread {
	if reg == nil {
		return nil
	}
	var out []*Thread
	for _, thread := range reg.SortedThreads() {
		if thread.Status != ThreadStatusActive || !thread.IsStale(now, window) {
			continue
		}
		if thread.Surface != "automation" && thread.WakeMechanism != "resident-loop" {
			continue
		}
		out = append(out, thread)
	}
	return out
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
	// MachineID is the stable per-machine identity captured at registration.
	// The reaper scopes OS-truth liveness to the machine that wrote the record
	// (ADR-022) via this, NOT via Host — a hostname is mutable across networks,
	// which stranded inboxes when a laptop's name changed. Empty on legacy
	// records (treated as local; the registry is per-machine). See machineid.go.
	MachineID string `json:"machine_id,omitempty"`
	// ConsumerCapable records whether this thread can actually DRAIN the agent's
	// inbox, as opposed to merely proving the process is alive.
	//
	// The distinction is the whole point: a wake loop with no declared consumer
	// heartbeats exactly like one that works the queue, and because armed[] was
	// computed from heartbeat freshness alone, a watch-only loop credited its own
	// lane as armed and suppressed the wake pass that would otherwise have
	// escalated it. Agent-session surfaces set this implicitly (they ARE the
	// consumer); worker loops set it only when a consumer resolved.
	ConsumerCapable bool `json:"consumer_capable,omitempty"`

	// SessionID is the stable conversation identity for app-hosted surfaces that
	// have no durable OS PID (e.g. CLAUDE_CODE_SESSION_ID for Claude Code desktop
	// sessions). When set, the mint key is (session_id, surface) instead of
	// (pid, agent_id). pid becomes optional evidence — a null PID is the normal
	// shape for an app-hosted record, not an error state. One record per
	// (session_id, surface) is enforced at mint time: a returning hook fire finds
	// its own record and renews its lease (LastSeenAt) instead of minting a fresh
	// one. The record expires when LastSeenAt + LeaseSessionTTL elapses without
	// renewal. Populated from CLAUDE_CODE_SESSION_ID by `sirsi thread register`.
	SessionID string `json:"session_id,omitempty"`

	// SuspendPayload carries resumable continuation state while Status is
	// suspended (ADR-025). Nil for active/terminal threads.
	SuspendPayload *SuspendPayload `json:"suspend_payload,omitempty"`
}

// IsInboxConsumer reports whether this thread can actually drain its agent's
// inbox — the honest replacement for "has a fresh heartbeat".
//
// An agent-session surface (a live claude/codex/gemini session) IS a consumer:
// that is what drained claude-deck's queue while its worker loop sat watching.
// A worker loop is a consumer only if it resolved a declared consumer capability.
func (t *Thread) IsInboxConsumer() bool {
	if t == nil {
		return false
	}
	if t.ConsumerCapable {
		return true
	}
	// ALLOW-LIST, not a deny-list. The earlier shape credited every surface
	// except four named observers, so `automation`, `resident-loop`, `mcp`,
	// `api`, `webhook`, `vscode`, `jetbrains`, `cursor`, `gemini`, `gemma`,
	// `qwen` — and any surface spelling invented after this line was written —
	// became armed by default without ever declaring a consumer contract.
	//
	// That is the same false-green-by-default class this whole change exists to
	// remove: a thread credited as draining an inbox it cannot drain suppresses
	// its own rescue in WakePass. A new surface must now EARN the credit via
	// ConsumerCapable rather than inherit it by not being on a list someone
	// remembered to update (codex-pantheon, PR #389).
	switch t.Surface {
	case surfaceClaude, surfaceCodex:
		// Real agent sessions: a live one works its own inbox.
		return true
	default:
		return false
	}
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
	Threads  map[string]*Thread `json:"threads"`
	baseline map[string]routerstore.ThreadRecord
}

const threadsFilename = "threads.json"

func threadsPath(routerRoot string) string {
	return filepath.Join(routerRoot, threadsFilename)
}

// LoadThreadRegistry reads threads.json. Missing file → empty registry.
func LoadThreadRegistry(routerRoot string) (*ThreadRegistry, error) {
	if routercfg.StoreWake() {
		store, err := openThreadStore()
		if err != nil {
			return nil, err
		}
		defer store.Close()
		records, err := store.ListThreads()
		if err != nil {
			return nil, err
		}
		if len(records) == 0 {
			if legacy, legacyErr := loadLegacyThreadRegistry(routerRoot); legacyErr == nil && len(legacy.Threads) > 0 {
				seed, seedErr := threadRecords(legacy)
				if seedErr != nil {
					return nil, seedErr
				}
				if importErr := store.ImportThreadsIfEmpty(seed); importErr != nil {
					return nil, importErr
				}
				records, err = store.ListThreads()
				if err != nil {
					return nil, err
				}
			}
		}
		reg := &ThreadRegistry{Threads: make(map[string]*Thread, len(records)), baseline: make(map[string]routerstore.ThreadRecord, len(records))}
		for _, record := range records {
			var thread Thread
			if err := json.Unmarshal(record.Payload, &thread); err != nil {
				return nil, fmt.Errorf("parse store thread %q: %w", record.ThreadID, err)
			}
			if thread.ThreadID == "" {
				thread.ThreadID = record.ThreadID
			}
			reg.Threads[record.ThreadID] = &thread
			reg.baseline[record.ThreadID] = record
		}
		return reg, nil
	}
	return loadLegacyThreadRegistry(routerRoot)
}

func loadLegacyThreadRegistry(routerRoot string) (*ThreadRegistry, error) {
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
	if routercfg.StoreWake() {
		store, err := openThreadStore()
		if err != nil {
			return err
		}
		defer store.Close()
		records, err := threadRecords(reg)
		if err != nil {
			return err
		}
		dirty := records[:0]
		for _, record := range records {
			old, exists := reg.baseline[record.ThreadID]
			if !exists || string(old.Payload) != string(record.Payload) {
				dirty = append(dirty, record)
			}
		}
		for _, record := range dirty {
			applied, err := store.UpsertThreadCAS(record)
			if err != nil {
				return err
			}
			if !applied {
				return fmt.Errorf("thread %q mutation lost lifecycle fence", record.ThreadID)
			}
		}
		for id, old := range reg.baseline {
			if _, ok := reg.Threads[id]; !ok {
				deleted, err := store.DeleteThreadCAS(id, old.Status, old.LastSeenAt)
				if err != nil {
					return err
				}
				if !deleted {
					return fmt.Errorf("thread %q prune lost lifecycle fence", id)
				}
			}
		}
		return nil
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

func threadRecords(reg *ThreadRegistry) ([]routerstore.ThreadRecord, error) {
	records := make([]routerstore.ThreadRecord, 0, len(reg.Threads))
	for id, thread := range reg.Threads {
		if thread == nil {
			continue
		}
		payload, err := json.Marshal(thread)
		if err != nil {
			return nil, fmt.Errorf("marshal store thread %q: %w", id, err)
		}
		records = append(records, routerstore.ThreadRecord{ThreadID: id, Agent: thread.AgentID, Status: string(thread.Status), LastSeenAt: thread.LastSeenAt.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"), Payload: payload})
	}
	return records, nil
}

func openThreadStore() (*routerstore.Store, error) {
	path, err := routerstore.DefaultStorePath()
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o755); mkdirErr != nil {
			return nil, fmt.Errorf("create thread store directory: %w", mkdirErr)
		}
	}
	store, err := routerstore.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open thread store: %w", err)
	}
	return store, nil
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

	// Stamp the stable machine identity the reaper scopes liveness to (ADR-022).
	// Captured here so both the mint and reuse paths carry it; empty on platforms
	// without a probe, which the reaper treats as a local (per-machine) record.
	if t.MachineID == "" {
		t.MachineID = MachineID()
	}

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

	// Session-keyed reuse path (app-hosted surfaces, e.g. Claude Code desktop).
	// App-hosted sessions have no durable PID — each hook fire gets a fresh OS
	// process. The stable identity is the session_id (CLAUDE_CODE_SESSION_ID),
	// which is constant across all turns of one conversation. If the caller
	// provides one, search for an existing LIVE record keyed on (session_id,
	// surface) and renew it instead of minting. This is what structurally bounds
	// the pile: one record per (session_id, surface) is now impossible to exceed.
	if t.ThreadID == "" && t.SessionID != "" {
		for _, existing := range reg.Threads {
			if existing == nil {
				continue
			}
			if existing.SessionID != t.SessionID || existing.Surface != t.Surface {
				continue
			}
			if existing.Status.IsTerminal() || existing.Status == ThreadStatusSuspended {
				continue
			}
			// Found the live record for this session — renew the lease.
			existing.LastSeenAt = now
			if existing.MachineID == "" {
				existing.MachineID = t.MachineID
			}
			// Carry forward the caller's PID as optional evidence (may vary
			// turn-to-turn on app-hosted surfaces; the session_id is the
			// true identity, so PID is evidence-only, not the key).
			if t.PID >= minAgentPID {
				existing.PID = t.PID
			}
			if t.CurrentItem != "" {
				existing.CurrentItem = t.CurrentItem
			}
			if len(t.Watches) > 0 {
				existing.Watches = normalizeWatches(existing.AgentID, t.Watches)
			}
			if t.Workstream != "" {
				existing.Workstream = t.Workstream
			}
			if t.WakeMechanism != "" {
				existing.WakeMechanism = t.WakeMechanism
			}
			if t.Repo != "" {
				existing.Repo = t.Repo
			}
			if err := SaveThreadRegistry(routerRoot, reg); err != nil {
				return nil, err
			}
			return existing, nil
		}
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
				if existing.MachineID == "" {
					existing.MachineID = t.MachineID // backfill legacy record in place
				}
				if t.CurrentItem != "" {
					existing.CurrentItem = t.CurrentItem
				}
				// REPLACE-when-non-empty (claude-home design call, 2026-06-19): a
				// re-register with a non-empty declaration AUTHORITATIVELY re-states
				// the live thread's watches/metadata. This fixes the bug where new
				// --watch values were silently dropped on the reuse path (codex-home,
				// item 133134) AND lets an agent NARROW its watch set — "re-register
				// tightens the live declaration." An empty incoming field is a bare
				// heartbeat-style register and must NOT wipe the existing value.
				// (Union was rejected: it accretes watches forever and can never narrow.)
				if len(t.Watches) > 0 {
					existing.Watches = normalizeWatches(existing.AgentID, t.Watches)
				}
				if t.Workstream != "" {
					existing.Workstream = t.Workstream
				}
				if t.WakeMechanism != "" {
					existing.WakeMechanism = t.WakeMechanism
				}
				if t.Repo != "" {
					existing.Repo = t.Repo
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
	// normalizeWatches puts self first + dedupes, so empty input → just [self] and a
	// declaration omitting self still watches its own inbox (the A27 contract).
	t.Watches = normalizeWatches(t.AgentID, t.Watches)

	reg.Threads[t.ThreadID] = t
	if err := SaveThreadRegistry(routerRoot, reg); err != nil {
		return nil, err
	}
	return t, nil
}

// normalizeWatches returns the watch set with the thread's OWN agent id FIRST,
// then the declared watches stable-de-duped with blank/whitespace entries dropped.
// Every thread watches its own inbox (A27), so self is GUARANTEED present and
// primary even when a register/re-register declares only other agents — that
// self-watch is the precursor honest liveness depends on (codex-validated #76
// follow-up: a thread that doesn't watch its own inbox can never be truthfully
// "armed"). A blank agentID is ignored. Used on both the mint and reuse paths.
func normalizeWatches(agentID string, watches []string) []string {
	seen := make(map[string]struct{}, len(watches)+1)
	out := make([]string, 0, len(watches)+1)
	add := func(w string) {
		w = strings.TrimSpace(w)
		if w == "" {
			return
		}
		if _, ok := seen[w]; ok {
			return
		}
		seen[w] = struct{}{}
		out = append(out, w)
	}
	add(agentID) // self FIRST — the inbox a thread must always watch
	for _, w := range watches {
		add(w)
	}
	return out
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
	suspendedAt := reg.baseline[threadID].LastSeenAt
	t.Status = ThreadStatusActive
	t.LastSeenAt = time.Now().UTC()
	t.SuspendPayload = nil
	if routercfg.StoreWake() {
		store, err := openThreadStore()
		if err != nil {
			return nil, err
		}
		defer store.Close()
		records, err := threadRecords(&ThreadRegistry{Threads: map[string]*Thread{threadID: t}})
		if err != nil {
			return nil, err
		}
		if err := store.ResumeThreadCAS(records[0], suspendedAt); err != nil {
			return nil, err
		}
	} else if err := SaveThreadRegistry(routerRoot, reg); err != nil {
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
	thisMachine := MachineID()
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
			// Idle is not exit. A record whose PID is still ALIVE belongs to a
			// session that simply has not fired a hook lately (a desktop session
			// sitting between prompts idles well past staleAfter). Suspending it
			// drove mint churn: the session's next hook fire found no ACTIVE
			// record for its anchor pid and minted a fresh one, so the same live
			// process cycled through thread ids every few minutes and each new id
			// read as unarmed and spawned another /loop watcher (claude-home run
			// 20260726-2013 — 6 concurrent claude-home records, two watchers still
			// heartbeating store-absent threads after 2d16h).
			//
			// OS truth is the discriminator ReapDeadThreads already uses: only a
			// gone/defunct PID proves the session ended. PIDUnknown (foreign
			// machine, unreadable table) keeps the old idle-based behavior rather
			// than pinning a record alive forever.
			if t.PID > 0 && SameMachine(t.MachineID, thisMachine) &&
				getPIDStateFn()(t.PID) == PIDAlive {
				continue // live session, just quiet — leave it active
			}
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
// The sweep is scoped to THIS machine's process table by stable machine identity
// (MachineID / SameMachine), not hostname — a record with a different machine id
// is provably foreign and left untouched; an id-less legacy record is treated as
// local (the registry is per-machine). Threads without a PID are also handled
// (stale phantoms retire; fresh pid-less surfaces stay alive).
//
// Returns the reaped records (empty if none). The registry is saved only when
// at least one thread was reaped.
func ReapDeadThreads(routerRoot string) ([]ReapedThread, error) {
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil, err
	}
	var reaped []ReapedThread
	now := time.Now().UTC()
	thisMachine := MachineID()
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
		// Scope to THIS machine's process table by stable machine identity, not
		// hostname. A hostname is mutable across networks, so the old host-equality
		// guard treated a laptop's own older records (written under a prior name)
		// as a foreign host and NEVER reaped their dead PIDs — the root cause of a
		// 1d16h stranded inbox. A record with a DIFFERENT machine id is provably
		// foreign; an id-less legacy record is local (the registry is per-machine).
		if !SameMachine(t.MachineID, thisMachine) {
			continue
		}
		// Phantom PID (<minAgentPID, i.e. 0/launchd-1) is "unverifiable" by
		// the cmdline-identity check, but PR #29 established: a stale phantom
		// heartbeat is dead — reap it. A pid-less surface (e.g. MCP server)
		// that is freshly heartbeating stays alive; only stale phantoms retire.
		//
		// Session-keyed records (SessionID != "") use LeaseSessionTTL rather
		// than DefaultThreadStaleAfter. Their PID is intentionally null — that
		// is the normal shape for an app-hosted surface, not a phantom. Their
		// liveness is established by the lease (LastSeenAt + LeaseSessionTTL),
		// not by OS-truth (ADR-022: a LIVE pid is never reaped; no pid means
		// there is no OS-truth claim to probe). Stale-after-lease is still a
		// reap so the terminal invariant holds and the supersession path
		// (ReapStrayThreads) remains unaffected.
		if t.PID < minAgentPID {
			ttl := DefaultThreadStaleAfter
			if t.SessionID != "" {
				ttl = LeaseSessionTTL
			}
			if now.Sub(t.LastSeenAt) > ttl {
				t.Status = ThreadStatusReaped
				t.LastSeenAt = now
				if t.SessionID != "" {
					t.LastError = fmt.Sprintf("reaped: session %s lease expired (no renewal in > %s) at %s", t.SessionID, ttl, now.Format(time.RFC3339))
				} else {
					t.LastError = fmt.Sprintf("reaped: phantom PID %d stale > %s at %s", t.PID, ttl, now.Format(time.RFC3339))
				}
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
			// Return nil, not reaped: the in-memory mutations were never persisted,
			// so a caller checking len(reaped)>0 must not print a completion banner
			// for a mutation that did not happen. Fail closed — no claim without
			// persistence (sirsi-io #18 amendment).
			return nil, err
		}
	}
	return reaped, nil
}

// isLiveWatcher reports whether t is a live process currently holding its
// surface: a non-suspended, non-terminal record whose PID is alive by OS truth.
// Suspended is excluded even if its PID happens to linger — it is a parked
// resting state (ADR-025), never an anchor that supersedes a sibling.
func isLiveWatcher(t *Thread) bool {
	if t == nil || t.Status.IsTerminal() || t.Status == ThreadStatusSuspended {
		return false
	}
	if t.PID < minAgentPID {
		return false
	}
	return !DeadByOSTruth(PIDStateOfThread(t))
}

// ReapStrayThreads enforces the ADR-024 invariant "one live watcher per
// (agent, surface, machine)". After ReapDeadThreads has retired dead-PID actives,
// any group that still has a LIVE watcher has its remaining NON-live siblings —
// superseded suspends and any leftover dead-PID stray — retired to `closed`.
// This is what bounds the ghost pile: the moment a successor session registers a
// live watcher, every prior record for that surface is provably superseded and
// swept, so a churny surface (the scheduled conduit re-registers every 15 min)
// can never accrete hundreds of tombstones.
//
// SAFETY — ADR-025 preserved: a suspended record is retired ONLY when a live
// sibling supersedes it. An un-superseded suspend (its group has NO live watcher)
// is a genuine resumable pause and is left untouched; the resume-later guarantee
// holds until a successor actually takes the surface. A live PID is NEVER retired
// (OS-truth): if two real watchers race the same surface, both survive until one
// dies — we only sweep records that are not themselves alive.
//
// NOTHING LOST — owner directive 2026-07-22: before retiring any record that
// carries salvageable state (a non-boilerplate resume prompt, owned open items,
// a thoth ref, or an in-flight current item), its continuation is inscribed to
// the Stele ledger so the state is durably captured even as the record is swept.
// Owned open items are additionally safe because the live successor shares the
// same agent inbox — they are never stranded, only recorded.
//
// Returns the retired records (empty if none). The registry is saved only when at
// least one record was retired.
func ReapStrayThreads(routerRoot string) ([]ReapedThread, error) {
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	thisMachine := MachineID()

	type groupKey struct{ agent, surface string }
	// Pass 1: find the canonical live watcher per group (newest by last_seen).
	live := map[groupKey]*Thread{}
	for _, t := range reg.Threads {
		if t == nil || !SameMachine(t.MachineID, thisMachine) {
			continue
		}
		if !isLiveWatcher(t) {
			continue
		}
		k := groupKey{t.AgentID, t.Surface}
		if cur, ok := live[k]; !ok || t.LastSeenAt.After(cur.LastSeenAt) {
			live[k] = t
		}
	}
	if len(live) == 0 {
		return nil, nil // nothing anchors a supersession — leave every record be
	}

	// Pass 2: sweep non-live siblings of any group that has a live watcher.
	var retired []ReapedThread
	for _, t := range reg.Threads {
		if t == nil || t.Status.IsTerminal() || !SameMachine(t.MachineID, thisMachine) {
			continue
		}
		anchor, ok := live[groupKey{t.AgentID, t.Surface}]
		if !ok || anchor.ThreadID == t.ThreadID || isLiveWatcher(t) {
			continue // no live anchor, this IS the anchor, or itself still live
		}
		inscribeStraySalvage(t, anchor.ThreadID)
		t.Status = ThreadStatusClosed
		t.LastSeenAt = now
		t.LastError = fmt.Sprintf("retired: superseded by live watcher %s (ADR-024 one-watcher-per-surface) at %s",
			anchor.ThreadID, now.Format(time.RFC3339))
		retired = append(retired, ReapedThread{ThreadID: t.ThreadID, AgentID: t.AgentID, PID: t.PID, State: PIDStateOfThread(t)})
	}
	if len(retired) > 0 {
		if err := SaveThreadRegistry(routerRoot, reg); err != nil {
			// Return nil, not retired: in-memory mutations were never persisted;
			// a caller must not announce a completed sweep that did not persist
			// (sirsi-io #18 amendment — same invariant as ReapDeadThreads).
			return nil, err
		}
	}
	return retired, nil
}

// boilerplateResumePrompts are the auto-generated continuation strings that carry
// no salvageable work (the SessionEnd hook stamps "session ended"). A stray whose
// only state is one of these is an empty tombstone — swept without a ledger entry.
var boilerplateResumePrompts = map[string]bool{"": true, "session ended": true}

// straySalvage checks a soon-to-be-retired stray against durable memory and
// returns its salvageable continuation state, or ok=false when the record is an
// empty tombstone (nothing to save). This is the pure decision half of the
// "nothing lost" guarantee — the check runs on every reap; inscribeStraySalvage
// wraps it with the Stele side effect (Rule A16 keeps the side effect isolated
// and the predicate unit-testable without touching the ledger).
func straySalvage(t *Thread, supersededBy string) (map[string]string, bool) {
	if t == nil {
		return nil, false
	}
	p := t.SuspendPayload
	hasResume := p != nil && !boilerplateResumePrompts[strings.TrimSpace(p.ResumePrompt)] && p.ResumePrompt != ""
	hasThothRef := p != nil && p.ThothRef != ""
	hasOwned := p != nil && len(p.OwnedOpenItems) > 0
	hasCurrent := t.CurrentItem != ""
	if !hasResume && !hasThothRef && !hasOwned && !hasCurrent {
		return nil, false // checked: nothing salvageable — safe to sweep
	}
	data := map[string]string{
		"thread_id":     t.ThreadID,
		"agent":         t.AgentID,
		"surface":       t.Surface,
		"prior_status":  string(t.Status),
		"superseded_by": supersededBy,
	}
	if hasCurrent {
		data["current_item"] = t.CurrentItem
	}
	if hasResume {
		data["resume_prompt"] = p.ResumePrompt
	}
	if hasThothRef {
		data["thoth_ref"] = p.ThothRef
	}
	if hasOwned {
		data["owned_open_items"] = strings.Join(p.OwnedOpenItems, ",")
	}
	return data, true
}

// inscribeStraySalvage inscribes a stray's salvageable state to the Stele ledger
// so nothing is lost (owner directive 2026-07-22). Empty tombstones inscribe
// nothing — the check still runs on every reap; it simply finds nothing to save.
func inscribeStraySalvage(t *Thread, supersededBy string) {
	if data, ok := straySalvage(t, supersededBy); ok {
		stele.Inscribe("horus", stele.TypeThreadReap, t.Repo, data)
	}
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
	// Tiebreak on ThreadID. Ordering by LastSeenAt alone is NOT a total order:
	// sort.Slice is unstable, so any two threads sharing a timestamp come back in
	// arbitrary order that varies run to run. Shared timestamps are routine —
	// heartbeats land in the same second, and a fixed test clock makes every
	// record identical — so this randomly reordered the fleet board between
	// refreshes and randomly reddened CI for every PR in the repo via
	// TestStaleActiveSupervisors.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].LastSeenAt.Equal(out[j].LastSeenAt) {
			return out[i].LastSeenAt.After(out[j].LastSeenAt)
		}
		return out[i].ThreadID < out[j].ThreadID
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
