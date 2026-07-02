package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Deterministic console tests. Every screen's data is fed as canned JSON through
// the injected runner (Rule A16) — the tests NEVER shell out. Because the runner
// is a package global swapped under a lock, these tests MUST NOT use t.Parallel()
// (repo lessons #129/#131/#139).
//
// The interactive path cannot be driven through a real TTY headlessly, so the
// evidence is golden-frame comparisons of each screen's rendered View() at a
// fixed size plus model-behavior assertions on Update.

// stubRunner returns canned JSON keyed by verb. A missing verb returns an empty
// object so a decode still succeeds (the screen then shows its empty state).
type stubRunner map[string]string

func (s stubRunner) RunJSON(_ context.Context, verb string, _ ...string) ([]byte, error) {
	if body, ok := s[verb]; ok {
		return []byte(body), nil
	}
	return []byte(`{}`), nil
}

// withStub installs a canned runner for the duration of fn and restores the
// previous runner afterward (under the lock, Rule A21).
func withStub(t *testing.T, stub stubRunner, fn func()) {
	t.Helper()
	old := getRunner()
	setRunner(stub)
	defer setRunner(old)
	fn()
}

// capturedCall records one dispatch through the runner seam: the verb and its
// args (excluding the runner-appended --json). It is the evidence for the
// per-item --only dispatch and the confirm-gated Health fix.
type capturedCall struct {
	verb string
	args []string
}

// captureRunner wraps a body-source with a recorded call log so a test can assert
// the EXACT verb+args a screen dispatched. It never shells out.
type captureRunner struct {
	body  func(verb string, args []string) string
	calls *[]capturedCall
}

func (c captureRunner) RunJSON(_ context.Context, verb string, args ...string) ([]byte, error) {
	*c.calls = append(*c.calls, capturedCall{verb: verb, args: append([]string(nil), args...)})
	body := "{}"
	if c.body != nil {
		body = c.body(verb, args)
	}
	return []byte(body), nil
}

// withCapture installs a capturing runner that returns fullStub() bodies for the
// data verbs (so screens load) and records every call. fn receives the live call
// log pointer.
func withCapture(t *testing.T, fn func(calls *[]capturedCall)) {
	t.Helper()
	stub := fullStub()
	var calls []capturedCall
	cr := captureRunner{
		calls: &calls,
		body: func(verb string, _ []string) string {
			if body, ok := stub[verb]; ok {
				return body
			}
			return `{}`
		},
	}
	old := getRunner()
	setRunner(cr)
	defer setRunner(old)
	fn(&calls)
}

// firstCallOf returns the first recorded call whose verb == verb (an action
// dispatch, before any post-action reload), or fails.
func firstCallOf(t *testing.T, calls []capturedCall, verb string) capturedCall {
	t.Helper()
	for _, c := range calls {
		if c.verb == verb {
			return c
		}
	}
	t.Fatalf("no runner call for verb %q; recorded: %+v", verb, calls)
	return capturedCall{}
}

// runTeaCmd executes a tea.Cmd (and each cmd in a batch) purely for its side
// effects — the runner call it makes is recorded. It does NOT deliver the result
// back, so no post-action reload fires; the recorded call is exactly the action
// dispatch under test.
func runTeaCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			runTeaCmd(c)
		}
	}
}

// hasArg reports whether args contains v.
func hasArg(args []string, v string) bool {
	for _, a := range args {
		if a == v {
			return true
		}
	}
	return false
}

// onlyPaths extracts every value following an --only flag in args.
func onlyPaths(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--only" && i+1 < len(args) {
			out = append(out, args[i+1])
		}
	}
	return out
}

// testCaps is a deterministic, ASCII-only, color-free capability profile so
// golden frames are stable across terminals (no ANSI bytes to diff).
func testCaps() Capabilities {
	return Capabilities{Color: ColorNone, UnicodeLayout: false, ReducedMotion: true, AltScreen: true}
}

// newTestApp builds an App at a fixed 100x30 size with the plain test caps, then
// drives the given screen to ready by delivering its load message.
func newTestApp(t *testing.T) *App {
	t.Helper()
	app, err := NewApp(testCaps())
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	app.width, app.height = 100, 30
	return app
}

// drive delivers a message to the app's active screen (bypassing the tea loop)
// and applies the returned screen, mirroring App.Update's default branch.
func drive(app *App, msg tea.Msg) {
	next, _ := app.activeScreen().Update(msg, app.caps)
	app.screens[app.active] = next
}

// driveCmd is drive but returns the screen's follow-up command so a test can run
// an action dispatch (the runner call fires when the command is executed).
func driveCmd(app *App, msg tea.Msg) tea.Cmd {
	next, cmd := app.activeScreen().Update(msg, app.caps)
	app.screens[app.active] = next
	return cmd
}

// --- canned contract fixtures ---

const (
	fxVitals = `{"command":"sirsi vitals","total_bytes":51539607552,"used_bytes":20959772672,"free_bytes":30579834880,"swap_used_bytes":13474526658,"pressure":"normal","pressure_source":"bootstrap-snapshot","top":[{"name":"mediaanalysisd","pid":29361,"rss_bytes":1784528896},{"name":"codex","pid":74439,"rss_bytes":1019805696}]}`

	fxScan = `{"Findings":[{"RuleName":"npm_global_cache","Category":"dev","Description":"npm/yarn/pnpm Cache","Path":"/Users/x/.npm/_cacache","SizeBytes":2554200676,"FileCount":12258,"Severity":"caution","CanFix":true},{"RuleName":"firebase_caches","Category":"cloud","Description":"Firebase CLI","Path":"/Users/x/.cache/firebase","SizeBytes":1133483950,"FileCount":35,"Severity":"safe","CanFix":true}],"TotalSize":9328484647,"ReclaimableSize":3687684626,"RulesRan":81}`

	fxGhosts = `{"command":"sirsi ghosts","summary":"Found 2 ghost apps with 920.7 KB of reclaimable waste","ghost_count":2,"total_waste_bytes":942813,"total_waste":"920.7 KB","ghosts":[{"app_name":"Sky","bundle_id":"com.openai.sky.CUAService","total_size_bytes":387189,"total_files":8,"in_launch_services":false,"residuals":[{"path":"/Users/x/Library/Caches/com.openai.sky","type":"Caches","size_bytes":255813,"file_count":4}]},{"app_name":"GoogleUpdater","bundle_id":"com.google.GoogleUpdater","total_size_bytes":242416,"total_files":6,"in_launch_services":false,"residuals":[]}]}`

	fxActivity = `{"command":"sirsi activity","log_path":"/tmp/ops.log","count":2,"entries":[{"time":"2026-07-01T22:04:00","action":"purge","target":"/Users/x/Development/node_modules","bytes":100,"source":"oplog"},{"time":"2026-06-29T19:54:21","action":"clean","target":"/Users/x/.cache/firebase","bytes":512,"source":"oplog"}]}`

	fxDiag = `{"timestamp":"2026-07-01T22:39:38","duration":"1.171s","findings":[{"check":"RAM Pressure","severity":0,"message":"RAM healthy at 34%"},{"check":"binary-drift","severity":2,"message":"Sirsi binary drift detected","detail":"PATH binary differs","fix":"sirsi self-update","fixKind":"instant"},{"check":"Jetsam Events (7d)","severity":1,"message":"7 Jetsam memory kills","trend":true,"activeDays":7,"fix":"sirsi relieve --memory","fixKind":"relief"}]}`
)

func fullStub() stubRunner {
	return stubRunner{
		"vitals":   fxVitals,
		"scan":     fxScan,
		"ghosts":   fxGhosts,
		"activity": fxActivity,
		"diagnose": fxDiag,
	}
}

// loadScreen drives the active screen to ready by running its Load command and
// delivering the resulting data message. Load() fetches data only — the Pulse
// auto-refresh tick is scheduled by the App runloop (Init/keyMsg), never inside
// Load — so there is no blocking timer to sidestep here.
func loadScreen(app *App) {
	cmd := app.activeScreen().Load()
	if cmd == nil {
		return
	}
	deliver(app, cmd())
}

// deliver applies a message (or each message in a batch) to the active screen.
func deliver(app *App, msg tea.Msg) {
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				deliver(app, c())
			}
		}
		return
	}
	drive(app, msg)
}

// --- golden frames: every screen renders ready without a blank frame ---

func TestGoldenPulse(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t) // active = Pulse
		loadScreen(app)
		frame := strings.Join(app.render(), "\n")
		for _, want := range []string{"Horus", "Pulse", "MEMORY", "TOP MEMORY USERS", "mediaanalysisd", "PRESSURE", "press r"} {
			if !strings.Contains(strings.ToUpper(frame), strings.ToUpper(want)) {
				t.Errorf("Pulse frame missing %q\n---\n%s", want, frame)
			}
		}
		// The relieve hero beat is surfaced (bound to r, proof §3.2: "r relieve").
		// The status bar wraps the key glyph in bold, so assert on the hint LABEL;
		// the r=relieve binding itself is proven by TestPulseRelieveBoundToR /
		// TestRefreshBoundToU_NotR.
		if !strings.Contains(frame, "relieve") {
			t.Errorf("Pulse status bar missing the relieve hint\n---\n%s", frame)
		}
		if strings.Contains(frame, "refresh") {
			t.Errorf("Pulse still advertises the old refresh hint (r should be relieve)\n---\n%s", frame)
		}
	})
}

func TestGoldenWaste(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		app.focus(1) // Waste
		loadScreen(app)
		frame := strings.Join(app.render(), "\n")
		for _, want := range []string{"Waste", "npm/yarn/pnpm Cache", "reclaimable", "RULE", "TIER", "space checks", "0 selected"} {
			if !strings.Contains(frame, want) {
				t.Errorf("Waste frame missing %q\n---\n%s", want, frame)
			}
		}
	})
}

func TestGoldenGhosts(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		app.focus(2)
		loadScreen(app)
		frame := strings.Join(app.render(), "\n")
		for _, want := range []string{"Ghosts", "Sky", "GoogleUpdater", "BUNDLE", "WASTE"} {
			if !strings.Contains(frame, want) {
				t.Errorf("Ghosts frame missing %q\n---\n%s", want, frame)
			}
		}
	})
}

func TestGoldenHealth(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		app.focus(3)
		loadScreen(app)
		frame := strings.Join(app.render(), "\n")
		for _, want := range []string{"Health", "binary drift", "CHECK", "STATUS", "BLOCK"} {
			if !strings.Contains(frame, want) {
				t.Errorf("Health frame missing %q\n---\n%s", want, frame)
			}
		}
	})
}

func TestGoldenActivity(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		app.focus(4)
		loadScreen(app)
		frame := strings.Join(app.render(), "\n")
		for _, want := range []string{"Activity", "purge", "clean", "TARGET", "audit ledger"} {
			if !strings.Contains(frame, want) {
				t.Errorf("Activity frame missing %q\n---\n%s", want, frame)
			}
		}
	})
}

// --- empty / error states never render a blank frame ---

func TestEmptyStates(t *testing.T) {
	withStub(t, stubRunner{}, func() { // every verb returns "{}"
		for i, want := range map[int]string{
			1: "no reclaimable waste",
			2: "no ghost residuals",
			4: "no operations logged",
		} {
			app := newTestApp(t)
			app.focus(i)
			loadScreen(app)
			frame := strings.Join(app.render(), "\n")
			if !strings.Contains(frame, want) {
				t.Errorf("screen %d empty state missing %q\n%s", i, want, frame)
			}
		}
	})
}

func TestLoadingStateBeforeData(t *testing.T) {
	app := newTestApp(t)
	// Before Load resolves, the screen is idle → loading frame, never blank.
	frame := strings.Join(app.render(), "\n")
	if !strings.Contains(frame, "sampling memory") {
		t.Errorf("Pulse pre-load frame not a loading state:\n%s", frame)
	}
}

// --- resize safety ---

func TestResizeSafe(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		loadScreen(app)
		for _, wh := range [][2]int{{80, 24}, {120, 40}, {200, 60}, {100, 30}} {
			m, _ := app.Update(tea.WindowSizeMsg{Width: wh[0], Height: wh[1]})
			app = m.(*App)
			lines := app.render()
			if len(lines) != wh[1] {
				t.Errorf("at %dx%d render produced %d lines, want %d", wh[0], wh[1], len(lines), wh[1])
			}
			for i, ln := range lines {
				if visibleWidth(ln) > wh[0] {
					t.Errorf("at %dx%d line %d exceeds width: %q", wh[0], wh[1], i, ln)
				}
			}
		}
	})
}

func TestTooSmallTakeover(t *testing.T) {
	app := newTestApp(t)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	app = m.(*App)
	frame := strings.Join(app.render(), "\n")
	if !strings.Contains(frame, "Horus needs") {
		t.Errorf("small terminal did not show the takeover:\n%s", frame)
	}
}

// --- navigation: 1-5 jump, tab cycles ---

func TestScreenJumpAndTab(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		wants := []string{"Pulse", "Waste", "Ghosts", "Health", "Activity"}
		for i, key := range []string{"1", "2", "3", "4", "5"} {
			m, _ := app.handleKey(key)
			app = m.(*App)
			if got := app.activeScreen().Name(); got != wants[i] {
				t.Errorf("key %q -> %q, want %q", key, got, wants[i])
			}
		}
		// tab from Activity wraps to Pulse.
		m, _ := app.handleKey("tab")
		app = m.(*App)
		if got := app.activeScreen().Name(); got != "Pulse" {
			t.Errorf("tab wrap -> %q, want Pulse", got)
		}
	})
}

// --- destructive confirm gate (Rule A1): c arms, it does not clean ---

func TestCleanRequiresSecondConfirmation(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		app.focus(1) // Waste
		loadScreen(app)
		ws := app.activeScreen().(*wasteScreen)
		if ws.confirm {
			t.Fatal("confirm armed before pressing c")
		}
		// With nothing checked, c is a no-op — there is no set to confirm.
		clean, _ := app.reg.ResolveKey("c")
		drive(app, keyMsg{cmd: clean})
		ws = app.activeScreen().(*wasteScreen)
		if ws.confirm {
			t.Error("c armed the confirm modal with an empty selection")
		}
		// Check the selected (first, largest) row, then press c.
		toggle, _ := app.reg.ResolveKey(" ")
		drive(app, keyMsg{cmd: toggle})
		ws = app.activeScreen().(*wasteScreen)
		if ws.checkedCount() != 1 {
			t.Fatalf("space did not check a row (checked=%d)", ws.checkedCount())
		}
		// Press c: arms the confirm modal, does NOT dispatch a clean.
		drive(app, keyMsg{cmd: clean})
		ws = app.activeScreen().(*wasteScreen)
		if !ws.confirm {
			t.Error("pressing c did not arm the confirm modal")
		}
		if ws.cleaning {
			t.Error("pressing c dispatched a clean without the second confirmation")
		}
		frame := strings.Join(app.render(), "\n")
		if !strings.Contains(frame, "CONFIRM CLEAN") {
			t.Errorf("confirm frame not shown:\n%s", frame)
		}
		// esc cancels, nothing cleaned.
		back, _ := app.reg.ResolveKey("esc")
		drive(app, keyMsg{cmd: back})
		ws = app.activeScreen().(*wasteScreen)
		if ws.confirm || ws.cleaning {
			t.Error("esc did not cancel the confirm")
		}
	})
}

// --- fixKind honesty (ADR-033): a guidance fix is never offered as a one-key fix ---

func TestGuidanceFixNotOffered(t *testing.T) {
	if hasOfferableFix(diagFinding{Fix: "sirsi spotlight", FixKind: "guidance"}) {
		t.Error("a guidance-kind fix must NOT be offerable as a one-key fix (ADR-033)")
	}
	if !hasOfferableFix(diagFinding{Fix: "sirsi self-update", FixKind: "instant"}) {
		t.Error("an instant fix must be offerable")
	}
	if !hasOfferableFix(diagFinding{Fix: "sirsi relieve --memory", FixKind: "relief"}) {
		t.Error("a relief fix must be offerable")
	}
	if hasOfferableFix(diagFinding{Fix: "", FixKind: ""}) {
		t.Error("a finding with no fix is not offerable")
	}
}

func TestFixKindLabelHonesty(t *testing.T) {
	cases := map[string]string{"instant": "fixes now", "relief": "relieves live cause", "guidance": "guidance only"}
	for kind, want := range cases {
		if got := fixKindLabel(kind); got != want {
			t.Errorf("fixKindLabel(%q) = %q, want %q", kind, got, want)
		}
	}
}

// --- relief before/after delta parses out of the report ---

func TestReliefDelta(t *testing.T) {
	r := cleanReport{Evidence: []cleanEvidence{
		{Label: "Free before", Value: "15.5 GB"},
		{Label: "Free after", Value: "17.9 GB"},
	}}
	got := reliefDeltaFrom(r)
	if !strings.Contains(got, "15.5 GB") || !strings.Contains(got, "17.9 GB") {
		t.Errorf("relief delta = %q, want the before/after values", got)
	}
}

// --- help overlay opens and any key closes it ---

func TestHelpOverlay(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		m, _ := app.handleKey("?")
		app = m.(*App)
		if !app.helpOpen {
			t.Fatal("? did not open help")
		}
		frame := strings.Join(app.render(), "\n")
		if !strings.Contains(frame, "Operator Console") {
			t.Errorf("help overlay not rendered:\n%s", frame)
		}
		m, _ = app.handleKey("x") // any key closes
		app = m.(*App)
		if app.helpOpen {
			t.Error("a key did not close the help overlay")
		}
	})
}

// --- a failed dispatch raises a danger toast (proof §4) ---

func TestDispatchErrorRaisesToast(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		app.focus(1) // Waste
		loadScreen(app)
		// Deliver a failed clean result through the App (not the screen) so the
		// App-level toast wiring runs.
		m, _ := app.Update(dispatchDone{kind: "clean", err: errTest})
		app = m.(*App)
		if app.toast == nil {
			t.Fatal("failed dispatch did not raise a toast")
		}
		if app.toast.Token != TokDanger {
			t.Errorf("toast token = %v, want danger", app.toast.Token)
		}
		frame := strings.Join(app.render(), "\n")
		if !strings.Contains(frame, "clean failed") {
			t.Errorf("toast text not rendered:\n%s", frame)
		}
		// toastExpireMsg clears it.
		m, _ = app.Update(toastExpireMsg{})
		app = m.(*App)
		if app.toast != nil {
			t.Error("toastExpireMsg did not clear the toast")
		}
	})
}

var errTest = &testError{"boom"}

type testError struct{ s string }

func (e *testError) Error() string { return e.s }

// --- exactly five screens (cap is LAW) ---

func TestExactlyFiveScreens(t *testing.T) {
	app, err := NewApp(testCaps())
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if len(app.screens) != 5 {
		t.Fatalf("console has %d screens, want exactly 5", len(app.screens))
	}
	names := make([]string, len(app.screens))
	for i, s := range app.screens {
		names[i] = s.Name()
	}
	want := []string{"Pulse", "Waste", "Ghosts", "Health", "Activity"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("screen %d = %q, want %q", i, names[i], want[i])
		}
	}
}

// ============================================================================
// P0+P1 review-fix tests (review 20260702-030841). Each maps to a ship-blocker.
// ============================================================================

// fxScanMixed exercises the P0#1 safety boundary: one safe row (cleanable), one
// caution row (cleanable), one warning row (BLOCK — data/config), and one
// caution+AI model-weights row (BLOCK — excluded from the reclaimable headline
// and the one-click cleaner). The reclaimable headline counts ONLY the safe +
// caution non-AI rows (2.4 GB + 1.1 GB); the 67 GB AI weights and the warning row
// are NOT reclaimable.
const fxScanMixed = `{"Findings":[` +
	`{"RuleName":"npm_global_cache","Category":"dev","Description":"npm Cache","Path":"/Users/x/.npm/_cacache","SizeBytes":2554200676,"FileCount":12258,"Severity":"caution","CanFix":true},` +
	`{"RuleName":"firebase_caches","Category":"cloud","Description":"Firebase CLI","Path":"/Users/x/.cache/firebase","SizeBytes":1133483950,"FileCount":35,"Severity":"safe","CanFix":true},` +
	`{"RuleName":"ollama_weights","Category":"ai","Description":"Ollama model weights","Path":"/Users/x/.ollama/models","SizeBytes":71940702208,"FileCount":40,"Severity":"caution","CanFix":true},` +
	`{"RuleName":"vscode_config","Category":"ides","Description":"VS Code settings","Path":"/Users/x/.vscode/settings.json","SizeBytes":4096,"FileCount":1,"Severity":"warning","CanFix":false}` +
	`],"TotalSize":75632390930,"ReclaimableSize":3687684626,"RulesRan":81}`

func mixedStub() stubRunner {
	s := fullStub()
	s["scan"] = fxScanMixed
	return s
}

// loadWasteMixed focuses Waste, loads the mixed fixture, and returns the screen.
func loadWasteMixed(t *testing.T) (*App, *wasteScreen) {
	t.Helper()
	app := newTestApp(t)
	app.focus(1) // Waste
	loadScreen(app)
	return app, app.activeScreen().(*wasteScreen)
}

// findRow returns the index of the finding whose Path contains sub.
func findRow(t *testing.T, ws *wasteScreen, sub string) int {
	t.Helper()
	for i, f := range ws.report.Findings {
		if strings.Contains(f.Path, sub) {
			return i
		}
	}
	t.Fatalf("no finding with path containing %q", sub)
	return -1
}

// checkRow selects and toggles the row whose path contains sub, via the real key
// path (space through the registry) so the test drives what a user drives.
func checkRow(app *App, ws *wasteScreen, idx int) {
	ws.selected = idx
	toggle, _ := app.reg.ResolveKey(" ")
	drive(app, keyMsg{cmd: toggle})
}

// --- P1#2: r = relieve (the hero beat), not refresh; f is no longer relief ---

func TestPulseRelieveBoundToR(t *testing.T) {
	withCapture(t, func(calls *[]capturedCall) {
		app := newTestApp(t) // Pulse
		loadScreen(app)
		// r resolves to CmdRelieve and dispatches `relieve --memory --confirm`.
		m, cmd := app.handleKey("r")
		app = m.(*App)
		ps := app.activeScreen().(*pulseScreen)
		if !ps.relieving {
			t.Fatal("pressing r did not start a relieve dispatch")
		}
		runTeaCmd(cmd) // execute the dispatch so the runner call is recorded
		call := firstCallOf(t, *calls, "relieve")
		if !hasArg(call.args, "--memory") || !hasArg(call.args, "--confirm") {
			t.Errorf("relieve args = %v, want --memory --confirm", call.args)
		}
	})
}

func TestRefreshBoundToU_NotR(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	// r must resolve to relieve, u to refresh — the exact P1#2 rebinding.
	if c, ok := reg.ResolveKey("r"); !ok || c.ID != CmdRelieve {
		t.Errorf("ResolveKey(r) = %v (ok=%v), want CmdRelieve", c.ID, ok)
	}
	if c, ok := reg.ResolveKey("u"); !ok || c.ID != CmdRefresh {
		t.Errorf("ResolveKey(u) = %v (ok=%v), want CmdRefresh", c.ID, ok)
	}
	// f must NOT be relief anymore — it is the Health apply-fix key.
	if c, ok := reg.ResolveKey("f"); !ok || c.ID != CmdFix {
		t.Errorf("ResolveKey(f) = %v (ok=%v), want CmdFix", c.ID, ok)
	}
}

// --- P0#1 + P1#4: per-item toggles dispatch clean via anubis clean --only ---

func TestWasteBlockRowsNotCheckable(t *testing.T) {
	withStub(t, mixedStub(), func() {
		app, ws := loadWasteMixed(t)
		// Warning-tier and AI rows are BLOCK — toggling them is a no-op.
		warnIdx := findRow(t, ws, ".vscode/settings.json")
		aiIdx := findRow(t, ws, ".ollama/models")
		checkRow(app, ws, warnIdx)
		checkRow(app, ws, aiIdx)
		if ws.checkedCount() != 0 {
			t.Errorf("BLOCK rows (warning, AI) must not be checkable; checked=%d", ws.checkedCount())
		}
		// A safe row IS checkable.
		safeIdx := findRow(t, ws, ".cache/firebase")
		checkRow(app, ws, safeIdx)
		if ws.checkedCount() != 1 {
			t.Errorf("safe row must be checkable; checked=%d", ws.checkedCount())
		}
	})
}

func TestWasteCleanDispatchesOnlyCheckedPaths(t *testing.T) {
	withCapture(t, func(calls *[]capturedCall) {
		// Use the mixed fixture through the capture runner.
		stub := mixedStub()
		setRunner(captureRunner{calls: calls, body: func(verb string, _ []string) string {
			if b, ok := stub[verb]; ok {
				return b
			}
			return `{}`
		}})
		app := newTestApp(t)
		app.focus(1)
		loadScreen(app)
		ws := app.activeScreen().(*wasteScreen)

		// Check the safe row (Firebase) and the caution row (npm); leave AI + warning.
		checkRow(app, ws, findRow(t, ws, ".cache/firebase"))
		checkRow(app, ws, findRow(t, ws, ".npm/_cacache"))
		if ws.checkedCount() != 2 {
			t.Fatalf("expected 2 checked rows, got %d", ws.checkedCount())
		}

		// Arm confirm (c) then apply (enter).
		clean, _ := app.reg.ResolveKey("c")
		drive(app, keyMsg{cmd: clean})
		if !ws.confirm {
			t.Fatal("c did not arm the confirm modal for a non-empty selection")
		}
		enter, _ := app.reg.ResolveKey("enter")
		cmd := driveCmd(app, keyMsg{cmd: enter})
		runTeaCmd(cmd) // execute the clean dispatch

		call := firstCallOf(t, *calls, "anubis")
		if len(call.args) == 0 || call.args[0] != "clean" {
			t.Fatalf("anubis args[0] = %v, want clean", call.args)
		}
		// Exactly the two checked paths are passed as --only; AI + warning are absent.
		got := onlyPaths(call.args)
		want := map[string]bool{"/Users/x/.cache/firebase": true, "/Users/x/.npm/_cacache": true}
		if len(got) != len(want) {
			t.Fatalf("--only paths = %v, want the 2 checked paths", got)
		}
		for _, p := range got {
			if !want[p] {
				t.Errorf("--only included an unchecked path: %s", p)
			}
		}
		if hasArg(call.args, "/Users/x/.ollama/models") {
			t.Error("AI model weights path leaked into the --only dispatch (P0#1)")
		}
		if hasArg(call.args, "/Users/x/.vscode/settings.json") {
			t.Error("warning-tier path leaked into the --only dispatch (P0#1)")
		}
		// A caution row is checked → --include-caution must be present so
		// narrowToPaths keeps it in scope.
		if !hasArg(call.args, "--include-caution") {
			t.Error("a caution row is checked but --include-caution was not passed")
		}
		// Apply is real, not a preview: --confirm --yes present.
		if !hasArg(call.args, "--confirm") || !hasArg(call.args, "--yes") {
			t.Errorf("clean dispatch missing --confirm --yes: %v", call.args)
		}
	})
}

func TestWasteCleanOmitsIncludeCautionWhenOnlySafeChecked(t *testing.T) {
	withCapture(t, func(calls *[]capturedCall) {
		stub := mixedStub()
		setRunner(captureRunner{calls: calls, body: func(verb string, _ []string) string {
			if b, ok := stub[verb]; ok {
				return b
			}
			return `{}`
		}})
		app := newTestApp(t)
		app.focus(1)
		loadScreen(app)
		ws := app.activeScreen().(*wasteScreen)
		// Only the safe row checked → no --include-caution.
		checkRow(app, ws, findRow(t, ws, ".cache/firebase"))
		clean, _ := app.reg.ResolveKey("c")
		drive(app, keyMsg{cmd: clean})
		enter, _ := app.reg.ResolveKey("enter")
		runTeaCmd(driveCmd(app, keyMsg{cmd: enter}))
		call := firstCallOf(t, *calls, "anubis")
		if hasArg(call.args, "--include-caution") {
			t.Errorf("safe-only selection must NOT pass --include-caution: %v", call.args)
		}
	})
}

// --- P0#1 (Rule A1): the confirm-modal total EQUALS the checked-set total, and
// EQUALS the sum of the exact --only paths dispatched. Preview == apply. ---

func TestConfirmTotalEqualsCheckedSetTotal(t *testing.T) {
	withStub(t, mixedStub(), func() {
		app, ws := loadWasteMixed(t)
		// Check the safe (1,133,483,950 B) and caution (2,554,200,676 B) rows.
		checkRow(app, ws, findRow(t, ws, ".cache/firebase"))
		checkRow(app, ws, findRow(t, ws, ".npm/_cacache"))

		wantBytes := int64(1133483950 + 2554200676)
		if got := ws.checkedBytes(); got != wantBytes {
			t.Fatalf("checkedBytes = %d, want %d", got, wantBytes)
		}
		// The confirm modal renders exactly checkedBytes — never the full
		// reclaimable/scan total (which would include unchecked rows).
		clean, _ := app.reg.ResolveKey("c")
		drive(app, keyMsg{cmd: clean})
		frame := strings.Join(app.render(), "\n")
		if !strings.Contains(frame, "CONFIRM CLEAN") {
			t.Fatalf("confirm modal not shown:\n%s", frame)
		}
		if !strings.Contains(frame, fmtBytes(wantBytes)) {
			t.Errorf("confirm total %q not in modal (preview != apply):\n%s", fmtBytes(wantBytes), frame)
		}
		// The modal must NOT show the full reclaimable headline as the amount to
		// clean (that would be the P0#1 preview!=apply bug).
		if strings.Contains(frame, fmtBytes(ws.report.ReclaimableSize)) && ws.report.ReclaimableSize != wantBytes {
			t.Errorf("confirm modal showed the reclaimable headline %q, not the checked total", fmtBytes(ws.report.ReclaimableSize))
		}
		// Structural proof: the dispatched --only paths' sizes sum to the confirm
		// total — the applied set is exactly the checked set (Rule A1).
		args := ws.cleanArgs()
		var dispatched int64
		for _, p := range onlyPaths(args) {
			for _, f := range ws.report.Findings {
				if f.Path == p {
					dispatched += f.SizeBytes
				}
			}
		}
		if dispatched != wantBytes {
			t.Errorf("sum of dispatched --only sizes = %d, want checked total %d (preview != apply)", dispatched, wantBytes)
		}
	})
}

// --- P1#3: Health fix applies for real (--confirm --yes) and routes destructive
// fixes through the confirm modal; self-update is special-cased (no confirm). ---

func TestHealthDestructiveFixGoesThroughModal(t *testing.T) {
	withCapture(t, func(calls *[]capturedCall) {
		// A diagnose report whose worst finding's fix is a destructive clean.
		stub := fullStub()
		stub["diagnose"] = `{"timestamp":"t","duration":"1s","findings":[{"check":"Disk Space","severity":1,"message":"disk low","fix":"sirsi clean --include-caution","fixKind":"instant"}]}`
		setRunner(captureRunner{calls: calls, body: func(verb string, _ []string) string {
			if b, ok := stub[verb]; ok {
				return b
			}
			return `{}`
		}})
		app := newTestApp(t)
		app.focus(3) // Health
		loadScreen(app)
		hs := app.activeScreen().(*healthScreen)

		// First f arms the confirm modal — it does NOT dispatch.
		before := len(*calls)
		f, _ := app.reg.ResolveKey("f")
		drive(app, keyMsg{cmd: f})
		if !hs.confirm {
			t.Fatal("destructive fix did not arm the confirm modal")
		}
		if hs.fixing {
			t.Error("destructive fix dispatched without the second confirmation")
		}
		if len(*calls) != before {
			t.Errorf("a runner call fired before confirmation: %+v", (*calls)[before:])
		}
		frame := strings.Join(app.render(), "\n")
		if !strings.Contains(frame, "CONFIRM FIX") {
			t.Errorf("confirm-fix modal not shown:\n%s", frame)
		}
		// enter applies it, with real apply flags.
		enter, _ := app.reg.ResolveKey("enter")
		runTeaCmd(driveCmd(app, keyMsg{cmd: enter}))
		call := firstCallOf(t, *calls, "clean")
		if !hasArg(call.args, "--confirm") || !hasArg(call.args, "--yes") {
			t.Errorf("destructive fix must apply for real (--confirm --yes): %v", call.args)
		}
	})
}

func TestHealthReclaimFixConfirmNoYes(t *testing.T) {
	withCapture(t, func(calls *[]capturedCall) {
		stub := fullStub()
		stub["diagnose"] = `{"timestamp":"t","duration":"1s","findings":[{"check":"Local Snapshots","severity":1,"message":"snapshots","fix":"sirsi reclaim-snapshots","fixKind":"instant"}]}`
		setRunner(captureRunner{calls: calls, body: func(verb string, _ []string) string {
			if b, ok := stub[verb]; ok {
				return b
			}
			return `{}`
		}})
		app := newTestApp(t)
		app.focus(3)
		loadScreen(app)
		f, _ := app.reg.ResolveKey("f")
		drive(app, keyMsg{cmd: f}) // arms modal (destructive)
		enter, _ := app.reg.ResolveKey("enter")
		runTeaCmd(driveCmd(app, keyMsg{cmd: enter})) // applies
		call := firstCallOf(t, *calls, "reclaim-snapshots")
		if !hasArg(call.args, "--confirm") {
			t.Errorf("reclaim-snapshots fix must pass --confirm to apply: %v", call.args)
		}
		// reclaim-snapshots rejects --yes — dispatching it would error. Never pass it.
		if hasArg(call.args, "--yes") {
			t.Errorf("reclaim-snapshots does not accept --yes; must not be passed: %v", call.args)
		}
	})
}

func TestHealthSelfUpdateFixIsSpecialCased(t *testing.T) {
	withCapture(t, func(calls *[]capturedCall) {
		stub := fullStub()
		stub["diagnose"] = `{"timestamp":"t","duration":"1s","findings":[{"check":"binary-drift","severity":2,"message":"drift","fix":"sirsi self-update","fixKind":"instant"}]}`
		setRunner(captureRunner{calls: calls, body: func(verb string, _ []string) string {
			if b, ok := stub[verb]; ok {
				return b
			}
			return `{}`
		}})
		app := newTestApp(t)
		app.focus(3)
		loadScreen(app)
		hs := app.activeScreen().(*healthScreen)
		f, _ := app.reg.ResolveKey("f")
		cmd := driveCmd(app, keyMsg{cmd: f})
		// self-update is NOT destructive → no modal, dispatches immediately, verbatim.
		if hs.confirm {
			t.Error("self-update must not arm the confirm modal (special-cased)")
		}
		runTeaCmd(cmd)
		call := firstCallOf(t, *calls, "self-update")
		if hasArg(call.args, "--confirm") || hasArg(call.args, "--yes") {
			t.Errorf("self-update must run verbatim (no injected --confirm/--yes): %v", call.args)
		}
	})
}

func TestFixPlanFlagsPerVerb(t *testing.T) {
	cases := []struct {
		fix         string
		wantVerb    string
		wantArgs    []string
		destructive bool
	}{
		{"sirsi clean --include-caution", "clean", []string{"--include-caution", "--confirm", "--yes"}, true},
		{"sirsi clean", "clean", []string{"--confirm", "--yes"}, true},
		{"sirsi reclaim-snapshots", "reclaim-snapshots", []string{"--confirm"}, true},
		{"sirsi relieve --memory", "relieve", []string{"--memory", "--confirm"}, false},
		{"sirsi self-update", "self-update", nil, false},
		{"sirsi spotlight-exclude ~/Development", "spotlight-exclude", []string{"~/Development"}, false},
	}
	for _, tc := range cases {
		p := fixPlan(tc.fix)
		if p.verb != tc.wantVerb {
			t.Errorf("fixPlan(%q).verb = %q, want %q", tc.fix, p.verb, tc.wantVerb)
		}
		if p.destructive != tc.destructive {
			t.Errorf("fixPlan(%q).destructive = %v, want %v", tc.fix, p.destructive, tc.destructive)
		}
		if strings.Join(p.args, " ") != strings.Join(tc.wantArgs, " ") {
			t.Errorf("fixPlan(%q).args = %v, want %v", tc.fix, p.args, tc.wantArgs)
		}
	}
}

// --- P1#5: ctrl+c quits gracefully (bubbletea v2 does not auto-quit) ---

func TestCtrlCQuits(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		_, cmd := app.handleKey("ctrl+c")
		if cmd == nil {
			t.Fatal("ctrl+c returned no command (expected tea.Quit)")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Error("ctrl+c command is not tea.Quit")
		}
		// ctrl+c wins even over an open help overlay / a screen mode.
		app2 := newTestApp(t)
		app2.helpOpen = true
		_, cmd2 := app2.handleKey("ctrl+c")
		if cmd2 == nil {
			t.Fatal("ctrl+c must quit even with the help overlay open")
		}
		if _, ok := cmd2().(tea.QuitMsg); !ok {
			t.Error("ctrl+c with the help overlay open is not tea.Quit")
		}
	})
}

// TestCtrlCIsQuitMsg confirms the returned command is tea.Quit (a QuitMsg), so the
// program actually exits — not just a state flag.
func TestCtrlCIsQuitMsg(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		_, cmd := app.handleKey("ctrl+c")
		if cmd == nil {
			t.Fatal("no command from ctrl+c")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Error("ctrl+c command is not tea.Quit")
		}
	})
}

// ============================================================================
// P2 polish tests (review 20260702-030841 P2#6/#7/#8).
// ============================================================================

// --- P2#6: viewport windowing — the selected row can never scroll off-frame ---

// TestTableWindowInvariants exhaustively pins the pure window math: for every
// list length, budget, and selection, the selected row is inside the window,
// the indicator counts are exactly the clipped rows, and the block (rows +
// shown indicators) consumes exactly the budget when clipping — never more.
func TestTableWindowInvariants(t *testing.T) {
	mk := func(n, sel int) []listRow {
		rows := make([]listRow, n)
		if sel >= 0 && sel < n {
			rows[sel].selected = true
		}
		return rows
	}
	for n := 1; n <= 60; n++ {
		for budget := 3; budget <= 30; budget++ {
			for sel := 0; sel < n; sel++ {
				start, end, above, below := tableWindow(mk(n, sel), budget)
				if sel < start || sel >= end {
					t.Fatalf("n=%d budget=%d sel=%d: selection outside window [%d,%d)", n, budget, sel, start, end)
				}
				if above != start || below != n-end {
					t.Fatalf("n=%d budget=%d sel=%d: dishonest indicators above=%d below=%d for window [%d,%d)", n, budget, sel, above, below, start, end)
				}
				used := end - start
				if above > 0 {
					used++
				}
				if below > 0 {
					used++
				}
				if n <= budget {
					if used != n || above != 0 || below != 0 {
						t.Fatalf("n=%d budget=%d sel=%d: list fits but was windowed (used=%d above=%d below=%d)", n, budget, sel, used, above, below)
					}
				} else if used != budget {
					t.Fatalf("n=%d budget=%d sel=%d: block uses %d slots, want exactly %d", n, budget, sel, used, budget)
				}
			}
		}
	}
	// budget < 0 disables windowing entirely.
	if start, end, above, below := tableWindow(mk(50, 49), -1); start != 0 || end != 50 || above != 0 || below != 0 {
		t.Errorf("tableWindow(budget<0) windowed: [%d,%d) above=%d below=%d", start, end, above, below)
	}
}

// bigScanStub builds a scan fixture with n findings so the Waste list far
// exceeds the viewport. Sizes strictly descend with the index, so sortFindings
// keeps this order and row i is finding "Cache %03d"==i.
func bigScanStub(n int) stubRunner {
	items := make([]string, n)
	for i := 0; i < n; i++ {
		items[i] = fmt.Sprintf(
			`{"RuleName":"rule_%03d","Category":"dev","Description":"Cache %03d","Path":"/Users/x/cache/%03d","SizeBytes":%d,"FileCount":1,"Severity":"safe","CanFix":true}`,
			i, i, i, int64(n-i)*1000000)
	}
	s := fullStub()
	s["scan"] = `{"Findings":[` + strings.Join(items, ",") + `],"TotalSize":0,"ReclaimableSize":0,"RulesRan":81}`
	return s
}

// TestViewportKeepsSelectionVisible drives an 80-row Waste list through top,
// bottom (G), and middle positions at 100x30 and asserts the selected row is
// always inside the rendered frame with honest clip indicators. Geometry at
// 100x30: 28 interior − 2 headline − 2 footer = 24 table lines − 2 header =
// 22 row slots; a one-sided window shows 21 rows + 1 indicator, a two-sided
// window 20 rows + 2 indicators. ASCII caps render ↑/↓ as ^/v.
func TestViewportKeepsSelectionVisible(t *testing.T) {
	withStub(t, bigScanStub(80), func() {
		app := newTestApp(t)
		app.focus(1) // Waste
		loadScreen(app)

		// Top: first row selected and visible; 59 rows clipped below.
		frame := strings.Join(app.render(), "\n")
		if !strings.Contains(frame, "Cache 000") {
			t.Errorf("top window missing the selected first row:\n%s", frame)
		}
		if !strings.Contains(frame, "v 59 more") {
			t.Errorf("top window missing the below-clip indicator (want %q):\n%s", "v 59 more", frame)
		}
		if strings.Contains(frame, "^ ") {
			t.Errorf("top window shows an above-clip indicator with nothing clipped above:\n%s", frame)
		}

		// Bottom (G): last row visible, 59 clipped above — before this fix the
		// cursor sat 50+ rows below the frame.
		bottom, _ := app.reg.ResolveKey("G")
		drive(app, keyMsg{cmd: bottom})
		frame = strings.Join(app.render(), "\n")
		if !strings.Contains(frame, "Cache 079") {
			t.Errorf("selected bottom row not visible after G:\n%s", frame)
		}
		if !strings.Contains(frame, "^ 59 more") {
			t.Errorf("bottom window missing the above-clip indicator (want %q):\n%s", "^ 59 more", frame)
		}
		if strings.Contains(frame, "Cache 000") {
			t.Errorf("bottom window still shows the first row (not windowed):\n%s", frame)
		}

		// Middle (up 39x from the bottom → row 40): both indicators, selection
		// centered — 20 row slots, rows 30..49 visible, 30 clipped each side.
		up, _ := app.reg.ResolveKey("up")
		for i := 0; i < 39; i++ {
			drive(app, keyMsg{cmd: up})
		}
		frame = strings.Join(app.render(), "\n")
		for _, want := range []string{"Cache 040", "^ 30 more", "v 30 more"} {
			if !strings.Contains(frame, want) {
				t.Errorf("middle window missing %q:\n%s", want, frame)
			}
		}
		for _, clip := range []string{"Cache 029", "Cache 050"} {
			if strings.Contains(frame, clip) {
				t.Errorf("middle window shows %q, which should be clipped:\n%s", clip, frame)
			}
		}
	})
}

// TestViewportResizeSafe shrinks the window mid-session (tea.WindowSizeMsg) and
// asserts the selection is re-windowed into the smaller frame — the offset is
// recomputed per render, so no stale scroll state can strand the cursor.
func TestViewportResizeSafe(t *testing.T) {
	withStub(t, bigScanStub(80), func() {
		app := newTestApp(t)
		app.focus(1)
		loadScreen(app)
		bottom, _ := app.reg.ResolveKey("G")
		drive(app, keyMsg{cmd: bottom})

		m, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		app = m.(*App)
		lines := app.render()
		if len(lines) != 24 {
			t.Fatalf("render at 80x24 produced %d lines, want 24", len(lines))
		}
		frame := strings.Join(lines, "\n")
		// 22 interior − 4 chrome = 18 table lines − 2 header = 16 slots: bottom
		// window shows 15 rows (65..79) + "^ 65 more".
		if !strings.Contains(frame, "Cache 079") {
			t.Errorf("selected row lost after shrink to 80x24:\n%s", frame)
		}
		if !strings.Contains(frame, "^ 65 more") {
			t.Errorf("shrunk window missing the recomputed clip indicator (want %q):\n%s", "^ 65 more", frame)
		}
	})
}

// --- P2#7: the runner ERROR path renders the error frame, never spinner-forever ---

// failRunner is the injected runner ERROR path: every verb fails with err.
type failRunner struct{ err error }

func (f failRunner) RunJSON(context.Context, string, ...string) ([]byte, error) {
	return nil, f.err
}

// TestGoldenErrorStateEveryScreen drives every screen's load command through a
// failing runner and asserts the golden error frame: the error banner with the
// reason (never swallowed) plus the retry hint — not a spinner forever, not a
// panic, not a blank frame.
func TestGoldenErrorStateEveryScreen(t *testing.T) {
	old := getRunner()
	setRunner(failRunner{err: errTest}) // every verb fails with "boom"
	defer setRunner(old)

	screens := []string{"Pulse", "Waste", "Ghosts", "Health", "Activity"}
	loading := []string{"sampling memory", "scanning for waste", "hunting ghost apps", "running diagnostics", "reading operations ledger"}
	for i, name := range screens {
		app := newTestApp(t)
		app.focus(i)
		loadScreen(app) // executes Load; the injected runner returns the error
		if got := app.activeScreen().State(); got != stateError {
			t.Errorf("%s: state = %v after failed load, want stateError", name, got)
		}
		frame := strings.Join(app.render(), "\n")
		if !strings.Contains(frame, "BLOCK boom") {
			t.Errorf("%s error frame missing the error banner with the reason:\n%s", name, frame)
		}
		if !strings.Contains(frame, "press u to retry") {
			t.Errorf("%s error frame missing the retry hint:\n%s", name, frame)
		}
		if strings.Contains(frame, loading[i]) {
			t.Errorf("%s error frame still shows the loading state %q (spinner-forever):\n%s", name, loading[i], frame)
		}
	}
}

// TestErrorStateRetryRecovers proves the "press u to retry" hint is honest: a
// failed load followed by u against a recovered runner reaches ready.
func TestErrorStateRetryRecovers(t *testing.T) {
	old := getRunner()
	defer setRunner(old)
	setRunner(failRunner{err: errTest})

	app := newTestApp(t)
	app.focus(1) // Waste
	loadScreen(app)
	if app.activeScreen().State() != stateError {
		t.Fatal("setup: failed load did not reach stateError")
	}

	setRunner(fullStub()) // the runner recovers
	m, cmd := app.handleKey("u")
	app = m.(*App)
	if cmd == nil {
		t.Fatal("u in the error state produced no reload command")
	}
	deliver(app, cmd())
	if got := app.activeScreen().State(); got != stateReady {
		t.Fatalf("retry did not recover: state = %v, want stateReady", got)
	}
	frame := strings.Join(app.render(), "\n")
	if !strings.Contains(frame, "npm/yarn/pnpm Cache") {
		t.Errorf("recovered frame missing loaded data:\n%s", frame)
	}
}

// --- P2#8: q mid-operation confirms instead of quitting; ctrl+c stays immediate ---

// makeWasteCleaning drives Waste into a REAL in-flight clean via the key path
// (space → c → enter) WITHOUT executing the dispatched command, so cleaning
// stays true — exactly the mid-operation window the quit guard protects.
func makeWasteCleaning(t *testing.T, app *App) *wasteScreen {
	t.Helper()
	app.focus(1)
	loadScreen(app)
	ws := app.activeScreen().(*wasteScreen)
	checkRow(app, ws, 0)
	c, _ := app.reg.ResolveKey("c")
	drive(app, keyMsg{cmd: c})
	enter, _ := app.reg.ResolveKey("enter")
	_ = driveCmd(app, keyMsg{cmd: enter}) // clean dispatched; result never delivered
	if !ws.Busy() {
		t.Fatal("setup: dispatched clean did not mark the screen busy")
	}
	return ws
}

func TestQuitGuardMidOperation(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		makeWasteCleaning(t, app)

		// First q does NOT quit: it arms the confirm and says so.
		m, _ := app.handleKey("q")
		app = m.(*App)
		if !app.quitArmed {
			t.Fatal("q mid-operation did not arm the quit confirm")
		}
		if app.toast == nil {
			t.Fatal("quit guard raised no confirm toast")
		}
		frame := strings.Join(app.render(), "\n")
		if !strings.Contains(frame, "operation running — q again to quit, esc to stay") {
			t.Errorf("quit-guard hint not rendered:\n%s", frame)
		}

		// Second q quits for real.
		m, cmd := app.handleKey("q")
		app = m.(*App)
		if cmd == nil {
			t.Fatal("second q returned no command")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Error("second q is not tea.Quit")
		}
		if app.quitArmed {
			t.Error("quit left the confirm armed")
		}
	})
}

func TestQuitGuardEscStays(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		ws := makeWasteCleaning(t, app)

		m, _ := app.handleKey("q") // arm
		app = m.(*App)
		m, cmd := app.handleKey("esc") // "esc to stay"
		app = m.(*App)
		if app.quitArmed {
			t.Error("esc did not disarm the quit confirm")
		}
		if app.toast != nil {
			t.Error("esc did not clear the confirm hint toast")
		}
		if cmd != nil {
			t.Error("esc while armed must be consumed, not forwarded")
		}
		if !ws.cleaning {
			t.Error("staying must not touch the in-flight operation")
		}
	})
}

func TestQuitGuardDuringScanLoad(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		app.focus(1) // Waste Load dispatched, result not yet delivered: scan running
		if !app.activeScreen().Busy() {
			t.Fatal("setup: loading screen not busy")
		}
		m, _ := app.handleKey("q")
		app = m.(*App)
		if !app.quitArmed {
			t.Error("q during a running scan load did not arm the quit confirm")
		}
	})
}

func TestQuitImmediateWhenIdle(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		app.focus(1)
		loadScreen(app) // ready — nothing in flight
		m, cmd := app.handleKey("q")
		app = m.(*App)
		if app.quitArmed {
			t.Error("idle q armed the confirm instead of quitting")
		}
		if cmd == nil {
			t.Fatal("idle q returned no command")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Error("idle q is not tea.Quit")
		}
	})
}

func TestCtrlCImmediateMidOperation(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		makeWasteCleaning(t, app)
		_, cmd := app.handleKey("ctrl+c")
		if cmd == nil {
			t.Fatal("ctrl+c mid-operation returned no command")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Error("ctrl+c mid-operation must quit immediately, without the confirm")
		}
	})
}

// TestQuitGuardArmExpiresWithToast pins the arm's lifetime to its visible hint:
// once the toast expires, a later single q must re-arm visibly, never quit
// silently while the operation still runs.
func TestQuitGuardArmExpiresWithToast(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		makeWasteCleaning(t, app)
		m, _ := app.handleKey("q") // arm
		app = m.(*App)
		m, _ = app.Update(toastExpireMsg{})
		app = m.(*App)
		if app.quitArmed {
			t.Fatal("toast expiry did not disarm the quit confirm")
		}
		m, _ = app.handleKey("q") // must RE-arm, not quit
		app = m.(*App)
		if !app.quitArmed || app.toast == nil {
			t.Error("q after expiry did not re-arm the visible confirm")
		}
	})
}

// TestQuitGuardOtherKeyDisarms: any non-q, non-esc key drops the pending
// confirm and runs normally — a stale arm cannot linger behind other work.
func TestQuitGuardOtherKeyDisarms(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		makeWasteCleaning(t, app)
		m, _ := app.handleKey("q") // arm
		app = m.(*App)
		m, _ = app.handleKey("tab") // disarms AND still switches screens
		app = m.(*App)
		if app.quitArmed {
			t.Error("another key did not disarm the quit confirm")
		}
		if got := app.activeScreen().Name(); got != "Ghosts" {
			t.Errorf("tab while armed did not run normally: active = %q, want Ghosts", got)
		}
	})
}
