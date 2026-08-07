// Package routerboard is the Go port of the out-of-repo Python router board
// that served 127.0.0.1:8734.
//
// The Python original earned its behavior the hard way — nearly every rule
// below exists because a specific lie reached the owner. They are ported
// deliberately, not incidentally:
//
//   - Absolute path to the sirsi binary. Under launchd, PATH lacks ~/.local/bin,
//     so a bare "sirsi" failed silently and the board rendered ZEROS as if that
//     were data.
//   - No-store on the HTML. The owner was served a cached page for hours while
//     the new one was verified in a force-refreshed browser and reported fixed.
//   - 503 before the first successful poll, never an all-zero board — zeros read
//     as a dead fleet.
//   - Seed-don't-burst on status diffs: a task's first sighting emits no event,
//     so a restart cannot fake a burst of activity.
//   - UTC-correct ages. Parsing router timestamps as local time shifted every
//     age by the UTC offset and printed negative ages as "-1d20h".
//   - "in progress" is a LIVE GAUGE, not a cumulative tally; "completed" is
//     cumulative because done is terminal.
//   - blocked_tasks is a SUBSET of active_tasks, never a third segment.
//
// Ported to Go on owner directive 2026-08-06: everything on this machine is Go
// unless Python is required or genuinely better. Nothing here required Python.
//
// Live step burndown (owner directive 2026-08-07): "phase 0 task 1 xxx, yyy,
// zzz, 0/100 steps complete" — the owner needs to SEE task progress land in
// real time, not ask an agent for a status report. Two pieces, both already
// present in the schema, neither requiring a migration:
//
//   - Phase-level burndown was ALREADY COMPUTED here (Phase.Total/Done/Active/
//     Blocked/PctDone) and simply never rendered. index.html now has a
//     "Phase burndown" section reading Payload.Board.Phases directly.
//   - Task-level step burndown reuses the existing free-text Outline field —
//     NOT Timeline, which is a real, already-validated day/owner/hours
//     accounting log (routerstore rejects anything else written there; this
//     was tried first and correctly bounced). An agent working a multi-step
//     task writes `--outline @steps.json`, a JSON array of
//     `{"id":"<stable-slug>","step":"<text>","done":true|false}`. `id` is
//     required, not cosmetic: it's the stable handle a later
//     `--outline @file` rewrite uses to flip one specific step, since text
//     matching breaks the moment the wording changes. Steps done/total is
//     DERIVED by the frontend from the array length, never a separate field.
//   - This is a convention every actor (claude-*, codex-*, gemma) is expected
//     to follow on multi-step work, not just claude-nexus — see the owner
//     reporting standard in ~/.claude/CLAUDE.md (chart-first, ELI5 second).
//     Deliberately NOT cited as "A32" here: this repo's own A32 is Load-
//     Bearing Recognition (§2.29) — a different rule, same letter, in a
//     different canon file. Citing it unqualified is exactly the collision
//     class Rule A35 exists to prevent.
package routerboard

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// stallSeconds matches the Python STALL_SECONDS and the v7 liveness contract:
// a pending/in-progress task untouched this long is stalled, not active.
const stallSeconds = 4 * 3600

// activityCap bounds the transition feed. Unbounded growth in a long-lived
// server is a slow leak and nobody scrolls past a few dozen transitions.
const activityCap = 200

// Board polls the router and holds the payload every surface renders.
type Board struct {
	sirsiBin   string
	agentsJSON string

	mu       sync.RWMutex
	payload  []byte // marshaled Payload
	version  uint64 // bumped only when the payload actually CHANGES
	buildID  string
	prev     map[string]string // agent\x00task_id -> last seen status
	activity []Event
	// completed is a cumulative tally of transitions INTO done. Done is
	// terminal, so counting arrivals is the honest metric for it — unlike
	// in-progress, which is counted live.
	completed int
}

func New(sirsiBin, agentsJSON, buildID string) *Board {
	return &Board{
		sirsiBin: sirsiBin, agentsJSON: agentsJSON, buildID: buildID,
		prev: map[string]string{}, activity: []Event{},
	}
}

// Event is one observed status transition.
type Event struct {
	At      string `json:"at"`
	Agent   string `json:"agent"`
	TaskID  string `json:"task_id"`
	Subject string `json:"subject"`
	From    string `json:"from"`
	To      string `json:"to"`
}

// Payload is the whole board. Field names and shapes are the Python original's
// verbatim, because index.html renders them directly and is unchanged: a
// renamed key is an empty cell, and an empty cell reads as "no data" rather
// than as a bug.
type Payload struct {
	Build       string       `json:"build"`
	DataErrors  []string     `json:"data_errors"`
	GeneratedAt string       `json:"generated_at"`
	Counters    Counters     `json:"counters"`
	Activity    []Event      `json:"activity"`
	Fleet       []Lane       `json:"fleet"`
	Board       BoardSummary `json:"board"`
	// Ledger is the SAME BoardSummary under the key index.html actually reads.
	// The UI renders d.ledger; the payload carried only d.board, so every tile
	// bound to it read undefined and the page displayed nonsense while the API
	// endpoints returned correct numbers — the board was "broken" and the CLI
	// was right at the same moment. Emitting both keeps the page working without
	// editing it: index.html is the surface the owner reads, and renaming its
	// field to match mine would have risked the very thing being repaired.
	Ledger           BoardSummary `json:"ledger"`
	Threads          []Thread     `json:"threads"`
	RegistrationGaps []string     `json:"registration_gaps"`
	Tasks            []TaskDetail `json:"tasks"`
	SchemaBanner     string       `json:"schema_banner,omitempty"`
}

type Counters struct {
	Completed     int `json:"completed"`
	InProgressNow int `json:"in_progress_now"`
	Total         int `json:"total"`
	Blocked       int `json:"blocked"`
	Pending       int `json:"pending"`
	Done          int `json:"done"`
}

type Registered struct {
	Router bool `json:"router"`
	Thread bool `json:"thread"`
	Ledger bool `json:"ledger"`
}

type Consumer struct {
	Armed bool   `json:"armed"`
	Loop  string `json:"loop"`
}

type Item struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	From       string   `json:"from"`
	Type       string   `json:"type"`
	AgeSeconds *float64 `json:"age_seconds"`
	Opened     string   `json:"opened"`
	Picked     bool     `json:"picked"`
	Blocked    bool     `json:"blocked"`
	Stale      bool     `json:"stale"`
}

type Lane struct {
	Agent          string         `json:"agent"`
	Type           string         `json:"type"`
	WakeState      string         `json:"wake_state"`
	Activity       string         `json:"activity"`
	OpenItems      int            `json:"open_items"`
	Items          []Item         `json:"items"`
	OldestMsgStale bool           `json:"oldest_msg_stale"`
	TasksTotal     int            `json:"tasks_total"`
	Counts         map[string]int `json:"counts"`
	UnblockedOpen  int            `json:"unblocked_open"`
	IdleWithWork   bool           `json:"idle_with_work"`
	Registered     Registered     `json:"registered"`
	LastTouch      string         `json:"last_touch"`
	Tasks          []rawTask      `json:"tasks"`
	Consumer       *Consumer      `json:"consumer"`
}

type Thread struct {
	ThreadID    string   `json:"thread_id"`
	Agent       string   `json:"agent"`
	Workstream  string   `json:"workstream"`
	IdleSeconds *float64 `json:"idle_seconds"`
	Stale       bool     `json:"stale"`
}

type Phase struct {
	Name    string  `json:"name"`
	Total   int     `json:"total"`
	Done    int     `json:"done"`
	Active  int     `json:"active"`
	Blocked int     `json:"blocked"`
	PctDone float64 `json:"pct_done"`
}

type Blocker struct {
	Agent  string `json:"agent"`
	ItemID string `json:"item_id"`
	Title  string `json:"title"`
	Age    string `json:"age"`
}

type BoardSummary struct {
	TotalTasks   int       `json:"total_tasks"`
	DoneTasks    int       `json:"done_tasks"`
	ActiveTasks  int       `json:"active_tasks"`
	BlockedTasks int       `json:"blocked_tasks"`
	PctDone      float64   `json:"pct_done"`
	Phases       []Phase   `json:"phases"`
	OpenItems    int       `json:"open_items"`
	BlockedItems int       `json:"blocked_items"`
	Blockers     []Blocker `json:"blockers"`
}

type TaskDetail struct {
	TaskID           string        `json:"task_id"`
	Agent            string        `json:"agent"`
	Subject          string        `json:"subject"`
	Status           string        `json:"status"`
	BlockedBy        *string       `json:"blocked_by"`
	ResponsibleParty interface{}   `json:"responsible_party"`
	Updated          string        `json:"updated"`
	Age              string        `json:"age"`
	Liveness         string        `json:"liveness"`
	Charter          interface{}   `json:"charter"`
	CommissionedAt   interface{}   `json:"commissioned_at"`
	CommissionedBy   interface{}   `json:"commissioned_by"`
	Outline          interface{}   `json:"outline"`
	Timeline         []interface{} `json:"timeline"`
	Links            []interface{} `json:"links"`
	TestState        string        `json:"test_state"`
	Stage            string        `json:"stage"`
	TokensConsumed   int           `json:"tokens_consumed"`
	DurationSeconds  int           `json:"duration_seconds"`
}

// rawTask keeps the ledger's task object intact so unknown/new fields survive
// the round trip. Re-marshaling a struct would silently drop any v7 field this
// build does not know about, which is how a board starts under-reporting.
type rawTask map[string]interface{}

func (t rawTask) str(k string) string {
	if v, ok := t[k].(string); ok {
		return v
	}
	return ""
}

// runJSON executes a sirsi subcommand and decodes its JSON.
//
// Absolute binary path, and a non-zero exit is recorded as a DATA ERROR rather
// than swallowed: the Python original rendered zeros for hours because a bare
// "sirsi" was not on launchd's PATH and the failure was silent.
func (b *Board) runJSON(ctx context.Context, timeout time.Duration, args []string, out interface{}, errs *[]string) bool {
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(c, b.sirsiBin, args...)
	stdout, err := cmd.Output()
	label := strings.Join(args, " ")
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: %v", label, err))
		return false
	}
	// The CLI prefixes glyphs/ANSI on some paths; decode from the first brace
	// or bracket rather than assuming the stream starts clean.
	s := stdout
	if i := indexAny(s, '{', '['); i > 0 {
		s = s[i:]
	}
	if len(strings.TrimSpace(string(s))) == 0 {
		*errs = append(*errs, label+": empty output")
		return false
	}
	if err := json.Unmarshal(s, out); err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: decode: %v", label, err))
		return false
	}
	return true
}

func indexAny(b []byte, a, c byte) int {
	for i, x := range b {
		if x == a || x == c {
			return i
		}
	}
	return -1
}

// ageSeconds returns seconds since an RFC3339 UTC router timestamp.
//
// Parsed as UTC explicitly. The Python original used a local-time parse at
// first, which shifted every age by the UTC offset: stalls under-reported by
// four hours and negative ages printed as "-1d20h".
func ageSeconds(ts string) (float64, bool) {
	if len(ts) < 19 {
		return 0, false
	}
	t, err := time.Parse("2006-01-02T15:04:05", ts[:19])
	if err != nil {
		return 0, false
	}
	return time.Since(t.UTC()).Seconds(), true
}

func ageStr(ts string) string {
	secs, ok := ageSeconds(ts)
	if !ok {
		return "?"
	}
	if secs < 0 {
		secs = 0 // clock skew must read 0m, never a negative duration
	}
	d := int(secs) / 86400
	h := (int(secs) % 86400) / 3600
	if d > 0 {
		return fmt.Sprintf("%dd%dh", d, h)
	}
	return fmt.Sprintf("%dh%dm", h, (int(secs)%3600)/60)
}

// Version reports the current payload version. Callers stream on change.
func (b *Board) Version() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.version
}

// Snapshot returns the marshaled payload and its version. version 0 means no
// successful poll has completed — callers must 503 rather than serve zeros.
func (b *Board) Snapshot() ([]byte, uint64) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.payload, b.version
}
