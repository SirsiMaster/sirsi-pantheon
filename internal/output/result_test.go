package output

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCommandResultJSONDurationUsesMilliseconds(t *testing.T) {
	r := CommandResult{Command: "sirsi maat audit", Duration: 1375*time.Millisecond + 900*time.Microsecond}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded struct {
		Duration int64 `json:"duration_ms"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Duration != 1375 {
		t.Fatalf("duration_ms = %d, want 1375; payload=%s", decoded.Duration, raw)
	}
}

func TestCommandResult_AddEvidence(t *testing.T) {
	r := &CommandResult{Command: "sirsi scan"}
	r.AddEvidence("Waste", "2.4 GB")
	r.AddEvidence("Findings", "47")

	if len(r.Evidence) != 2 {
		t.Fatalf("expected 2 evidence items, got %d", len(r.Evidence))
	}
	if r.Evidence[0].Label != "Waste" || r.Evidence[0].Value != "2.4 GB" {
		t.Errorf("evidence[0] = %+v, want {Waste, 2.4 GB}", r.Evidence[0])
	}
}

func TestCommandResult_AddWarning(t *testing.T) {
	r := &CommandResult{Command: "sirsi scan"}
	r.AddWarning("Scan error: %v", "timeout")

	if len(r.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(r.Warnings))
	}
	if r.Warnings[0] != "Scan error: timeout" {
		t.Errorf("warning = %q, want %q", r.Warnings[0], "Scan error: timeout")
	}
}

func TestCommandResult_AddError(t *testing.T) {
	r := &CommandResult{Command: "sirsi clean"}
	r.AddError("Permission denied: %s", "/usr/local")

	if len(r.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(r.Errors))
	}
	if r.Errors[0] != "Permission denied: /usr/local" {
		t.Errorf("error = %q, want %q", r.Errors[0], "Permission denied: /usr/local")
	}
}

func TestCommandResult_AddNextAction(t *testing.T) {
	r := &CommandResult{Command: "sirsi scan"}
	r.AddNextAction("sirsi clean", "Remove safe items")
	r.AddNextAction("sirsi ghosts", "Hunt ghost residuals")

	if len(r.NextActions) != 2 {
		t.Fatalf("expected 2 next actions, got %d", len(r.NextActions))
	}
	if r.NextActions[0].Command != "sirsi clean" {
		t.Errorf("action[0].Command = %q, want %q", r.NextActions[0].Command, "sirsi clean")
	}
	if r.NextActions[0].Description != "Remove safe items" {
		t.Errorf("action[0].Description = %q, want %q", r.NextActions[0].Description, "Remove safe items")
	}
	// Label should be derived from command (after "sirsi ")
	if r.NextActions[0].Label != "clean" {
		t.Errorf("action[0].Label = %q, want %q", r.NextActions[0].Label, "clean")
	}
}

func TestCommandResult_RenderQuiet(t *testing.T) {
	// Set quiet mode and verify no panic
	SetOutputMode(false, true)
	defer SetOutputMode(false, false)

	r := &CommandResult{
		Command:  "sirsi scan",
		Summary:  "Found 2.4 GB waste",
		Duration: 500 * time.Millisecond,
	}
	r.AddEvidence("Waste", "2.4 GB")
	r.AddNextAction("sirsi clean", "Remove safe items")

	// Should not panic — output goes to stderr
	r.Render()
}

func TestCommandResult_RenderNormal(t *testing.T) {
	// Normal mode — verify no panic with full rendering
	SetOutputMode(false, false)

	r := &CommandResult{
		Command:  "sirsi diagnose",
		Summary:  "Health: 85/100",
		Duration: 200 * time.Millisecond,
	}
	r.AddEvidence("Score", "85/100")
	r.AddWarning("DNS is not encrypted")
	r.AddNextAction("sirsi fix", "Auto-fix issues")

	// Should not panic
	r.Render()
}
