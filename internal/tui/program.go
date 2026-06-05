package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// NewView builds a data-backed View from real values. The renderer reads
// Table() each frame, so a caller refreshes by rebuilding the views. This is
// the public seam that lets cmd/sirsi feed live scan/router/thread data into
// the same contract the fixtures use.
func NewView(name string, layout Layout, hints []CommandID, table Table) View {
	return fixtureView{name: name, layout: layout, hintIDs: hints, table: table}
}

// RunViews runs the live console over caller-provided views (real data). It
// validates every view against the canonical registry first, so a view that
// advertises an unwired hint fails fast rather than rendering a dead key.
// Falls back to the fixture proof views when views is empty.
func RunViews(views []View) error {
	if len(views) == 0 {
		return Run()
	}
	caps := DetectCapabilities(os.Getenv)
	reg, err := DefaultRegistry()
	if err != nil {
		return err
	}
	for _, v := range views {
		if err := ValidateView(reg, v); err != nil {
			return fmt.Errorf("tui: invalid view %q: %w", v.Name(), err)
		}
	}
	app := &AppState{
		Views:     views,
		Active:    0,
		Registry:  reg,
		Caps:      caps,
		ViewState: ViewState{RowCount: len(views[0].Table().Rows), Selected: 0},
	}
	m := program{app: app, renderer: NewRenderer(caps), width: MinWidth, height: MinHeight}
	_, err = tea.NewProgram(m).Run()
	return err
}

// program is the Bubbletea model that makes the rendering contract live.
//
// The contract is already Elm-shaped — AppState is the model, Reduce is a pure
// update, and Renderer projects state to lines — so this adapter is thin: a
// keypress resolves to a Command via the Registry; app-scope commands
// (focus/quit) act on AppState; everything else flows through Reduce into
// ViewState. No key can mutate the screen except through a registered command,
// so the v0.22 "keys did nothing" failure is unrepresentable here too.
type program struct {
	app      *AppState
	renderer Renderer
	width    int
	height   int
}

// Run builds and runs the live console over the canonical views. It blocks
// until the user quits and returns any terminal/runtime error.
func Run() error {
	caps := DetectCapabilities(os.Getenv)
	app, err := NewApp(caps)
	if err != nil {
		return err
	}
	m := program{
		app:      app,
		renderer: NewRenderer(caps),
		width:    MinWidth,
		height:   MinHeight,
	}
	_, err = tea.NewProgram(m).Run()
	return err
}

// Init implements tea.Model.
func (m program) Init() tea.Cmd { return nil }

// Update implements tea.Model. Window resizes update the render box; key
// presses route through the registry and the pure reducer.
func (m program) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		cmd, ok := m.app.Registry.ResolveKey(msg.String())
		if !ok {
			return m, nil
		}
		switch cmd.ID {
		case CmdQuit:
			return m, tea.Quit
		case CmdFocusNext:
			m.focusNext()
		default:
			m.app.ViewState = Reduce(m.app.ViewState, cmd)
		}
	}
	return m, nil
}

// focusNext cycles to the next view and resets selection to that view's bounds.
func (m program) focusNext() {
	if len(m.app.Views) == 0 {
		return
	}
	m.app.Active = (m.app.Active + 1) % len(m.app.Views)
	m.app.ViewState = ViewState{
		RowCount: len(m.app.ActiveView().Table().Rows),
		Selected: 0,
	}
}

// View implements tea.Model, projecting AppState through the renderer.
func (m program) View() tea.View {
	return tea.NewView(strings.Join(m.renderer.Render(m.app, m.width, m.height), "\n"))
}
