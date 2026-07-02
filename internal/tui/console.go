package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// toastLife is how long a transient banner stays before auto-dismissing (§1.1:
// toasts auto-dismiss and never steal focus).
const toastLife = 5 * time.Second

// The five-screen operator console (ADR-020, docs/TUI_DESIGN_PROOF.md).
//
// This supersedes the Gate-2 fixture scaffold (Scan/Ra/Inbox proof views). The
// console is a projection of wired state: navigation resolves through the
// command Registry, screen data is loaded lazily through the injectable Runner,
// and every screen renders inside the same Frame chrome. The cap is LAW — there
// are exactly five screens (Pulse, Waste, Ghosts, Health, Activity).

// loadState is a screen's data-loading lifecycle. Every screen renders an
// explicit loading / empty / error state — never a blank frame (quality bar).
type loadState int

const (
	stateIdle    loadState = iota // not yet requested
	stateLoading                  // request in flight
	stateReady                    // data present
	stateError                    // load failed; err holds the reason
)

// Screen is one of the five console screens. A screen owns its own data, its
// drill-in state, and its rendering; it never draws the Frame chrome (title /
// status rows) — the console does. Screens are value-semantic where possible so
// the model stays easy to test.
type Screen interface {
	// Name is the title-bar label.
	Name() string
	// Sigil is the safe deity/status sigil name (glyph.go) for this screen.
	Sigil() string
	// Layout is the named tiling the screen wants (§1.3).
	Layout() Layout
	// HintIDs are the status-bar hints, in order. Every id MUST be registered
	// and keyed — validateScreens enforces the §7 delta-2 guarantee.
	HintIDs() []CommandID
	// Load returns the tea.Cmd that fetches this screen's data. The resulting
	// tea.Msg is delivered back to Update; the screen decides how to apply it.
	Load() tea.Cmd
	// Update applies a message (a load result, a key action) and returns the
	// evolved screen plus an optional follow-up command.
	Update(msg tea.Msg, caps Capabilities) (Screen, tea.Cmd)
	// View renders the screen body into the content region (no chrome). width and
	// height are the interior cell budget.
	View(width, height int, caps Capabilities) []string
	// RightMeta is the right-anchored title-bar field (e.g. "34% used", "12 apps").
	RightMeta() string
	// State reports the load lifecycle so the console can validate transitions.
	State() loadState
}

// App is the whole-console bubbletea model. It owns the five screens, the active
// index, the shared registry and capabilities, a transient toast, and a help
// overlay flag. There is no path by which a key mutates the surface except
// through a registered command routed here — the v0.22 "keys did nothing"
// failure is unrepresentable.
type App struct {
	screens  []Screen
	active   int
	reg      *Registry
	caps     Capabilities
	width    int
	height   int
	toast    *Toast
	helpOpen bool
	quitting bool
}

// NewApp builds the console over the five screens with the canonical registry.
// Every screen is validated up front so an unwired hint fails at construction,
// not at render.
func NewApp(caps Capabilities) (*App, error) {
	reg, err := DefaultRegistry()
	if err != nil {
		return nil, err
	}
	screens := []Screen{
		newPulseScreen(),
		newWasteScreen(),
		newGhostsScreen(),
		newHealthScreen(),
		newActivityScreen(),
	}
	if err := validateScreens(reg, screens); err != nil {
		return nil, err
	}
	return &App{
		screens: screens,
		active:  0,
		reg:     reg,
		caps:    caps,
		width:   MinWidth,
		height:  MinHeight,
	}, nil
}

// validateScreens asserts every screen's hints resolve to registered, keyed
// commands (§7 delta 2). A dead hint cannot ship.
func validateScreens(reg *Registry, screens []Screen) error {
	if len(screens) != 5 {
		return fmt.Errorf("tui: console must have exactly 5 screens, got %d", len(screens))
	}
	for _, s := range screens {
		if _, err := reg.Hints(s.HintIDs()); err != nil {
			return fmt.Errorf("tui: screen %q: %w", s.Name(), err)
		}
	}
	return nil
}

// activeScreen returns the focused screen.
func (a *App) activeScreen() Screen { return a.screens[a.active] }

// Init loads the first (Pulse) screen and arms the recurring gauge tick. Startup
// stays under budget by lazy-loading every other screen on first focus, never
// blocking the first frame on a scan (quality bar: <100ms startup).
func (a *App) Init() tea.Cmd {
	return tea.Batch(a.activeScreen().Load(), pulseScheduleTick())
}

// Update routes messages. Window resizes update the render box. Key presses
// resolve through the registry to a command; global commands act on App,
// everything else flows to the active screen. Non-key messages (load results,
// tick) are delivered to the active screen too — but a stale result addressed to
// a screen the user has left is simply ignored by that screen.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		return a, nil

	case tea.KeyPressMsg:
		return a.handleKey(msg.String())

	case toastExpireMsg:
		a.toast = nil
		return a, nil

	case pulseTickMsg:
		// The App owns the recurring gauge tick: re-arm it every time, and forward
		// the tick to the active screen (only Pulse acts on it; others ignore it).
		// Re-arming here — not inside a screen — keeps the loop alive regardless of
		// which screen is focused and gives the screen a pure, testable Update.
		next, cmd := a.activeScreen().Update(msg, a.caps)
		a.screens[a.active] = next
		return a, tea.Batch(cmd, pulseScheduleTick())

	case dispatchDone:
		// An action's result. On failure, raise a danger toast (proof §4: action
		// failure → toast, persists until acknowledged) in addition to the screen's
		// own inline error, so a failed clean/fix/relieve is never silent. Then
		// forward to the screen so it can clear its in-flight state.
		var cmd tea.Cmd
		if msg.err != nil {
			a.setToast(&Toast{Text: msg.kind + " failed: " + msg.err.Error(), Token: TokDanger})
			cmd = tea.Tick(toastLife, func(time.Time) tea.Msg { return toastExpireMsg{} })
		}
		next, screenCmd := a.activeScreen().Update(msg, a.caps)
		a.screens[a.active] = next
		return a, tea.Batch(cmd, screenCmd)

	default:
		// Data messages (load results) go to the active screen.
		next, cmd := a.activeScreen().Update(msg, a.caps)
		a.screens[a.active] = next
		return a, cmd
	}
}

// handleKey resolves a keypress to a wired command and dispatches it. An
// unresolved key is a no-op (never a silent partial action). The help overlay
// captures the next key: any key dismisses it.
func (a *App) handleKey(key string) (tea.Model, tea.Cmd) {
	if a.helpOpen {
		a.helpOpen = false
		return a, nil
	}
	cmd, ok := a.reg.ResolveKey(key)
	if !ok {
		return a, nil
	}
	switch cmd.ID {
	case CmdQuit:
		a.quitting = true
		return a, tea.Quit
	case CmdHelp:
		a.helpOpen = true
		return a, nil
	case CmdTab:
		return a.focus((a.active + 1) % len(a.screens))
	case CmdScreenPulse:
		return a.focus(0)
	case CmdScreenWaste:
		return a.focus(1)
	case CmdScreenGhosts:
		return a.focus(2)
	case CmdScreenHealth:
		return a.focus(3)
	case CmdScreenActivity:
		return a.focus(4)
	default:
		// Screen-scoped command (move, inspect, clean, fix, toggle, refresh, back).
		next, teacmd := a.activeScreen().Update(keyMsg{cmd: cmd}, a.caps)
		a.screens[a.active] = next
		return a, teacmd
	}
}

// focus switches to screen i, lazily loading it the first time it is shown so
// the first frame never blocks on a scan.
func (a *App) focus(i int) (tea.Model, tea.Cmd) {
	a.active = i
	if a.activeScreen().State() == stateIdle {
		return a, a.activeScreen().Load()
	}
	return a, nil
}

// keyMsg carries a resolved command into a screen's Update. Screens switch on
// cmd.ID; they never see a raw keystroke, so a screen cannot act on an unwired
// key (delta 2).
type keyMsg struct{ cmd Command }

// toastExpireMsg dismisses the current toast.
type toastExpireMsg struct{}

// setToast is a helper screens use (via the returned command) to raise a
// transient banner. It is applied by App, then auto-dismissed.
func (a *App) setToast(t *Toast) { a.toast = t }

// View projects the whole console: title row, framed content, status row, with
// an optional toast above the status row and an optional help overlay. The
// alternate screen is requested here (bubbletea v2 carries it on the View) when
// the capability profile allows it, so quitting restores the shell cleanly.
func (a *App) View() tea.View {
	v := tea.NewView(strings.Join(a.render(), "\n"))
	v.AltScreen = a.caps.AltScreen
	return v
}

// render builds the full frame as lines. It is separated from View so tests can
// assert on the exact rendered strings (golden frames) without a terminal.
func (a *App) render() []string {
	if a.width < MinWidth || a.height < MinHeight {
		return tooSmall(a.width, a.height)
	}
	if a.helpOpen {
		return a.renderHelp()
	}

	scr := a.activeScreen()
	out := make([]string, 0, a.height)
	out = append(out, a.titleBar())

	// Content region: total height minus title, status, and toast (if any).
	reserved := 2 // title + status
	if a.toast != nil {
		reserved++
	}
	interiorH := a.height - reserved
	body := scr.View(a.width, interiorH, a.caps)
	for i := 0; i < interiorH; i++ {
		if i < len(body) {
			out = append(out, clampLine(body[i], a.width))
		} else {
			out = append(out, "")
		}
	}
	if a.toast != nil {
		out = append(out, a.renderToast())
	}
	out = append(out, a.statusBar())
	return out
}

// titleBar builds row 0: the console sigil, name, active screen, and the
// screen's right-anchored meta field (§2.2).
func (a *App) titleBar() string {
	scr := a.activeScreen()
	bullet := Sigil("bullet", a.caps)
	left := fmt.Sprintf("%s %s %s %s",
		Paint(Sigil("horus", a.caps), TokBrand, a.caps),
		Paint("Horus", TokBrand, a.caps),
		Paint(bullet, TokDim, a.caps),
		Sigil(scr.Sigil(), a.caps)+" "+scr.Name(),
	)
	right := scr.RightMeta()
	// Pad using display width (styling adds invisible bytes, so measure the plain
	// left label length, not the styled string).
	plainLeft := fmt.Sprintf("%s Horus %s %s", Sigil("horus", a.caps), bullet, Sigil(scr.Sigil(), a.caps)+" "+scr.Name())
	gap := a.width - len([]rune(plainLeft)) - len([]rune(right))
	if gap < 1 {
		return clampLine(left, a.width)
	}
	return left + strings.Repeat(" ", gap) + Paint(right, TokDim, a.caps)
}

// statusBar builds the last row from the active screen's registered hints plus a
// mode chip and the screen count. A hint that references an unwired command
// cannot render — the registry refuses to build it (§7 delta 2).
func (a *App) statusBar() string {
	scr := a.activeScreen()
	hints, err := a.reg.Hints(scr.HintIDs())
	if err != nil {
		return clampLine("status unavailable", a.width)
	}
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = fmt.Sprintf("%s %s", Paint(h.Key, TokBrand, a.caps), Paint(h.Label, TokDim, a.caps))
	}
	sep := " " + Paint(Sigil("bullet", a.caps), TokDim, a.caps) + " "
	left := strings.Join(parts, sep)
	chip := Chip(strings.ToUpper(scr.Name()), a.caps)
	// Right side: mode chip. Left-align hints; the chip floats right.
	plainLeftLen := 0
	for i, h := range hints {
		if i > 0 {
			plainLeftLen += 3 // " · "
		}
		plainLeftLen += len([]rune(h.Key)) + 1 + len([]rune(h.Label))
	}
	plainChipLen := len([]rune(scr.Name())) + 2
	gap := a.width - plainLeftLen - plainChipLen
	if gap < 1 {
		return clampLine(left, a.width)
	}
	return left + strings.Repeat(" ", gap) + chip
}

// renderToast renders the transient banner above the status bar, right-anchored,
// carrying its severity token AND its text so meaning survives without color.
func (a *App) renderToast() string {
	t := a.toast
	label := t.Token.SeverityLabel()
	text := t.Text
	if label != "" {
		text = label + " " + text
	}
	styled := Paint(text, t.Token, a.caps)
	gap := a.width - len([]rune(text))
	if gap < 0 {
		return clampLine(styled, a.width)
	}
	return strings.Repeat(" ", gap) + styled
}

// renderHelp draws the full-frame help overlay: the reserved key set and the
// five screens. Any key dismisses it.
func (a *App) renderHelp() []string {
	lines := []string{
		Paint("  Horus — Operator Console", TokBrand, a.caps),
		"",
		Paint("  SCREENS", TokBrand, a.caps),
		"    1  Pulse      memory-first: RAM gauge, pressure, top hogs, relieve",
		"    2  Waste      scan → review → clean, with freed-space proof",
		"    3  Ghosts     residuals of uninstalled apps → clean",
		"    4  Health     diagnose findings, each with its honest one-key fix",
		"    5  Activity   provenance ledger — what sirsi actually changed",
		"",
		Paint("  GLOBAL", TokBrand, a.caps),
		"    tab  next screen      r  refresh        ? help",
		"    ↑↓   move             enter  inspect     esc back",
		"    g/G  top / bottom     q  quit",
		"",
		Paint("  PER SCREEN", TokBrand, a.caps),
		"    s  rescan (Waste)     spc toggle item    c  clean (confirm)",
		"    f  apply fix (Health) d  re-diagnose",
		"",
		Paint("  press any key to close", TokDim, a.caps),
	}
	out := make([]string, 0, a.height)
	for i := 0; i < a.height; i++ {
		if i < len(lines) {
			out = append(out, clampLine(lines[i], a.width))
		} else {
			out = append(out, "")
		}
	}
	return out
}

// clampLine guarantees a line never exceeds width DISPLAY cells. It measures the
// visible width (ignoring ANSI escape bytes) so styled strings are not truncated
// mid-escape. Every glyph the renderer places is width 1 (glyph.go), so visible
// rune count equals display width.
func clampLine(s string, width int) string {
	if visibleWidth(s) <= width {
		return s
	}
	// Walk runes, skipping ANSI escape sequences, until we hit the width budget.
	var b strings.Builder
	visible := 0
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\x1b' {
			// Copy the whole escape sequence (through the terminating letter).
			b.WriteRune(r)
			for i+1 < len(runes) {
				i++
				b.WriteRune(runes[i])
				if (runes[i] >= 'a' && runes[i] <= 'z') || (runes[i] >= 'A' && runes[i] <= 'Z') {
					break
				}
			}
			continue
		}
		if visible >= width {
			break
		}
		b.WriteRune(r)
		visible++
	}
	return b.String()
}

// visibleWidth returns the display width of s, ignoring ANSI escape sequences.
func visibleWidth(s string) int {
	n := 0
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\x1b' {
			for i+1 < len(runes) {
				i++
				if (runes[i] >= 'a' && runes[i] <= 'z') || (runes[i] >= 'A' && runes[i] <= 'Z') {
					break
				}
			}
			continue
		}
		n++
	}
	return n
}

// tooSmall renders the single centered "needs >= 80x24" takeover (§2.2).
func tooSmall(width, height int) []string {
	msg := fmt.Sprintf("Horus needs >= %dx%d", MinWidth, MinHeight)
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		if i == height/2 && width >= len(msg) {
			pad := strings.Repeat(" ", (width-len(msg))/2)
			out = append(out, clampLine(pad+msg, width))
		} else {
			out = append(out, "")
		}
	}
	if len(out) == 0 {
		out = append(out, clampLine(msg, width))
	}
	return out
}
