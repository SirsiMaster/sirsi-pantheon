package isis

import (
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

// FromMaatReport converts a Ma'at report into Isis findings.
// This is the bridge between observation (Ma'at) and remediation (Isis).
func FromMaatReport(report *maat.Report) []Finding {
	var findings []Finding

	for _, a := range report.Assessments {
		if a.Verdict == maat.VerdictPass {
			continue // Only heal non-passing assessments
		}

		findings = append(findings, Finding{
			Domain:      string(a.Domain),
			Subject:     a.Subject,
			Message:     a.Message,
			Remediation: a.Remediation,
			Severity:    a.Verdict.String(),
			Weight:      a.FeatherWeight,
		})
	}

	return findings
}
