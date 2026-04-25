package jackal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PersistedScan is a ScanResult with metadata for disk storage.
type PersistedScan struct {
	Timestamp  time.Time                    `json:"timestamp"`
	DurationMs int64                        `json:"duration_ms"`
	TotalSize  int64                        `json:"total_size"`
	RulesRan   int                          `json:"rules_ran"`
	RulesHit   int                          `json:"rules_with_findings"`
	Findings   []PersistedFinding           `json:"findings"`
	ByCategory map[Category]CategorySummary `json:"by_category"`
}

// PersistedFinding is a Finding serializable to JSON.
type PersistedFinding struct {
	RuleName     string   `json:"rule"`
	Category     Category `json:"category"`
	Description  string   `json:"description"`
	Path         string   `json:"path"`
	SizeBytes    int64    `json:"size_bytes"`
	SizeHuman    string   `json:"size_human"`
	FileCount    int      `json:"file_count,omitempty"`
	Severity     Severity `json:"severity"`
	LastModified string   `json:"last_modified,omitempty"`
	IsDir        bool     `json:"is_dir,omitempty"`
	Advisory     string   `json:"advisory"`
	Remediation  string   `json:"remediation"`
	CanFix       bool     `json:"can_fix"`
	Breaking     bool     `json:"breaking,omitempty"`
}

// FindingsDir returns the directory where scan results are persisted.
func FindingsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "pantheon", "findings")
}

// LatestScanPath returns the path to the latest scan results file.
func LatestScanPath() string {
	return filepath.Join(FindingsDir(), "latest-scan.json")
}

// Persist writes a ScanResult to disk as the latest scan.
func Persist(result *ScanResult, duration time.Duration) error {
	dir := FindingsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create findings dir: %w", err)
	}

	ps := PersistedScan{
		Timestamp:  time.Now(),
		DurationMs: duration.Milliseconds(),
		TotalSize:  result.TotalSize,
		RulesRan:   result.RulesRan,
		RulesHit:   result.RulesWithFindings,
		ByCategory: result.ByCategory,
	}

	for _, f := range result.Findings {
		pf := PersistedFinding{
			RuleName:    f.RuleName,
			Category:    f.Category,
			Description: f.Description,
			Path:        f.Path,
			SizeBytes:   f.SizeBytes,
			SizeHuman:   FormatSize(f.SizeBytes),
			FileCount:   f.FileCount,
			Severity:    f.Severity,
			IsDir:       f.IsDir,
			Advisory:    f.Advisory,
			Remediation: f.Remediation,
			CanFix:      f.CanFix,
			Breaking:    f.Breaking,
		}
		if !f.LastModified.IsZero() {
			pf.LastModified = f.LastModified.Format(time.RFC3339)
		}
		ps.Findings = append(ps.Findings, pf)
	}

	if ps.Findings == nil {
		ps.Findings = []PersistedFinding{}
	}

	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal findings: %w", err)
	}

	return os.WriteFile(LatestScanPath(), data, 0644)
}

// LoadLatest reads the most recent persisted scan from disk.
func LoadLatest() (*PersistedScan, error) {
	data, err := os.ReadFile(LatestScanPath())
	if err != nil {
		return nil, err
	}
	var ps PersistedScan
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("parse findings: %w", err)
	}
	return &ps, nil
}
