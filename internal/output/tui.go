// Package output — Pantheon TUI
//
// The primary interface for Pantheon. When the user types `pantheon` with no
// subcommand, this TUI launches. It is a persistent session: commands execute
// inside the TUI, output streams into a viewport, and the input bar re-enables
// when the command completes. The user stays in Pantheon until they explicitly quit.
package output

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// ── Deity Definitions ─────────────────────────────────────────────────

type deityInfo struct {
	Key   string
	Glyph string
	Name  string
	Role  string // Short role (shown in roster)
}

// Canonical deity roster — ordered by hierarchy (Rule D6).
// Two-word roles: verb/adjective + noun. Fits in a 30-col grid cell.
var deityRoster = []deityInfo{
	{"ra", "𓇶", "Ra", "Agent Orchestrator"},
	{"neith", "𓁯", "Neith", "Context Weaver"},
	{"thoth", "𓁟", "Thoth", "Session Memory"},
	{"maat", "𓆄", "Ma'at", "Quality Gate"},
	{"isis", "𓁐", "Isis", "Code Remedy"},
	{"seshat", "𓁆", "Seshat", "Knowledge Bridge"},
	{"horus", "𓂀", "Horus", "Storage Index"},
	{"anubis", "𓃣", "Anubis", "System Jackal"},
	{"ka", "𓂓", "Ka", "Ghost Hunter"},
	{"sekhmet", "𓁵", "Sekhmet", "System Watchdog"},
	{"hapi", "𓈗", "Hapi", "Hardware Profiler"},
	{"khepri", "𓆣", "Khepri", "Fleet Scanner"},
	{"seba", "𓇽", "Seba", "Arch Mapper"},
	{"osiris", "𓁹", "Osiris", "State Keeper"},
	{"hathor", "𓉡", "Hathor", "File Dedup"},
}

// intentKeywords maps natural-language keywords to deity keys for routing.
var intentKeywords = map[string][]string{
	"ra":      {"deploy", "orchestrate", "sprint", "agent", "watch", "command center"},
	"neith":   {"scope", "weave", "context", "canon", "align", "tile", "drift"},
	"thoth":   {"memory", "sync", "compact", "journal", "remember", "persist"},
	"maat":    {"quality", "audit", "coverage", "test", "lint", "feather", "gate", "qa"},
	"isis":    {"fix", "heal", "remediate", "repair", "auto-fix"},
	"seshat":  {"knowledge", "graft", "ingest", "notes", "gemini", "notebooklm"},
	"horus":   {"index", "storage", "manifest", "filesystem", "launch services"},
	"anubis":  {"scan", "waste", "clean", "judge", "purge", "hygiene", "infrastructure"},
	"ka":      {"ghost", "dead", "remnant", "uninstall", "residual", "haunt"},
	"sekhmet": {"guard", "watchdog", "monitor", "ram", "cpu", "doctor", "process"},
	"hapi":    {"gpu", "vram", "hardware", "accelerator", "ane", "cuda", "metal", "npu"},
	"khepri":  {"network", "fleet", "subnet", "container", "docker", "kubernetes"},
	"seba":    {"architecture", "topology", "diagram", "map", "dependency", "graph"},
	"osiris":  {"checkpoint", "state", "preserve", "restore"},
	"hathor":  {"dedup", "duplicate", "mirror", "ranking"},
}

// Top-level CLI aliases that bypass intent matching.
// These map user shorthand to the deity that owns the verb.
var cliAliases = map[string]string{
	"scan":    "anubis",
	"hunt":    "anubis",
	"ghosts":  "ka",
	"dedup":   "anubis",
	"guard":   "sekhmet",
	"doctor":  "sekhmet",
	"version": "version",
}

// ── TUI State ─────────────────────────────────────────────────────────

type tuiMode int

const (
	modeIdle tuiMode = iota
	modeRunning
)

type TUIModel struct {
	width  int
	height int

	input    textinput.Model
	viewport viewport.Model

	outputLines  []string
	mode         tuiMode
	runningDeity string
	runningCmd   string
	spinner      spinner.Model
	history      []historyEntry

	// Inline predictions + history recall
	cmdHistory   []string // deduplicated command strings for up-arrow
	historyIdx   int      // -1 = not browsing; 0..len-1 = position
	historySaved string   // input text saved when user starts browsing

	activeDeity map[string]bool
	steleReader *stele.Reader
	quitting    bool
}

type historyEntry struct {
	deity, command, output string
}

// ── Messages ──────────────────────────────────────────────────────────

type refreshMsg time.Time
type cmdBatchMsg struct {
	lines []string
	err   error
}

func refreshTick() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg { return refreshMsg(t) })
}

// ── Constructor ───────────────────────────────────────────────────────

func NewTUIModel() TUIModel {
	ti := textinput.New()
	ti.Placeholder = "scan my dev environment for ghost processes and dead symlinks"
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 76
	ti.PromptStyle = lipgloss.NewStyle().Foreground(Gold).Bold(true)
	ti.Prompt = "𓉴 "
	ti.TextStyle = lipgloss.NewStyle().Foreground(White)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(Gold)

	// Fish-shell-style inline predictions
	ti.ShowSuggestions = true
	ti.CompletionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	ti.KeyMap.AcceptSuggestion = key.NewBinding(key.WithKeys("right"))
	ti.KeyMap.NextSuggestion = key.NewBinding() // unbind — Up is for history
	ti.KeyMap.PrevSuggestion = key.NewBinding() // unbind — Down is for history
	ti.SetSuggestions(topLevelCommands)

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(Gold)

	vp := viewport.New(80, 10)

	m := TUIModel{
		input:       ti,
		viewport:    vp,
		spinner:     sp,
		width:       100,
		height:      40,
		mode:        modeIdle,
		historyIdx:  -1,
		activeDeity: make(map[string]bool),
		steleReader: stele.NewReader("tui"),
	}
	m.refreshActive()
	return m
}

func (m TUIModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, refreshTick())
}

// ── Update ────────────────────────────────────────────────────────────

func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = min(msg.Width-8, 80)
		m.recalcViewportHeight()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case cmdBatchMsg:
		return m.handleBatchOutput(msg)

	case refreshMsg:
		m.refreshActive()
		return m, refreshTick()

	case spinner.TickMsg:
		if m.mode == modeRunning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	if m.mode == modeIdle {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m TUIModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit

	case tea.KeyEsc:
		if m.mode == modeRunning {
			return m, nil
		}
		if len(m.outputLines) > 0 {
			m.outputLines = nil
			m.viewport.SetContent("")
			m.input.Placeholder = "scan my dev environment for ghost processes and dead symlinks"
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit

	case tea.KeyEnter:
		if m.mode == modeRunning {
			return m, nil
		}
		raw := strings.TrimSpace(m.input.Value())
		if raw == "" {
			return m, nil
		}
		if raw == "q" || raw == "quit" || raw == "exit" {
			m.quitting = true
			return m, tea.Quit
		}
		if raw == "clear" {
			m.outputLines = nil
			m.viewport.SetContent("")
			m.input.Reset()
			return m, nil
		}
		m.historyIdx = -1
		return m.executeCommand(raw)

	case tea.KeyUp:
		if m.mode == modeRunning {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		// History recall — walk backward
		if len(m.cmdHistory) == 0 {
			return m, nil
		}
		if m.historyIdx == -1 {
			m.historySaved = m.input.Value()
			m.historyIdx = len(m.cmdHistory)
		}
		if m.historyIdx > 0 {
			m.historyIdx--
			m.input.SetValue(m.cmdHistory[m.historyIdx])
			m.input.CursorEnd()
			m.updateSuggestionList()
		}
		return m, nil

	case tea.KeyDown:
		if m.mode == modeRunning {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		if m.historyIdx >= 0 {
			m.historyIdx++
			if m.historyIdx >= len(m.cmdHistory) {
				m.historyIdx = -1
				m.input.SetValue(m.historySaved)
			} else {
				m.input.SetValue(m.cmdHistory[m.historyIdx])
			}
			m.input.CursorEnd()
			m.updateSuggestionList()
		}
		return m, nil

	case tea.KeyRight:
		if m.mode != modeIdle {
			return m, nil
		}
		// Only accept suggestion when cursor is at end of input
		cursorAtEnd := m.input.Position() >= len([]rune(m.input.Value()))
		if !cursorAtEnd {
			// Cursor not at end — move cursor, don't accept suggestion
			saved := m.input.KeyMap.AcceptSuggestion
			m.input.KeyMap.AcceptSuggestion = key.NewBinding()
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.input.KeyMap.AcceptSuggestion = saved
			return m, cmd
		}
		// Cursor at end — let bubbles accept the suggestion
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.updateSuggestionList()
		return m, cmd
	}

	if m.mode == modeIdle {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.historyIdx = -1 // reset history on any typed input
		m.updateSuggestionList()
		return m, cmd
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// updateSuggestionList refreshes inline predictions based on current input.
func (m *TUIModel) updateSuggestionList() {
	suggestions := buildSuggestions(m.input.Value(), m.cmdHistory)
	m.input.SetSuggestions(suggestions)
}

// ── Command Execution ─────────────────────────────────────────────────

func (m TUIModel) executeCommand(raw string) (TUIModel, tea.Cmd) {
	deity, args := m.dispatch(raw)

	m.mode = modeRunning
	m.runningDeity = deity
	m.runningCmd = raw
	m.outputLines = nil
	m.input.Blur()
	m.input.Reset()

	glyph, name := deityDisplay(deity)
	if deity != "" {
		m.outputLines = append(m.outputLines,
			lipgloss.NewStyle().Foreground(Gold).Bold(true).Render(
				fmt.Sprintf("  %s %s", glyph, name)),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(
				fmt.Sprintf("  %s", raw)),
			"")
	}
	m.viewport.SetContent(strings.Join(m.outputLines, "\n"))
	m.recalcViewportHeight()

	exe, _ := os.Executable()
	cmd := exec.Command(exe, args...)

	return m, tea.Batch(m.spinner.Tick, m.runCommand(cmd))
}

func (m *TUIModel) dispatch(raw string) (string, []string) {
	lower := strings.ToLower(raw)
	tokens := strings.Fields(lower)
	rawTokens := strings.Fields(raw)

	if len(tokens) == 0 {
		return "", nil
	}

	for _, d := range deityRoster {
		if tokens[0] == d.Key {
			return d.Key, rawTokens
		}
	}

	if target, ok := cliAliases[tokens[0]]; ok {
		return target, rawTokens
	}

	bestDeity := ""
	bestScore := 0
	for deity, keywords := range intentKeywords {
		score := 0
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestDeity = deity
		}
	}

	if bestDeity != "" {
		return bestDeity, []string{bestDeity}
	}

	return "", rawTokens
}

func (m TUIModel) runCommand(cmd *exec.Cmd) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		err := cmd.Run()
		return cmdBatchMsg{lines: strings.Split(buf.String(), "\n"), err: err}
	}
}

func (m TUIModel) handleBatchOutput(msg cmdBatchMsg) (TUIModel, tea.Cmd) {
	for _, line := range msg.lines {
		m.outputLines = append(m.outputLines, "  "+line)
	}

	m.mode = modeIdle
	m.input.Focus()
	m.input.Placeholder = "What next?"

	if m.runningDeity != "" {
		m.activeDeity[m.runningDeity] = true
	}
	m.history = append(m.history, historyEntry{
		deity: m.runningDeity, command: m.runningCmd,
		output: strings.Join(m.outputLines, "\n"),
	})
	m.cmdHistory = deduplicateHistory(m.history)

	if msg.err != nil {
		m.outputLines = append(m.outputLines, "",
			lipgloss.NewStyle().Foreground(Red).Render(fmt.Sprintf("  ✗ %v", msg.err)))
	} else {
		m.outputLines = append(m.outputLines, "",
			lipgloss.NewStyle().Foreground(Green).Render("  ✓ Done"))
	}
	m.viewport.SetContent(strings.Join(m.outputLines, "\n"))
	m.viewport.GotoBottom()
	m.runningDeity = ""
	m.runningCmd = ""
	return m, nil
}

// ── Active Deity Detection ────────────────────────────────────────────

func (m *TUIModel) refreshActive() {
	for k := range m.activeDeity {
		delete(m.activeDeity, k)
	}

	entries, _ := m.steleReader.ReadNew()
	now := time.Now()
	for _, e := range entries {
		ts, err := time.Parse(time.RFC3339, e.TS)
		if err != nil {
			continue
		}
		if now.Sub(ts) < 5*time.Minute {
			deity := strings.ToLower(e.Deity)
			if !strings.Contains(deity, ":") {
				m.activeDeity[deity] = true
			}
		}
	}

	home, _ := os.UserHomeDir()
	pidDir := filepath.Join(home, ".config", "ra", "pids")
	pidEntries, _ := os.ReadDir(pidDir)
	for _, f := range pidEntries {
		if f.IsDir() {
			continue
		}
		name := strings.TrimSuffix(f.Name(), ".pid")
		for _, d := range deityRoster {
			if strings.Contains(strings.ToLower(name), d.Key) {
				m.activeDeity[d.Key] = true
			}
		}
	}
}

// ── Layout ────────────────────────────────────────────────────────────

const leftPaneWidth = 42

func (m TUIModel) View() string {
	if m.quitting {
		return ""
	}

	hasOutput := len(m.outputLines) > 0
	maxW := min(m.width-2, 90)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#333333"))
	divider := dim.Render(strings.Repeat("─", maxW))

	header := lipgloss.NewStyle().Foreground(Gold).Bold(true).Render("𓉴  Sirsi Pantheon")
	desc := lipgloss.NewStyle().Foreground(lipgloss.Color("#999999")).
		Render("DevOps intelligence for developers and infrastructure teams")
	signage := lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).
		Render(" Sirsi Technologies, Inc. 2026 (MIT License)")

	var b strings.Builder
	usedLines := 0

	if !hasOutput {
		// ── Single-pane: full roster
		b.WriteString("\n")
		b.WriteString(" " + header + "\n")
		b.WriteString(" " + desc + "\n")
		b.WriteString(" " + divider + "\n")
		usedLines += 4

		roster := m.renderRosterColumns(false)
		b.WriteString(roster)
		usedLines += strings.Count(roster, "\n")

		status := m.renderStatusLine()
		b.WriteString(status)
		usedLines += strings.Count(status, "\n")

		b.WriteString(" " + divider + "\n")
		b.WriteString(" " + m.input.View() + "\n")
		b.WriteString(m.renderHints(false) + "\n")
		usedLines += 3
	} else {
		// ── Split-pane: left roster | right output
		left := m.renderLeftPane()
		right := m.renderRightPane()

		leftStyle := lipgloss.NewStyle().
			Width(leftPaneWidth).
			BorderRight(true).
			BorderStyle(lipgloss.Border{Right: "│"}).
			BorderForeground(lipgloss.Color("#333333")).
			PaddingRight(1)

		rightWidth := m.width - leftPaneWidth - 3
		if rightWidth < 20 {
			rightWidth = 20
		}
		rightStyle := lipgloss.NewStyle().Width(rightWidth).PaddingLeft(1)

		panes := lipgloss.JoinHorizontal(lipgloss.Top,
			leftStyle.Render(left),
			rightStyle.Render(right),
		)

		b.WriteString("\n")
		b.WriteString(" " + header + "\n")
		b.WriteString(" " + divider + "\n")
		usedLines += 3

		b.WriteString(panes + "\n")
		usedLines += strings.Count(panes, "\n") + 1

		b.WriteString(" " + divider + "\n")
		b.WriteString(" " + m.input.View() + "\n")
		b.WriteString(m.renderHints(true) + "\n")
		usedLines += 3
	}

	// Pad to push signage to the bottom — exactly once
	remaining := m.height - usedLines - 2
	if remaining > 0 {
		b.WriteString(strings.Repeat("\n", remaining))
	}
	b.WriteString(signage)

	return b.String()
}

// renderRosterColumns renders deities in a column grid that fits the terminal.
// 3 columns if width >= 90, 2 columns if >= 60, single column otherwise.
func (m TUIModel) renderRosterColumns(compact bool) string {
	var b strings.Builder

	cols := 3
	if m.width < 90 {
		cols = 2
	}
	if m.width < 60 {
		cols = 1
	}

	rows := (len(deityRoster) + cols - 1) / cols
	colWidth := (m.width - 2) / cols
	if colWidth > 34 {
		colWidth = 34
	}

	for r := 0; r < rows; r++ {
		var rowParts []string
		for c := 0; c < cols; c++ {
			idx := c*rows + r // column-major: fill down then across
			if idx < len(deityRoster) {
				rowParts = append(rowParts, m.renderDeityCell(deityRoster[idx], colWidth))
			}
		}
		b.WriteString(" " + strings.Join(rowParts, "") + "\n")
	}

	return b.String()
}

// renderDeityCell renders one deity as a fixed-width cell for the grid.
// Avoids lipgloss Width/MaxWidth for layout — Egyptian glyphs have
// unpredictable terminal widths. Instead we measure with lipgloss.Width()
// and pad with real spaces so the error model is consistent.
func (m TUIModel) renderDeityCell(d deityInfo, width int) string {
	active := m.activeDeity[d.Key]

	var nameColor, roleColor lipgloss.Color
	if active {
		nameColor = Gold
		roleColor = lipgloss.Color("#CCCCCC")
	} else {
		nameColor = lipgloss.Color("#BBBBBB")
		roleColor = lipgloss.Color("#777777")
	}

	dot := lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")).Render("·")
	if active {
		dot = lipgloss.NewStyle().Foreground(Gold).Render("●")
	}

	glyph := lipgloss.NewStyle().Foreground(nameColor).Render(d.Glyph)
	name := lipgloss.NewStyle().Bold(true).Foreground(nameColor).Render(d.Name)
	role := lipgloss.NewStyle().Foreground(roleColor).Render(d.Role)

	// Pad name to a fixed visual column so roles align across rows.
	prefix := dot + " " + glyph + " " + name
	prefixW := lipgloss.Width(prefix)
	const nameEnd = 14 // target column where role text starts
	if prefixW < nameEnd {
		prefix += strings.Repeat(" ", nameEnd-prefixW)
	}

	cell := prefix + role
	cellW := lipgloss.Width(cell)
	if cellW < width {
		cell += strings.Repeat(" ", width-cellW)
	}
	return cell
}

func (m TUIModel) renderStatusLine() string {
	activeCount := 0
	for _, d := range deityRoster {
		if m.activeDeity[d.Key] {
			activeCount++
		}
	}
	if activeCount > 0 {
		return lipgloss.NewStyle().Foreground(Green).
			Render(fmt.Sprintf(" %d %s active", activeCount, pluralize("deity", activeCount))) + "\n"
	}
	return ""
}

func (m TUIModel) renderLeftPane() string {
	var b strings.Builder
	b.WriteString(m.renderRosterColumns(true))
	b.WriteString(m.renderStatusLine())
	return b.String()
}

func (m TUIModel) renderRightPane() string {
	var b strings.Builder
	if m.mode == modeRunning {
		glyph, name := deityDisplay(m.runningDeity)
		b.WriteString(m.spinner.View() + " " +
			lipgloss.NewStyle().Foreground(Gold).Render(glyph+" "+name) +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(" running...") + "\n\n")
	}
	b.WriteString(m.viewport.View())
	return b.String()
}

func (m TUIModel) renderHints(splitMode bool) string {
	var hints []string
	if m.mode == modeRunning {
		hints = append(hints, "↑/↓ scroll")
	} else {
		hints = append(hints, "→ accept", "↑ history")
		if splitMode {
			hints = append(hints, "esc back")
		}
	}
	hints = append(hints, "clear reset", "ctrl+c quit")
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).
		Render(" " + strings.Join(hints, " · "))
}

// ── Helpers ───────────────────────────────────────────────────────────

func deityDisplay(key string) (string, string) {
	for _, d := range deityRoster {
		if d.Key == key {
			return d.Glyph, d.Name
		}
	}
	return "⚙", key
}

func (m *TUIModel) recalcViewportHeight() {
	// Reserve: header(2) + divider(1) + input divider(1) + input(1) + hints(1) + padding(2)
	vpHeight := m.height - 8
	if vpHeight < 5 {
		vpHeight = 5
	}
	m.viewport.Height = vpHeight

	rightWidth := m.width - leftPaneWidth - 5
	if rightWidth < 20 {
		rightWidth = 20
	}
	m.viewport.Width = rightWidth
}

func pluralize(word string, n int) string {
	if n == 1 {
		return word
	}
	if strings.HasSuffix(word, "y") {
		return word[:len(word)-1] + "ies"
	}
	return word + "s"
}

// ── Launcher ──────────────────────────────────────────────────────────

func LaunchTUI() error {
	p := tea.NewProgram(NewTUIModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
