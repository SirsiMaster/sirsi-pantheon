package guard

import (
	"strings"
	"testing"
)

// ── Mock ps output ──────────────────────────────────────────────────────

func mockOrphanPs(entries []orphanPsEntry) func() ([]orphanPsEntry, error) {
	return func() ([]orphanPsEntry, error) {
		return entries, nil
	}
}

// ── Pattern Tests ───────────────────────────────────────────────────────

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		proc     string
		patterns []string
		want     bool
	}{
		{"Playwright match", "ms-playwright/node run-driver", []string{"ms-playwright", "run-driver"}, true},
		{"Partial match", "/usr/bin/ms-playwright-go/1.50.1/node", []string{"ms-playwright"}, true},
		{"No match", "Google Chrome Helper", []string{"ms-playwright", "run-driver"}, false},
		{"Case insensitive", "MS-PLAYWRIGHT", []string{"ms-playwright"}, true},
		{"Empty patterns", "anything", []string{}, false},
		{"Antigravity profile", "Chrome --user-data-dir=antigravity-browser-profile", []string{"antigravity-browser-profile"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesPattern(tt.proc, tt.patterns)
			if got != tt.want {
				t.Errorf("matchesPattern(%q, %v) = %v, want %v", tt.proc, tt.patterns, got, tt.want)
			}
		})
	}
}

// ── ScanOrphans Tests ──────────────────────────────────────────────────

func TestScanOrphans_DetectsPlaywright(t *testing.T) {
	entries := []orphanPsEntry{
		{PID: 100, PPID: 1, RSS: 64 * 1024, CPU: 0.0, Name: "ms-playwright/node run-driver", ElapsedTime: "01:30:00"},
		{PID: 101, PPID: 1, RSS: 64 * 1024, CPU: 0.0, Name: "ms-playwright/node run-driver", ElapsedTime: "01:30:00"},
		{PID: 200, PPID: 50, RSS: 256 * 1024, CPU: 2.0, Name: "Google Chrome", ElapsedTime: "05:00:00"},
	}

	report, err := scanOrphansWithFn(mockOrphanPs(entries))
	if err != nil {
		t.Fatal(err)
	}

	if report.TotalOrphans != 2 {
		t.Errorf("Expected 2 Playwright orphans, got %d", report.TotalOrphans)
	}
	if report.ByCategory["browser_automation"] != 2 {
		t.Errorf("Expected browser_automation=2, got %d", report.ByCategory["browser_automation"])
	}

	for _, o := range report.Orphans {
		if o.Pattern != "Playwright" {
			t.Errorf("Expected pattern=Playwright, got %s", o.Pattern)
		}
		if !o.IsOrphaned {
			t.Error("PPID=1 should be marked as orphaned")
		}
	}
}

func TestScanOrphans_DetectsAntigravityBrowser(t *testing.T) {
	entries := []orphanPsEntry{
		{PID: 300, PPID: 1, RSS: 128 * 1024, CPU: 0.5, Name: "Chrome Helper --user-data-dir=antigravity-browser-profile", ElapsedTime: "00:45:00"},
	}

	report, err := scanOrphansWithFn(mockOrphanPs(entries))
	if err != nil {
		t.Fatal(err)
	}

	if report.TotalOrphans != 1 {
		t.Errorf("Expected 1 orphan, got %d", report.TotalOrphans)
	}
	if len(report.Orphans) > 0 && report.Orphans[0].Pattern != "Stale Chrome Profiles" {
		t.Errorf("Expected pattern='Stale Chrome Profiles', got %s", report.Orphans[0].Pattern)
	}
}

func TestScanOrphans_DetectsStaleParent(t *testing.T) {
	// LSP running under launchd (parent died)
	entries := []orphanPsEntry{
		{PID: 500, PPID: 1, RSS: 200 * 1024, CPU: 12.0, Name: "gopls", ElapsedTime: "03:00:00"},
	}

	report, err := scanOrphansWithFn(mockOrphanPs(entries))
	if err != nil {
		t.Fatal(err)
	}

	if report.TotalOrphans != 1 {
		t.Errorf("Expected 1 orphan, got %d", report.TotalOrphans)
	}
	if len(report.Orphans) > 0 {
		if report.Orphans[0].Category != "ide" {
			t.Errorf("Expected category=ide, got %s", report.Orphans[0].Category)
		}
	}
}

func TestScanOrphans_IgnoresHealthyProcesses(t *testing.T) {
	// gopls running under cursor (healthy — not an orphan)
	entries := []orphanPsEntry{
		{PID: 50, PPID: 0, RSS: 100 * 1024, CPU: 1.0, Name: "cursor", ElapsedTime: "05:00:00"},
		{PID: 500, PPID: 50, RSS: 200 * 1024, CPU: 12.0, Name: "gopls", ElapsedTime: "03:00:00"},
	}

	report, err := scanOrphansWithFn(mockOrphanPs(entries))
	if err != nil {
		t.Fatal(err)
	}

	if report.TotalOrphans != 0 {
		t.Errorf("Expected 0 orphans (gopls is under cursor), got %d", report.TotalOrphans)
	}
}

func TestScanOrphans_EmptySystem(t *testing.T) {
	report, err := scanOrphansWithFn(mockOrphanPs(nil))
	if err != nil {
		t.Fatal(err)
	}

	if report.TotalOrphans != 0 {
		t.Error("Empty system should have 0 orphans")
	}
	if report.TotalRAMHuman != "0 B" {
		t.Errorf("Expected '0 B', got %q", report.TotalRAMHuman)
	}
}

func TestScanOrphans_MultipleCategories(t *testing.T) {
	entries := []orphanPsEntry{
		{PID: 100, PPID: 1, RSS: 64 * 1024, CPU: 0.0, Name: "ms-playwright/node run-driver", ElapsedTime: "01:00:00"},
		{PID: 200, PPID: 1, RSS: 200 * 1024, CPU: 5.0, Name: "gopls", ElapsedTime: "02:00:00"},
		{PID: 300, PPID: 1, RSS: 32 * 1024, CPU: 0.0, Name: "fswatch", ElapsedTime: "00:30:00"},
	}

	report, err := scanOrphansWithFn(mockOrphanPs(entries))
	if err != nil {
		t.Fatal(err)
	}

	if report.TotalOrphans != 3 {
		t.Errorf("Expected 3 orphans, got %d", report.TotalOrphans)
	}
	if report.ByCategory["browser_automation"] != 1 {
		t.Errorf("browser_automation=%d, want 1", report.ByCategory["browser_automation"])
	}
	if report.ByCategory["ide"] != 1 {
		t.Errorf("ide=%d, want 1", report.ByCategory["ide"])
	}
	if report.ByCategory["build_tool"] != 1 {
		t.Errorf("build_tool=%d, want 1", report.ByCategory["build_tool"])
	}
}

// ── Format Tests ───────────────────────────────────────────────────────

func TestFormatOrphanReport_Empty(t *testing.T) {
	report := &OrphanReport{ByCategory: make(map[string]int)}
	output := FormatOrphanReport(report)
	if !strings.Contains(output, "No orphaned") {
		t.Errorf("Empty report should say 'No orphaned', got %q", output)
	}
}

func TestFormatOrphanReport_WithOrphans(t *testing.T) {
	report := &OrphanReport{
		TotalOrphans:  2,
		TotalRAM:      128 * 1024,
		TotalRAMHuman: "128.0 KB",
		ByCategory:    map[string]int{"browser_automation": 2},
		Orphans: []OrphanProcess{
			{
				ProcessInfo: ProcessInfo{PID: 100, Name: "playwright", RSS: 64 * 1024},
				Pattern:     "Playwright", Category: "browser_automation",
				ParentPID: 1, IsOrphaned: true, RunningFor: "01:30:00",
			},
			{
				ProcessInfo: ProcessInfo{PID: 101, Name: "playwright", RSS: 64 * 1024},
				Pattern:     "Playwright", Category: "browser_automation",
				ParentPID: 1, IsOrphaned: true, RunningFor: "01:30:00",
			},
		},
	}

	output := FormatOrphanReport(report)
	if !strings.Contains(output, "Isis") {
		t.Error("Report should contain Isis branding")
	}
	if !strings.Contains(output, "orphaned") {
		t.Error("Report should mention 'orphaned'")
	}
	if !strings.Contains(output, "Playwright") {
		t.Error("Report should show Playwright pattern")
	}
	if !strings.Contains(output, "2 orphan") {
		t.Error("Report should show total count")
	}
}

// ── KnownOrphanPatterns Tests ───────────────────────────────────────────

func TestKnownOrphanPatterns_Coverage(t *testing.T) {
	expectedPatterns := []string{
		"Playwright", "Puppeteer", "Chrome DevTools",
		"Electron Helper", "LSP Servers", "Build Watchers",
		"Stale Chrome Profiles",
	}

	for _, name := range expectedPatterns {
		found := false
		for _, p := range KnownOrphanPatterns {
			if p.Name == name {
				found = true
				if len(p.ProcessNames) == 0 {
					t.Errorf("Pattern %q has no ProcessNames", name)
				}
				if p.Category == "" {
					t.Errorf("Pattern %q has no Category", name)
				}
				break
			}
		}
		if !found {
			t.Errorf("Missing expected orphan pattern: %s", name)
		}
	}
}
