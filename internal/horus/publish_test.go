package horus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── parseCaseStudyMD Tests ──────────────────────────────────────────────

func TestParseCaseStudyMD_WithTitle(t *testing.T) {
	content := `# Docker Desktop Ghost Discovery

**Date**: 2026-03-22

Pantheon found 64 GB of unused Docker VM images.`

	study := parseCaseStudyMD("docker-ghost.md", content)

	if study.Title != "Docker Desktop Ghost Discovery" {
		t.Errorf("Title = %q, want 'Docker Desktop Ghost Discovery'", study.Title)
	}
	if study.Date != "2026-03-22" {
		t.Errorf("Date = %q, want '2026-03-22'", study.Date)
	}
	if study.Body != content {
		t.Error("Body should contain full content")
	}
}

func TestParseCaseStudyMD_NoTitle(t *testing.T) {
	content := "Just some content without headers."
	study := parseCaseStudyMD("simple.md", content)

	if study.Title != "simple" {
		t.Errorf("Title = %q, want 'simple'", study.Title)
	}
}

// ── parseJournalSessions Tests ──────────────────────────────────────────

func TestParseJournalSessions_MultipleSessions(t *testing.T) {
	content := `# Thoth Journal

## Session 17: Cross-Platform Architecture
**Date**: 2026-03-24
Built Platform interface with 12 methods.

## Session 18: Menu Bar Application
**Date**: 2026-03-25
Building the macOS menu bar app.`

	sessions := parseJournalSessions(content)

	if len(sessions) != 2 {
		t.Fatalf("Got %d sessions, want 2", len(sessions))
	}

	if sessions[0].Number != 17 {
		t.Errorf("Session[0].Number = %d, want 17", sessions[0].Number)
	}
	if sessions[0].Title != "Cross-Platform Architecture" {
		t.Errorf("Session[0].Title = %q", sessions[0].Title)
	}
	if sessions[0].Date != "2026-03-24" {
		t.Errorf("Session[0].Date = %q", sessions[0].Date)
	}

	if sessions[1].Number != 18 {
		t.Errorf("Session[1].Number = %d, want 18", sessions[1].Number)
	}
	if sessions[1].Title != "Menu Bar Application" {
		t.Errorf("Session[1].Title = %q", sessions[1].Title)
	}
}

func TestParseJournalSessions_EmDash(t *testing.T) {
	content := `## Session 16 — Coverage Sprint
The big sprint.`

	sessions := parseJournalSessions(content)
	if len(sessions) != 1 {
		t.Fatalf("Got %d sessions, want 1", len(sessions))
	}
	if sessions[0].Number != 16 {
		t.Errorf("Number = %d, want 16", sessions[0].Number)
	}
	if sessions[0].Title != "Coverage Sprint" {
		t.Errorf("Title = %q, want 'Coverage Sprint'", sessions[0].Title)
	}
}

func TestParseJournalSessions_Empty(t *testing.T) {
	sessions := parseJournalSessions("")
	if len(sessions) != 0 {
		t.Errorf("Got %d sessions from empty content, want 0", len(sessions))
	}
}

// ── parseInt Tests ──────────────────────────────────────────────────────

func TestParseInt(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"18", 18},
		{"17:", 17},
		{"0", 0},
		{"abc", 0},
		{"123abc", 123},
	}

	for _, tt := range tests {
		got := parseInt(tt.input)
		if got != tt.want {
			t.Errorf("parseInt(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// ── escapeHTML Tests ────────────────────────────────────────────────────

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<script>", "&lt;script&gt;"},
		{"a & b", "a &amp; b"},
		{`"quoted"`, "&quot;quoted&quot;"},
		{"clean text", "clean text"},
	}

	for _, tt := range tests {
		got := escapeHTML(tt.input)
		if got != tt.want {
			t.Errorf("escapeHTML(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── truncate Tests ──────────────────────────────────────────────────────

func TestTruncate(t *testing.T) {
	if truncate("short", 100) != "short" {
		t.Error("Short string should not be truncated")
	}
	if !strings.HasSuffix(truncate("this is a long string", 10), "...") {
		t.Error("Long string should end with ...")
	}
	if len(truncate("abcdefghij", 5)) != 8 { // "abcde..."
		t.Errorf("Truncated length = %d, want 8", len(truncate("abcdefghij", 5)))
	}
}

// ── DefaultPublishConfig Tests ──────────────────────────────────────────

func TestDefaultPublishConfig(t *testing.T) {
	cfg := DefaultPublishConfig("/Users/test/project")

	if cfg.RepoRoot != "/Users/test/project" {
		t.Errorf("RepoRoot = %q", cfg.RepoRoot)
	}
	if !strings.Contains(cfg.JournalPath, ".thoth") {
		t.Errorf("JournalPath = %q, should contain .thoth", cfg.JournalPath)
	}
	if !strings.Contains(cfg.BuildLogPath, "build-log.html") {
		t.Errorf("BuildLogPath = %q", cfg.BuildLogPath)
	}
	if !strings.Contains(cfg.CaseStudyPath, "case-studies.html") {
		t.Errorf("CaseStudyPath = %q", cfg.CaseStudyPath)
	}
}

// ── Publish Integration Tests ───────────────────────────────────────────

func TestPublish_EmptyRepoRoot(t *testing.T) {
	_, err := Publish(PublishConfig{})
	if err == nil {
		t.Error("Should error with empty repo root")
	}
}

func TestPublish_WithTempDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create case studies dir with a file
	csDir := filepath.Join(tmpDir, "docs", "case-studies")
	if err := os.MkdirAll(csDir, 0o755); err != nil {
		t.Fatal(err)
	}
	csContent := `# Test Case Study

**Date**: 2026-03-25

This is a test case study.`
	if err := os.WriteFile(filepath.Join(csDir, "test.md"), []byte(csContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create journal
	thothDir := filepath.Join(tmpDir, ".thoth")
	if err := os.MkdirAll(thothDir, 0o755); err != nil {
		t.Fatal(err)
	}
	journalContent := `# Thoth Journal

## Session 1: Genesis
**Date**: 2026-03-21
First session.`
	if err := os.WriteFile(filepath.Join(thothDir, "journal.md"), []byte(journalContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultPublishConfig(tmpDir)
	result, err := Publish(cfg)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if !result.CaseStudyUpdated {
		t.Error("Case studies should have been updated")
	}
	if !result.BuildLogUpdated {
		t.Error("Build log should have been updated")
	}
	if result.EntriesAdded < 2 {
		t.Errorf("EntriesAdded = %d, want >= 2", result.EntriesAdded)
	}

	// Verify HTML files were created
	if _, err := os.Stat(cfg.BuildLogPath); os.IsNotExist(err) {
		t.Error("build-log.html should exist")
	}
	if _, err := os.Stat(cfg.CaseStudyPath); os.IsNotExist(err) {
		t.Error("case-studies.html should exist")
	}

	// Verify content
	blData, _ := os.ReadFile(cfg.BuildLogPath)
	if !strings.Contains(string(blData), "Pantheon Build Log") {
		t.Error("build-log.html should contain Pantheon header")
	}
	if !strings.Contains(string(blData), "Genesis") {
		t.Error("build-log.html should contain session title")
	}

	csData, _ := os.ReadFile(cfg.CaseStudyPath)
	if !strings.Contains(string(csData), "Test Case Study") {
		t.Error("case-studies.html should contain case study title")
	}
}

// ── collectCaseStudies Tests ────────────────────────────────────────────

func TestCollectCaseStudies_EmptyDir(t *testing.T) {
	_, err := collectCaseStudies("")
	if err == nil {
		t.Error("Should error with empty dir")
	}
}

func TestCollectCaseStudies_NonexistentDir(t *testing.T) {
	_, err := collectCaseStudies("/nonexistent/path")
	if err == nil {
		t.Error("Should error with nonexistent dir")
	}
}

func TestCollectCaseStudies_SkipsNonMD(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("not markdown"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "good.md"), []byte("# Good"), 0o644)

	studies, err := collectCaseStudies(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(studies) != 1 {
		t.Errorf("Got %d studies, want 1 (should skip .txt)", len(studies))
	}
}

// ── HTML Header/Footer Tests ────────────────────────────────────────────

func TestBuildLogHTMLHeader(t *testing.T) {
	header := buildLogHTMLHeader()
	if !strings.Contains(header, "<!DOCTYPE html>") {
		t.Error("Header should contain DOCTYPE")
	}
	if !strings.Contains(header, "Pantheon Build Log") {
		t.Error("Header should contain title")
	}
	if !strings.Contains(header, "#C8A951") {
		t.Error("Header should contain gold color")
	}
}

func TestCaseStudyHTMLHeader(t *testing.T) {
	header := caseStudyHTMLHeader()
	if !strings.Contains(header, "Case Studies") {
		t.Error("Header should contain Case Studies")
	}
}

func TestBuildLogHTMLFooter(t *testing.T) {
	footer := buildLogHTMLFooter()
	if !strings.Contains(footer, "Horus") {
		t.Error("Footer should mention Horus")
	}
	if !strings.Contains(footer, "</html>") {
		t.Error("Footer should close HTML")
	}
}
