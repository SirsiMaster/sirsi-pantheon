package tui

import (
	"context"
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
		for _, want := range []string{"Horus", "Pulse", "MEMORY", "TOP MEMORY USERS", "mediaanalysisd", "PRESSURE", "press f"} {
			if !strings.Contains(strings.ToUpper(frame), strings.ToUpper(want)) {
				t.Errorf("Pulse frame missing %q\n---\n%s", want, frame)
			}
		}
	})
}

func TestGoldenWaste(t *testing.T) {
	withStub(t, fullStub(), func() {
		app := newTestApp(t)
		app.focus(1) // Waste
		loadScreen(app)
		frame := strings.Join(app.render(), "\n")
		for _, want := range []string{"Waste", "npm/yarn/pnpm Cache", "reclaimable", "RULE", "TIER", "space toggles"} {
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
		// Press c: arms the confirm modal, does NOT dispatch a clean.
		clean, _ := app.reg.ResolveKey("c")
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
