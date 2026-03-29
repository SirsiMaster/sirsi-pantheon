// Package isis implements the autonomous remediation engine for the Sirsi Pantheon.
//
// Named after Isis — the Egyptian goddess of healing, magic, and restoration.
// She reassembled Osiris from scattered pieces. She restores what is broken.
//
// The Ma'at-Isis cycle:
//
//	Ma'at.Weigh()  ──→  Report{Assessments}
//	                         │
//	                         ▼
//	                Isis.Heal(report)
//	                    ├── LintStrategy      (goimports, gofmt)
//	                    ├── VetStrategy       (go vet, structured parse)
//	                    ├── CoverageStrategy  (identify untested exports)
//	                    └── CanonStrategy     (thoth sync trigger)
//	                         │
//	                         ▼
//	                Ma'at.ReWeigh() ──→ updated Report
//
// See CONTINUATION-PROMPT.md §V for the strategic mandate.
package isis

import (
	"fmt"
	"strings"
	"time"
)

// Strategy is the interface for pluggable remediation strategies.
// Each strategy handles a specific class of quality findings.
type Strategy interface {
	// Name returns the human-readable name of the strategy.
	Name() string

	// CanHeal returns true if this strategy can handle the given finding.
	CanHeal(finding Finding) bool

	// Heal attempts to remediate the finding. Returns a HealResult.
	// If dryRun is true, no changes are written — only a preview.
	Heal(finding Finding, dryRun bool) HealResult
}

// Finding represents a quality issue discovered by Ma'at (or any assessor).
type Finding struct {
	// Domain identifies the quality dimension (coverage, canon, pipeline, lint, vet).
	Domain string

	// Subject is the specific target (module name, file path, commit SHA).
	Subject string

	// Message describes the issue.
	Message string

	// Remediation is the suggested fix (from Ma'at's Assessment.Remediation).
	Remediation string

	// Severity: "fail" or "warning".
	Severity string

	// Weight is the current feather weight (0-100).
	Weight int
}

// HealResult captures the outcome of a single remediation attempt.
type HealResult struct {
	// Finding is the original issue.
	Finding Finding

	// Strategy is the name of the strategy that handled this.
	Strategy string

	// Healed is true if the issue was successfully remediated.
	Healed bool

	// DryRun is true if this was a preview (no changes written).
	DryRun bool

	// Action describes what was done (or would be done).
	Action string

	// FilesChanged lists the files that were modified (or would be).
	FilesChanged []string

	// Error is set if the remediation failed.
	Error error
}

// Report is the aggregated result of a full healing cycle.
type Report struct {
	// Results is the list of individual heal results.
	Results []HealResult

	// TotalFindings is how many findings were presented.
	TotalFindings int

	// Healed is how many were successfully fixed.
	Healed int

	// Skipped is how many had no applicable strategy.
	Skipped int

	// Failed is how many remediation attempts errored.
	Failed int

	// DryRun indicates this was a preview run.
	DryRun bool

	// Duration is how long the healing took.
	Duration time.Duration
}

// Healer is the core remediation engine.
// It accepts Ma'at's findings and dispatches to registered strategies.
type Healer struct {
	// Strategies is the ordered list of remediation strategies.
	Strategies []Strategy

	// ProjectRoot is the root of the project to heal.
	ProjectRoot string

	// Verbose enables detailed output.
	Verbose bool
}

// NewHealer creates a Healer with the default strategy stack.
func NewHealer(projectRoot string) *Healer {
	return &Healer{
		ProjectRoot: projectRoot,
		Strategies: []Strategy{
			NewLintStrategy(projectRoot),
			NewVetStrategy(projectRoot),
			NewCoverageStrategy(projectRoot),
			NewCanonStrategy(projectRoot),
		},
	}
}

// Heal runs all applicable strategies against the findings.
// If dryRun is true, no changes are written to disk.
func (h *Healer) Heal(findings []Finding, dryRun bool) *Report {
	start := time.Now()
	report := &Report{
		TotalFindings: len(findings),
		DryRun:        dryRun,
	}

	for _, finding := range findings {
		handled := false
		for _, strategy := range h.Strategies {
			if strategy.CanHeal(finding) {
				result := strategy.Heal(finding, dryRun)
				report.Results = append(report.Results, result)
				if result.Error != nil {
					report.Failed++
				} else if result.Healed {
					report.Healed++
				}
				handled = true
				break // First matching strategy wins
			}
		}
		if !handled {
			report.Skipped++
			report.Results = append(report.Results, HealResult{
				Finding:  finding,
				Strategy: "none",
				Action:   "no applicable strategy",
			})
		}
	}

	report.Duration = time.Since(start)
	return report
}

// Format returns a human-readable summary of the healing report.
func (r *Report) Format() string {
	var sb strings.Builder

	mode := "HEAL"
	if r.DryRun {
		mode = "DRY RUN"
	}

	sb.WriteString(fmt.Sprintf("𓁐 Isis Healing Report [%s]\n", mode))
	sb.WriteString(fmt.Sprintf("  Findings: %d | Healed: %d | Skipped: %d | Failed: %d\n",
		r.TotalFindings, r.Healed, r.Skipped, r.Failed))
	sb.WriteString(fmt.Sprintf("  Duration: %s\n\n", r.Duration.Round(time.Millisecond)))

	for _, result := range r.Results {
		icon := "⏭️"
		if result.Healed {
			icon = "✅"
		} else if result.Error != nil {
			icon = "❌"
		}
		sb.WriteString(fmt.Sprintf("  %s [%s] %s — %s\n",
			icon, result.Finding.Domain, result.Finding.Subject, result.Action))
		if result.Error != nil {
			sb.WriteString(fmt.Sprintf("     Error: %v\n", result.Error))
		}
		if len(result.FilesChanged) > 0 {
			sb.WriteString(fmt.Sprintf("     Files: %s\n", strings.Join(result.FilesChanged, ", ")))
		}
	}

	// Overall verdict
	sb.WriteString("\n")
	if r.TotalFindings == 0 {
		sb.WriteString("  𓂀 The feather is already balanced. Nothing to heal.\n")
	} else if r.Failed == 0 && r.Healed > 0 {
		sb.WriteString("  𓂀 Isis has restored the weight of the feather.\n")
	} else if r.Failed > 0 {
		sb.WriteString("  ⚠️ Some wounds remain. Manual intervention required.\n")
	} else {
		sb.WriteString("  ℹ️ No actionable findings for auto-remediation.\n")
	}

	return sb.String()
}
