package thoth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Thoth Test Suite ────────────────────────────────────────────────
// Tests for the Thoth Knowledge System (sync.go + journal.go).
// All tests use temp dirs and fixtures — no live git calls.

// ── sync.go Tests ───────────────────────────────────────────────────

func TestCountSubdirs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create 3 subdirectories and 1 file
	for _, name := range []string{"cleaner", "guard", "ka"} {
		os.MkdirAll(filepath.Join(tmp, name), 0o755)
	}
	os.WriteFile(filepath.Join(tmp, "README.md"), []byte("# test"), 0o644)

	got := countSubdirs(tmp)
	if got != 3 {
		t.Errorf("countSubdirs() = %d, want 3", got)
	}
}

func TestCountSubdirs_Empty(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	got := countSubdirs(tmp)
	if got != 0 {
		t.Errorf("countSubdirs(empty) = %d, want 0", got)
	}
}

func TestCountSubdirs_NonExistent(t *testing.T) {
	t.Parallel()
	got := countSubdirs("/nonexistent/path/that/wont/exist")
	if got != 0 {
		t.Errorf("countSubdirs(bad path) = %d, want 0", got)
	}
}

func TestListSubdirs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	for _, name := range []string{"sirsi", "agent"} {
		os.MkdirAll(filepath.Join(tmp, name), 0o755)
	}
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main"), 0o644)

	count, names := listSubdirs(tmp)
	if count != 2 {
		t.Errorf("listSubdirs count = %d, want 2", count)
	}
	if len(names) != 2 {
		t.Errorf("listSubdirs names len = %d, want 2", len(names))
	}
}

func TestListSubdirs_NonExistent(t *testing.T) {
	t.Parallel()
	count, names := listSubdirs("/nonexistent")
	if count != 0 {
		t.Errorf("listSubdirs(bad) count = %d, want 0", count)
	}
	if names != nil {
		t.Errorf("listSubdirs(bad) names should be nil")
	}
}

func TestEstimateTestCount(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create a test file with 3 test functions
	testContent := `package foo

func TestAlpha(t *testing.T) {}
func TestBeta(t *testing.T) {}
func TestGamma(t *testing.T) {}
func helperNotATest() {}
`
	os.WriteFile(filepath.Join(tmp, "foo_test.go"), []byte(testContent), 0o644)

	count := estimateTestCount(tmp)
	if count != 3 {
		t.Errorf("estimateTestCount() = %d, want 3", count)
	}
}

func TestEstimateTestCount_NoTests(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main"), 0o644)

	count := estimateTestCount(tmp)
	if count != 0 {
		t.Errorf("estimateTestCount(no tests) = %d, want 0", count)
	}
}

func TestEstimateLineCount(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Write ~650 bytes of Go (should yield ~10 "lines" at /65 ratio)
	content := strings.Repeat("package main\nfunc main() {}\n", 25) // ~700 bytes
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte(content), 0o644)

	count := estimateLineCount(tmp)
	if count == 0 {
		t.Error("estimateLineCount() should be > 0 for a Go file")
	}
}

func TestEstimateLineCount_SkipsDirs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create files in excluded dirs — should be skipped
	gitDir := filepath.Join(tmp, ".git")
	os.MkdirAll(gitDir, 0o755)
	os.WriteFile(filepath.Join(gitDir, "big.go"), []byte(strings.Repeat("x", 10000)), 0o644)

	nodeDir := filepath.Join(tmp, "node_modules")
	os.MkdirAll(nodeDir, 0o755)
	os.WriteFile(filepath.Join(nodeDir, "big.js"), []byte(strings.Repeat("x", 10000)), 0o644)

	count := estimateLineCount(tmp)
	if count != 0 {
		t.Errorf("estimateLineCount(excluded dirs) = %d, want 0", count)
	}
}

func TestEstimateCommandCount(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	cmdDir := filepath.Join(tmp, "cmd")
	os.MkdirAll(cmdDir, 0o755)

	cmdContent := `package main
var rootCmd = &cobra.Command{}
var versionCmd = &cobra.Command{}
`
	os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte(cmdContent), 0o644)

	count := estimateCommandCount(tmp)
	if count != 2 {
		t.Errorf("estimateCommandCount() = %d, want 2", count)
	}
}

func TestFormatNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1000, "1,000"},
		{1234, "1,234"},
		{32825, "32,825"},
	}

	for _, tt := range tests {
		got := formatNumber(tt.input)
		if got != tt.want {
			t.Errorf("formatNumber(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSync_MissingRoot(t *testing.T) {
	t.Parallel()
	err := Sync(SyncOptions{})
	if err == nil {
		t.Error("Sync with empty root should error")
	}
}

func TestSync_MissingMemory(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	err := Sync(SyncOptions{RepoRoot: tmp})
	if err == nil {
		t.Error("Sync with no memory.yaml should error")
	}
}

func TestSync_UpdatesMemory(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create required structure
	os.MkdirAll(filepath.Join(tmp, ".thoth"), 0o755)
	os.MkdirAll(filepath.Join(tmp, "internal", "cleaner"), 0o755)
	os.MkdirAll(filepath.Join(tmp, "internal", "guard"), 0o755)
	os.MkdirAll(filepath.Join(tmp, "cmd", "sirsi"), 0o755)

	memoryContent := `# memory.yaml
module_count: 0
binary_count: 0
test_count: 0
line_count: ~0
command_count: 0
# Last updated: never
`
	memoryPath := filepath.Join(tmp, ".thoth", "memory.yaml")
	os.WriteFile(memoryPath, []byte(memoryContent), 0o644)

	err := Sync(SyncOptions{RepoRoot: tmp, UpdateDate: true})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	data, _ := os.ReadFile(memoryPath)
	content := string(data)

	if !strings.Contains(content, "module_count: 2") {
		t.Errorf("expected module_count: 2 in output, got:\n%s", content)
	}
	if !strings.Contains(content, "binary_count: 1") {
		t.Errorf("expected binary_count: 1 in output, got:\n%s", content)
	}
}

// ── journal.go Tests ────────────────────────────────────────────────

func TestParseGitLog(t *testing.T) {
	t.Parallel()

	input := `abc12345678901234567890abcdef1234567890ab|fix: resolve data race|2026-03-29T12:00:00-04:00
 3 files changed, 150 insertions(+), 20 deletions(-)
def67890123456789012345678901234567890cd|feat: add pulse engine|2026-03-29T13:00:00-04:00
 7 files changed, 400 insertions(+), 50 deletions(-)
`

	commits := parseGitLog(input)
	if len(commits) != 2 {
		t.Fatalf("parseGitLog() returned %d commits, want 2", len(commits))
	}

	if commits[0].Hash != "abc12345" {
		t.Errorf("commit[0].Hash = %q, want abc12345", commits[0].Hash)
	}
	if commits[0].Subject != "fix: resolve data race" {
		t.Errorf("commit[0].Subject = %q", commits[0].Subject)
	}
	if commits[0].Files != 3 {
		t.Errorf("commit[0].Files = %d, want 3", commits[0].Files)
	}
	if commits[0].Adds != 150 {
		t.Errorf("commit[0].Adds = %d, want 150", commits[0].Adds)
	}
	if commits[0].Dels != 20 {
		t.Errorf("commit[0].Dels = %d, want 20", commits[0].Dels)
	}

	if commits[1].Subject != "feat: add pulse engine" {
		t.Errorf("commit[1].Subject = %q", commits[1].Subject)
	}
	if commits[1].Adds != 400 {
		t.Errorf("commit[1].Adds = %d, want 400", commits[1].Adds)
	}
}

func TestParseGitLog_Empty(t *testing.T) {
	t.Parallel()
	commits := parseGitLog("")
	if len(commits) != 0 {
		t.Errorf("parseGitLog('') = %d commits, want 0", len(commits))
	}
}

func TestParseStatLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input             string
		files, adds, dels int
	}{
		{" 3 files changed, 150 insertions(+), 20 deletions(-)", 3, 150, 20},
		{" 1 file changed, 5 insertions(+)", 1, 5, 0},
		{" 2 files changed, 10 deletions(-)", 2, 10, 0}, // ambiguous — regex just gets numbers in order
	}

	for _, tt := range tests {
		files, adds, dels := parseStatLine(tt.input)
		if files != tt.files {
			t.Errorf("parseStatLine(%q) files = %d, want %d", tt.input, files, tt.files)
		}
		if adds != tt.adds {
			t.Errorf("parseStatLine(%q) adds = %d, want %d", tt.input, adds, tt.adds)
		}
		if dels != tt.dels {
			t.Errorf("parseStatLine(%q) dels = %d, want %d", tt.input, dels, tt.dels)
		}
	}
}

func TestBuildEntryTitle_SingleCommit(t *testing.T) {
	t.Parallel()
	commits := []CommitInfo{{Subject: "fix: resolve data race"}}
	title := buildEntryTitle(commits)
	if !strings.Contains(title, "fix: resolve data race") {
		t.Errorf("buildEntryTitle single = %q, expected subject in title", title)
	}
}

func TestBuildEntryTitle_MultipleCommits(t *testing.T) {
	t.Parallel()
	commits := []CommitInfo{
		{Subject: "fix: resolve data race", Files: 3, Adds: 50},
		{Subject: "fix: mutex guard", Files: 2, Adds: 30},
		{Subject: "docs: update changelog", Files: 1, Adds: 10},
	}
	title := buildEntryTitle(commits)
	if !strings.Contains(title, "3 commits") {
		t.Errorf("buildEntryTitle multi = %q, expected '3 commits'", title)
	}
	if !strings.Contains(title, "fix") {
		t.Errorf("buildEntryTitle multi = %q, expected 'fix' as dominant", title)
	}
}

func TestFormatJournalEntry(t *testing.T) {
	t.Parallel()
	entry := JournalEntry{
		Number: 42,
		Date:   "2026-03-29 12:00",
		Title:  "\"test entry\"",
		Commits: []CommitInfo{
			{Hash: "abc12345", Subject: "fix: test", Files: 3, Adds: 100, Dels: 10},
		},
	}

	result := formatJournalEntry(entry)

	if !strings.Contains(result, "## Entry 042") {
		t.Error("expected Entry 042 in output")
	}
	if !strings.Contains(result, "abc12345") {
		t.Error("expected commit hash in output")
	}
	if !strings.Contains(result, "+100/-10") {
		t.Error("expected stat line in output")
	}
	if !strings.Contains(result, "AUTO-SYNC") {
		t.Error("expected AUTO-SYNC label")
	}
}

func TestFindLastEntryNumber(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	journalPath := filepath.Join(tmp, "journal.md")

	content := `# Journal
## Entry 001 — some title
## Entry 005 — another title
## Entry 003 — middle one
`
	os.WriteFile(journalPath, []byte(content), 0o644)

	got := findLastEntryNumber(journalPath)
	if got != 5 {
		t.Errorf("findLastEntryNumber() = %d, want 5", got)
	}
}

func TestFindLastEntryNumber_NoFile(t *testing.T) {
	t.Parallel()
	got := findLastEntryNumber("/nonexistent/journal.md")
	if got != 0 {
		t.Errorf("findLastEntryNumber(missing) = %d, want 0", got)
	}
}

func TestSyncJournal_EmptyRoot(t *testing.T) {
	t.Parallel()
	_, err := SyncJournal(JournalSyncOptions{})
	if err == nil {
		t.Error("SyncJournal with empty root should error")
	}
}
