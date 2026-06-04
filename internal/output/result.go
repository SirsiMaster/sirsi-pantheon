// Package output — result.go
//
// CommandResult is the shared structured output contract for all Pro commands.
// Every user-facing command should build a CommandResult and call Render()
// to produce consistent output with summaries, evidence, and next actions.
package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// CommandResult represents the structured output of any Pro command.
type CommandResult struct {
	Command     string        `json:"command"`
	Summary     string        `json:"summary"`
	Duration    time.Duration `json:"duration_ms"`
	BriefTitle  string        `json:"brief_title,omitempty"`
	Status      string        `json:"status,omitempty"`
	Confidence  string        `json:"confidence,omitempty"`
	Priority    []string      `json:"priority,omitempty"`
	Evidence    []Evidence    `json:"evidence,omitempty"`
	Warnings    []string      `json:"warnings,omitempty"`
	Errors      []string      `json:"errors,omitempty"`
	NextActions []NextAction  `json:"next_actions,omitempty"`
}

// Evidence is a single data point from the command result.
type Evidence struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// NextAction is a suggested follow-up command with context.
type NextAction struct {
	Label       string `json:"label"`
	Command     string `json:"command"`
	Description string `json:"description"`
}

// Render outputs the CommandResult in the appropriate format.
// JSON mode: writes JSON to stdout.
// Quiet mode: writes only the summary line.
// Normal mode: writes the full branded result to stderr.
func (r *CommandResult) Render() {
	if IsJSON() {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(r)
		return
	}

	if IsQuiet() {
		fmt.Fprintln(os.Stderr, r.Summary)
		return
	}

	r.renderFull()
}

func (r *CommandResult) renderFull() {
	if r.BriefTitle != "" {
		r.renderBrief()
		return
	}

	gold := TitleStyle
	dim := DimStyle
	green := SuccessStyle
	warn := WarningStyle
	errStyle := ErrorStyle

	// Summary line
	fmt.Fprintf(os.Stderr, "\n  %s\n", gold.Render(r.Summary))

	// Duration
	if r.Duration > 0 {
		fmt.Fprintf(os.Stderr, "  %s\n", dim.Render(fmt.Sprintf("Completed in %s", r.Duration.Truncate(time.Millisecond))))
	}

	// Evidence
	if len(r.Evidence) > 0 {
		fmt.Fprintf(os.Stderr, "\n")
		for _, e := range r.Evidence {
			fmt.Fprintf(os.Stderr, "  %s  %s\n", green.Render(padRight(e.Label, 18)), e.Value)
		}
	}

	// Warnings
	if len(r.Warnings) > 0 {
		fmt.Fprintf(os.Stderr, "\n")
		for _, w := range r.Warnings {
			fmt.Fprintf(os.Stderr, "  %s %s\n", warn.Render("⚠"), w)
		}
	}

	// Errors
	if len(r.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "\n")
		for _, e := range r.Errors {
			fmt.Fprintf(os.Stderr, "  %s %s\n", errStyle.Render("✗"), e)
		}
	}

	// Next actions
	if len(r.NextActions) > 0 {
		fmt.Fprintf(os.Stderr, "\n  %s\n\n", dim.Render("── What's Next ──────────────────────────"))
		for _, a := range r.NextActions {
			fmt.Fprintf(os.Stderr, "  %s  %s\n", gold.Render(padRight(a.Command, 22)), dim.Render(a.Description))
		}
	}

	fmt.Fprintf(os.Stderr, "\n")
}

func (r *CommandResult) renderBrief() {
	gold := TitleStyle
	dim := DimStyle
	green := SuccessStyle
	warn := WarningStyle
	errStyle := ErrorStyle

	fmt.Fprintf(os.Stderr, "\n  %s\n", gold.Render(r.BriefTitle))
	if r.Summary != "" {
		fmt.Fprintf(os.Stderr, "  %s\n", dim.Render(r.Summary))
	}
	if r.Duration > 0 {
		fmt.Fprintf(os.Stderr, "  %s\n", dim.Render(fmt.Sprintf("Checked in %s", r.Duration.Truncate(time.Millisecond))))
	}

	if r.Status != "" || r.Confidence != "" {
		fmt.Fprintln(os.Stderr)
		if r.Status != "" {
			style := green
			statusLower := strings.ToLower(r.Status)
			if strings.Contains(statusLower, "critical") || strings.Contains(statusLower, "unstable") {
				style = errStyle
			} else if strings.Contains(statusLower, "attention") || strings.Contains(statusLower, "degraded") {
				style = warn
			}
			fmt.Fprintf(os.Stderr, "  %s  %s\n", dim.Render(padRight("Status", 14)), style.Render(r.Status))
		}
		if r.Confidence != "" {
			fmt.Fprintf(os.Stderr, "  %s  %s\n", dim.Render(padRight("Confidence", 14)), r.Confidence)
		}
	}

	if len(r.Evidence) > 0 {
		fmt.Fprintln(os.Stderr)
		for _, e := range r.Evidence {
			fmt.Fprintf(os.Stderr, "  %s  %s\n", dim.Render(padRight(e.Label, 14)), e.Value)
		}
	}

	if len(r.Priority) > 0 {
		fmt.Fprintf(os.Stderr, "\n  %s\n", gold.Render("Priority"))
		for i, p := range r.Priority {
			fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, p)
		}
	}

	if len(r.NextActions) > 0 {
		a := r.NextActions[0]
		fmt.Fprintf(os.Stderr, "\n  %s\n", gold.Render("Recommended action"))
		fmt.Fprintf(os.Stderr, "  %s\n", a.Description)
		fmt.Fprintf(os.Stderr, "  %s\n", dim.Render(a.Command))
	}

	fmt.Fprintln(os.Stderr)
}

// AddEvidence appends a labeled evidence item.
func (r *CommandResult) AddEvidence(label, value string) {
	r.Evidence = append(r.Evidence, Evidence{Label: label, Value: value})
}

// AddWarning appends a warning message.
func (r *CommandResult) AddWarning(msg string, args ...interface{}) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(msg, args...))
}

// AddError appends an error message.
func (r *CommandResult) AddError(msg string, args ...interface{}) {
	r.Errors = append(r.Errors, fmt.Sprintf(msg, args...))
}

// AddNextAction appends a suggested follow-up.
func (r *CommandResult) AddNextAction(command, description string) {
	label := command
	if idx := strings.Index(command, " "); idx > 0 {
		label = command[idx+1:]
	}
	r.NextActions = append(r.NextActions, NextAction{
		Label:       label,
		Command:     command,
		Description: description,
	})
}
