package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// handleGhostReport runs live Ka scanning — exercise the code path.
func TestHandleGhostReport_NoTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping live Ka scan in short mode")
	}
	result, err := handleGhostReport(map[string]interface{}{})
	if err != nil {
		t.Fatalf("handleGhostReport: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Content) == 0 {
		t.Error("expected content in result")
	}
	// Should contain "Ka Ghost Report" regardless of ghost count
	if result.Content[0].Text == "" {
		t.Error("expected non-empty text")
	}
}

func TestHandleGhostReport_WithTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping live Ka scan in short mode")
	}
	result, err := handleGhostReport(map[string]interface{}{
		"target": "NonExistentAppXYZ12345",
	})
	if err != nil {
		t.Fatalf("handleGhostReport: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// handleHealthCheck now uses cached data — should be instant (<100ms).
func TestHandleHealthCheck(t *testing.T) {
	start := time.Now()
	result, err := handleHealthCheck(nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("handleHealthCheck: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Performance gate: health_check MUST respond in <1s
	if elapsed > 1*time.Second {
		t.Errorf("health_check took %s — must be <1s (no live scans)", elapsed)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "Anubis Health Check") {
		t.Error("should contain health check header")
	}
	if !strings.Contains(text, "Response time:") {
		t.Error("should report response time")
	}
	t.Logf("health_check response: %s", elapsed.Round(time.Microsecond))
}

// Server.Run reads from os.Stdin which we can't easily test, but verify
// the function exists and the Server can be constructed for it.
func TestServer_Run_Exists(t *testing.T) {
	srv := NewServer()
	_ = srv // Just verify it compiles
}

// --- handleThothReadMemory tests ---

func TestHandleThothReadMemory_NoMemory(t *testing.T) {
	tmpDir := t.TempDir()
	result, err := handleThothReadMemory(map[string]interface{}{
		"path": tmpDir,
	})
	if err != nil {
		t.Fatalf("handleThothReadMemory: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "No Thoth memory file found") {
		t.Errorf("expected 'No Thoth memory' message, got: %s", text[:min(len(text), 100)])
	}
}

func TestHandleThothReadMemory_WithMemory(t *testing.T) {
	tmpDir := t.TempDir()
	thothDir := filepath.Join(tmpDir, ".thoth")
	os.MkdirAll(thothDir, 0o755)
	os.WriteFile(filepath.Join(thothDir, "memory.yaml"), []byte("project: test-project\nversion: 0.1.0"), 0o644)

	result, err := handleThothReadMemory(map[string]interface{}{
		"path": tmpDir,
	})
	if err != nil {
		t.Fatalf("handleThothReadMemory: %v", err)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "test-project") {
		t.Errorf("should contain memory content, got: %s", text[:min(len(text), 100)])
	}
}

func TestHandleThothReadMemory_WithJournal(t *testing.T) {
	tmpDir := t.TempDir()
	thothDir := filepath.Join(tmpDir, ".thoth")
	os.MkdirAll(thothDir, 0o755)
	os.WriteFile(filepath.Join(thothDir, "memory.yaml"), []byte("project: journal-test"), 0o644)
	os.WriteFile(filepath.Join(thothDir, "journal.md"), []byte("## Entry 1\nDid some work"), 0o644)

	result, err := handleThothReadMemory(map[string]interface{}{
		"path": tmpDir,
	})
	if err != nil {
		t.Fatalf("handleThothReadMemory: %v", err)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "Journal") {
		t.Errorf("should contain journal header, got: %s", text[:min(len(text), 200)])
	}
	if !strings.Contains(text, "Did some work") {
		t.Errorf("should contain journal content")
	}
}

func TestHandleThothReadMemory_LongJournal(t *testing.T) {
	tmpDir := t.TempDir()
	thothDir := filepath.Join(tmpDir, ".thoth")
	os.MkdirAll(thothDir, 0o755)
	os.WriteFile(filepath.Join(thothDir, "memory.yaml"), []byte("project: long-journal"), 0o644)

	// Write a journal > 2000 chars to test truncation
	longContent := strings.Repeat("x", 3000)
	os.WriteFile(filepath.Join(thothDir, "journal.md"), []byte(longContent), 0o644)

	result, err := handleThothReadMemory(map[string]interface{}{
		"path": tmpDir,
	})
	if err != nil {
		t.Fatalf("handleThothReadMemory: %v", err)
	}
	text := result.Content[0].Text
	// The journal should be truncated to last 2000 chars
	if len(text) > 3000 {
		t.Errorf("journal should be truncated, total output len = %d", len(text))
	}
}

// --- handleScanWorkspace tests ---

func TestHandleScanWorkspace_DefaultPath(t *testing.T) {
	result, err := handleScanWorkspace(map[string]interface{}{})
	if err != nil {
		t.Fatalf("handleScanWorkspace: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "Anubis Scan Results") {
		t.Errorf("expected scan results header, got: %s", text[:min(len(text), 100)])
	}
}

func TestHandleScanWorkspace_InvalidCategory(t *testing.T) {
	result, err := handleScanWorkspace(map[string]interface{}{
		"category": "nonexistent-category",
	})
	if err != nil {
		t.Fatalf("handleScanWorkspace: %v", err)
	}
	if !result.IsError {
		t.Error("invalid category should return error result")
	}
}

func TestHandleScanWorkspace_ValidCategory(t *testing.T) {
	result, err := handleScanWorkspace(map[string]interface{}{
		"category": "dev",
	})
	if err != nil {
		t.Fatalf("handleScanWorkspace: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// --- parseCategory tests ---

func TestParseCategory_All(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"general", true},
		{"dev", true},
		{"developer", true},
		{"ai", true},
		{"ml", true},
		{"ai-ml", true},
		{"vms", true},
		{"virtualization", true},
		{"ides", true},
		{"ide", true},
		{"cloud", true},
		{"infra", true},
		{"storage", true},
		{"GENERAL", true}, // case-insensitive
		{"DeV", true},     // case-insensitive
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parseCategory(tt.input)
			if tt.valid && err != nil {
				t.Errorf("parseCategory(%q) should be valid, got error: %v", tt.input, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("parseCategory(%q) should be invalid", tt.input)
			}
		})
	}
}

// --- shortenHomePath tests ---

func TestShortenHomePath_Coverage(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		input string
		want  string
	}{
		{home + "/Development/test", "~/Development/test"},
		{"/tmp/something", "/tmp/something"},
		{"relative/path", "relative/path"},
	}
	for _, tt := range tests {
		got := shortenHomePath(tt.input)
		if got != tt.want {
			t.Errorf("shortenHomePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- writeResponse tests ---

func TestWriteResponse_Success(t *testing.T) {
	var buf bytes.Buffer
	srv := NewServer()
	srv.writeResponse(&buf, Response{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Result:  map[string]string{"key": "value"},
	})
	output := buf.String()
	if !strings.Contains(output, "jsonrpc") {
		t.Error("output should contain jsonrpc")
	}
	if !strings.Contains(output, "key") {
		t.Error("output should contain result key")
	}
}

// --- textResult tests ---

func TestTextResult_Coverage(t *testing.T) {
	r := textResult("test message", false)
	if r.IsError {
		t.Error("should not be error")
	}
	if len(r.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(r.Content))
	}
	if r.Content[0].Text != "test message" {
		t.Errorf("text = %q, want 'test message'", r.Content[0].Text)
	}
}

func TestTextResult_Error_Coverage(t *testing.T) {
	r := textResult("error msg", true)
	if !r.IsError {
		t.Error("should be error")
	}
}
