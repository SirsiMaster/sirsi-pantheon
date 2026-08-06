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

// TestHomeView_CommandsAreClickable pins the affordance fix: the home screen
// listed eight commands as inert text, so the only way to act on one was to
// retype it. Each is now a row that dispatches through the same exec() the
// input box uses — one dispatch, so a click can never drift from the typed word.
func TestHomeView_CommandsAreClickable(t *testing.T) {
	page := fetchSPA(t)

	for _, cmd := range []string{"scan", "ghosts", "guard", "doctor", "network", "hardware", "quality", "dedup"} {
		if !strings.Contains(page, "cmdRow('"+cmd+"'") {
			t.Errorf("home command %q is not rendered as a clickable row", cmd)
		}
	}
	if !strings.Contains(page, "row.addEventListener('click',go)") {
		t.Error("command rows carry no click handler")
	}
	if !strings.Contains(page, "input.focus()") {
		t.Error("command input is never focused — the first keystroke goes nowhere")
	}
}
