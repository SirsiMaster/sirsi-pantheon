package dashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

// fetchSPA returns the single-page-app HTML, which carries every view's
// JavaScript inline. All views ship in this one document, so this is the whole
// client surface.
func fetchSPA(t *testing.T) string {
	t.Helper()
	srv := testServer(t, Config{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// TestGuardView_ReadsDoctorJSONKeysNotGoFieldNames pins the seam that broke:
// the Guard view consumes /api/doctor, which marshals guard.Report through its
// json tags (lowercase). The view read the Go field names instead — so
// rpt.Score was undefined and rpt.Findings fell through `||[]`, discarding
// every diagnostic. The page still rendered, still returned 200, and still
// looked like a working health screen reporting nothing wrong.
//
// Go's compiler cannot catch this: the client is a string literal. So the
// assertion is derived from a real marshaled Report rather than hardcoded —
// rename a json tag and this test fails instead of the UI silently emptying.
func TestGuardView_ReadsDoctorJSONKeysNotGoFieldNames(t *testing.T) {
	raw, err := json.Marshal(guard.DoctorReport{
		Score:    91,
		Findings: []guard.DiagnosticFinding{{Check: "RAM Pressure", Message: "healthy", Severity: 0}},
	})
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	page := fetchSPA(t)

	for _, key := range []string{"score", "findings"} {
		if _, ok := report[key]; !ok {
			t.Fatalf("guard.DoctorReport no longer marshals a %q key — update the Guard view to match", key)
		}
		if !strings.Contains(page, "rpt."+key) {
			t.Errorf("Guard view never reads rpt.%s, but /api/doctor emits it", key)
		}
	}

	findings, _ := report["findings"].([]any)
	if len(findings) != 1 {
		t.Fatalf("expected one marshaled finding, got %d", len(findings))
	}
	finding, _ := findings[0].(map[string]any)
	for _, key := range []string{"check", "message", "severity"} {
		if _, ok := finding[key]; !ok {
			t.Fatalf("guard.DiagnosticFinding no longer marshals a %q key — update the Guard view to match", key)
		}
		if !strings.Contains(page, "f."+key) {
			t.Errorf("Guard view never reads f.%s, but findings carry it", key)
		}
	}

	// The exact defect, stated as an anti-assertion so it cannot come back by
	// someone "restoring" what looks like the Go field name.
	for _, bad := range []string{"rpt.Score", "rpt.Findings", "f.Severity", "f.Check", "f.Message"} {
		if strings.Contains(page, bad) {
			t.Errorf("Guard view reads %q — that is the Go field name; /api/doctor emits lowercase json tags, so this is always undefined", bad)
		}
	}
}

func TestGuardViewProvidesSeveritySummaryAndProgressiveDisclosure(t *testing.T) {
	page := fetchSPA(t)

	for _, want := range []string{
		"guard-summary",
		"Needs attention",
		"Show all '+fs.length+' checks",
		"guard-finding-message",
		"Health score",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Guard view missing structured summary contract %q", want)
		}
	}
}

// TestHomeView_CommandsAreClickable keeps every tool reachable while the
// first-question path stays focused on the selected route and prompt.
func TestHomeView_CommandsAreClickable(t *testing.T) {
	page := fetchSPA(t)

	for _, action := range []string{
		"doctor','Check system health", "scan','Scan infrastructure", "guard','Review system controls",
		"ghosts','Find application remnants", "network','Audit network security", "hardware','Inspect hardware",
		"quality','Check code governance", "dedup','Find duplicate files",
	} {
		if !strings.Contains(page, action) {
			t.Errorf("Home tool missing %q", action)
		}
	}
	if !strings.Contains(page, "moreSummary.textContent='All tools'") || !strings.Contains(page, "makeHomeAction") {
		t.Error("secondary Home tools are not grouped under the All tools disclosure")
	}
	if !strings.Contains(page, "button.type='button'") || !strings.Contains(page, "button.addEventListener('click',function(){input.value='';exec(item[0])})") {
		t.Error("Home actions are not real buttons dispatched through the shared command handler")
	}
	for _, want := range []string{"Ask Horus about this machine", "Choose an engine, ask one question", "home-route", "Open engine selector"} {
		if !strings.Contains(page, want) {
			t.Errorf("home demo path missing %q", want)
		}
	}
	for _, want := range []string{"home-hero", "Start with a question", "View worker evidence", "home-primary-action"} {
		if !strings.Contains(page, want) {
			t.Errorf("home investor path missing %q", want)
		}
	}
	for _, want := range []string{
		"document.createElement('h1')",
		"intro.setAttribute('aria-labelledby','home-title')",
		"tools.setAttribute('aria-label','Additional tools')",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Home accessible naming contract missing %q", want)
		}
	}
	if !strings.Contains(page, "routeAction.addEventListener('click',function(){switchView('engine')})") {
		t.Error("Home engine selector action is not connected to the engine view")
	}
	if !strings.Contains(page, "start.addEventListener('click',function(){input.value='';input.focus()})") {
		t.Error("command input is never focused — the first keystroke goes nowhere")
	}
}

func TestHomeViewDemoPathUsesCanonicalRoutes(t *testing.T) {
	page := fetchSPA(t)

	for _, want := range []string{
		"Start with a question",
		"Open engine selector",
		"routeAction.addEventListener('click',function(){switchView('engine')})",
		"start.addEventListener('click',function(){input.value='';input.focus()})",
		"fleet.addEventListener('click',function(){switchView('fleet')})",
		"Policy selected; live availability is proved when a session opens.",
		"Choose SNE, MLX, or oMLX before asking a model-backed question.",
		"moreSummary.textContent='All tools'",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Home first-question path missing %q", want)
		}
	}
	if strings.Contains(page, "demo-steps") || strings.Contains(page, "Demo path") {
		t.Error("Home still repeats the route, prompt, and evidence actions in a separate demo rail")
	}
}

func TestSidebarNavigationIsLabelFirst(t *testing.T) {
	page := fetchSPA(t)

	for _, label := range []string{"Home", "Fleet", "Scan", "Ghosts", "Guard", "Notifications", "Horus", "Vault", "Engine", "SNE", "Recovery", "Ra"} {
		if !strings.Contains(page, `<span class="nav-label">`+label+`</span>`) {
			t.Errorf("sidebar is missing the readable %q label", label)
		}
	}
	if strings.Contains(page, `class="nav-glyph"`) {
		t.Error("sidebar still relies on decorative glyph spans instead of readable labels")
	}
	for _, want := range []string{
		`<nav class="sidebar-nav" aria-label="Primary navigation">`,
		`<a class="skip-link" href="#main-content">Skip to main content</a>`,
		`<main class="main" id="main-content" tabindex="-1">`,
		`.skip-link:focus{top:12px`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("dashboard keyboard/screen-reader landmark missing %q", want)
		}
	}
}
