// Package output handles terminal rendering for Sirsi Pantheon.
// Uses the Pantheon brand language: Gold + Black + Deep Lapis (Rule A10).
package output

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// Anubis brand colors (Rule A10)
var (
	Gold     = lipgloss.Color("#C8A951")
	Black    = lipgloss.Color("#0F0F0F")
	Lapis    = lipgloss.Color("#1A1A5E")
	White    = lipgloss.Color("#FAFAFA")
	DimWhite = lipgloss.Color("#888888")
	Red      = lipgloss.Color("#FF4444")
	Green    = lipgloss.Color("#44FF88")
	Yellow   = lipgloss.Color("#FFD700")
)

// Styles
var (
	// Title style — gold text, bold
	TitleStyle = lipgloss.NewStyle().
			Foreground(Gold).
			Bold(true)

	// Header style — gold, underlined
	HeaderStyle = lipgloss.NewStyle().
			Foreground(Gold).
			Bold(true).
			Underline(true)

	// Body text
	BodyStyle = lipgloss.NewStyle().
			Foreground(White)

	// Dim body text
	DimStyle = lipgloss.NewStyle().
			Foreground(DimWhite)

	// Error style
	ErrorStyle = lipgloss.NewStyle().
			Foreground(Red).
			Bold(true)

	// Success style
	SuccessStyle = lipgloss.NewStyle().
			Foreground(Green).
			Bold(true)

	// Warning style
	WarningStyle = lipgloss.NewStyle().
			Foreground(Yellow)

	// Info style (Thoth Lapis)
	InfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#51A9C8"))

	// Size style — gold, for file sizes
	SizeStyle = lipgloss.NewStyle().
			Foreground(Gold).
			Bold(true)

	// Category badge
	CategoryStyle = lipgloss.NewStyle().
			Foreground(Black).
			Background(Gold).
			Padding(0, 1).
			Bold(true)

	// Box style for major sections
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Gold).
			Padding(0, 2).
			MarginTop(1)

	// Column style
	ColumnStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(Lapis).
			Padding(0, 1).
			MarginRight(2)

	// Value style
	ValueStyle = lipgloss.NewStyle().
			Foreground(White).
			Bold(true)

	// Dashboard style
	DashboardStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(Gold)

	// Severity styles
	SeveritySafe    = lipgloss.NewStyle().Foreground(Green)
	SeverityCaution = lipgloss.NewStyle().Foreground(Yellow)
	SeverityWarning = lipgloss.NewStyle().Foreground(Red)
)

// Banner prints the Pantheon banner.
func Banner() {
	banner := TitleStyle.Render(`
   P A N T H E O N
   ───────────────────────────────
   Unified DevOps Intel Platform
   "One Install. All Deities."

   𓂀 Anubis  𓁟 Thoth  𓁆 Seshat
`)
	fmt.Fprintln(os.Stderr, banner)
}

// Header prints a section header with the 𓂀 prefix.
func Header(text string) {
	fmt.Fprintf(os.Stderr, "\n%s\n", HeaderStyle.Render("𓂀 "+text))
}

// Info prints a themed informational message.
func Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "  %s %s\n", InfoStyle.Render("𓁟"), BodyStyle.Render(msg))
}

// Dim prints a dimmed message.
func Dim(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "  %s\n", DimStyle.Render(msg))
}

// Success prints a themed success message.
func Success(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "  %s %s\n", SuccessStyle.Render("𓁐"), BodyStyle.Render(msg))
}

// Warn prints a themed warning message.
func Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "  %s %s\n", WarningStyle.Render("⚠️"), BodyStyle.Render(msg))
}

// Error prints a themed error message.
func Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "  %s %s\n", ErrorStyle.Render("𓁵"), BodyStyle.Render(msg))
}

// FindingRow formats a single finding as a table row.
func FindingRow(name, path, size, severity string) string {
	severityStyled := severity
	switch strings.ToLower(severity) {
	case "safe":
		severityStyled = SeveritySafe.Render(severity)
	case "caution":
		severityStyled = SeverityCaution.Render(severity)
	case "warning":
		severityStyled = SeverityWarning.Render(severity)
	}

	return fmt.Sprintf("  %-30s %s  %s  %s",
		BodyStyle.Render(name),
		SizeStyle.Render(fmt.Sprintf("%10s", size)),
		severityStyled,
		DimStyle.Render(path),
	)
}

// Dashboard prints a multi-column summary dashboard.
func Dashboard(metrics map[string]string) {
	var cols []string

	for label, value := range metrics {
		col := ColumnStyle.Render(
			fmt.Sprintf("%s\n%s",
				DimStyle.Render(label),
				ValueStyle.Render(value),
			),
		)
		cols = append(cols, col)
	}

	dash := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	fmt.Fprintf(os.Stderr, "\n%s\n", DashboardStyle.Render(dash))
}

// Table displays results in a beautiful TUI table.
func Table(headers []string, rows [][]string) {
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(Gold)).
		Headers(headers...).
		Rows(rows...)

	fmt.Fprintf(os.Stderr, "\n%s\n", t.Render())
}

// Summary prints a summary box with totals.
func Summary(totalSize string, findingCount int, ruleCount int) {
	content := fmt.Sprintf(
		"%s found across %s (%s rules matched)",
		SizeStyle.Render(totalSize),
		BodyStyle.Render(fmt.Sprintf("%d findings", findingCount)),
		DimStyle.Render(fmt.Sprintf("%d", ruleCount)),
	)
	fmt.Fprintf(os.Stderr, "\n%s\n", BoxStyle.Render("𓂀 "+content))
}

// Footer prints the completion ritual with elapsed time.
func Footer(elapsed time.Duration) {
	fmt.Fprintf(os.Stderr, "\n  %s %s\n",
		DimStyle.Render("Completed in"),
		ValueStyle.Render(elapsed.Round(time.Millisecond).String()),
	)
}

func Section(title string) {
	fmt.Fprintf(os.Stderr, "\n%s\n", TitleStyle.Render("𓂀 "+title))
}

// shortenPath replaces home dir with ~ and truncates long paths.
func ShortenPath(path string) string {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(path, home) {
		path = "~" + strings.TrimPrefix(path, home)
	}
	if len(path) > 60 {
		return "..." + path[len(path)-57:]
	}
	return path
}

// Truncate shortens a string to a max length with an ellipsis.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
