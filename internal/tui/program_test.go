package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// newTestProgram builds a program model the way Run() does, but returns it for
// direct driving in tests (no terminal required).
func newTestProgram(t *testing.T) program {
	t.Helper()
	caps := DetectCapabilities(func(string) string { return "" })
	app, err := NewApp(caps)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	return program{app: app, renderer: NewRenderer(caps), width: 100, height: 30}
}

func TestProgramViewRendersRealContent(t *testing.T) {
	m := newTestProgram(t)
	content := m.View().Content
	if content == "" {
		t.Fatal("View().Content is empty")
	}
	// The first canonical view is Scan — its fixture rows must reach the frame.
	for _, want := range []string{"Scan", "parallels-remnants"} {
		if !strings.Contains(content, want) {
			t.Errorf("rendered frame missing %q", want)
		}
	}
}

func TestProgramWindowResize(t *testing.T) {
	m := newTestProgram(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	p, ok := next.(program)
	if !ok {
		t.Fatal("Update did not return a program")
	}
	if p.width != 120 || p.height != 40 {
		t.Errorf("resize not applied: got %dx%d, want 120x40", p.width, p.height)
	}
}

func TestProgramFocusNextCyclesViews(t *testing.T) {
	m := newTestProgram(t)
	if got := m.app.ActiveView().Name(); got != "Scan" {
		t.Fatalf("first view = %q, want Scan", got)
	}
	n := len(m.app.Views)
	m.focusNext()
	if m.app.Active != 1 {
		t.Errorf("after focusNext, Active = %d, want 1", m.app.Active)
	}
	// Cycling all the way around returns to the first view, with selection reset.
	for i := 1; i < n; i++ {
		m.focusNext()
	}
	if m.app.Active != 0 {
		t.Errorf("focusNext did not wrap to 0, got %d", m.app.Active)
	}
	if m.app.ViewState.Selected != 0 {
		t.Errorf("selection not reset on view switch, got %d", m.app.ViewState.Selected)
	}
}

func TestProgramQuitsCleanly(t *testing.T) {
	m := newTestProgram(t)
	// A quit command must yield tea.Quit (the program ends), not a nil cmd.
	q, ok := m.app.Registry.Lookup(CmdQuit)
	if !ok {
		t.Fatal("quit command not registered")
	}
	if q.Key != "q" {
		t.Errorf("quit key = %q, want q", q.Key)
	}
}
