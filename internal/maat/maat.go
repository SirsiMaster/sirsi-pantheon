// Package maat implements the Ma'at QA/QC governance agent for the Sirsi Pantheon.
//
// Named after Ma'at — the Egyptian goddess of truth, justice, balance, and
// cosmic order. Her feather was the standard against which hearts were weighed.
// She is not a judge — she IS the standard.
//
// Ma'at observes, assesses, weighs, and reports on development quality:
//   - Canon linkage: every feature must be justified against ADRs/rules
//   - Coverage: per-module test coverage thresholds
//   - Pipeline: CI run status and failure categorization
//   - Code quality: lint, format, vet compliance
//
// See ADR-004 for the full architecture decision.
package maat

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// Verdict represents the outcome of a single assessment.
// Uses Egyptian mythology: the feather of Ma'at weighs the heart.
type Verdict int

const (
	// VerdictPass — the heart is lighter than the feather. All is well.
	VerdictPass Verdict = iota
	// VerdictWarning — the heart wavers. Attention required.
	VerdictWarning
	// VerdictFail — the heart is heavier than the feather. Action required.
	VerdictFail
)

// String returns the human-readable name for a verdict.
func (v Verdict) String() string {
	switch v {
	case VerdictPass:
		return "pass"
	case VerdictWarning:
		return "warning"
	case VerdictFail:
		return "fail"
	default:
		return "unknown"
	}
}

// Icon returns the emoji icon for a verdict.
func (v Verdict) Icon() string {
	switch v {
	case VerdictPass:
		return "✅"
	case VerdictWarning:
		return "⚠️"
	case VerdictFail:
		return "❌"
	default:
		return "❓"
	}
}

// Domain identifies what dimension of quality Ma'at is assessing.
type Domain string

const (
	DomainPipeline Domain = "pipeline"
	DomainCoverage Domain = "coverage"
	DomainCanon    Domain = "canon"
)

// Assessment is the result of weighing one quality dimension.
type Assessment struct {
	// Domain identifies what was assessed (pipeline, coverage, canon).
	Domain Domain `json:"domain"`

	// Subject is the specific thing that was weighed (e.g., module name, commit SHA).
	Subject string `json:"subject"`

	// Standard is what the subject was weighed against (e.g., "80% coverage", "ADR-001").
	Standard string `json:"standard"`

	// Verdict is the outcome: pass, warning, or fail.
	Verdict Verdict `json:"verdict"`

	// FeatherWeight is a score from 0-100 indicating quality.
	// 100 = perfect (light as a feather). 0 = critical failure.
	FeatherWeight int `json:"feather_weight"`

	// Message is a human-readable explanation of the verdict.
	Message string `json:"message"`

	// Remediation suggests how to fix a non-passing verdict.
	Remediation string `json:"remediation,omitempty"`
}

// Format returns a formatted string for terminal display.
func (a Assessment) Format() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s [%s] %s — %s (weight: %d/100)",
		a.Verdict.Icon(), a.Domain, a.Subject, a.Message, a.FeatherWeight)
	if a.Remediation != "" && a.Verdict != VerdictPass {
		fmt.Fprintf(&sb, "\n     Fix: %s", a.Remediation)
	}
	return sb.String()
}

// CanonLink ties a feature or change to its justification in the project canon.
type CanonLink struct {
	// Feature is the name or description of the feature/change.
	Feature string `json:"feature"`

	// Canon is the document that justifies the feature (e.g., "ADR-001", "ANUBIS_RULES A14").
	Canon string `json:"canon"`

	// Linked indicates whether the feature has a valid canon reference.
	Linked bool `json:"linked"`

	// CommitSHA is the commit that introduced the feature (optional).
	CommitSHA string `json:"commit_sha,omitempty"`
}

// Report is the aggregated result of a full Ma'at assessment.
type Report struct {
	// Assessments is the list of individual assessments.
	Assessments []Assessment `json:"assessments"`

	// OverallVerdict is the worst verdict across all assessments.
	OverallVerdict Verdict `json:"overall_verdict"`

	// OverallWeight is the average feather weight across all assessments.
	OverallWeight int `json:"overall_weight"`

	// Passes, Warnings, Failures count assessments by verdict.
	Passes   int `json:"passes"`
	Warnings int `json:"warnings"`
	Failures int `json:"failures"`

	// AssessedAt is when the assessment was performed.
	AssessedAt time.Time `json:"assessed_at"`
}

// NewReport creates a Report from a slice of assessments.
func NewReport(assessments []Assessment) *Report {
	r := &Report{
		Assessments:    assessments,
		OverallVerdict: VerdictPass,
		AssessedAt:     time.Now(),
	}

	if len(assessments) == 0 {
		r.OverallWeight = 100
		return r
	}

	totalWeight := 0
	for _, a := range assessments {
		totalWeight += a.FeatherWeight

		switch a.Verdict {
		case VerdictFail:
			r.Failures++
			r.OverallVerdict = VerdictFail
		case VerdictWarning:
			r.Warnings++
			if r.OverallVerdict != VerdictFail {
				r.OverallVerdict = VerdictWarning
			}
		case VerdictPass:
			r.Passes++
		}
	}

	r.OverallWeight = totalWeight / len(assessments)
	return r
}

// Assessor is the interface for pluggable quality dimensions.
// Each domain (pipeline, coverage, canon) implements this interface.
type Assessor interface {
	// Assess performs the quality assessment and returns assessments.
	Assess() ([]Assessment, error)

	// Domain returns the quality domain this assessor covers.
	Domain() Domain
}

// Weigh runs all provided assessors CONCURRENTLY on dedicated OS threads.
// Each assessor gets its own goroutine pinned to a separate CPU core via
// runtime.LockOSThread(). Pipeline, coverage, and canon assessments
// execute in true parallel across all available cores.
func Weigh(assessors ...Assessor) (*Report, error) {
	var mu sync.Mutex
	var all []Assessment
	var wg sync.WaitGroup

	for _, assessor := range assessors {
		wg.Add(1)
		go func(a Assessor) {
			defer wg.Done()
			// Pin to dedicated OS thread for true multithreading.
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()

			results, err := a.Assess()

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				all = append(all, Assessment{
					Domain:        a.Domain(),
					Subject:       "assessor",
					Standard:      "must complete without error",
					Verdict:       VerdictFail,
					FeatherWeight: 0,
					Message:       fmt.Sprintf("assessment failed: %v", err),
				})
				return
			}
			all = append(all, results...)
		}(assessor)
	}

	wg.Wait()

	report := NewReport(all)
	stele.Inscribe("maat", stele.TypeMaatWeigh, "", map[string]string{
		"score":       fmt.Sprintf("%d", report.OverallWeight),
		"assessments": fmt.Sprintf("%d", len(all)),
		"verdict":     report.OverallVerdict.String(),
	})
	return report, nil
}
