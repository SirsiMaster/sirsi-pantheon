package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

// ─────────────────────────────────────────────────
// Load
// ─────────────────────────────────────────────────

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".anubisignore")
	content := "# comment\nnode_modules\n*.log\n!important.log\n\n# another comment\n.venv\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	list, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(list.patterns) != 4 {
		t.Errorf("expected 4 patterns, got %d", len(list.patterns))
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".anubisignore")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	list, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(list.patterns) != 0 {
		t.Errorf("expected 0 patterns for empty file, got %d", len(list.patterns))
	}
}

func TestLoad_CommentsOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".anubisignore")
	content := "# This is a comment\n# Another comment\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	list, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(list.patterns) != 0 {
		t.Errorf("expected 0 patterns for comments-only file, got %d", len(list.patterns))
	}
}

func TestLoad_NonExistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/.anubisignore")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestLoad_NegationParsing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".anubisignore")
	content := "*.log\n!important.log\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	list, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(list.patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(list.patterns))
	}

	// First pattern: *.log, not negated
	if list.patterns[0].glob != "*.log" {
		t.Errorf("pattern[0].glob = %q, want %q", list.patterns[0].glob, "*.log")
	}
	if list.patterns[0].negate {
		t.Error("pattern[0] should not be negated")
	}

	// Second pattern: important.log, negated (! stripped)
	if list.patterns[1].glob != "important.log" {
		t.Errorf("pattern[1].glob = %q, want %q", list.patterns[1].glob, "important.log")
	}
	if !list.patterns[1].negate {
		t.Error("pattern[1] should be negated")
	}
}

func TestLoad_LeadingTrailingWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".anubisignore")
	content := "  node_modules  \n  *.log  \n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	list, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(list.patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(list.patterns))
	}
	if list.patterns[0].glob != "node_modules" {
		t.Errorf("pattern[0].glob = %q, want %q", list.patterns[0].glob, "node_modules")
	}
}

// ─────────────────────────────────────────────────
// ShouldIgnore
// ─────────────────────────────────────────────────

func TestShouldIgnore_EmptyList(t *testing.T) {
	list := &IgnoreList{}
	if list.ShouldIgnore("/any/path") {
		t.Error("empty IgnoreList should not ignore anything")
	}
}

func TestShouldIgnore_GlobMatch(t *testing.T) {
	list := &IgnoreList{
		patterns: []pattern{
			{glob: "*.log", negate: false},
		},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"/var/log/app.log", true},
		{"/home/user/debug.log", true},
		{"/home/user/debug.txt", false},
		{"error.log", true},
	}

	for _, tt := range tests {
		got := list.ShouldIgnore(tt.path)
		if got != tt.want {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestShouldIgnore_DirectoryName(t *testing.T) {
	list := &IgnoreList{
		patterns: []pattern{
			{glob: "node_modules", negate: false},
		},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"/project/node_modules", true},
		{"/project/node_modules/express/index.js", true}, // contains "node_modules"
		{"/project/src/app.js", false},
	}

	for _, tt := range tests {
		got := list.ShouldIgnore(tt.path)
		if got != tt.want {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestShouldIgnore_Negation(t *testing.T) {
	list := &IgnoreList{
		patterns: []pattern{
			{glob: "*.log", negate: false},
			{glob: "important.log", negate: true},
		},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"debug.log", true},           // matches *.log
		{"important.log", false},       // matched *.log, then un-ignored by negation
		{"app.txt", false},             // no match
	}

	for _, tt := range tests {
		got := list.ShouldIgnore(tt.path)
		if got != tt.want {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestShouldIgnore_NegationOrderMatters(t *testing.T) {
	// Negation first, then re-ignore — last matching pattern wins
	list := &IgnoreList{
		patterns: []pattern{
			{glob: "important.log", negate: true}, // un-ignore first (has no effect if not yet ignored)
			{glob: "*.log", negate: false},          // ignore all .log
		},
	}

	// *.log wins because it comes after the negation
	if !list.ShouldIgnore("important.log") {
		t.Error("expected important.log to be ignored (*.log pattern comes after negation)")
	}
}

func TestShouldIgnore_MultiplePatterns(t *testing.T) {
	list := &IgnoreList{
		patterns: []pattern{
			{glob: "*.log", negate: false},
			{glob: "*.tmp", negate: false},
			{glob: ".venv", negate: false},
		},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"app.log", true},
		{"session.tmp", true},
		{"/project/.venv", true},
		{"app.py", false},
	}

	for _, tt := range tests {
		got := list.ShouldIgnore(tt.path)
		if got != tt.want {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestShouldIgnore_SubstringContains(t *testing.T) {
	// matchPattern has a fallback: strings.Contains(path, glob)
	list := &IgnoreList{
		patterns: []pattern{
			{glob: ".cache", negate: false},
		},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"/home/user/.cache/app", true},   // contains ".cache"
		{"/home/user/.config/app", false},
	}

	for _, tt := range tests {
		got := list.ShouldIgnore(tt.path)
		if got != tt.want {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// ─────────────────────────────────────────────────
// LoadFromDir
// ─────────────────────────────────────────────────

func TestLoadFromDir_ProjectDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".anubisignore")
	content := "*.log\nnode_modules\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	list := LoadFromDir(dir)
	if len(list.patterns) != 2 {
		t.Errorf("expected 2 patterns from project dir, got %d", len(list.patterns))
	}
}

func TestLoadFromDir_NoFile(t *testing.T) {
	dir := t.TempDir()

	list := LoadFromDir(dir)
	// Should return empty list, not nil
	if list == nil {
		t.Fatal("LoadFromDir should return non-nil IgnoreList even when no file found")
	}
	if len(list.patterns) != 0 {
		t.Errorf("expected 0 patterns when no .anubisignore found, got %d", len(list.patterns))
	}
}

// ─────────────────────────────────────────────────
// matchPattern
// ─────────────────────────────────────────────────

func TestMatchPattern_DirectPathGlob(t *testing.T) {
	// filepath.Match against full path
	if !matchPattern("*.log", "error.log") {
		t.Error("expected *.log to match error.log")
	}
	if matchPattern("*.log", "error.txt") {
		t.Error("expected *.log to not match error.txt")
	}
}

func TestMatchPattern_BasenameMatch(t *testing.T) {
	// Should match against basename of path
	if !matchPattern("*.log", "/var/log/error.log") {
		t.Error("expected *.log to match /var/log/error.log via basename")
	}
}

func TestMatchPattern_SubstringContains(t *testing.T) {
	// Fallback: path contains the glob as substring
	if !matchPattern("node_modules", "/project/node_modules/express/index.js") {
		t.Error("expected node_modules to match via substring contains")
	}
	if matchPattern("node_modules", "/project/src/index.js") {
		t.Error("expected node_modules to not match /project/src/index.js")
	}
}

// ─────────────────────────────────────────────────
// DefaultIgnoreContent
// ─────────────────────────────────────────────────

func TestDefaultIgnoreContent_NotEmpty(t *testing.T) {
	content := DefaultIgnoreContent()
	if content == "" {
		t.Error("DefaultIgnoreContent() should return non-empty template")
	}
	// Should be a valid .anubisignore (comments + examples)
	if content[0] != '#' {
		t.Error("DefaultIgnoreContent() should start with a comment")
	}
}

// ─────────────────────────────────────────────────
// Integration: end-to-end Load + ShouldIgnore
// ─────────────────────────────────────────────────

func TestIntegration_LoadAndIgnore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".anubisignore")

	content := `# Anubis ignore rules
node_modules
*.log
!important.log
.venv
__pycache__
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	list, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	tests := []struct {
		path string
		want bool
	}{
		{"/project/node_modules", true},
		{"/project/node_modules/express/package.json", true},
		{"/project/app.log", true},
		{"/project/important.log", false}, // negated
		{"/project/.venv/bin/python", true},
		{"/project/__pycache__/module.pyc", true},
		{"/project/src/main.py", false},
	}

	for _, tt := range tests {
		got := list.ShouldIgnore(tt.path)
		if got != tt.want {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
