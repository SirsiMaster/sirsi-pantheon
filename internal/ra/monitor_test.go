package ra

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupRaDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	raDir := filepath.Join(tmp, ".ra")
	os.MkdirAll(filepath.Join(raDir, "logs"), 0o755)
	return raDir
}

func TestReadPIDFile_Valid(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "pid")
	os.WriteFile(path, []byte("12345\n"), 0o644)

	pid, err := readPIDFile(path)
	if err != nil {
		t.Fatalf("readPIDFile() error = %v", err)
	}
	if pid != 12345 {
		t.Errorf("pid = %d, want 12345", pid)
	}
}

func TestReadPIDFile_Invalid(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "pid")
	os.WriteFile(path, []byte("notanumber"), 0o644)

	_, err := readPIDFile(path)
	if err == nil {
		t.Error("expected error for invalid PID")
	}
}

func TestReadPIDFile_Missing(t *testing.T) {
	_, err := readPIDFile("/nonexistent/pid")
	if err == nil {
		t.Error("expected error for missing PID file")
	}
}

func TestReadExitFile_Valid(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "exit")
	os.WriteFile(path, []byte("0"), 0o644)

	code, err := readExitFile(path)
	if err != nil {
		t.Fatalf("readExitFile() error = %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestReadExitFile_NonZero(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "exit")
	os.WriteFile(path, []byte("1"), 0o644)

	code, err := readExitFile(path)
	if err != nil {
		t.Fatalf("readExitFile() error = %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestReadLogTail_UnderTenLines(t *testing.T) {
	raDir := setupRaDir(t)
	logPath := filepath.Join(raDir, "logs", "test-scope.log")
	os.WriteFile(logPath, []byte("line1\nline2\nline3"), 0o644)

	got := readLogTail(raDir, "test-scope")
	if got != "line1\nline2\nline3" {
		t.Errorf("readLogTail = %q, want all 3 lines", got)
	}
}

func TestReadLogTail_OverTenLines(t *testing.T) {
	raDir := setupRaDir(t)
	logPath := filepath.Join(raDir, "logs", "big.log")
	var lines string
	for i := 1; i <= 15; i++ {
		lines += "line" + string(rune('0'+i/10)) + string(rune('0'+i%10)) + "\n"
	}
	os.WriteFile(logPath, []byte(lines), 0o644)

	got := readLogTail(raDir, "big")
	gotLines := len(splitNonEmpty(got))
	if gotLines > 10 {
		t.Errorf("readLogTail should return ≤10 lines, got %d", gotLines)
	}
}

func splitNonEmpty(s string) []string {
	parts := make([]string, 0)
	for _, p := range filepath.SplitList(s) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func TestReadLogTail_MissingFile(t *testing.T) {
	got := readLogTail("/nonexistent", "nope")
	if got != "" {
		t.Errorf("readLogTail should return empty for missing file, got %q", got)
	}
}

func TestReadDeploymentMeta_Valid(t *testing.T) {
	raDir := setupRaDir(t)
	meta := deploymentMeta{
		Scopes:    []string{"test-scope"},
		StartedAt: "2026-05-14T12:00:00Z",
	}
	data, _ := json.Marshal(meta)
	os.WriteFile(filepath.Join(raDir, "deployment.json"), data, 0o644)

	got, err := readDeploymentMeta(raDir)
	if err != nil {
		t.Fatalf("readDeploymentMeta() error = %v", err)
	}
	if len(got.Scopes) != 1 || got.Scopes[0] != "test-scope" {
		t.Errorf("Scopes = %v, want [test-scope]", got.Scopes)
	}
}

func TestReadDeploymentMeta_Missing(t *testing.T) {
	_, err := readDeploymentMeta("/nonexistent")
	if err == nil {
		t.Error("expected error for missing deployment.json")
	}
}

func TestReadDeploymentMeta_MalformedJSON(t *testing.T) {
	raDir := setupRaDir(t)
	os.WriteFile(filepath.Join(raDir, "deployment.json"), []byte("{bad json"), 0o644)

	_, err := readDeploymentMeta(raDir)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		input string
		want  string
	}{
		{"~/Documents", home + "/Documents"},
		{"/usr/local", "/usr/local"},
		{"relative", "relative"},
		{"", ""},
	}
	for _, tt := range tests {
		got := expandHome(tt.input)
		if got != tt.want {
			t.Errorf("expandHome(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSyncStatus(t *testing.T) {
	synced := syncStatus(true)
	if synced == "" {
		t.Error("syncStatus(true) should not be empty")
	}
	skipped := syncStatus(false)
	if skipped == "" {
		t.Error("syncStatus(false) should not be empty")
	}
	if synced == skipped {
		t.Error("syncStatus(true) and syncStatus(false) should differ")
	}
}

func TestTryParseJSON_ValidArray(t *testing.T) {
	input := `[{"title":"Test","content":"Hello"}]`
	items := tryParseJSON(input)
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestTryParseJSON_InvalidJSON(t *testing.T) {
	items := tryParseJSON("not json at all")
	if len(items) != 0 {
		t.Errorf("expected 0 items for invalid JSON, got %d", len(items))
	}
}

func TestTryParseJSON_EmptyArray(t *testing.T) {
	items := tryParseJSON("[]")
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty array, got %d", len(items))
	}
}

// ── String escaping ─────────────────────────────────────────────────

func TestEscapeShell(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"it's", "'it'\"'\"'s'"},
		{"", "''"},
		{"/usr/local/bin", "'/usr/local/bin'"},
	}
	for _, tt := range tests {
		got := escapeShell(tt.input)
		if got != tt.want {
			t.Errorf("escapeShell(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEscapeAppleScript(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{`say "hi"`, `say \"hi\"`},
		{`path\to\file`, `path\\to\\file`},
		{"", ""},
	}
	for _, tt := range tests {
		got := escapeAppleScript(tt.input)
		if got != tt.want {
			t.Errorf("escapeAppleScript(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── AppleScript generation ──────────────────────────────────────────

func TestBuildTerminalScript(t *testing.T) {
	script := buildTerminalScript("echo hello", "Test Window")
	if !strings.Contains(script, "Terminal") {
		t.Error("buildTerminalScript should reference Terminal.app")
	}
	if !strings.Contains(script, "echo hello") {
		t.Error("buildTerminalScript should contain the command")
	}
	if !strings.Contains(script, "Test Window") {
		t.Error("buildTerminalScript should contain the title")
	}
}

func TestBuildITerm2Script(t *testing.T) {
	script := buildITerm2Script("ls -la", "iTerm Test")
	if !strings.Contains(script, "iTerm") {
		t.Error("buildITerm2Script should reference iTerm")
	}
	if !strings.Contains(script, "ls -la") {
		t.Error("buildITerm2Script should contain the command")
	}
}

// ── Pipeline status ─────────────────────────────────────────────────

func TestPipeline_StatusFile(t *testing.T) {
	p := &Pipeline{ThothDir: "/tmp/test/.thoth"}
	got := p.statusFile()
	if got != "/tmp/test/.thoth/ra_pipeline_status.json" {
		t.Errorf("statusFile() = %q", got)
	}
}

func TestPipeline_RecordAndReadStatus(t *testing.T) {
	tmp := t.TempDir()
	p := &Pipeline{ThothDir: tmp}

	if err := p.recordStatus(5, true); err != nil {
		t.Fatalf("recordStatus() error: %v", err)
	}

	status, err := p.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus() error: %v", err)
	}
	if status == nil {
		t.Fatal("ReadStatus() returned nil")
	}
	if status.ItemCount != 5 {
		t.Errorf("ItemCount = %d, want 5", status.ItemCount)
	}
	if status.LastRecorded.IsZero() {
		t.Error("LastRecorded should not be zero")
	}
	if status.ThothSynced.IsZero() {
		t.Error("ThothSynced should not be zero when synced=true")
	}
}

func TestPipeline_ReadStatus_NoFile(t *testing.T) {
	p := &Pipeline{ThothDir: t.TempDir()}
	status, err := p.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus() error: %v", err)
	}
	if status != nil {
		t.Error("expected nil status for missing file")
	}
}

func TestPipeline_RecordStatus_NoSync(t *testing.T) {
	tmp := t.TempDir()
	p := &Pipeline{ThothDir: tmp}

	p.recordStatus(3, false)
	status, _ := p.ReadStatus()
	if status.ThothSynced.IsZero() == false {
		t.Error("ThothSynced should be zero when synced=false")
	}
}

// ── CollectResults ──────────────────────────────────────────────────

func TestCollectResults_WithExitCodes(t *testing.T) {
	raDir := setupRaDir(t)

	// Create deployment meta
	meta := deploymentMeta{Scopes: []string{"scope-a", "scope-b"}, StartedAt: "2026-05-15T10:00:00Z"}
	data, _ := json.Marshal(meta)
	os.WriteFile(filepath.Join(raDir, "deployment.json"), data, 0o644)

	// Create exit files (format: scope.exit)
	os.MkdirAll(filepath.Join(raDir, "exits"), 0o755)
	os.WriteFile(filepath.Join(raDir, "exits", "scope-a.exit"), []byte("0"), 0o644)
	os.WriteFile(filepath.Join(raDir, "exits", "scope-b.exit"), []byte("1"), 0o644)

	// Create log files
	os.WriteFile(filepath.Join(raDir, "logs", "scope-a.log"), []byte("success\n"), 0o644)
	os.WriteFile(filepath.Join(raDir, "logs", "scope-b.log"), []byte("error: failed\n"), 0o644)

	results, err := CollectResults(raDir)
	if err != nil {
		t.Fatalf("CollectResults() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// scope-a should have exit 0
	for _, r := range results {
		if r.Name == "scope-a" && r.ExitCode != 0 {
			t.Errorf("scope-a exit code = %d, want 0", r.ExitCode)
		}
		if r.Name == "scope-b" && r.ExitCode != 1 {
			t.Errorf("scope-b exit code = %d, want 1", r.ExitCode)
		}
	}
}

// ── writeDeployMeta ─────────────────────────────────────────────────

func TestWriteDeployMeta(t *testing.T) {
	raDir := setupRaDir(t)
	writeDeployMeta(raDir, []string{"test-scope"})

	meta, err := readDeploymentMeta(raDir)
	if err != nil {
		t.Fatalf("readDeploymentMeta() error: %v", err)
	}
	if len(meta.Scopes) != 1 || meta.Scopes[0] != "test-scope" {
		t.Errorf("Scopes = %v, want [test-scope]", meta.Scopes)
	}
}
