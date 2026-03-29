package output

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MainModel represents the interactive Pantheon dashboard.
type MainModel struct {
	spinner  spinner.Model
	quitting bool
}

func NewMainModel() MainModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(Gold)
	return MainModel{spinner: s}
}

func (m MainModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		default:
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m MainModel) View() string {
	if m.quitting {
		return "𓂀 Pantheon ritual complete.\n"
	}

	header := TitleStyle.Render("   𓂀  Pantheon Command Center  ")

	// Stats Grid
	stats := lipgloss.JoinHorizontal(lipgloss.Top,
		ColumnStyle.Render(fmt.Sprintf("%s\n%s", DimStyle.Render("RAM"), ValueStyle.Render("Optimized"))),
		ColumnStyle.Render(fmt.Sprintf("%s\n%s", DimStyle.Render("VRAM"), ValueStyle.Render("Zero-Copy"))),
		ColumnStyle.Render(fmt.Sprintf("%s\n%s", DimStyle.Render("Neural"), ValueStyle.Render("Active"))),
	)

	// Deity Status
	deities := lipgloss.JoinVertical(lipgloss.Left,
		fmt.Sprintf("  𓂀 Anubis  %s %s", m.spinner.View(), lipgloss.NewStyle().Foreground(Green).Render("Scanning")),
		fmt.Sprintf("  🪶 Ma'at   %s %s", "✓", DimStyle.Render("Stable")),
		fmt.Sprintf("  𓁟 Thoth   %s %s", "✓", DimStyle.Render("Synced")),
	)

	s := fmt.Sprintf("\n%s\n\n%s\n\n%s\n\n  %s\n",
		header,
		BoxStyle.Render(stats),
		deities,
		DimStyle.Render("Press 'q' to exit ritual..."),
	)

	return s
}

// LaunchDashboard starts the interactive Pantheon TUI.
func LaunchDashboard() error {
	p := tea.NewProgram(NewMainModel())
	_, err := p.Run()
	return err
}
