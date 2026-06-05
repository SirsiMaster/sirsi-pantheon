package tui

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
)

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
