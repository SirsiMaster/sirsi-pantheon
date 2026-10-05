package dashboard

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// TestPageShellDerivesFromBrand pins the ADR-038 single-palette contract for
// the dashboard: every color comes from internal/brand via :root CSS vars —
// no hardcoded hex or stale-gold rgba literal may reappear in the shell.
func TestPageShellDerivesFromBrand(t *testing.T) {
	html := pageShell("Test", "home", "<p>body</p>", DashboardPort)

	if !strings.Contains(html, ":root{") || !strings.Contains(html, "--gold:") || !strings.Contains(html, "--ok:") {
		t.Fatal("pageShell missing the brand :root CSS-vars block")
	}
	// The %s-injected Color* constants are brand-derived hex — only literals
	// OUTSIDE the :root block are drift. Strip the vars block, then scan.
	stripped := regexp.MustCompile(`:root\{[^}]*\}`).ReplaceAllString(html, "")
	for _, c := range []string{"#C8A951", "#44FF88", "#FF4444", "#FF8844", "#555", "#666", "#444", "#333", "rgba(200,169,81"} {
		if strings.Contains(stripped, c) {
			t.Errorf("hardcoded color literal %q survives in the dashboard shell", c)
		}
	}
	if !strings.Contains(html, "var(--gold)") {
		t.Error("expected classes to reference var(--gold)")
	}
}

func TestPageShellResponsiveCommandSurface(t *testing.T) {
	t.Parallel()

	html := pageShell("Test", "home", "<p>body</p>", DashboardPort)
	for _, want := range []string{
		"overflow-wrap:anywhere;word-break:normal",
		"@media (max-width:760px)",
		".sidebar-nav{display:flex;min-width:0;overflow-x:auto",
		".main{margin-left:0;",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("responsive command surface missing %q", want)
		}
	}
}

func TestPageShellMarksCurrentNavigationForAssistiveTechnology(t *testing.T) {
	html := dashboardPage(t)
	if !strings.Contains(html, `<a href="/" class="nav-item active" data-view="home" aria-current="page"`) {
		t.Fatal("initial dashboard view is not identified as current in navigation")
	}
	if !strings.Contains(html, `<a href="/?view=fleet" class="nav-item" data-view="fleet"`) {
		t.Fatal("Fleet navigation is not a direct, shareable destination")
	}
	if strings.Contains(html, `<a href="/?view=fleet" class="nav-item active" data-view="fleet" aria-current="page"`) {
		t.Fatal("inactive navigation item is marked current")
	}
	if !strings.Contains(html, `n.setAttribute('aria-current','page')`) || !strings.Contains(html, `n.removeAttribute('aria-current')`) {
		t.Fatal("SPA navigation does not update the current-page accessibility state")
	}
}

func TestPageShellGroupsFrequentNavigationAndRevealsActiveTool(t *testing.T) {
	home := pageShell("Home", "home", "<p>body</p>", DashboardPort)
	if got := strings.Count(home, `class="nav-item`); got != 12 {
		t.Fatalf("navigation has %d destinations, want all 12", got)
	}
	group := strings.Index(home, `<details class="nav-tools">`)
	if group < 0 {
		t.Fatal("occasional destinations are not grouped in a Tools disclosure")
	}
	for _, key := range []string{"home", "fleet", "engine", "sne", "ra"} {
		if at := strings.Index(home, `data-view="`+key+`"`); at < 0 || at > group {
			t.Errorf("frequent destination %q is not visible before Tools", key)
		}
	}
	for _, key := range []string{"scan", "ghosts", "guard", "notifications", "horus", "vault", "recovery"} {
		if at := strings.Index(home, `data-view="`+key+`"`); at < group {
			t.Errorf("system destination %q is not inside Tools", key)
		}
	}
	if !strings.Contains(home, `<summary class="nav-tools-toggle">Tools</summary>`) {
		t.Fatal("Tools disclosure has no native, keyboard-operable summary")
	}

	scan := pageShell("Scan", "scan", "<p>body</p>", DashboardPort)
	if !strings.Contains(scan, `<details class="nav-tools" open>`) || !strings.Contains(scan, `data-view="scan" aria-current="page"`) {
		t.Fatal("initial active tool is not revealed and marked current")
	}
	if !strings.Contains(dashboardPage(t), `toolsNav.open=['scan','ghosts','guard','notifications','horus','vault','recovery'].indexOf(view)!==-1`) {
		t.Fatal("SPA view changes do not reveal the active tool group")
	}
}

func TestPageShellUsesSirsiPantheonProductIdentity(t *testing.T) {
	html := dashboardPage(t)
	for _, want := range []string{
		"<title>Home — Sirsi Pantheon</title>",
		"<div class=\"sidebar-brand\"><h1>Sirsi Pantheon</h1><div class=\"sidebar-build\" id=\"build-identity\" role=\"status\" aria-live=\"polite\">Reading running build…</div></div>",
		"Sirsi Pantheon — use the sidebar or type a command",
		"fetch('/api/identity')",
		"buildLabel.title='Running process: '",
		"Build identity unavailable — verify this running app before continuing.",
		`.sidebar-build[data-state="unavailable"]`,
		`white-space:normal;overflow:visible;text-overflow:clip`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("Pantheon shell branding missing %q", want)
		}
	}
}

func TestDashboardOverviewUsesOperatorLanguage(t *testing.T) {
	t.Parallel()

	html := dashboardPage(t)
	for _, want := range []string{
		`<div class="stat-label">Components</div>`,
		`Process controls — type: kill node | kill electron | kill docker | kill lsp | kill build | kill ai`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("dashboard is missing operator-facing wording %q", want)
		}
	}
	for _, internal := range []string{"Process Slayer", ">Deities<"} {
		if strings.Contains(html, internal) {
			t.Errorf("dashboard exposes internal jargon %q", internal)
		}
	}
}

func TestDashboardComponentStatusDoesNotInventZeroOnCollectionFailure(t *testing.T) {
	html := dashboardPage(t)
	if !strings.Contains(html, `s.components_known===true&&Number.isInteger(s.component_count)`) {
		t.Fatal("component count is not gated on successful inventory evidence")
	}
	if !strings.Contains(html, `:'Process inventory unavailable'`) {
		t.Fatal("unknown component inventory has no explicit UI state")
	}
}

func dashboardPage(t *testing.T) string {
	t.Helper()
	ts := testServer(t, Config{})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET dashboard status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	return string(body)
}

func TestDashboardCommandInputIsLabeled(t *testing.T) {
	t.Parallel()

	ts := testServer(t, Config{})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	page := string(body)
	for _, want := range []string{
		`<label class="sr-only" for="term-input">Ask a question about this machine or enter a Pantheon command</label>`,
		`aria-label="Ask a question about this machine or enter a Pantheon command"`,
		`id="term-submit" aria-label="Submit the question or command" aria-controls="terminal" disabled>Submit</button>`,
		`id="terminal" role="log" aria-label="Pantheon command and engine results" aria-live="polite"`,
		`min-height:44px`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("dashboard command input missing %q", want)
		}
	}
}

func TestDashboardFleetUnavailableStateIsActionable(t *testing.T) {
	t.Parallel()

	ts := testServer(t, Config{})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	page := string(body)
	for _, want := range []string{
		"t-empty-state",
		"Fleet is not connected",
		"canonical fleet producer",
		"Retry fleet data",
		`setAttribute('role','status')`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("fleet unavailable state missing %q", want)
		}
	}
}

func TestDashboardRaViewBridgesToCanonicalFleet(t *testing.T) {
	t.Parallel()

	ts := testServer(t, Config{})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	page := string(body)
	for _, want := range []string{
		"M5 is the single worker/router authority.",
		"Pantheon reads the same canonical Fleet board; it never keeps a local worker registry.",
		"Canonical worker board",
		"Authenticated control actions appear only when the M5 action surface is configured.",
		"Open canonical Fleet board",
		"fleet.onclick=function(){switchView('fleet')}",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Ra canonical Fleet bridge missing %q", want)
		}
	}
}
