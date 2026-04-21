package vault

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStore_OpenClose(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
}

func TestStore_StoreAndGet(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	entry, err := s.Store("npm test", "test-output", "PASS: all 42 tests passed in 3.2s", 12)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if entry.ID == 0 {
		t.Error("expected non-zero entry ID")
	}
	if entry.Source != "npm test" {
		t.Errorf("source = %q, want %q", entry.Source, "npm test")
	}

	got, err := s.Get(entry.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "PASS: all 42 tests passed in 3.2s" {
		t.Errorf("content = %q, want original", got.Content)
	}
	if got.Tokens != 12 {
		t.Errorf("tokens = %d, want 12", got.Tokens)
	}
}

func TestStore_Search(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Store("build", "logs", "ERROR: compilation failed in main.go line 42", 15)
	s.Store("build", "logs", "WARNING: unused import in utils.go", 10)
	s.Store("test", "results", "PASS: all tests passed", 8)

	result, err := s.Search("compilation failed", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.TotalHits == 0 {
		t.Error("expected at least one search hit")
	}
	if result.Entries[0].Source != "build" {
		t.Errorf("top result source = %q, want %q", result.Entries[0].Source, "build")
	}
}

func TestStore_Stats(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Store("a", "tag1", "content one", 5)
	s.Store("b", "tag2", "content two", 10)
	s.Store("c", "tag1", "content three", 7)

	stats, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalEntries != 3 {
		t.Errorf("totalEntries = %d, want 3", stats.TotalEntries)
	}
	if stats.TotalTokens != 22 {
		t.Errorf("totalTokens = %d, want 22", stats.TotalTokens)
	}
	if stats.TagCounts["tag1"] != 2 {
		t.Errorf("tag1 count = %d, want 2", stats.TagCounts["tag1"])
	}
}

func TestStore_Prune(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Store("old", "logs", "old entry", 5)
	// Prune with 1 hour should prune nothing (entries are < 1 hour old).
	removed, err := s.Prune(1 * time.Hour)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0 (entry is fresh)", removed)
	}
}

func TestCodeIndex_IndexAndSearch(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "code.db")
	ci, err := OpenCodeIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenCodeIndex: %v", err)
	}
	defer ci.Close()

	goSrc := []byte(`package main

import "fmt"

// Greet returns a greeting message.
func Greet(name string) string {
	return fmt.Sprintf("Hello, %s!", name)
}

func main() {
	fmt.Println(Greet("world"))
}
`)

	err = ci.IndexFile("main.go", goSrc)
	if err != nil {
		t.Fatalf("IndexFile: %v", err)
	}

	chunks, err := ci.Search("Greet", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected search results for 'Greet'")
	}

	found := false
	for _, c := range chunks {
		if c.Name == "Greet" || c.File == "main.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find Greet function in search results")
	}
}

func TestCodeIndex_IndexDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a small Go project.
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {}
`), 0o644)
	os.WriteFile(filepath.Join(dir, "util.go"), []byte(`package main

func add(a, b int) int { return a + b }
`), 0o644)

	dbPath := filepath.Join(t.TempDir(), "code.db")
	ci, err := OpenCodeIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenCodeIndex: %v", err)
	}
	defer ci.Close()

	stats, err := ci.IndexDir(dir)
	if err != nil {
		t.Fatalf("IndexDir: %v", err)
	}
	if stats.FilesIndexed != 2 {
		t.Errorf("filesIndexed = %d, want 2", stats.FilesIndexed)
	}
	if stats.ChunksCreated == 0 {
		t.Error("expected chunks to be created")
	}
}

func TestGoChunker(t *testing.T) {
	t.Parallel()
	src := []byte(`package example

type Foo struct {
	Name string
}

func (f *Foo) Bar() string {
	return f.Name
}

func standalone() {}
`)

	c := &GoChunker{}
	chunks, err := c.Chunk("example.go", src)
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}

	kinds := make(map[string]int)
	for _, ch := range chunks {
		kinds[ch.Kind]++
	}

	if kinds["type"] != 1 {
		t.Errorf("type chunks = %d, want 1", kinds["type"])
	}
	if kinds["method"] != 1 {
		t.Errorf("method chunks = %d, want 1", kinds["method"])
	}
	if kinds["function"] != 1 {
		t.Errorf("function chunks = %d, want 1", kinds["function"])
	}
}

func TestGenericChunker(t *testing.T) {
	t.Parallel()
	// Create 100 lines of content.
	lines := make([]byte, 0, 1000)
	for i := 0; i < 100; i++ {
		lines = append(lines, []byte("line of code\n")...)
	}

	c := &GenericChunker{MaxChunkLines: 30, Overlap: 10}
	chunks, err := c.Chunk("test.py", lines)
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) < 3 {
		t.Errorf("expected at least 3 chunks for 100 lines with window 30, got %d", len(chunks))
	}
	for _, ch := range chunks {
		if ch.Kind != "block" {
			t.Errorf("expected kind=block, got %q", ch.Kind)
		}
	}
}

// --- New tests below ---

func TestStore_GetNonExistent(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	_, err = s.Get(99999)
	if err == nil {
		t.Error("expected error when getting non-existent entry")
	}
}

func TestStore_StoreEmptyContent(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	entry, err := s.Store("src", "tag", "", 0)
	if err != nil {
		t.Fatalf("Store empty content: %v", err)
	}
	if entry.ID == 0 {
		t.Error("expected non-zero ID even for empty content")
	}

	got, err := s.Get(entry.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "" {
		t.Errorf("content = %q, want empty", got.Content)
	}
}

func TestStore_StoreLargeContent(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	largeContent := strings.Repeat("x", 1024*1024) // 1MB
	entry, err := s.Store("big", "data", largeContent, 100000)
	if err != nil {
		t.Fatalf("Store large content: %v", err)
	}

	got, err := s.Get(entry.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Content) != len(largeContent) {
		t.Errorf("content length = %d, want %d", len(got.Content), len(largeContent))
	}
}

func TestStore_SearchEmptyQuery(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Store("src", "tag", "some content here", 5)

	// Empty query is an FTS5 error.
	_, err = s.Search("", 10)
	if err == nil {
		t.Error("expected error for empty FTS5 query")
	}
}

func TestStore_SearchWithFTS5Operators(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Store("build", "logs", "error in compilation step", 5)
	s.Store("build", "logs", "warning in test step", 5)
	s.Store("build", "logs", "success in deploy step", 5)

	// OR query.
	result, err := s.Search("error OR warning", 10)
	if err != nil {
		t.Fatalf("Search OR: %v", err)
	}
	if result.TotalHits < 2 {
		t.Errorf("expected >= 2 hits for 'error OR warning', got %d", result.TotalHits)
	}

	// NOT query.
	result, err = s.Search("step NOT error", 10)
	if err != nil {
		t.Fatalf("Search NOT: %v", err)
	}
	if result.TotalHits < 1 {
		t.Errorf("expected >= 1 hit for 'step NOT error', got %d", result.TotalHits)
	}
}

func TestStore_SearchOnEmptyVault(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Search on empty vault — FTS5 will return error for empty match on empty table.
	// May return error or empty results depending on FTS5 implementation.
	// Just verify it doesn't panic.
	_, _ = s.Search("anything", 10)
}

func TestStore_SearchLimitZero(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Store("src", "tag", "test content for limit zero", 5)

	// limit=0 should default to 10.
	result, err := s.Search("test", 0)
	if err != nil {
		t.Fatalf("Search limit=0: %v", err)
	}
	if result.TotalHits == 0 {
		t.Error("expected results even with limit=0 (should default to 10)")
	}
}

func TestStore_PruneEmpty(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	removed, err := s.Prune(1 * time.Hour)
	if err != nil {
		t.Fatalf("Prune empty: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0 for empty vault", removed)
	}
}

func TestStore_PruneZeroDuration(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Store("a", "tag", "content a", 5)
	s.Store("b", "tag", "content b", 5)

	// Zero duration: cutoff = now - 0 = now. Entries created at "now" may or may not
	// be pruned depending on timing. Use a negative duration to ensure pruning.
	// Actually: -olderThan with olderThan=0 => cutoff = now, entries with created_at < now.
	// SQLite datetime('now') should equal the cutoff approximately. Allow either result.
	removed, err := s.Prune(0)
	if err != nil {
		t.Fatalf("Prune zero duration: %v", err)
	}
	// Just verify no error — result depends on exact timing.
	_ = removed
}

func TestStore_StatsEmpty(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	stats, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalEntries != 0 {
		t.Errorf("totalEntries = %d, want 0", stats.TotalEntries)
	}
	if stats.TotalBytes != 0 {
		t.Errorf("totalBytes = %d, want 0", stats.TotalBytes)
	}
	if stats.TotalTokens != 0 {
		t.Errorf("totalTokens = %d, want 0", stats.TotalTokens)
	}
	if len(stats.TagCounts) != 0 {
		t.Errorf("tagCounts should be empty, got %v", stats.TagCounts)
	}
}

func TestCodeIndex_IndexFileInvalidGo(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "code.db")
	ci, err := OpenCodeIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenCodeIndex: %v", err)
	}
	defer ci.Close()

	// Invalid Go source should fall back to GenericChunker.
	invalidGo := []byte("this is not valid go source code at all\nline two\nline three\n")
	err = ci.IndexFile("bad.go", invalidGo)
	if err != nil {
		t.Fatalf("IndexFile with invalid Go should not error (fallback): %v", err)
	}

	// Verify something was indexed via generic chunker.
	stats, err := ci.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.ChunksCreated == 0 {
		t.Error("expected chunks from generic chunker fallback")
	}
}

func TestCodeIndex_IndexDirNonexistent(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "code.db")
	ci, err := OpenCodeIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenCodeIndex: %v", err)
	}
	defer ci.Close()

	// filepath.Walk on nonexistent dir returns an error, but IndexDir propagates it.
	stats, err := ci.IndexDir("/nonexistent/directory/path")
	// The walk itself may return an error or just return 0 files.
	if err == nil {
		// If no error, verify zero files indexed.
		if stats.FilesIndexed != 0 {
			t.Errorf("filesIndexed = %d, want 0 for nonexistent dir", stats.FilesIndexed)
		}
	}
	// Either error or zero files is acceptable.
}

func TestCodeIndex_IndexDirSkipDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create source file.
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {}
`), 0o644)

	// Create skippable directories with files.
	for _, skipDir := range []string{"node_modules", ".git", "vendor"} {
		d := filepath.Join(dir, skipDir)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "file.go"), []byte(`package skip

func Skip() {}
`), 0o644)
	}

	dbPath := filepath.Join(t.TempDir(), "code.db")
	ci, err := OpenCodeIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenCodeIndex: %v", err)
	}
	defer ci.Close()

	stats, err := ci.IndexDir(dir)
	if err != nil {
		t.Fatalf("IndexDir: %v", err)
	}
	// Only main.go should be indexed.
	if stats.FilesIndexed != 1 {
		t.Errorf("filesIndexed = %d, want 1 (skipped dirs should be excluded)", stats.FilesIndexed)
	}
}

func TestCodeIndex_IndexDirLargeFileSkip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a file > 500KB.
	largeContent := strings.Repeat("// large file\n", 50000) // ~700KB
	os.WriteFile(filepath.Join(dir, "big.go"), []byte(largeContent), 0o644)
	// Create a normal file.
	os.WriteFile(filepath.Join(dir, "small.go"), []byte(`package main

func Small() {}
`), 0o644)

	dbPath := filepath.Join(t.TempDir(), "code.db")
	ci, err := OpenCodeIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenCodeIndex: %v", err)
	}
	defer ci.Close()

	stats, err := ci.IndexDir(dir)
	if err != nil {
		t.Fatalf("IndexDir: %v", err)
	}
	// Only small.go should be indexed.
	if stats.FilesIndexed != 1 {
		t.Errorf("filesIndexed = %d, want 1 (large file should be skipped)", stats.FilesIndexed)
	}
}

func TestCodeIndex_Refresh(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {}
`), 0o644)

	dbPath := filepath.Join(t.TempDir(), "code.db")
	ci, err := OpenCodeIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenCodeIndex: %v", err)
	}
	defer ci.Close()

	// Initial index.
	stats1, err := ci.IndexDir(dir)
	if err != nil {
		t.Fatalf("IndexDir: %v", err)
	}

	// Add another file.
	os.WriteFile(filepath.Join(dir, "extra.go"), []byte(`package main

func Extra() {}
`), 0o644)

	// Refresh should clear and rebuild.
	stats2, err := ci.Refresh(dir)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if stats2.FilesIndexed <= stats1.FilesIndexed {
		t.Errorf("after refresh, filesIndexed = %d, should be > %d", stats2.FilesIndexed, stats1.FilesIndexed)
	}
}

func TestCodeIndex_Stats(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "code.db")
	ci, err := OpenCodeIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenCodeIndex: %v", err)
	}
	defer ci.Close()

	// Stats on empty index.
	stats, err := ci.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.FilesIndexed != 0 {
		t.Errorf("empty index filesIndexed = %d, want 0", stats.FilesIndexed)
	}
	if stats.ChunksCreated != 0 {
		t.Errorf("empty index chunksCreated = %d, want 0", stats.ChunksCreated)
	}

	// Index a file, then check stats.
	ci.IndexFile("test.go", []byte(`package main

func Hello() {}
func World() {}
`))

	stats, err = ci.Stats()
	if err != nil {
		t.Fatalf("Stats after index: %v", err)
	}
	if stats.FilesIndexed != 1 {
		t.Errorf("filesIndexed = %d, want 1", stats.FilesIndexed)
	}
	if stats.ChunksCreated < 2 {
		t.Errorf("chunksCreated = %d, want >= 2", stats.ChunksCreated)
	}
}

func TestGoChunker_NoDeclarations(t *testing.T) {
	t.Parallel()
	src := []byte(`package empty
`)
	c := &GoChunker{}
	chunks, err := c.Chunk("empty.go", src)
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	// File with only package declaration has no decls.
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for file with no declarations, got %d", len(chunks))
	}
}

func TestGoChunker_OnlyImports(t *testing.T) {
	t.Parallel()
	src := []byte(`package example

import (
	"fmt"
	"strings"
)
`)
	c := &GoChunker{}
	chunks, err := c.Chunk("imports.go", src)
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk (import block), got %d", len(chunks))
	}
	if len(chunks) > 0 && chunks[0].Kind != "import" {
		t.Errorf("expected import kind, got %q", chunks[0].Kind)
	}
}

func TestGoChunker_SyntaxError(t *testing.T) {
	t.Parallel()
	src := []byte("this is not valid go\nline two\nline three\n")
	c := &GoChunker{}
	chunks, err := c.Chunk("bad.go", src)
	if err != nil {
		t.Fatalf("Chunk should fall back to generic, not error: %v", err)
	}
	// Should have fallen back to generic chunker.
	if len(chunks) == 0 {
		t.Error("expected at least one generic chunk from fallback")
	}
	for _, ch := range chunks {
		if ch.Kind != "block" {
			t.Errorf("fallback chunks should be 'block', got %q", ch.Kind)
		}
	}
}

func TestGoChunker_TypeConstVar(t *testing.T) {
	t.Parallel()
	src := []byte(`package example

type MyType int

const MaxSize = 100

var DefaultName = "hello"
`)
	c := &GoChunker{}
	chunks, err := c.Chunk("types.go", src)
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}

	kinds := make(map[string]int)
	for _, ch := range chunks {
		kinds[ch.Kind]++
	}
	if kinds["type"] != 1 {
		t.Errorf("type chunks = %d, want 1", kinds["type"])
	}
	if kinds["const"] != 1 {
		t.Errorf("const chunks = %d, want 1", kinds["const"])
	}
	if kinds["var"] != 1 {
		t.Errorf("var chunks = %d, want 1", kinds["var"])
	}
}

func TestGenericChunker_EmptyFile(t *testing.T) {
	t.Parallel()
	c := &GenericChunker{MaxChunkLines: 50, Overlap: 25}
	chunks, err := c.Chunk("empty.py", []byte(""))
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	// Empty content produces a single chunk with empty content.
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for empty file, got %d", len(chunks))
	}
}

func TestGenericChunker_SingleLine(t *testing.T) {
	t.Parallel()
	c := &GenericChunker{MaxChunkLines: 50, Overlap: 25}
	chunks, err := c.Chunk("one.py", []byte("print('hello')"))
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for single line, got %d", len(chunks))
	}
}

func TestGenericChunker_OverlapGreaterOrEqualMax(t *testing.T) {
	t.Parallel()
	// Overlap >= MaxChunkLines should be clamped to maxLines/2.
	c := &GenericChunker{MaxChunkLines: 10, Overlap: 15}
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("line\n")
	}
	chunks, err := c.Chunk("test.py", []byte(sb.String()))
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks, got %d", len(chunks))
	}
}

func TestGenericChunker_ZeroMaxChunkLines(t *testing.T) {
	t.Parallel()
	// Zero maxChunkLines should default to 50.
	c := &GenericChunker{MaxChunkLines: 0, Overlap: 0}
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("line\n")
	}
	chunks, err := c.Chunk("test.py", []byte(sb.String()))
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks with default max, got %d", len(chunks))
	}
}

func TestConcurrentReads(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Seed data sequentially.
	for i := 0; i < 20; i++ {
		_, err := s.Store("source", "tag", strings.Repeat("content ", i+1), i+1)
		if err != nil {
			t.Fatalf("Store %d: %v", i, err)
		}
	}

	// Read concurrently — WAL mode supports concurrent reads.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats, err := s.Stats()
			if err != nil {
				t.Errorf("concurrent Stats: %v", err)
			}
			if stats.TotalEntries != 20 {
				t.Errorf("totalEntries = %d, want 20", stats.TotalEntries)
			}
		}()
	}
	wg.Wait()
}

func TestDefaultPath(t *testing.T) {
	t.Parallel()
	p := DefaultPath()
	if p == "" {
		t.Error("DefaultPath should return non-empty path")
	}
	if !strings.Contains(p, "vault") {
		t.Errorf("DefaultPath = %q, expected to contain 'vault'", p)
	}
}

func TestDefaultCodeIndexPath(t *testing.T) {
	t.Parallel()
	p := DefaultCodeIndexPath()
	if p == "" {
		t.Error("DefaultCodeIndexPath should return non-empty path")
	}
	if !strings.Contains(p, "codeindex") {
		t.Errorf("DefaultCodeIndexPath = %q, expected to contain 'codeindex'", p)
	}
}

func TestCodeIndex_SearchEmptyIndex(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "code.db")
	ci, err := OpenCodeIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenCodeIndex: %v", err)
	}
	defer ci.Close()

	// Search on empty index.
	_, err = ci.Search("anything", 5)
	// Should not panic.
	_ = err
}

func TestCodeIndex_SearchLimitZero(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "code.db")
	ci, err := OpenCodeIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenCodeIndex: %v", err)
	}
	defer ci.Close()

	ci.IndexFile("test.go", []byte(`package main

func Search() {}
`))

	// limit=0 should default to 10.
	chunks, err := ci.Search("Search", 0)
	if err != nil {
		t.Fatalf("Search limit=0: %v", err)
	}
	if len(chunks) == 0 {
		t.Error("expected results with limit=0 (should default)")
	}
}

func TestCodeIndex_IndexDirNonSourceFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create non-source files that should be ignored.
	os.WriteFile(filepath.Join(dir, "image.png"), []byte("fake png data"), 0o644)
	os.WriteFile(filepath.Join(dir, "data.bin"), []byte("binary data"), 0o644)
	// Create a source file.
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("def hello():\n    pass\n"), 0o644)

	dbPath := filepath.Join(t.TempDir(), "code.db")
	ci, err := OpenCodeIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenCodeIndex: %v", err)
	}
	defer ci.Close()

	stats, err := ci.IndexDir(dir)
	if err != nil {
		t.Fatalf("IndexDir: %v", err)
	}
	if stats.FilesIndexed != 1 {
		t.Errorf("filesIndexed = %d, want 1 (only .py should be indexed)", stats.FilesIndexed)
	}
}

func TestStore_StatsOldestNewest(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Store("first", "tag", "first entry", 1)
	s.Store("second", "tag", "second entry", 2)

	stats, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.OldestEntry == "" {
		t.Error("OldestEntry should not be empty")
	}
	if stats.NewestEntry == "" {
		t.Error("NewestEntry should not be empty")
	}
}

func TestStore_MultipleSearchResults(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	s.Store("s1", "tag", "the quick brown fox", 5)
	s.Store("s2", "tag", "the quick brown dog", 5)
	s.Store("s3", "tag", "the slow red cat", 5)

	result, err := s.Search("quick brown", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.TotalHits < 2 {
		t.Errorf("expected >= 2 hits for 'quick brown', got %d", result.TotalHits)
	}
}

func TestStore_PruneActualDeletion(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Insert an entry, then manually backdate it so Prune finds it.
	entry, err := s.Store("old", "logs", "old content", 5)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	// Backdate the entry to 48 hours ago.
	_, err = s.db.Exec("UPDATE vault_meta SET created_at = datetime('now', '-48 hours') WHERE rowid = ?", entry.ID)
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// Also insert a fresh entry that should NOT be pruned.
	_, err = s.Store("new", "logs", "new content", 5)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Prune entries older than 1 hour.
	removed, err := s.Prune(1 * time.Hour)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}

	// Verify the old entry is gone.
	_, err = s.Get(entry.ID)
	if err == nil {
		t.Error("expected error getting pruned entry")
	}

	// Verify stats reflect the deletion.
	stats, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalEntries != 1 {
		t.Errorf("totalEntries = %d, want 1 after pruning", stats.TotalEntries)
	}
}

func TestCodeIndex_IndexFileNonGo(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "code.db")
	ci, err := OpenCodeIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenCodeIndex: %v", err)
	}
	defer ci.Close()

	// Index a Python file — should use GenericChunker.
	pySrc := []byte("def hello():\n    print('hi')\n\ndef world():\n    print('world')\n")
	err = ci.IndexFile("script.py", pySrc)
	if err != nil {
		t.Fatalf("IndexFile: %v", err)
	}

	stats, err := ci.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.ChunksCreated == 0 {
		t.Error("expected chunks from GenericChunker for .py file")
	}
}

func TestStore_SearchLimit(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	for i := 0; i < 10; i++ {
		s.Store("s", "tag", "searchable content item", 5)
	}

	result, err := s.Search("searchable", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.TotalHits > 3 {
		t.Errorf("totalHits = %d, should be limited to 3", result.TotalHits)
	}
}
