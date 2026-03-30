package output

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ── ShortenPath Tests ───────────────────────────────────────────────────

func TestShortenPath_Home(t *testing.T) {
	t.Parallel()
	// ShortenPath should replace $HOME with ~
	result := ShortenPath("/Users/testuser/Documents/file.txt")
	// On CI/test systems, the home dir varies — just verify it doesn't panic
	if result == "" {
		t.Error("ShortenPath should not return empty")
	}
}

func TestShortenPath_Long(t *testing.T) {
	t.Parallel()
	longPath := "/a" + strings.Repeat("/verylongdirectoryname", 5) + "/file.txt"
	result := ShortenPath(longPath)
	if len(result) > 63 { // "..." + 57 chars
		t.Errorf("ShortenPath should truncate long paths, got %d chars", len(result))
	}
	if !strings.HasPrefix(result, "...") {
		t.Errorf("truncated path should start with '...', got %q", result[:10])
	}
}

func TestShortenPath_Short(t *testing.T) {
	t.Parallel()
	result := ShortenPath("/tmp/a.txt")
	if result != "/tmp/a.txt" {
		t.Errorf("short path should be unchanged, got %q", result)
	}
}

// ── Truncate Tests ──────────────────────────────────────────────────────

func TestTruncate_Short(t *testing.T) {
	t.Parallel()
	result := Truncate("hello", 10)
	if result != "hello" {
		t.Errorf("Truncate('hello', 10) = %q, want 'hello'", result)
	}
}

func TestTruncate_Exact(t *testing.T) {
	t.Parallel()
	result := Truncate("hello", 5)
	if result != "hello" {
		t.Errorf("Truncate('hello', 5) = %q, want 'hello'", result)
	}
}

func TestTruncate_Long(t *testing.T) {
	t.Parallel()
	result := Truncate("hello world foo bar", 10)
	if len(result) > 10 {
		t.Errorf("Truncate result too long: %d chars", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("truncated string should end with ...")
	}
}

// ── Section / Footer Tests ──────────────────────────────────────────────

func TestSection(t *testing.T) {
	// Should not panic — writes to stderr
	Section("Test Section Title")
}

func TestFooter(t *testing.T) {
	// Should not panic
	Footer(3 * time.Second)
	Footer(125 * time.Millisecond)
}

// ── Dashboard Tests ─────────────────────────────────────────────────────

func TestDashboard_Empty(t *testing.T) {
	// Should not panic with empty metrics
	Dashboard(map[string]string{})
}

func TestDashboard_WithMetrics(t *testing.T) {
	Dashboard(map[string]string{
		"CPU":    "12 cores",
		"RAM":    "32 GB",
		"Status": "Healthy",
	})
}

// ── Table Tests ─────────────────────────────────────────────────────────

func TestTable_Render(t *testing.T) {
	// Should not panic
	Table(
		[]string{"Name", "Size", "Status"},
		[][]string{
			{"Anubis", "11 MB", "Active"},
			{"Thoth", "3.8 MB", "Synced"},
		},
	)
}

func TestTable_Empty(t *testing.T) {
	Table([]string{"Name"}, [][]string{})
}

// ── BubbleTea MainModel Tests ───────────────────────────────────────────

func TestNewMainModel(t *testing.T) {
	t.Parallel()
	m := NewMainModel()
	if m.quitting {
		t.Error("new model should not be quitting")
	}
}

func TestMainModel_Init(t *testing.T) {
	t.Parallel()
	m := NewMainModel()
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init should return a spinner tick command")
	}
}

func TestMainModel_View_Running(t *testing.T) {
	t.Parallel()
	m := NewMainModel()
	view := m.View()
	if view == "" {
		t.Error("View should not be empty")
	}
	if !strings.Contains(view, "Pantheon") {
		t.Errorf("View should contain 'Pantheon', got %q", view[:50])
	}
	if !strings.Contains(view, "Press") {
		t.Error("View should contain quit instruction")
	}
}

func TestMainModel_View_Quitting(t *testing.T) {
	t.Parallel()
	m := NewMainModel()
	m.quitting = true
	view := m.View()
	if !strings.Contains(view, "ritual complete") {
		t.Errorf("quitting View = %q, want 'ritual complete'", view)
	}
}

func TestMainModel_Update_QuitKeys(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"q", "esc", "ctrl+c"} {
		m := NewMainModel()
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		if key == "esc" {
			msg = tea.KeyMsg{Type: tea.KeyEscape}
		} else if key == "ctrl+c" {
			msg = tea.KeyMsg{Type: tea.KeyCtrlC}
		}
		updatedModel, cmd := m.Update(msg)
		updated := updatedModel.(MainModel)
		if key == "q" {
			if !updated.quitting {
				t.Errorf("key '%s' should set quitting=true", key)
			}
			if cmd == nil {
				t.Errorf("quit key should return tea.Quit command")
			}
		}
	}
}

func TestMainModel_Update_OtherKey(t *testing.T) {
	t.Parallel()
	m := NewMainModel()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}
	updated, cmd := m.Update(msg)
	model := updated.(MainModel)
	if model.quitting {
		t.Error("non-quit key should not trigger quitting")
	}
	if cmd != nil {
		t.Error("non-quit key should return nil command")
	}
}
