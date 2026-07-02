package tui

import "fmt"

// Command registry (docs/TUI_DESIGN_PROOF.md §3.4, §7 delta 2).
//
// v0.22's fatal trick was DECLARING key bindings that dispatched nowhere. Here
// every binding resolves through this registry, and — the structural guarantee
// — the status-bar hints are GENERATED from registered commands. A hint cannot
// exist for an unwired key, because the hint is a projection of the registry,
// not a hand-written string. ValidateScreen turns that guarantee into a test.

// CommandID is a stable command identifier. Where a command mirrors a CLI verb
// it uses the same id, so there is no parallel TUI verb list to drift (delta 5).
type CommandID string

// Canonical command IDs for the five-screen operator console. Global keys are
// the reserved set (§3.2); per-screen verbs mirror the live CLI (scan, clean,
// relieve, ghosts, diagnose) so the console dispatches the same engine the CLI
// does — there is no parallel TUI verb list to drift (delta 5).
const (
	// Screen jumps (1–5) — one per screen, always wired.
	CmdScreenPulse    CommandID = "screen.pulse"
	CmdScreenWaste    CommandID = "screen.waste"
	CmdScreenGhosts   CommandID = "screen.ghosts"
	CmdScreenHealth   CommandID = "screen.health"
	CmdScreenActivity CommandID = "screen.activity"

	// Global navigation / meta.
	CmdTab      CommandID = "tab"     // next screen
	CmdRefresh  CommandID = "refresh" // u — reload/update the focused screen (proof §3.2: "u update")
	CmdHelp     CommandID = "help"    // ? — help overlay
	CmdQuit     CommandID = "quit"    // q — quit
	CmdBack     CommandID = "back"    // esc — pop detail / dismiss overlay
	CmdMoveUp   CommandID = "move.up"
	CmdMoveDown CommandID = "move.down"
	CmdTop      CommandID = "top"    // g
	CmdBottom   CommandID = "bottom" // G

	// Per-screen verbs — mirror the CLI cobra tree.
	CmdInspect CommandID = "inspect" // enter — drill into the selected row
	CmdRelieve CommandID = "relieve" // r — memory relief (Pulse hero beat; proof §3.2: "r relieve")
	CmdScan    CommandID = "scan"    // s — rescan (Waste)
	CmdToggle  CommandID = "toggle"  // space — toggle a review item
	CmdClean   CommandID = "clean"   // c — clean (destructive; confirm-gated)
	CmdFix     CommandID = "fix"     // f — apply a finding's one-key fix (Health)
	CmdDiag    CommandID = "diagnose"
)

// Command is a single wired action. Key is the zero-keystroke binding shown in
// the status bar (empty means the command is action-only, dispatched by the
// screen without a status hint). Hint is the terse verb shown beside the key.
// Destructive commands never execute from the keystroke alone — the reducer
// routes them to a confirm modal (§4, Rule A1).
type Command struct {
	ID          CommandID
	Title       string // palette / fuzzy-search name
	Key         string // status-bar key, e.g. "enter", "c", "/"; "" = no hint
	Hint        string // status-bar verb, e.g. "inspect"
	Destructive bool
}

// Registry is the single source of truth for wired commands. It is the backing
// that every rendered affordance (status hint) projects from.
type Registry struct {
	byID  map[CommandID]Command
	byKey map[string]CommandID
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		byID:  make(map[CommandID]Command),
		byKey: make(map[string]CommandID),
	}
}

// Register wires a command. A duplicate id or a key already bound to a different
// command is a programming error and returns an error so wiring mistakes surface
// in tests rather than as silent dead keys.
func (r *Registry) Register(c Command) error {
	if c.ID == "" {
		return fmt.Errorf("tui: command with empty id")
	}
	if _, dup := r.byID[c.ID]; dup {
		return fmt.Errorf("tui: command %q already registered", c.ID)
	}
	if c.Key != "" {
		if existing, clash := r.byKey[c.Key]; clash {
			return fmt.Errorf("tui: key %q already bound to %q", c.Key, existing)
		}
		r.byKey[c.Key] = c.ID
	}
	r.byID[c.ID] = c
	return nil
}

// Lookup returns the command for id.
func (r *Registry) Lookup(id CommandID) (Command, bool) {
	c, ok := r.byID[id]
	return c, ok
}

// ResolveKey maps a keypress to its wired command, the only path by which a key
// produces an action.
func (r *Registry) ResolveKey(key string) (Command, bool) {
	id, ok := r.byKey[key]
	if !ok {
		return Command{}, false
	}
	return r.byID[id], true
}

// IDs returns every registered command id.
func (r *Registry) IDs() []CommandID {
	out := make([]CommandID, 0, len(r.byID))
	for id := range r.byID {
		out = append(out, id)
	}
	return out
}

// DefaultRegistry wires the canonical console command set once. Screens
// reference these ids by name; they never re-register, so there is a single
// source of truth for every wired key. Destructive verbs (clean) are flagged so
// the reducer routes them through a confirm modal rather than firing on a key.
func DefaultRegistry() (*Registry, error) {
	reg := NewRegistry()
	cmds := []Command{
		// Screen jumps.
		{ID: CmdScreenPulse, Title: "Pulse (memory)", Key: "1", Hint: "pulse"},
		{ID: CmdScreenWaste, Title: "Waste (scan & clean)", Key: "2", Hint: "waste"},
		{ID: CmdScreenGhosts, Title: "Ghosts (app residuals)", Key: "3", Hint: "ghosts"},
		{ID: CmdScreenHealth, Title: "Health (diagnose)", Key: "4", Hint: "health"},
		{ID: CmdScreenActivity, Title: "Activity (ledger)", Key: "5", Hint: "activity"},

		// Global navigation / meta.
		{ID: CmdTab, Title: "Next screen", Key: "tab", Hint: "next"},
		{ID: CmdRefresh, Title: "Update screen data", Key: "u", Hint: "update"},
		{ID: CmdHelp, Title: "Help", Key: "?", Hint: "help"},
		{ID: CmdQuit, Title: "Quit", Key: "q", Hint: "quit"},
		{ID: CmdBack, Title: "Back / dismiss", Key: "esc", Hint: "back"},
		{ID: CmdMoveUp, Title: "Move up", Key: "up", Hint: "move"},
		{ID: CmdMoveDown, Title: "Move down", Key: "down", Hint: "move"},
		{ID: CmdTop, Title: "Top of list", Key: "g", Hint: "top"},
		{ID: CmdBottom, Title: "Bottom of list", Key: "G", Hint: "bottom"},

		// Per-screen verbs.
		{ID: CmdInspect, Title: "Inspect selection", Key: "enter", Hint: "inspect"},
		{ID: CmdScan, Title: "Rescan for waste", Key: "s", Hint: "rescan"},
		{ID: CmdToggle, Title: "Toggle item", Key: " ", Hint: "toggle"},
		{ID: CmdClean, Title: "Clean selected", Key: "c", Hint: "clean", Destructive: true},
		{ID: CmdFix, Title: "Apply fix", Key: "f", Hint: "fix"},
		{ID: CmdDiag, Title: "Re-run diagnostics", Key: "d", Hint: "diagnose"},
		// Relieve is the Pulse hero beat, bound to r (proof §3.2: "r relieve").
		{ID: CmdRelieve, Title: "Relieve memory pressure", Key: "r", Hint: "relieve"},
	}
	for _, c := range cmds {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

// Hint is a rendered status-bar affordance: a key and the verb it triggers.
type Hint struct {
	Key   string
	Label string
}

// Hints projects the given command ids into status-bar hints, preserving order.
// It returns an error if any id is not registered or has no key — making
// "a visible hint references an unregistered/unwired command" a test failure,
// exactly the §7 delta-2 guarantee. Only registry-backed hints can be rendered.
func (r *Registry) Hints(ids []CommandID) ([]Hint, error) {
	hints := make([]Hint, 0, len(ids))
	for _, id := range ids {
		c, ok := r.byID[id]
		if !ok {
			return nil, fmt.Errorf("tui: hint references unregistered command %q", id)
		}
		if c.Key == "" {
			return nil, fmt.Errorf("tui: hint references action-only command %q (no key)", id)
		}
		hints = append(hints, Hint{Key: displayKey(c.Key), Label: c.Hint})
	}
	return hints, nil
}

// displayKey renders a key for the status bar. The space key prints as "spc" so
// the hint is legible rather than an invisible gap.
func displayKey(k string) string {
	if k == " " {
		return "spc"
	}
	return k
}
