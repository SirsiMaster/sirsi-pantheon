package osiris

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ── Mock command runner ─────────────────────────────────────────────────

type mockRunner struct {
	responses map[string]string
	errors    map[string]error
}

func (m *mockRunner) run(dir, name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	if err, ok := m.errors[key]; ok {
		return "", err
	}
	if resp, ok := m.responses[key]; ok {
		return resp, nil
	}
	return "", fmt.Errorf("mock: unhandled command: %s", key)
}

func setupMock(responses map[string]string) func() {
	m := &mockRunner{responses: responses}
	orig := runCommand
	runCommand = m.run
	return func() { runCommand = orig }
}

func setupMockWithErrors(responses map[string]string, errors map[string]error) func() {
	m := &mockRunner{responses: responses, errors: errors}
	orig := runCommand
	runCommand = m.run
	return func() { runCommand = orig }
}

// ── Assess Tests ────────────────────────────────────────────────────────

func TestAssess_CleanTree(t *testing.T) {
	commitTime := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
	cleanup := setupMock(map[string]string{
		"git rev-parse --show-toplevel":     "/Users/test/project",
		"git rev-parse --abbrev-ref HEAD":   "main",
		"git status --porcelain":            "",
		"git log -1 --format=%H|%s|%aI":     "abc1234567890|clean commit|" + commitTime,
		"git diff --stat --numstat":          "",
	})
	defer cleanup()

	cp, err := Assess(".")
	if err != nil {
		t.Fatalf("Assess failed: %v", err)
	}

	if cp.Risk != RiskNone {
		t.Errorf("Risk = %q, want %q", cp.Risk, RiskNone)
	}
	if cp.TotalChanges != 0 {
		t.Errorf("TotalChanges = %d, want 0", cp.TotalChanges)
	}
	if cp.Branch != "main" {
		t.Errorf("Branch = %q, want 'main'", cp.Branch)
	}
	if cp.ShouldWarn() {
		t.Error("Clean tree should not warn")
	}
	if !strings.Contains(cp.Summary(), "Clean tree") {
		t.Errorf("Summary = %q, should contain 'Clean tree'", cp.Summary())
	}
	if cp.StatusIcon() != "✅" {
		t.Errorf("StatusIcon = %q, want ✅", cp.StatusIcon())
	}
}

func TestAssess_FewChanges(t *testing.T) {
	commitTime := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
	cleanup := setupMock(map[string]string{
		"git rev-parse --show-toplevel":   "/Users/test/project",
		"git rev-parse --abbrev-ref HEAD": "feature/menu-bar",
		"git status --porcelain":          " M internal/guard/watchdog.go\n M internal/osiris/osiris.go\n?? new_file.txt",
		"git log -1 --format=%H|%s|%aI":   "def4567890abc|add watchdog|" + commitTime,
		"git diff --stat --numstat":        "10\t2\tinternal/guard/watchdog.go\n5\t1\tinternal/osiris/osiris.go",
	})
	defer cleanup()

	cp, err := Assess("")
	if err != nil {
		t.Fatalf("Assess failed: %v", err)
	}

	if cp.Risk != RiskLow {
		t.Errorf("Risk = %q, want %q", cp.Risk, RiskLow)
	}
	if cp.TotalChanges != 3 {
		t.Errorf("TotalChanges = %d, want 3", cp.TotalChanges)
	}
	if cp.ModifiedFiles != 2 {
		t.Errorf("ModifiedFiles = %d, want 2", cp.ModifiedFiles)
	}
	if cp.UntrackedFiles != 1 {
		t.Errorf("UntrackedFiles = %d, want 1", cp.UntrackedFiles)
	}
	if cp.LinesAdded != 15 {
		t.Errorf("LinesAdded = %d, want 15", cp.LinesAdded)
	}
	if cp.LinesDeleted != 3 {
		t.Errorf("LinesDeleted = %d, want 3", cp.LinesDeleted)
	}
	if cp.ShouldWarn() {
		t.Error("Low risk should not warn")
	}
	if cp.StatusIcon() != "🟢" {
		t.Errorf("StatusIcon = %q, want 🟢", cp.StatusIcon())
	}
	if cp.LastCommitHash != "def4567" {
		t.Errorf("LastCommitHash = %q, want 'def4567'", cp.LastCommitHash)
	}
}

func TestAssess_ModerateRisk(t *testing.T) {
	commitTime := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	// 8 modified files → moderate
	lines := []string{}
	for i := 0; i < 8; i++ {
		lines = append(lines, fmt.Sprintf(" M file%d.go", i))
	}
	cleanup := setupMock(map[string]string{
		"git rev-parse --show-toplevel":   "/Users/test/project",
		"git rev-parse --abbrev-ref HEAD": "main",
		"git status --porcelain":          strings.Join(lines, "\n"),
		"git log -1 --format=%H|%s|%aI":   "aaa1234567890|session work|" + commitTime,
		"git diff --stat --numstat":        "",
	})
	defer cleanup()

	cp, err := Assess(".")
	if err != nil {
		t.Fatalf("Assess failed: %v", err)
	}

	if cp.Risk != RiskModerate {
		t.Errorf("Risk = %q, want %q", cp.Risk, RiskModerate)
	}
	if cp.ShouldWarn() {
		t.Error("Moderate risk should not trigger ShouldWarn")
	}
	if cp.StatusIcon() != "🟡" {
		t.Errorf("StatusIcon = %q, want 🟡", cp.StatusIcon())
	}
}

func TestAssess_HighRisk(t *testing.T) {
	commitTime := time.Now().Add(-45 * time.Minute).Format(time.RFC3339)
	// 20 files → high
	lines := []string{}
	for i := 0; i < 20; i++ {
		lines = append(lines, fmt.Sprintf(" M file%d.go", i))
	}
	cleanup := setupMock(map[string]string{
		"git rev-parse --show-toplevel":   "/Users/test/project",
		"git rev-parse --abbrev-ref HEAD": "dev",
		"git status --porcelain":          strings.Join(lines, "\n"),
		"git log -1 --format=%H|%s|%aI":   "bbb1234567890|wip|" + commitTime,
		"git diff --stat --numstat":        "",
	})
	defer cleanup()

	cp, err := Assess(".")
	if err != nil {
		t.Fatalf("Assess failed: %v", err)
	}

	if cp.Risk != RiskHigh {
		t.Errorf("Risk = %q, want %q", cp.Risk, RiskHigh)
	}
	if !cp.ShouldWarn() {
		t.Error("High risk should trigger ShouldWarn")
	}
	if cp.StatusIcon() != "🟠" {
		t.Errorf("StatusIcon = %q, want 🟠", cp.StatusIcon())
	}
	if cp.Warning == "" {
		t.Error("High risk should have a warning")
	}
}

func TestAssess_CriticalRisk_ManyFiles(t *testing.T) {
	commitTime := time.Now().Add(-15 * time.Minute).Format(time.RFC3339)
	// 35 files → critical
	lines := []string{}
	for i := 0; i < 35; i++ {
		lines = append(lines, fmt.Sprintf(" M big_file%d.go", i))
	}
	cleanup := setupMock(map[string]string{
		"git rev-parse --show-toplevel":   "/Users/test/project",
		"git rev-parse --abbrev-ref HEAD": "feature/big",
		"git status --porcelain":          strings.Join(lines, "\n"),
		"git log -1 --format=%H|%s|%aI":   "ccc1234567890|big change|" + commitTime,
		"git diff --stat --numstat":        "",
	})
	defer cleanup()

	cp, err := Assess(".")
	if err != nil {
		t.Fatalf("Assess failed: %v", err)
	}

	if cp.Risk != RiskCritical {
		t.Errorf("Risk = %q, want %q", cp.Risk, RiskCritical)
	}
	if !cp.ShouldWarn() {
		t.Error("Critical risk should warn")
	}
	if cp.StatusIcon() != "🔴" {
		t.Errorf("StatusIcon = %q, want 🔴", cp.StatusIcon())
	}
	if !strings.Contains(cp.Warning, "OSIRIS WARNING") {
		t.Errorf("Warning = %q, should contain 'OSIRIS WARNING'", cp.Warning)
	}
}

func TestAssess_CriticalRisk_TimeElapsed(t *testing.T) {
	// Only 3 files, but 3 hours since last commit → critical
	commitTime := time.Now().Add(-3 * time.Hour).Format(time.RFC3339)
	cleanup := setupMock(map[string]string{
		"git rev-parse --show-toplevel":   "/Users/test/project",
		"git rev-parse --abbrev-ref HEAD": "main",
		"git status --porcelain":          " M a.go\n M b.go\n M c.go",
		"git log -1 --format=%H|%s|%aI":   "ddd1234567890|old commit|" + commitTime,
		"git diff --stat --numstat":        "",
	})
	defer cleanup()

	cp, err := Assess(".")
	if err != nil {
		t.Fatalf("Assess failed: %v", err)
	}

	if cp.Risk != RiskCritical {
		t.Errorf("Risk = %q, want %q (time-based escalation)", cp.Risk, RiskCritical)
	}
}

func TestAssess_NotGitRepo(t *testing.T) {
	cleanup := setupMockWithErrors(
		map[string]string{},
		map[string]error{
			"git rev-parse --show-toplevel": fmt.Errorf("not a git repo"),
		},
	)
	defer cleanup()

	_, err := Assess(".")
	if err == nil {
		t.Error("Should error on non-git directory")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("Error = %q, want 'not a git repository'", err.Error())
	}
}

func TestAssess_StagedAndDeleted(t *testing.T) {
	commitTime := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
	cleanup := setupMock(map[string]string{
		"git rev-parse --show-toplevel":   "/Users/test/project",
		"git rev-parse --abbrev-ref HEAD": "main",
		"git status --porcelain":          "A  new.go\nD  old.go\nMM both.go",
		"git log -1 --format=%H|%s|%aI":   "eee1234567890|stage test|" + commitTime,
		"git diff --stat --numstat":        "",
	})
	defer cleanup()

	cp, err := Assess(".")
	if err != nil {
		t.Fatalf("Assess failed: %v", err)
	}

	if cp.StagedFiles != 2 { // A and first M of MM
		t.Errorf("StagedFiles = %d, want 2", cp.StagedFiles)
	}
	if cp.DeletedFiles != 1 {
		t.Errorf("DeletedFiles = %d, want 1", cp.DeletedFiles)
	}
	if cp.TotalChanges != 3 {
		t.Errorf("TotalChanges = %d, want 3", cp.TotalChanges)
	}
}

// ── FormatReport Tests ──────────────────────────────────────────────────

func TestFormatReport(t *testing.T) {
	cp := &Checkpoint{
		Branch:            "main",
		RepoRoot:          "/Users/test/project",
		Risk:              RiskHigh,
		TotalChanges:      20,
		StagedFiles:       5,
		ModifiedFiles:     10,
		UntrackedFiles:    3,
		DeletedFiles:      2,
		LinesAdded:        100,
		LinesDeleted:      50,
		LastCommitHash:    "abc1234",
		LastCommitMessage: "last commit msg",
		LastCommitTime:    time.Now().Add(-1 * time.Hour),
		TimeSinceCommit:   1 * time.Hour,
		Warning:           "test warning",
	}

	report := cp.FormatReport()
	if !strings.Contains(report, "Osiris Checkpoint") {
		t.Error("Report should contain 'Osiris Checkpoint'")
	}
	if !strings.Contains(report, "main") {
		t.Error("Report should contain branch name")
	}
	if !strings.Contains(report, "+100 / -50") {
		t.Error("Report should contain diff stats")
	}
	if !strings.Contains(report, "test warning") {
		t.Error("Report should contain warning")
	}
	if !strings.Contains(report, "abc1234") {
		t.Error("Report should contain commit hash")
	}
}

func TestFormatReport_NoDiff(t *testing.T) {
	cp := &Checkpoint{
		Branch:   "main",
		RepoRoot: "/Users/test/project",
		Risk:     RiskNone,
	}

	report := cp.FormatReport()
	if strings.Contains(report, "+0 / -0") {
		t.Error("Report should not show diff stats when zero")
	}
}

// ── formatDuration Tests ────────────────────────────────────────────────

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h30m"},
		{2 * time.Hour, "2h"},
		{25 * time.Hour, "1d"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// ── RiskLevel Constants ─────────────────────────────────────────────────

func TestRiskLevelConstants(t *testing.T) {
	levels := map[RiskLevel]string{
		RiskNone:     "none",
		RiskLow:      "low",
		RiskModerate: "moderate",
		RiskHigh:     "high",
		RiskCritical: "critical",
	}
	for level, expected := range levels {
		if string(level) != expected {
			t.Errorf("RiskLevel %q should be %q", level, expected)
		}
	}
}

// ── minInt Tests ────────────────────────────────────────────────────────

func TestMinInt(t *testing.T) {
	if minInt(3, 5) != 3 {
		t.Error("minInt(3,5) should be 3")
	}
	if minInt(5, 3) != 3 {
		t.Error("minInt(5,3) should be 3")
	}
	if minInt(3, 3) != 3 {
		t.Error("minInt(3,3) should be 3")
	}
}

// ── StatusIcon default branch ───────────────────────────────────────────

func TestStatusIcon_Unknown(t *testing.T) {
	cp := &Checkpoint{Risk: RiskLevel("unknown")}
	if cp.StatusIcon() != "⚪" {
		t.Errorf("Unknown risk should return ⚪, got %q", cp.StatusIcon())
	}
}

// ── Summary with time ───────────────────────────────────────────────────

func TestSummary_WithTime(t *testing.T) {
	cp := &Checkpoint{
		TotalChanges:    5,
		LastCommitTime:  time.Now().Add(-30 * time.Minute),
		TimeSinceCommit: 30 * time.Minute,
	}
	s := cp.Summary()
	if !strings.Contains(s, "5 files changed") {
		t.Errorf("Summary = %q, should contain '5 files changed'", s)
	}
	if !strings.Contains(s, "last commit") {
		t.Errorf("Summary = %q, should contain 'last commit'", s)
	}
}
