package output

import (
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// ── formatElapsed ────────────────────────────────────────────────────────

func TestFormatElapsed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"seconds", 45 * time.Second, "45s"},
		{"minutes", 3*time.Minute + 15*time.Second, "3m15s"},
		{"hours", 2*time.Hour + 30*time.Minute, "2h30m"},
		{"zero", 0, "0s"},
		{"sub-second", 500 * time.Millisecond, "0s"},
		{"exactly one minute", time.Minute, "1m0s"},
		{"exactly one hour", time.Hour, "1h0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatElapsed(tt.d)
			if got != tt.want {
				t.Errorf("formatElapsed(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

// ── min ──────────────────────────────────────────────────────────────────

func TestMin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{5, 3, 3},
		{7, 7, 7},
		{-1, 0, -1},
	}
	for _, tt := range tests {
		got := min(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// ── atoi ──────────────────────────────────────────────────────────────────

func TestAtoi(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  int
	}{
		{"42", 42},
		{"0", 0},
		{"", 0},
		{"invalid", 0},
		{"-5", -5},
	}
	for _, tt := range tests {
		got := atoi(tt.input)
		if got != tt.want {
			t.Errorf("atoi(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// ── pluralize ────────────────────────────────────────────────────────────

func TestPluralize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		word string
		n    int
		want string
	}{
		{"deity", 1, "deity"},
		{"deity", 2, "deities"},
		{"deity", 0, "deities"},
		{"scope", 1, "scope"},
		{"scope", 3, "scopes"},
		{"cat", 2, "cats"},
	}
	for _, tt := range tests {
		got := pluralize(tt.word, tt.n)
		if got != tt.want {
			t.Errorf("pluralize(%q, %d) = %q, want %q", tt.word, tt.n, got, tt.want)
		}
	}
}

// ── deityDisplay ─────────────────────────────────────────────────────────

func TestDeityDisplay(t *testing.T) {
	t.Parallel()

	// Known deity
	glyph, name := deityDisplay("ra")
	if glyph != "𓇶" {
		t.Errorf("ra glyph = %q, want 𓇶", glyph)
	}
	if name != "Ra" {
		t.Errorf("ra name = %q, want Ra", name)
	}

	// Unknown deity falls back
	glyph, name = deityDisplay("unknown")
	if glyph != "⚙" {
		t.Errorf("unknown glyph = %q, want ⚙", glyph)
	}
	if name != "unknown" {
		t.Errorf("unknown name = %q, want unknown", name)
	}
}

// ── buildSuggestions ─────────────────────────────────────────────────────

func TestBuildSuggestions_Empty(t *testing.T) {
	t.Parallel()
	got := buildSuggestions("", nil)
	if got != nil {
		t.Errorf("empty input should return nil, got %v", got)
	}
}

func TestBuildSuggestions_FirstWord(t *testing.T) {
	t.Parallel()
	got := buildSuggestions("sc", nil)
	if len(got) == 0 {
		t.Fatal("expected suggestions for 'sc'")
	}
	found := false
	for _, s := range got {
		if s == "scan" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'scan' in suggestions, got %v", got)
	}
}

func TestBuildSuggestions_DeitySubcommand(t *testing.T) {
	t.Parallel()
	// After typing "ra " (trailing space), should suggest ra subcommands
	got := buildSuggestions("ra ", nil)
	found := false
	for _, s := range got {
		if s == "ra deploy" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'ra deploy' in suggestions for 'ra ', got %v", got)
	}
}

func TestBuildSuggestions_DeityFlags(t *testing.T) {
	t.Parallel()
	// "ra deploy " should suggest flags
	got := buildSuggestions("ra deploy ", nil)
	found := false
	for _, s := range got {
		if strings.Contains(s, "--scope") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --scope flag in suggestions for 'ra deploy ', got %v", got)
	}
}

func TestBuildSuggestions_HistoryFirst(t *testing.T) {
	t.Parallel()
	history := []string{"anubis ka", "anubis weigh"}
	got := buildSuggestions("anubis", history)
	if len(got) == 0 {
		t.Fatal("expected suggestions")
	}
	// History match should appear before command tree
	if got[0] != "anubis weigh" && got[0] != "anubis ka" {
		t.Errorf("first suggestion should be from history, got %q", got[0])
	}
}

func TestBuildSuggestions_SubcommandCompletion(t *testing.T) {
	t.Parallel()
	got := buildSuggestions("thoth s", nil)
	found := false
	for _, s := range got {
		if s == "thoth sync" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'thoth sync' in suggestions for 'thoth s', got %v", got)
	}
}

// ── deduplicateHistory ───────────────────────────────────────────────────

func TestDeduplicateHistory(t *testing.T) {
	t.Parallel()
	history := []historyEntry{
		{command: "anubis weigh"},
		{command: "ra status"},
		{command: "anubis weigh"}, // dup
		{command: "thoth sync"},
	}
	got := deduplicateHistory(history)
	if len(got) != 3 {
		t.Fatalf("expected 3 unique commands, got %d: %v", len(got), got)
	}
}

func TestDeduplicateHistory_Empty(t *testing.T) {
	t.Parallel()
	got := deduplicateHistory(nil)
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

func TestDeduplicateHistory_EmptyCommand(t *testing.T) {
	t.Parallel()
	history := []historyEntry{
		{command: ""},
		{command: "scan"},
	}
	got := deduplicateHistory(history)
	if len(got) != 1 {
		t.Errorf("expected 1 (empty should be skipped), got %d", len(got))
	}
}

// ── dispatch ─────────────────────────────────────────────────────────────

func TestDispatch_DirectDeity(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	deity, args, intent := m.dispatch("ra status")
	if deity != "ra" {
		t.Errorf("deity = %q, want ra", deity)
	}
	if len(args) != 2 || args[0] != "ra" || args[1] != "status" {
		t.Errorf("args = %v, want [ra status]", args)
	}
	if intent {
		t.Error("direct deity command should not be intent-matched")
	}
}

func TestDispatch_CLIAlias(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	deity, _, intent := m.dispatch("scan")
	if deity != "anubis" {
		t.Errorf("deity = %q, want anubis", deity)
	}
	if intent {
		t.Error("CLI alias should not be intent-matched")
	}
}

func TestDispatch_IntentMatch(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	deity, _, intent := m.dispatch("check how secure my network is")
	if deity == "" {
		t.Fatal("expected deity match for network intent")
	}
	if !intent {
		t.Error("natural language should be intent-matched")
	}
}

func TestDispatch_Empty(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	deity, _, _ := m.dispatch("")
	if deity != "" {
		t.Errorf("empty input should return empty deity, got %q", deity)
	}
}

// ── inferSubcommand ──────────────────────────────────────────────────────

func TestInferSubcommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		deity   string
		input   string
		wantCmd string
	}{
		{"isis network", "isis", "check network dns", "isis"},
		{"anubis ghosts", "anubis", "find ghost apps", "anubis"},
		{"thoth sync", "thoth", "sync memory", "thoth"},
		{"maat audit", "maat", "run audit checks", "maat"},
		{"seba gpu", "seba", "show gpu status", "seba"},
		{"ra status", "ra", "show status", "ra"},
		{"fallback bare", "seshat", "do something", "seshat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := inferSubcommand(tt.deity, tt.input)
			if len(args) == 0 {
				t.Fatal("expected at least 1 arg")
			}
			if args[0] != tt.wantCmd {
				t.Errorf("inferSubcommand(%q, %q)[0] = %q, want %q", tt.deity, tt.input, args[0], tt.wantCmd)
			}
		})
	}
}

// ── renderHints ──────────────────────────────────────────────────────────

func TestRenderHints(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()

	// Idle mode
	hints := m.renderHints(false)
	if !strings.Contains(hints, "history") {
		t.Errorf("idle hints should contain 'history', got %q", hints)
	}

	// Split mode
	hints = m.renderHints(true)
	if !strings.Contains(hints, "esc back") {
		t.Errorf("split mode hints should contain 'esc back', got %q", hints)
	}

	// Running mode
	m.mode = modeRunning
	hints = m.renderHints(false)
	if !strings.Contains(hints, "scroll") {
		t.Errorf("running hints should contain 'scroll', got %q", hints)
	}
}

// ── renderQuickActions ───────────────────────────────────────────────────

func TestRenderQuickActions(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	actions := m.renderQuickActions()
	if !strings.Contains(actions, "1") {
		t.Error("quick actions should contain option 1")
	}
	if !strings.Contains(actions, "2") {
		t.Error("quick actions should contain option 2")
	}
	if !strings.Contains(actions, "3") {
		t.Error("quick actions should contain option 3")
	}
}

// ── renderStatusLine ─────────────────────────────────────────────────────

func TestRenderStatusLine_NoActive(t *testing.T) {
	t.Parallel()
	m := TUIModel{
		activeDeity: make(map[string]bool),
	}
	status := m.renderStatusLine()
	if status != "" {
		t.Errorf("no active deities should produce empty string, got %q", status)
	}
}

func TestRenderStatusLine_Active(t *testing.T) {
	t.Parallel()
	m := TUIModel{
		activeDeity: map[string]bool{"ra": true, "thoth": true},
	}
	status := m.renderStatusLine()
	if !strings.Contains(status, "2") {
		t.Errorf("should report 2 active, got %q", status)
	}
	if !strings.Contains(status, "deities") {
		t.Errorf("should pluralize, got %q", status)
	}
}

func TestRenderStatusLine_SingleActive(t *testing.T) {
	t.Parallel()
	m := TUIModel{
		activeDeity: map[string]bool{"ra": true},
	}
	status := m.renderStatusLine()
	if !strings.Contains(status, "1") {
		t.Errorf("should report 1 active, got %q", status)
	}
	if !strings.Contains(status, "deity") {
		t.Errorf("should use singular, got %q", status)
	}
}

// ── renderRosterColumns ──────────────────────────────────────────────────

func TestRenderRosterColumns(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	m.width = 100

	roster := m.renderRosterColumns(false)
	if roster == "" {
		t.Error("roster should not be empty")
	}
	// Should contain at least some deity names
	if !strings.Contains(roster, "Ra") {
		t.Errorf("roster should contain Ra, got %q", roster[:min(len(roster), 200)])
	}
}

func TestRenderRosterColumns_Compact(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	roster := m.renderRosterColumns(true)
	if roster == "" {
		t.Error("compact roster should not be empty")
	}
}

func TestRenderRosterColumns_Narrow(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	m.width = 50
	roster := m.renderRosterColumns(false)
	if roster == "" {
		t.Error("narrow roster should not be empty")
	}
}

// ── renderDeityCell ──────────────────────────────────────────────────────

func TestRenderDeityCell(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	cell := m.renderDeityCell(deityRoster[0], 30)
	if cell == "" {
		t.Error("deity cell should not be empty")
	}
	if !strings.Contains(cell, "Ra") {
		t.Errorf("cell should contain Ra, got %q", cell)
	}
}

func TestRenderDeityCell_Active(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	m.activeDeity["ra"] = true
	cell := m.renderDeityCell(deityRoster[0], 30)
	if cell == "" {
		t.Error("active deity cell should not be empty")
	}
}

// ── recalcViewportHeight ─────────────────────────────────────────────────

func TestRecalcViewportHeight(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	m.height = 40
	m.width = 100
	m.recalcViewportHeight()
	if m.viewport.Height < 5 {
		t.Errorf("viewport height = %d, expected >= 5", m.viewport.Height)
	}
}

func TestRecalcViewportHeight_Small(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	m.height = 5
	m.width = 60
	m.recalcViewportHeight()
	if m.viewport.Height < 5 {
		t.Errorf("small terminal viewport height = %d, expected >= 5", m.viewport.Height)
	}
}

// ── applySteleEntry ──────────────────────────────────────────────────────

func TestApplySteleEntry_Variants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		scope     string
		entryType string
		data      map[string]string
	}{
		{"tool use", "scope1", stele.TypeToolUse, map[string]string{"tool": "read"}},
		{"governance", "scope1", stele.TypeGovernance, map[string]string{"maat": "PASS", "thoth": "compacted"}},
		{"text", "scope1", stele.TypeText, map[string]string{"text": "hello world"}},
		{"commit", "scope1", stele.TypeCommit, map[string]string{"hash": "abc123"}},
		{"complete", "scope1", stele.TypeComplete, nil},
		{"failed", "scope1", stele.TypeFailed, nil},
		{"sprint start", "scope1", stele.TypeSprintStart, map[string]string{"sprint": "2", "sprints": "5"}},
		{"sprint end", "scope1", stele.TypeSprintEnd, map[string]string{"sprint": "2", "sprints": "5"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMainModel()
			data := tt.data
			if data == nil {
				data = make(map[string]string)
			}
			m.applySteleEntry(stele.Entry{Scope: tt.scope, Type: tt.entryType, Data: data, Deity: "test"})
		})
	}
}

func TestApplyGlobalEvent(t *testing.T) {
	t.Parallel()
	m := NewMainModel()

	for i := 0; i < 15; i++ {
		m.applyGlobalEvent(stele.Entry{Deity: "test", Type: "test", Data: map[string]string{"key": "value"}})
	}
	if len(m.globalLog) > 12 {
		t.Errorf("globalLog should cap at 12, got %d", len(m.globalLog))
	}
}

func TestApplyGlobalEvent_LongValue(t *testing.T) {
	t.Parallel()
	m := NewMainModel()
	m.globalLog = nil // clear any pre-loaded entries
	longVal := strings.Repeat("x", 100)
	m.applyGlobalEvent(stele.Entry{Deity: "test", Type: "test", Data: map[string]string{"longkey": longVal}})
	if len(m.globalLog) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(m.globalLog))
	}
	if strings.Contains(m.globalLog[0], longVal) {
		t.Error("long value should be truncated")
	}
}

// ── renderGovernanceLegend ────────────────────────────────────────────────

func TestRenderGovernanceLegend(t *testing.T) {
	t.Parallel()
	m := NewMainModel()
	legend := m.renderGovernanceLegend()
	if !strings.Contains(legend, "Governance") {
		t.Error("legend should contain 'Governance'")
	}
}

// ── renderAcceptancePrompt ───────────────────────────────────────────────

func TestRenderAcceptancePrompt(t *testing.T) {
	t.Parallel()
	m := NewMainModel()
	m.scopes = []scopeView{
		{Name: "test1", State: "completed"},
		{Name: "test2", State: "failed"},
		{Name: "test3", State: "running"},
	}
	prompt := m.renderAcceptancePrompt()
	if !strings.Contains(prompt, "complete") {
		t.Error("acceptance prompt should mention completion")
	}
}

// ── renderScope ──────────────────────────────────────────────────────────

func TestRenderScope(t *testing.T) {
	t.Parallel()

	m := NewMainModel()
	m.width = 100

	tests := []struct {
		name  string
		sv    scopeView
		focus bool
	}{
		{"running", scopeView{Name: "test", State: "running", Sprint: 1, Sprints: 3, Icon: "🔄", LastTool: "[tool: read]"}, false},
		{"completed", scopeView{Name: "test", State: "completed", Icon: "✅"}, false},
		{"failed", scopeView{Name: "test", State: "failed", Icon: "❌"}, false},
		{"idle", scopeView{Name: "test", State: "idle", Icon: "⚫"}, false},
		{"focused with logs", scopeView{Name: "test", State: "running", LogTail: []string{"line1", "line2"}, Icon: "🔄"}, true},
		{"with governance", scopeView{Name: "test", State: "running", MaatGate: "PASS", ThothSync: "compacted", Icon: "🔄"}, false},
		{"maat fail", scopeView{Name: "test", State: "running", MaatGate: "FAIL", Icon: "🔄"}, false},
		{"multi sprint completed", scopeView{Name: "test", State: "completed", Sprint: 3, Sprints: 5, Icon: "✅"}, false},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.focus {
				m.focused = i
			}
			result := m.renderScope(i, tt.sv)
			if result == "" {
				t.Error("renderScope should not return empty string")
			}
		})
	}
}

// ── TUI View ─────────────────────────────────────────────────────────────

func TestTUIModel_View_NoOutput(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	m.width = 100
	m.height = 40
	view := m.View()
	if !strings.Contains(view, "Sirsi Pantheon") {
		t.Error("view should contain 'Sirsi Pantheon'")
	}
}

func TestTUIModel_View_Quitting(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	m.quitting = true
	view := m.View()
	if view != "" {
		t.Errorf("quitting view should be empty, got len=%d", len(view))
	}
}

func TestTUIModel_View_WithOutput(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	m.width = 100
	m.height = 40
	m.outputLines = []string{"test output line 1", "test output line 2"}
	view := m.View()
	if view == "" {
		t.Error("view with output should not be empty")
	}
}

func TestTUIModel_View_Narrow(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	m.width = 50
	m.height = 30
	m.outputLines = []string{"narrow output"}
	view := m.View()
	if view == "" {
		t.Error("narrow view should not be empty")
	}
}

// ── showHelp ─────────────────────────────────────────────────────────────

func TestShowHelp(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	m.width = 100
	m.height = 40
	updatedModel, _ := m.showHelp()
	if len(updatedModel.outputLines) == 0 {
		t.Error("showHelp should populate output lines")
	}
}

// ── renderLeftPane / renderRightPane ─────────────────────────────────────

func TestRenderLeftPane(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	pane := m.renderLeftPane()
	if pane == "" {
		t.Error("left pane should not be empty")
	}
}

func TestRenderRightPane(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	pane := m.renderRightPane()
	// No output lines, so it should just be the viewport
	if pane == "" {
		t.Error("right pane should not be empty")
	}
}

func TestRenderRightPane_Running(t *testing.T) {
	t.Parallel()
	m := NewTUIModel()
	m.mode = modeRunning
	m.runningDeity = "ra"
	pane := m.renderRightPane()
	if !strings.Contains(pane, "running") {
		t.Error("running right pane should show 'running'")
	}
}
