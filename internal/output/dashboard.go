// Package output — Ra Command Center TUI
//
// 𓇶 The Ra Command Center is the primary interface for monitoring and governing
// multi-sprint autonomous agent deployments. It shows:
//   - Live sprint progress per scope (Sprint 2/5, running, etc.)
//   - Governance loop status (Ma'at QA → Seshat scribe → Thoth compact)
//   - Agent activity (last tool call, log tail)
//   - Post-sprint acceptance flow with verification options
//
// Built with BubbleTea + Lipgloss using the Pantheon brand (Gold + Black).
package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// ── Stele Integration ──────────────────────────────────────────────────
//
// The Command Center is a read-only consumer of The Stele (ADR-014).
// Every deity and agent appends hash-chained entries to ~/.config/ra/stele.jsonl.
// The CC reads new entries every 6 seconds via its own tracked offset.
// PID liveness checks run every 30 seconds (the expensive operation).
// On Apple Silicon unified memory, the mmap'd Stele is zero-copy.

// ── Model ──────────────────────────────────────────────────────────────

// MainModel is the Ra Command Center TUI state.
type MainModel struct {
	raDir        string
	scopes       []scopeView
	scopeMap     map[string]int // name → index into scopes
	spinner      spinner.Model
	quitting     bool
	width        int
	height       int
	allDone      bool
	focused      int // which scope is focused for log view
	steleReader  *stele.Reader
	lastPIDCheck time.Time
	globalLog    []string // deity-level events with no scope
}

// scopeView tracks one deployed scope's live state.
type scopeView struct {
	Name      string
	State     string // "running", "completed", "failed", "idle"
	Sprint    int
	Sprints   int
	MaatGate  string
	ThothSync string
	LogTail   []string // last 8 text lines
	LastTool  string
	Duration  string
	Icon      string
	PID       int
}

type tickMsg time.Time
type pidCheckMsg time.Time

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func pidCheckEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return pidCheckMsg(t) })
}

// NewMainModel creates the Ra Command Center.
func NewMainModel() MainModel {
	home, _ := os.UserHomeDir()
	raDir := filepath.Join(home, ".config", "ra")

	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(Gold)

	m := MainModel{
		raDir:       raDir,
		spinner:     s,
		scopeMap:    make(map[string]int),
		steleReader: stele.NewReader("command-center"),
		width:       120,
		height:      40,
	}

	// Initial load — read deployment metadata, Stele entries, and check PIDs
	m.loadDeployment()
	m.readStele()
	m.checkPIDs()

	return m
}

func (m MainModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		tickEvery(6*time.Second),      // event reads every 6s
		pidCheckEvery(30*time.Second), // PID liveness every 30s
	)
}

// ── Update ─────────────────────────────────────────────────────────────

func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "tab", "j", "down":
			if len(m.scopes) > 0 {
				m.focused = (m.focused + 1) % len(m.scopes)
			}
			return m, nil
		case "shift+tab", "k", "up":
			if len(m.scopes) > 0 {
				m.focused = (m.focused - 1 + len(m.scopes)) % len(m.scopes)
			}
			return m, nil
		}
		return m, nil

	case tickMsg:
		// Every 6s: read new Stele entries + reload deployment if scopes changed
		m.loadDeployment()
		m.readStele()
		m.updateElapsed()
		return m, tickEvery(6 * time.Second)

	case pidCheckMsg:
		// Every 30s: check PID liveness (expensive — signal 0 per scope)
		m.checkPIDs()
		return m, pidCheckEvery(30 * time.Second)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	default:
		return m, nil
	}
}

// loadDeployment reads deployment.json to discover scopes.
func (m *MainModel) loadDeployment() {
	metaPath := filepath.Join(m.raDir, "deployment.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return
	}

	var meta struct {
		StartedAt string   `json:"started_at"`
		Scopes    []string `json:"scopes"`
	}
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return
	}

	// Add new scopes we haven't seen
	for _, name := range meta.Scopes {
		if _, exists := m.scopeMap[name]; !exists {
			m.scopeMap[name] = len(m.scopes)
			m.scopes = append(m.scopes, scopeView{
				Name:    name,
				State:   "running",
				Icon:    "🔄",
				Sprint:  1,
				Sprints: 1,
			})
		}
	}
}

// readStele reads new entries from The Stele since the CC's last offset.
// The Stele is append-only — agents and deities push entries, CC reads them.
func (m *MainModel) readStele() {
	entries, err := m.steleReader.ReadNew()
	if err != nil || len(entries) == 0 {
		return
	}

	for _, e := range entries {
		m.applySteleEntry(e)
	}
}

// applySteleEntry updates scope state from a Stele entry.
func (m *MainModel) applySteleEntry(e stele.Entry) {
	scope := e.Scope

	// Deity-level events (no scope) go to the global activity log.
	if scope == "" {
		m.applyGlobalEvent(e)
		return
	}

	idx, exists := m.scopeMap[scope]
	if !exists {
		m.scopeMap[scope] = len(m.scopes)
		m.scopes = append(m.scopes, scopeView{Name: scope, State: "running", Icon: "🔄"})
		idx = len(m.scopes) - 1
	}
	sv := &m.scopes[idx]

	switch e.Type {
	case stele.TypeSprintStart:
		sv.Sprint = atoi(e.Data["sprint"])
		sv.Sprints = atoi(e.Data["sprints"])
		sv.State = "running"
		sv.Icon = "🔄"
		sv.MaatGate = ""
		sv.ThothSync = ""

	case stele.TypeSprintEnd:
		sv.Sprint = atoi(e.Data["sprint"])
		sv.Sprints = atoi(e.Data["sprints"])

	case stele.TypeToolUse:
		sv.LastTool = fmt.Sprintf("[tool: %s]", e.Data["tool"])

	case stele.TypeGovernance:
		sv.MaatGate = e.Data["maat"]
		sv.ThothSync = e.Data["thoth"]

	case stele.TypeText:
		if text := strings.TrimSpace(e.Data["text"]); text != "" {
			sv.LogTail = append(sv.LogTail, text)
			if len(sv.LogTail) > 8 {
				sv.LogTail = sv.LogTail[len(sv.LogTail)-8:]
			}
		}

	case stele.TypeCommit:
		sv.LogTail = append(sv.LogTail, fmt.Sprintf("commit: %s", e.Data["hash"]))
		if len(sv.LogTail) > 8 {
			sv.LogTail = sv.LogTail[len(sv.LogTail)-8:]
		}

	case stele.TypeComplete:
		sv.State = "completed"
		sv.Icon = "✅"

	case stele.TypeFailed:
		sv.State = "failed"
		sv.Icon = "❌"

	// Deity events with a scope (e.g., thoth_sync on a specific repo)
	case stele.TypeThothSync, stele.TypeThothCompact,
		stele.TypeMaatWeigh, stele.TypeMaatPulse,
		stele.TypeNeithWeave, stele.TypeNeithDrift,
		stele.TypeSeshatIngest,
		stele.TypeKaHunt, stele.TypeKaClean,
		stele.TypeSebaRender, stele.TypeHapiDetect,
		stele.TypeGuardStart:
		sv.LogTail = append(sv.LogTail, fmt.Sprintf("[%s] %s", e.Deity, e.Type))
		if len(sv.LogTail) > 8 {
			sv.LogTail = sv.LogTail[len(sv.LogTail)-8:]
		}
	}
}

// applyGlobalEvent handles deity events that have no scope (global operations).
// These are displayed in the activity feed area of the dashboard.
func (m *MainModel) applyGlobalEvent(e stele.Entry) {
	line := fmt.Sprintf("[%s] %s", e.Deity, e.Type)
	for k, v := range e.Data {
		if len(v) > 40 {
			v = v[:40] + "..."
		}
		line += fmt.Sprintf(" %s=%s", k, v)
	}
	m.globalLog = append(m.globalLog, line)
	if len(m.globalLog) > 12 {
		m.globalLog = m.globalLog[len(m.globalLog)-12:]
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// checkPIDs verifies process liveness for all scopes (runs every 30s).
func (m *MainModel) checkPIDs() {
	m.lastPIDCheck = time.Now()
	m.allDone = true

	for i := range m.scopes {
		sv := &m.scopes[i]

		pidFile := filepath.Join(m.raDir, "pids", sv.Name+".pid")
		pidData, err := os.ReadFile(pidFile)
		if err != nil {
			if sv.State == "running" {
				sv.State = "idle"
				sv.Icon = "⚫"
			}
			continue
		}

		pid, _ := strconv.Atoi(strings.TrimSpace(string(pidData)))
		sv.PID = pid

		if pid > 0 && syscall.Kill(pid, 0) == nil {
			if sv.State != "completed" && sv.State != "failed" {
				sv.State = "running"
				sv.Icon = "🔄"
			}
			m.allDone = false
		} else if sv.State == "running" {
			// Process died — check exit code
			exitFile := filepath.Join(m.raDir, "exits", sv.Name+".exit")
			exitData, _ := os.ReadFile(exitFile)
			code, _ := strconv.Atoi(strings.TrimSpace(string(exitData)))
			if code == 0 {
				sv.State = "completed"
				sv.Icon = "✅"
			} else {
				sv.State = "failed"
				sv.Icon = "❌"
			}
		}

		if sv.State == "running" {
			m.allDone = false
		}
	}
}

// updateElapsed refreshes the duration display for all scopes.
func (m *MainModel) updateElapsed() {
	metaPath := filepath.Join(m.raDir, "deployment.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return
	}

	var meta struct {
		StartedAt string `json:"started_at"`
	}
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return
	}

	startedAt, _ := time.Parse(time.RFC3339, meta.StartedAt)
	elapsed := formatElapsed(time.Since(startedAt))

	for i := range m.scopes {
		m.scopes[i].Duration = elapsed
	}
}

// ── View ───────────────────────────────────────────────────────────────

func (m MainModel) View() string {
	if m.quitting {
		return "\n  𓇶 Ra Command Center closed.\n\n"
	}

	var b strings.Builder

	// Header
	header := lipgloss.NewStyle().
		Foreground(Gold).
		Bold(true).
		Padding(0, 1).
		Render("𓇶  Ra Command Center")

	version := DimStyle.Render("v0.10.0")
	headerLine := lipgloss.JoinHorizontal(lipgloss.Center, header, "  ", version)
	b.WriteString("\n " + headerLine + "\n")
	b.WriteString(" " + lipgloss.NewStyle().Foreground(Gold).Render(strings.Repeat("─", min(m.width-2, 90))) + "\n\n")

	if len(m.scopes) == 0 {
		b.WriteString("  No active deployment. Run: pantheon ra deploy\n")
		return b.String()
	}

	// Scope cards
	for i, sv := range m.scopes {
		b.WriteString(m.renderScope(i, sv))
		b.WriteString("\n")
	}

	// Global deity activity feed
	if len(m.globalLog) > 0 {
		b.WriteString(DimStyle.Render("  ── Deity Activity ──") + "\n")
		for _, line := range m.globalLog {
			if len(line) > 88 {
				line = line[:88] + "..."
			}
			b.WriteString("  " + DimStyle.Render(line) + "\n")
		}
		b.WriteString("\n")
	}

	// Governance legend
	b.WriteString(m.renderGovernanceLegend())

	// Footer
	if m.allDone {
		b.WriteString(m.renderAcceptancePrompt())
	} else {
		b.WriteString(DimStyle.Render("  ↑/↓ navigate  ·  q quit") + "\n")
	}

	return b.String()
}

func (m MainModel) renderScope(idx int, sv scopeView) string {
	// Card styling
	focused := idx == m.focused
	borderColor := lipgloss.Color("#444444")
	if focused {
		borderColor = Gold
	}

	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(min(m.width-4, 88))

	var card strings.Builder

	// Title line: icon + name + state + sprint progress
	title := lipgloss.NewStyle().Bold(true).Foreground(White).Render(sv.Name)
	stateStyle := lipgloss.NewStyle()
	switch sv.State {
	case "running":
		stateStyle = stateStyle.Foreground(Green)
	case "completed":
		stateStyle = stateStyle.Foreground(Green)
	case "failed":
		stateStyle = stateStyle.Foreground(Red)
	default:
		stateStyle = stateStyle.Foreground(DimWhite)
	}

	stateLabel := stateStyle.Render(sv.State)
	sprintLabel := ""
	if sv.State == "running" {
		sprintLabel = fmt.Sprintf(" · Sprint %d/%d %s", sv.Sprint, sv.Sprints, m.spinner.View())
	} else if sv.Sprints > 1 {
		sprintLabel = fmt.Sprintf(" · Sprint %d/%d", sv.Sprint, sv.Sprints)
	}

	card.WriteString(fmt.Sprintf("%s  %s  %s%s  %s\n",
		sv.Icon, title, stateLabel, sprintLabel, DimStyle.Render(sv.Duration)))

	// Governance status line
	if sv.MaatGate != "" || sv.ThothSync != "" {
		var govParts []string
		if sv.MaatGate == "PASS" {
			govParts = append(govParts, SuccessStyle.Render("🪶 Ma'at: PASS"))
		} else if sv.MaatGate == "FAIL" {
			govParts = append(govParts, ErrorStyle.Render("🪶 Ma'at: FAIL"))
		}
		if sv.ThothSync == "compacted" {
			govParts = append(govParts, DimStyle.Render("𓁟 Thoth: compacted"))
		}
		if len(govParts) > 0 {
			card.WriteString("  " + strings.Join(govParts, "  ·  ") + "\n")
		}
	}

	// Last tool + activity indicator
	if sv.LastTool != "" && sv.State == "running" {
		card.WriteString("  " + DimStyle.Render(sv.LastTool) + "\n")
	}

	// Log tail (only for focused scope)
	if focused && len(sv.LogTail) > 0 {
		card.WriteString("\n")
		logStyle := lipgloss.NewStyle().Foreground(DimWhite)
		for _, line := range sv.LogTail {
			maxLen := min(m.width-10, 82)
			if len(line) > maxLen {
				line = line[:maxLen-3] + "..."
			}
			card.WriteString("  " + logStyle.Render(line) + "\n")
		}
	}

	return cardStyle.Render(card.String())
}

func (m MainModel) renderGovernanceLegend() string {
	legend := lipgloss.NewStyle().
		Foreground(DimWhite).
		Padding(0, 2).
		Render("Governance: Agent → 🪶 Ma'at QA → 𓁆 Seshat scribe → 𓁟 Thoth compact → 𓇶 Ra next sprint")

	return legend + "\n\n"
}

func (m MainModel) renderAcceptancePrompt() string {
	var b strings.Builder

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(Gold).
		Padding(0, 1).
		Width(min(m.width-4, 88))

	var inner strings.Builder
	inner.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Gold).Render("𓇶 All sprints complete") + "\n\n")

	// Summary
	var passed, failed, running int
	for _, sv := range m.scopes {
		switch sv.State {
		case "completed":
			passed++
		case "failed":
			failed++
		case "running":
			running++
		}
	}
	inner.WriteString(fmt.Sprintf("  ✅ %d completed  ❌ %d failed  🔄 %d running\n\n", passed, failed, running))

	inner.WriteString(DimStyle.Render("  Review the logs above. To accept and continue:\n"))
	inner.WriteString(DimStyle.Render("    pantheon ra collect    — gather results into Seshat\n"))
	inner.WriteString(DimStyle.Render("    pantheon ra deploy     — start next sprint set\n"))
	inner.WriteString(DimStyle.Render("    q                      — exit command center\n"))

	b.WriteString(boxStyle.Render(inner.String()))
	b.WriteString("\n")
	return b.String()
}

// ── Helpers ────────────────────────────────────────────────────────────

func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", h, m)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// LaunchDashboard starts the Ra Command Center TUI.
func LaunchDashboard() error {
	p := tea.NewProgram(NewMainModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
