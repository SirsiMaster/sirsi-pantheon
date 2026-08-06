package neith

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Loom Test Suite ────────────────────────────────────────────────
// Tests for Neith's Loom — scope loading, canon reading, prompt weaving.

// ── LoadScopes ─────────────────────────────────────────────────────

func TestLoadScopes_ValidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "test-scope.yaml"), `
name: test-scope
display_name: "Test Scope"
repo_path: "/tmp/test-repo"
deadline: "2026-04-15"
priority: "P0"
max_turns: 50
scope_of_work: |
  1. Do something
  2. Do something else
`)

	loom := NewLoom(dir)
	scopes, err := loom.LoadScopes()
	if err != nil {
		t.Fatalf("LoadScopes() error: %v", err)
	}
	if len(scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(scopes))
	}

	s := scopes[0]
	if s.Name != "test-scope" {
		t.Errorf("Name = %q, want %q", s.Name, "test-scope")
	}
	if s.DisplayName != "Test Scope" {
		t.Errorf("DisplayName = %q, want %q", s.DisplayName, "Test Scope")
	}
	if s.RepoPath != "/tmp/test-repo" {
		t.Errorf("RepoPath = %q", s.RepoPath)
	}
	if s.Priority != "P0" {
		t.Errorf("Priority = %q", s.Priority)
	}
	if s.MaxTurns != 50 {
		t.Errorf("MaxTurns = %d", s.MaxTurns)
	}
	if !strings.Contains(s.ScopeOfWork, "Do something") {
		t.Errorf("ScopeOfWork missing expected content")
	}
}

func TestLoadScopes_MultipleFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scope-a.yaml"), "name: alpha\ndisplay_name: Alpha")
	writeFile(t, filepath.Join(dir, "scope-b.yaml"), "name: beta\ndisplay_name: Beta")

	loom := NewLoom(dir)
	scopes, err := loom.LoadScopes()
	if err != nil {
		t.Fatalf("LoadScopes() error: %v", err)
	}
	if len(scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %d", len(scopes))
	}
}

func TestLoadScopes_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	loom := NewLoom(dir)
	_, err := loom.LoadScopes()
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
	if !strings.Contains(err.Error(), "no scope configs found") {
		t.Errorf("error = %q, want 'no scope configs found'", err.Error())
	}
}

func TestLoadScopes_InvalidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bad.yaml"), "{{not valid yaml")

	loom := NewLoom(dir)
	_, err := loom.LoadScopes()
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadScopes_YMLExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scope.yml"), "name: yml-scope\ndisplay_name: YML")

	loom := NewLoom(dir)
	scopes, err := loom.LoadScopes()
	if err != nil {
		t.Fatalf("LoadScopes() error: %v", err)
	}
	if len(scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(scopes))
	}
	if scopes[0].Name != "yml-scope" {
		t.Errorf("Name = %q", scopes[0].Name)
	}
}

func TestLoadScopes_DynamicScope(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "dynamic.yaml"), `
name: dynamic
display_name: "Dynamic Scope"
repo_path: "/tmp/repo"
priority: "P2"
max_turns: 50
scope_of_work: ""
`)

	loom := NewLoom(dir)
	scopes, err := loom.LoadScopes()
	if err != nil {
		t.Fatalf("LoadScopes() error: %v", err)
	}
	if strings.TrimSpace(scopes[0].ScopeOfWork) != "" {
		t.Errorf("dynamic scope should have empty scope_of_work, got %q", scopes[0].ScopeOfWork)
	}
}

// ── LoadCanon ──────────────────────────────────────────────────────

func TestLoadCanon_FullRepo(t *testing.T) {
	t.Parallel()

	repo := setupTestRepo(t)
	loom := NewLoom("")

	ctx, err := loom.LoadCanon(repo)
	if err != nil {
		t.Fatalf("LoadCanon() error: %v", err)
	}

	if ctx.ClaudeMD != "# Rules\nTest rules" {
		t.Errorf("ClaudeMD = %q", ctx.ClaudeMD)
	}
	if ctx.ThothMemory != "project: test\nversion: 1.0" {
		t.Errorf("ThothMemory = %q", ctx.ThothMemory)
	}
	if ctx.ThothJournal != "## Entry 001\nDid stuff" {
		t.Errorf("ThothJournal = %q", ctx.ThothJournal)
	}
	if ctx.ContinuationPrompt != "# Next steps\nBuild things" {
		t.Errorf("ContinuationPrompt = %q", ctx.ContinuationPrompt)
	}
	if ctx.Version != "0.9.0" {
		t.Errorf("Version = %q", ctx.Version)
	}
	if ctx.Changelog != "# Changelog\n## 0.9.0\n- Added stuff" {
		t.Errorf("Changelog = %q", ctx.Changelog)
	}
	if len(ctx.ADRs) != 2 {
		t.Errorf("ADRs count = %d, want 2", len(ctx.ADRs))
	}
	if len(ctx.PlanningDocs) < 1 {
		t.Errorf("PlanningDocs count = %d, want >= 1", len(ctx.PlanningDocs))
	}
}

func TestLoadCanon_GeminiMDFallback(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "GEMINI.md"), "# Gemini rules")

	loom := NewLoom("")
	ctx, err := loom.LoadCanon(repo)
	if err != nil {
		t.Fatalf("LoadCanon() error: %v", err)
	}
	if ctx.ClaudeMD != "# Gemini rules" {
		t.Errorf("should fall back to GEMINI.md, got %q", ctx.ClaudeMD)
	}
}

func TestLoadCanon_EmptyRepo(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	loom := NewLoom("")

	ctx, err := loom.LoadCanon(repo)
	if err != nil {
		t.Fatalf("LoadCanon() error: %v", err)
	}
	if ctx.ClaudeMD != "" {
		t.Error("expected empty ClaudeMD for empty repo")
	}
	if ctx.Version != "" {
		t.Error("expected empty Version for empty repo")
	}
}

func TestLoadCanon_DeduplicatesPlanningDocs(t *testing.T) {
	t.Parallel()

	// A file matching multiple patterns (e.g. ARCHITECTURE_DESIGN.md)
	// should only appear once.
	repo := t.TempDir()
	docsDir := filepath.Join(repo, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// This matches both *ARCHITECTURE* and *DESIGN* patterns
	writeFile(t, filepath.Join(docsDir, "ARCHITECTURE_DESIGN.md"), "architecture content")

	loom := NewLoom("")
	ctx, err := loom.LoadCanon(repo)
	if err != nil {
		t.Fatalf("LoadCanon() error: %v", err)
	}

	count := 0
	for _, doc := range ctx.PlanningDocs {
		if doc.Name == "ARCHITECTURE_DESIGN.md" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("ARCHITECTURE_DESIGN.md appeared %d times, want 1", count)
	}
}

// ── WeaveScope ─────────────────────────────────────────────────────

func TestWeaveScope_StaticScope(t *testing.T) {
	t.Parallel()

	repo := setupTestRepo(t)
	loom := NewLoom("")

	scope := ScopeConfig{
		Name:        "test",
		DisplayName: "Test Scope",
		RepoPath:    repo,
		Deadline:    "2026-04-15",
		Priority:    "P0",
		ScopeOfWork: "1. Build the thing\n2. Test the thing",
	}

	prompt, err := loom.WeaveScope(scope)
	if err != nil {
		t.Fatalf("WeaveScope() error: %v", err)
	}

	// Verify Ra Autonomy Directive is present
	if !strings.Contains(prompt, "Ra Autonomy Directive") {
		t.Error("prompt missing Ra Autonomy Directive")
	}
	// Rule 17 is Sprint Planning; Rule 14 is Do No Harm. The §1 list ordinal for
	// Sprint Planning happens to BE 14, which is how the wrong number got written
	// here and into four canon files. Assert the correct rule AND that the old
	// wrong one never comes back — a Ra agent told to "Override Rule 14" is being
	// told it may break working processes.
	if !strings.Contains(prompt, "Override Rule 17 (Sprint Planning is Mandatory)") {
		t.Error("prompt missing Rule 17 (Sprint Planning) override")
	}
	if strings.Contains(prompt, "Override Rule 14") {
		t.Error("prompt overrides Rule 14 (Do No Harm) — must be Rule 17 (Sprint Planning)")
	}

	// Verify scope of work is present
	if !strings.Contains(prompt, "Build the thing") {
		t.Error("prompt missing scope of work content")
	}

	// Verify canon context sections
	if !strings.Contains(prompt, "Test rules") {
		t.Error("prompt missing CLAUDE.md content")
	}
	if !strings.Contains(prompt, "project: test") {
		t.Error("prompt missing Thoth memory")
	}
	if !strings.Contains(prompt, "Next steps") {
		t.Error("prompt missing continuation prompt")
	}
	if !strings.Contains(prompt, "0.9.0") {
		t.Error("prompt missing version")
	}
}

func TestWeaveScope_DynamicScope(t *testing.T) {
	t.Parallel()

	repo := setupTestRepo(t)
	loom := NewLoom("")

	scope := ScopeConfig{
		Name:        "dynamic",
		DisplayName: "Dynamic Scope",
		RepoPath:    repo,
		Priority:    "P2",
		ScopeOfWork: "", // empty = dynamic
	}

	prompt, err := loom.WeaveScope(scope)
	if err != nil {
		t.Fatalf("WeaveScope() error: %v", err)
	}

	// Dynamic scope should include instructions
	if !strings.Contains(prompt, "Dynamic — Derived from Canon") {
		t.Error("prompt missing dynamic scope header")
	}
	if !strings.Contains(prompt, "Read the Continuation Prompt") {
		t.Error("prompt missing dynamic instructions")
	}
	if !strings.Contains(prompt, "git log --oneline") {
		t.Error("prompt missing git log instruction")
	}
}

func TestWeaveScope_SectionOrder(t *testing.T) {
	t.Parallel()

	repo := setupTestRepo(t)
	loom := NewLoom("")

	scope := ScopeConfig{
		Name:        "order-test",
		DisplayName: "Order Test",
		RepoPath:    repo,
		Priority:    "P1",
		ScopeOfWork: "static scope content here",
	}

	prompt, err := loom.WeaveScope(scope)
	if err != nil {
		t.Fatalf("WeaveScope() error: %v", err)
	}

	// Verify section ordering: directive < scope < continuation < planning < ADRs < thoth < identity < changelog
	directiveIdx := strings.Index(prompt, "Ra Autonomy Directive")
	scopeIdx := strings.Index(prompt, "static scope content here")
	continuationIdx := strings.Index(prompt, "Continuation Prompt")
	planningIdx := strings.Index(prompt, "Planning Documents")
	adrIdx := strings.Index(prompt, "Architecture Decision Records")
	thothIdx := strings.Index(prompt, "Project State (Thoth Memory)")
	identityIdx := strings.Index(prompt, "Project Identity (CLAUDE.md)")
	changelogIdx := strings.Index(prompt, "## Changelog")

	positions := []struct {
		name string
		idx  int
	}{
		{"directive", directiveIdx},
		{"scope", scopeIdx},
		{"continuation", continuationIdx},
		{"planning", planningIdx},
		{"ADRs", adrIdx},
		{"thoth", thothIdx},
		{"identity", identityIdx},
		{"changelog", changelogIdx},
	}

	for i := 1; i < len(positions); i++ {
		if positions[i].idx < 0 {
			continue // section not present
		}
		if positions[i-1].idx < 0 {
			continue
		}
		if positions[i].idx < positions[i-1].idx {
			t.Errorf("section %q (pos %d) appears before %q (pos %d)",
				positions[i].name, positions[i].idx,
				positions[i-1].name, positions[i-1].idx)
		}
	}
}

func TestWeaveScope_InvalidRepoPath(t *testing.T) {
	t.Parallel()

	loom := NewLoom("")
	scope := ScopeConfig{
		Name:     "bad",
		RepoPath: "/nonexistent/path/that/does/not/exist",
	}

	// Should not error — LoadCanon gracefully handles missing files
	prompt, err := loom.WeaveScope(scope)
	if err != nil {
		t.Fatalf("WeaveScope() error: %v", err)
	}
	// Should still have the directive
	if !strings.Contains(prompt, "Ra Autonomy Directive") {
		t.Error("prompt missing directive even with invalid repo")
	}
}

// ── WritePrompt ────────────────────────────────────────────────────

func TestWritePrompt(t *testing.T) {
	t.Parallel()

	loom := NewLoom("")
	path, err := loom.WritePrompt("test-scope", "# Test Prompt\nHello world")
	if err != nil {
		t.Fatalf("WritePrompt() error: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read prompt file: %v", err)
	}
	if string(data) != "# Test Prompt\nHello world" {
		t.Errorf("file content = %q", string(data))
	}
	if !strings.HasSuffix(path, "test-scope-prompt.md") {
		t.Errorf("path = %q, want suffix test-scope-prompt.md", path)
	}
}

// ── EvaluateDrift ──────────────────────────────────────────────────

func TestEvaluateDrift_NoDrift(t *testing.T) {
	t.Parallel()

	loom := NewLoom("")
	scope := ScopeConfig{
		Name:        "test",
		ScopeOfWork: "internal/brain inference.go CoreML",
	}

	diff := `--- a/internal/brain/inference.go
+++ b/internal/brain/inference.go
@@ -10,3 +10,5 @@
+func newFunction() {}
`

	report, err := loom.EvaluateDrift(scope, diff)
	if err != nil {
		t.Fatalf("EvaluateDrift() error: %v", err)
	}
	if report.DriftFound {
		t.Errorf("expected no drift, got findings: %v", report.Findings)
	}
}

func TestEvaluateDrift_WithDrift(t *testing.T) {
	t.Parallel()

	loom := NewLoom("")
	scope := ScopeConfig{
		Name:        "test",
		ScopeOfWork: "internal/brain inference.go",
	}

	diff := `--- a/internal/brain/inference.go
+++ b/internal/brain/inference.go
@@ -1 +1 @@
-old
+new
--- a/cmd/pantheon/unrelated.go
+++ b/cmd/pantheon/unrelated.go
@@ -1 +1 @@
-old
+new
`

	report, err := loom.EvaluateDrift(scope, diff)
	if err != nil {
		t.Fatalf("EvaluateDrift() error: %v", err)
	}
	if !report.DriftFound {
		t.Error("expected drift for out-of-scope file")
	}
	if len(report.Findings) == 0 {
		t.Error("expected findings")
	}
}

func TestEvaluateDrift_EmptyDiff(t *testing.T) {
	t.Parallel()

	loom := NewLoom("")
	scope := ScopeConfig{Name: "test"}

	report, err := loom.EvaluateDrift(scope, "")
	if err != nil {
		t.Fatalf("EvaluateDrift() error: %v", err)
	}
	if report.DriftFound {
		t.Error("expected no drift for empty diff")
	}
}

func TestEvaluateDrift_NewGoDependency(t *testing.T) {
	t.Parallel()

	loom := NewLoom("")
	scope := ScopeConfig{
		Name:        "test",
		ScopeOfWork: "internal/ go",
	}

	diff := `--- a/go.mod
+++ b/go.mod
@@ -5,3 +5,4 @@
+require github.com/newdep v1.0.0
`

	report, err := loom.EvaluateDrift(scope, diff)
	if err != nil {
		t.Fatalf("EvaluateDrift() error: %v", err)
	}

	foundDepWarning := false
	for _, f := range report.Findings {
		if strings.Contains(f, "Go dependency") {
			foundDepWarning = true
		}
	}
	if !foundDepWarning {
		t.Errorf("expected Go dependency warning, findings: %v", report.Findings)
	}
}

func TestEvaluateDrift_EmptyScopeKeywords(t *testing.T) {
	t.Parallel()

	loom := NewLoom("")
	scope := ScopeConfig{
		Name:        "test",
		ScopeOfWork: "", // dynamic scope = no keywords
	}

	diff := `+++ b/random/file.go
--- a/random/file.go
`

	report, err := loom.EvaluateDrift(scope, diff)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// With no keywords, fileMatchesScopeKeywords returns true (no evaluation possible)
	if report.DriftFound {
		t.Error("expected no drift when scope has no keywords")
	}
}

// ── Helper functions ───────────────────────────────────────────────

func TestExpandHome(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"~/test", filepath.Join(home, "test")},
		{"~/a/b/c", filepath.Join(home, "a/b/c")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		got := expandHome(tt.input)
		if got != tt.expected {
			t.Errorf("expandHome(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDeduplicateDocs(t *testing.T) {
	t.Parallel()

	docs := []namedDoc{
		{Name: "A.md", Content: "content A"},
		{Name: "B.md", Content: "content B"},
		{Name: "A.md", Content: "duplicate A"},
		{Name: "C.md", Content: "content C"},
		{Name: "B.md", Content: "duplicate B"},
	}

	result := deduplicateDocs(docs)
	if len(result) != 3 {
		t.Fatalf("expected 3 unique docs, got %d", len(result))
	}

	// First occurrence should win
	if result[0].Content != "content A" {
		t.Errorf("first A should be 'content A', got %q", result[0].Content)
	}
}

func TestDeduplicateDocs_Empty(t *testing.T) {
	t.Parallel()
	result := deduplicateDocs(nil)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestLastNJournalEntries(t *testing.T) {
	t.Parallel()

	journal := "## Entry 1\nContent 1\n---\n## Entry 2\nContent 2\n---\n## Entry 3\nContent 3"

	result := lastNJournalEntries(journal, 2)
	if !strings.Contains(result, "Entry 2") {
		t.Error("should contain Entry 2")
	}
	if !strings.Contains(result, "Entry 3") {
		t.Error("should contain Entry 3")
	}
}

func TestLastNJournalEntries_AllEntries(t *testing.T) {
	t.Parallel()
	journal := "## Entry 1\nContent"
	result := lastNJournalEntries(journal, 5)
	if result != journal {
		t.Errorf("when n > entries, should return full content")
	}
}

func TestLastNJournalEntries_HeaderSplit(t *testing.T) {
	t.Parallel()
	journal := "## Entry 1\nContent 1\n## Entry 2\nContent 2\n## Entry 3\nContent 3\n"
	result := lastNJournalEntries(journal, 2)
	if !strings.Contains(result, "Entry 2") || !strings.Contains(result, "Entry 3") {
		t.Errorf("should contain last 2 entries, got %q", result)
	}
}

func TestReadFirstNLines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	writeFile(t, path, "line1\nline2\nline3\nline4\nline5")

	result := readFirstNLines(path, 3)
	if result != "line1\nline2\nline3" {
		t.Errorf("readFirstNLines(3) = %q", result)
	}
}

func TestReadFirstNLines_FileNotFound(t *testing.T) {
	t.Parallel()
	result := readFirstNLines("/nonexistent/file.md", 3)
	if result != "" {
		t.Errorf("expected empty for nonexistent file, got %q", result)
	}
}

func TestReadFirstParagraph(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	writeFile(t, path, "# Title\n\nFirst paragraph here.\nWith multiple lines.\n\nSecond paragraph.")

	result := readFirstParagraph(path)
	if !strings.Contains(result, "First paragraph") {
		t.Errorf("readFirstParagraph = %q, want first real paragraph", result)
	}
}

func TestReadFirstParagraph_HeadingOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	writeFile(t, path, "# Title\n\nContent paragraph.")

	result := readFirstParagraph(path)
	if !strings.Contains(result, "Content paragraph") {
		t.Errorf("should skip heading-only paragraph, got %q", result)
	}
}

func TestFirstNChangelogSections(t *testing.T) {
	t.Parallel()

	// Note: "# Changelog" contains "v" in "Changelog" so it counts as a section header.
	// Asking for 3 sections gets the header + first 2 version entries.
	changelog := `# Changelog

## [0.9.0] - 2026-04-01
- Added feature A
- Fixed bug B

## [0.8.0] - 2026-03-31
- Added feature C

## [0.7.0] - 2026-03-27
- Initial release
`

	result := firstNChangelogSections(changelog, 3)
	if !strings.Contains(result, "0.9.0") {
		t.Error("should contain first version section")
	}
	if !strings.Contains(result, "0.8.0") {
		t.Error("should contain second version section")
	}
	if strings.Contains(result, "0.7.0") {
		t.Error("should NOT contain third version section")
	}
}

func TestExtractDiffFiles(t *testing.T) {
	t.Parallel()

	diff := `diff --git a/internal/brain/inference.go b/internal/brain/inference.go
--- a/internal/brain/inference.go
+++ b/internal/brain/inference.go
@@ -1 +1 @@
diff --git a/cmd/pantheon/main.go b/cmd/pantheon/main.go
--- a/cmd/pantheon/main.go
+++ b/cmd/pantheon/main.go
@@ -1 +1 @@
`

	files := extractDiffFiles(diff)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}

	found := map[string]bool{}
	for _, f := range files {
		found[f] = true
	}
	if !found["internal/brain/inference.go"] {
		t.Error("missing internal/brain/inference.go")
	}
	if !found["cmd/pantheon/main.go"] {
		t.Error("missing cmd/pantheon/main.go")
	}
}

func TestExtractDiffFiles_NoDuplicates(t *testing.T) {
	t.Parallel()

	diff := `--- a/file.go
+++ b/file.go
--- a/file.go
+++ b/file.go`

	files := extractDiffFiles(diff)
	if len(files) != 1 {
		t.Errorf("expected 1 unique file, got %d", len(files))
	}
}

func TestExtractScopeKeywords(t *testing.T) {
	t.Parallel()

	scope := "internal/brain/inference.go — extend CoreML bridge. Add docs and tests."
	keywords := extractScopeKeywords(scope)

	found := map[string]bool{}
	for _, kw := range keywords {
		found[kw] = true
	}

	if !found["internal/brain/inference.go"] {
		t.Error("missing path keyword internal/brain/inference.go")
	}
	if !found["docs"] {
		t.Error("missing tech keyword 'docs'")
	}
	if !found["tests"] {
		t.Error("missing tech keyword 'tests'")
	}
}

func TestExtractScopeKeywords_EmptyScope(t *testing.T) {
	t.Parallel()
	keywords := extractScopeKeywords("")
	if len(keywords) != 0 {
		t.Errorf("expected 0 keywords for empty scope, got %d", len(keywords))
	}
}

func TestFileMatchesScopeKeywords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		file     string
		keywords []string
		expected bool
	}{
		{"internal/brain/inference.go", []string{"internal/brain"}, true},
		{"cmd/pantheon/main.go", []string{"internal/brain"}, false},
		{"any/file.go", nil, true},        // no keywords = no evaluation = true
		{"any/file.go", []string{}, true}, // empty keywords = true
		{"docs/README.md", []string{"docs"}, true},
	}

	for _, tt := range tests {
		got := fileMatchesScopeKeywords(tt.file, tt.keywords)
		if got != tt.expected {
			t.Errorf("fileMatchesScopeKeywords(%q, %v) = %v, want %v",
				tt.file, tt.keywords, got, tt.expected)
		}
	}
}

// ── Test Fixtures ──────────────────────────────────────────────────

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func setupTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()

	// CLAUDE.md
	writeFile(t, filepath.Join(repo, "CLAUDE.md"), "# Rules\nTest rules")

	// .thoth/
	writeFile(t, filepath.Join(repo, ".thoth", "memory.yaml"), "project: test\nversion: 1.0")
	writeFile(t, filepath.Join(repo, ".thoth", "journal.md"), "## Entry 001\nDid stuff")

	// docs/
	writeFile(t, filepath.Join(repo, "docs", "CONTINUATION-PROMPT.md"), "# Next steps\nBuild things")
	writeFile(t, filepath.Join(repo, "docs", "ADR-001-FOUNDING.md"), "# ADR-001\nFounding architecture")
	writeFile(t, filepath.Join(repo, "docs", "ADR-002-TESTING.md"), "# ADR-002\nTesting strategy")
	writeFile(t, filepath.Join(repo, "docs", "PANTHEON_ROADMAP.md"), "# Roadmap\nPhase 1: Build")
	writeFile(t, filepath.Join(repo, "docs", "QA_PLAN.md"), "# QA Plan\nTest everything")

	// VERSION and CHANGELOG
	writeFile(t, filepath.Join(repo, "VERSION"), "0.9.0\n")
	writeFile(t, filepath.Join(repo, "CHANGELOG.md"), "# Changelog\n## 0.9.0\n- Added stuff")

	return repo
}
