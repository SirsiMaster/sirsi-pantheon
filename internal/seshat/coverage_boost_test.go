package seshat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ── Adapter Registry ─────────────────────────────────────────────────────

func TestNewRegistry(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if reg.Sources == nil {
		t.Error("Sources map should be initialized")
	}
	if reg.Targets == nil {
		t.Error("Targets map should be initialized")
	}
}

func TestRegisterSourceAndTarget(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.RegisterSource(&mockSource{})
	reg.RegisterTarget(&mockTarget{})

	if len(reg.Sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(reg.Sources))
	}
	if len(reg.Targets) != 1 {
		t.Errorf("expected 1 target, got %d", len(reg.Targets))
	}
}

func TestDefaultRegistry(t *testing.T) {
	t.Parallel()
	reg := DefaultRegistry()
	if len(reg.Sources) == 0 {
		t.Error("DefaultRegistry should have sources")
	}
	if len(reg.Targets) == 0 {
		t.Error("DefaultRegistry should have targets")
	}
}

func TestIngestAll(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.RegisterSource(&mockSource{})

	items, err := reg.IngestAll(time.Time{})
	if err != nil {
		t.Fatalf("IngestAll: %v", err)
	}
	// mockSource returns nil items, so items should be nil
	if items != nil {
		t.Errorf("expected nil items from empty mock, got %d", len(items))
	}
}

func TestIngestAll_WithErrorAdapter(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.RegisterSource(&errorSource{})

	// Should not return error, just log warning
	_, err := reg.IngestAll(time.Time{})
	if err != nil {
		t.Errorf("IngestAll should handle adapter errors gracefully: %v", err)
	}
}

type errorSource struct{}

func (e *errorSource) Name() string                                { return "error-source" }
func (e *errorSource) Description() string                         { return "always errors" }
func (e *errorSource) Ingest(_ time.Time) ([]KnowledgeItem, error) { return nil, os.ErrNotExist }

// ── ChromeBaseDir ────────────────────────────────────────────────────────

func TestChromeBaseDir(t *testing.T) {
	t.Parallel()
	dir := ChromeBaseDir()
	if dir == "" {
		t.Error("ChromeBaseDir should not be empty")
	}
}

// ── ChromeProfile struct ─────────────────────────────────────────────────

func TestChromeProfile_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	p := ChromeProfile{
		DirName:     "Default",
		DisplayName: "SirsiMaster",
		GaiaName:    "Cylton",
		Email:       "test@example.com",
		AvatarIcon:  "chrome://avatar/1",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var loaded ChromeProfile
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if loaded.DirName != "Default" {
		t.Errorf("DirName = %q", loaded.DirName)
	}
	if loaded.Email != "test@example.com" {
		t.Errorf("Email = %q", loaded.Email)
	}
}

// ── Adapter Name/Description ─────────────────────────────────────────────

func TestAdapterNames(t *testing.T) {
	t.Parallel()
	adapters := []struct {
		name string
		sa   SourceAdapter
	}{
		{"chrome-history", &ChromeHistoryAdapter{}},
		{"gemini", &GeminiAdapter{}},
		{"claude", &ClaudeAdapter{}},
		{"apple-notes", &AppleNotesSourceAdapter{}},
		{"google-workspace", &GoogleWorkspaceAdapter{}},
	}
	for _, a := range adapters {
		t.Run(a.name, func(t *testing.T) {
			if a.sa.Name() != a.name {
				t.Errorf("Name() = %q, want %q", a.sa.Name(), a.name)
			}
			if a.sa.Description() == "" {
				t.Error("Description should not be empty")
			}
		})
	}
}

func TestTargetAdapterNames(t *testing.T) {
	t.Parallel()
	targets := []struct {
		name string
		ta   TargetAdapter
	}{
		{"apple-notes", &AppleNotesTargetAdapter{}},
		{"notebooklm", &NotebookLMAdapter{}},
		{"thoth", &ThothAdapter{}},
	}
	for _, a := range targets {
		t.Run(a.name, func(t *testing.T) {
			if a.ta.Name() != a.name {
				t.Errorf("Name() = %q, want %q", a.ta.Name(), a.name)
			}
			if a.ta.Description() == "" {
				t.Error("Description should not be empty")
			}
		})
	}
}

// ── sanitizeFilename ─────────────────────────────────────────────────────

func TestSanitizeFilename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello_world"},
		{"My!File@Name#123", "myfilename123"},
		{"", ""},
		{"A Very Long Title That Should Be Truncated Because It Exceeds Sixty Characters Limit By Far", "a_very_long_title_that_should_be_truncated_because_it_exceed"},
	}
	for _, tt := range tests {
		got := sanitizeFilename(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── truncate (from adapter_gemini.go) ────────────────────────────────────

func TestTruncate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		s    string
		max  int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncate(tt.s, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
		}
	}
}

// ── NotebookLMAdapter Export ─────────────────────────────────────────────

func TestNotebookLMAdapter_Export(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	adapter := &NotebookLMAdapter{OutputDir: tmp}

	items := []KnowledgeItem{
		{
			Title:   "Test Item",
			Summary: "A test knowledge item.",
			References: []KIReference{
				{Type: "source", Value: "test"},
			},
		},
	}

	err := adapter.Export(items)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Verify file was created
	entries, _ := os.ReadDir(tmp)
	if len(entries) != 1 {
		t.Errorf("expected 1 file, got %d", len(entries))
	}
}

func TestNotebookLMAdapter_Export_Empty(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	adapter := &NotebookLMAdapter{OutputDir: tmp}

	err := adapter.Export(nil)
	if err != nil {
		t.Fatalf("Export(nil): %v", err)
	}
}

func TestNotebookLMAdapter_Export_EmptyTitle(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	adapter := &NotebookLMAdapter{OutputDir: tmp}

	items := []KnowledgeItem{
		{Title: "", Summary: "No title item."},
	}
	err := adapter.Export(items)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
}

// ── ThothAdapter Export ──────────────────────────────────────────────────

func TestThothAdapter_Export(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	adapter := &ThothAdapter{ProjectDir: tmp}

	items := []KnowledgeItem{
		{
			Title:   "Thoth Test",
			Summary: "Testing Thoth grafting.",
			References: []KIReference{
				{Type: "source", Value: "test"},
			},
		},
	}

	err := adapter.Export(items)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	seshatDir := filepath.Join(tmp, ".thoth", "seshat")
	entries, _ := os.ReadDir(seshatDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file in .thoth/seshat/, got %d", len(entries))
	}
}

func TestThothAdapter_Export_EmptyTitle(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	adapter := &ThothAdapter{ProjectDir: tmp}

	items := []KnowledgeItem{
		{Title: "", Summary: "No title — should be skipped."},
	}
	err := adapter.Export(items)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
}

// ── FilteredSourceAdapter ────────────────────────────────────────────────

func TestFilteredSourceAdapter_Ingest(t *testing.T) {
	t.Parallel()
	inner := &ingestingSource{
		items: []KnowledgeItem{
			{Title: "Has key AKIAIOSFODNN7EXAMPLE", Summary: "Safe summary"},
			{Title: "Clean Item", Summary: "Nothing bad here"},
		},
	}
	adapter := &FilteredSourceAdapter{
		Adapter: inner,
		Filter:  DefaultFilter(),
	}

	if adapter.Name() != "ingesting-source" {
		t.Errorf("Name() = %q", adapter.Name())
	}
	if adapter.Description() == "" {
		t.Error("Description should not be empty")
	}

	items, err := adapter.Ingest(time.Time{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

type ingestingSource struct {
	items []KnowledgeItem
}

func (s *ingestingSource) Name() string                                { return "ingesting-source" }
func (s *ingestingSource) Description() string                         { return "returns items" }
func (s *ingestingSource) Ingest(_ time.Time) ([]KnowledgeItem, error) { return s.items, nil }

// ── FilteredTargetAdapter ────────────────────────────────────────────────

func TestFilteredTargetAdapter_Export(t *testing.T) {
	t.Parallel()
	inner := &recordingTarget{}
	adapter := &FilteredTargetAdapter{
		Adapter: inner,
		Filter:  DefaultFilter(),
	}

	if adapter.Name() != "recording-target" {
		t.Errorf("Name() = %q", adapter.Name())
	}

	items := []KnowledgeItem{
		{Title: "Test", Summary: "key: AKIAIOSFODNN7EXAMPLE"},
	}
	err := adapter.Export(items)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !inner.called {
		t.Error("inner target should have been called")
	}
}

type recordingTarget struct {
	called bool
}

func (r *recordingTarget) Name() string                   { return "recording-target" }
func (r *recordingTarget) Description() string            { return "records calls" }
func (r *recordingTarget) Export(_ []KnowledgeItem) error { r.called = true; return nil }

// ── GeminiAdapter conversationToKI ───────────────────────────────────────

func TestGeminiAdapter_ConversationToKI(t *testing.T) {
	t.Parallel()
	adapter := &GeminiAdapter{}

	// Empty conversation
	ki := adapter.conversationToKI(geminiTakeoutConversation{}, "test.json")
	if ki != nil {
		t.Error("empty conversation should return nil")
	}

	// Normal conversation
	conv := geminiTakeoutConversation{
		ID:    "test-id",
		Title: "Test Conversation",
		Messages: []struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			Timestamp string `json:"timestamp"`
		}{
			{Role: "user", Content: "Hello Gemini", Timestamp: "2026-01-01T00:00:00Z"},
			{Role: "model", Content: "Hello!", Timestamp: "2026-01-01T00:00:01Z"},
		},
	}
	ki = adapter.conversationToKI(conv, "test.json")
	if ki == nil {
		t.Fatal("expected non-nil KI")
	}
	if ki.Title != "Test Conversation" {
		t.Errorf("Title = %q", ki.Title)
	}

	// Conversation with no title
	conv2 := geminiTakeoutConversation{
		ID: "no-title",
		Messages: []struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			Timestamp string `json:"timestamp"`
		}{
			{Role: "user", Content: "A question"},
		},
	}
	ki2 := adapter.conversationToKI(conv2, "test2.json")
	if ki2 == nil {
		t.Fatal("expected non-nil KI for untitled conversation")
	}
	if ki2.Title == "" {
		t.Error("title should be auto-generated")
	}
}

// ── GeminiAdapter Ingest (filesystem paths) ─────────────────────────────

func TestGeminiAdapter_IngestTakeout(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create fake takeout data
	conv := geminiTakeoutConversation{
		ID:    "conv-1",
		Title: "Test Export",
		Messages: []struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			Timestamp string `json:"timestamp"`
		}{
			{Role: "user", Content: "Hello"},
		},
	}
	data, _ := json.Marshal(conv)
	os.WriteFile(filepath.Join(tmp, "conversation.json"), data, 0644)

	adapter := &GeminiAdapter{TakeoutDir: tmp}
	items, err := adapter.ingestTakeout(time.Time{})
	if err != nil {
		t.Fatalf("ingestTakeout: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestGeminiAdapter_IngestTakeout_Array(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	convs := []geminiTakeoutConversation{
		{ID: "c1", Title: "Conv 1", Messages: []struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			Timestamp string `json:"timestamp"`
		}{{Role: "user", Content: "Hello"}}},
		{ID: "c2", Title: "Conv 2", Messages: []struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			Timestamp string `json:"timestamp"`
		}{{Role: "user", Content: "Hi"}}},
	}
	data, _ := json.Marshal(convs)
	os.WriteFile(filepath.Join(tmp, "conversations.json"), data, 0644)

	adapter := &GeminiAdapter{TakeoutDir: tmp}
	items, err := adapter.ingestTakeout(time.Time{})
	if err != nil {
		t.Fatalf("ingestTakeout: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestGeminiAdapter_IngestLocal_NoDir(t *testing.T) {
	t.Parallel()
	adapter := &GeminiAdapter{LocalDir: "/nonexistent/path"}
	items, err := adapter.ingestLocal(time.Time{})
	if err != nil {
		t.Fatalf("ingestLocal on missing dir should not error: %v", err)
	}
	if items != nil {
		t.Error("expected nil items for missing dir")
	}
}

func TestGeminiAdapter_Ingest(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	adapter := &GeminiAdapter{
		TakeoutDir: filepath.Join(tmp, "takeout"),
		LocalDir:   filepath.Join(tmp, "local"),
	}

	// Both dirs missing — should still return without error
	items, err := adapter.Ingest(time.Time{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	_ = items // may be nil or empty
}

// ── ClaudeAdapter ────────────────────────────────────────────────────────

func TestClaudeAdapter_ParseTranscript(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create a minimal transcript
	lines := []string{
		`{"type":"summary","role":"user","sessionId":"s1","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"What is Go?"}}`,
		`{"type":"summary","role":"assistant","sessionId":"s1","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","content":"Go is a programming language."}}`,
		`{"type":"summary","role":"user","sessionId":"s1","timestamp":"2026-01-01T00:00:02Z","message":{"role":"user","content":"Tell me more"}}`,
	}
	path := filepath.Join(tmp, "transcript.jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	os.WriteFile(path, []byte(content), 0644)

	adapter := &ClaudeAdapter{}
	ki, err := adapter.parseTranscript(path)
	if err != nil {
		t.Fatalf("parseTranscript: %v", err)
	}
	if ki == nil {
		t.Fatal("expected non-nil KI")
	}
	if ki.Title == "" {
		t.Error("title should not be empty")
	}
}

func TestClaudeAdapter_ParseTranscript_TooSmall(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	path := filepath.Join(tmp, "small.jsonl")
	os.WriteFile(path, []byte(`{"type":"summary","message":{"role":"user","content":"hi"}}`+"\n"), 0644)

	adapter := &ClaudeAdapter{}
	ki, err := adapter.parseTranscript(path)
	if err != nil {
		t.Fatalf("parseTranscript: %v", err)
	}
	if ki != nil {
		t.Error("transcript with < 2 messages should return nil")
	}
}

func TestClaudeAdapter_ParseTranscript_ArrayContent(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	lines := []string{
		`{"type":"summary","sessionId":"s1","timestamp":"t1","message":{"role":"user","content":[{"type":"text","text":"Question one"}]}}`,
		`{"type":"summary","sessionId":"s1","timestamp":"t2","message":{"role":"assistant","content":[{"type":"text","text":"Answer one"}]}}`,
		`{"type":"summary","sessionId":"s1","timestamp":"t3","message":{"role":"user","content":[{"type":"text","text":"Question two"}]}}`,
	}
	path := filepath.Join(tmp, "array.jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	os.WriteFile(path, []byte(content), 0644)

	adapter := &ClaudeAdapter{}
	ki, err := adapter.parseTranscript(path)
	if err != nil {
		t.Fatalf("parseTranscript: %v", err)
	}
	if ki == nil {
		t.Fatal("expected non-nil KI for array content messages")
	}
}

// ── WrapRegistry ─────────────────────────────────────────────────────────

func TestWrapRegistry_Comprehensive(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.RegisterSource(&mockSource{})
	reg.RegisterSource(&ingestingSource{items: []KnowledgeItem{{Title: "Test"}}})
	reg.RegisterTarget(&mockTarget{})
	reg.RegisterTarget(&recordingTarget{})

	filter := DefaultFilter()
	wrapped := WrapRegistry(reg, filter)

	if len(wrapped.Sources) != 2 {
		t.Errorf("expected 2 wrapped sources, got %d", len(wrapped.Sources))
	}
	if len(wrapped.Targets) != 2 {
		t.Errorf("expected 2 wrapped targets, got %d", len(wrapped.Targets))
	}
}

// ── hasCritical helper ───────────────────────────────────────────────────

func TestHasCritical(t *testing.T) {
	t.Parallel()

	criticalMatch := []FilterMatch{{Rule: "aws-access-key"}}
	nonCriticalMatch := []FilterMatch{{Rule: "email-address"}}
	noMatch := []FilterMatch{}

	if !hasCritical(criticalMatch) {
		t.Error("should detect critical AWS key match")
	}
	if hasCritical(nonCriticalMatch) {
		t.Error("email-address is medium severity, not critical")
	}
	if hasCritical(noMatch) {
		t.Error("empty matches should not be critical")
	}
}
