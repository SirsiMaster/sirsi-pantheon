package horus

import (
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// WorkstationReport is the aggregated state of a single workstation.
// Horus collects this from all local deities and can send it to Ra.
// This is the contract between Horus (local lord) and Ra (fleet lord).
type WorkstationReport struct {
	// Identity
	Hostname  string    `json:"hostname"`
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
	Timestamp time.Time `json:"timestamp"`

	// Scan (Anubis)
	ScanFindings int   `json:"scan_findings"`
	ScanWaste    int64 `json:"scan_waste_bytes"`
	SafeCount    int   `json:"safe_count"`
	CautionCount int   `json:"caution_count"`
	WarningCount int   `json:"warning_count"`
	FixableCount int   `json:"fixable_count"`

	// Ghosts (Ka)
	GhostCount int   `json:"ghost_count"`
	GhostWaste int64 `json:"ghost_waste_bytes"`

	// Health (Isis/Guard)
	HealthScore int     `json:"health_score"` // 0-100 from Doctor
	RAMPercent  float64 `json:"ram_percent"`
	RAMPressure string  `json:"ram_pressure"` // low/medium/high

	// Quality (Ma'at)
	QualityScore int `json:"quality_score,omitempty"` // 0-100 if available

	// Code (Horus — self)
	CodeFiles    int `json:"code_files"`
	CodePackages int `json:"code_packages"`
	CodeSymbols  int `json:"code_symbols"`

	// Git
	GitBranch        string `json:"git_branch"`
	UncommittedFiles int    `json:"uncommitted_files"`

	// Stale — when was each deity last active
	LastScan    time.Time `json:"last_scan,omitempty"`
	LastGuard   time.Time `json:"last_guard,omitempty"`
	LastQuality time.Time `json:"last_quality,omitempty"`
}

// ScanSummary populates scan fields from a persisted scan.
func (r *WorkstationReport) ScanSummary(ps *jackal.PersistedScan) {
	if ps == nil {
		return
	}
	r.ScanFindings = len(ps.Findings)
	r.ScanWaste = ps.TotalSize
	r.LastScan = ps.Timestamp

	for _, f := range ps.Findings {
		switch f.Severity {
		case jackal.SeveritySafe:
			r.SafeCount++
		case jackal.SeverityCaution:
			r.CautionCount++
		case jackal.SeverityWarning:
			r.WarningCount++
		}
		if f.CanFix {
			r.FixableCount++
		}
	}
}
