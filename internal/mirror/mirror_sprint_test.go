package mirror

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── handleStatus ─────────────────────────────────────────────────────────

func TestHandleStatus_Initial(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	srv.handleStatus(w, req)

	var body map[string]interface{}
	json.NewDecoder(w.Result().Body).Decode(&body)
	if body["scanning"] != false {
		t.Error("scanning should be false initially")
	}
	if body["has_result"] != false {
		t.Error("has_result should be false initially")
	}
}

func TestHandleStatus_WithResult(t *testing.T) {
	t.Parallel()
	srv := &Server{result: &MirrorResult{TotalScanned: 42}}
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	srv.handleStatus(w, req)

	var body map[string]interface{}
	json.NewDecoder(w.Result().Body).Decode(&body)
	if body["has_result"] != true {
		t.Error("has_result should be true when result exists")
	}
}

// ── handleResult ─────────────────────────────────────────────────────────

func TestHandleResult_NoResult(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	req := httptest.NewRequest("GET", "/api/result", nil)
	w := httptest.NewRecorder()

	srv.handleResult(w, req)

	var body map[string]string
	json.NewDecoder(w.Result().Body).Decode(&body)
	if body["status"] != "no_result" {
		t.Errorf("status = %q, want 'no_result'", body["status"])
	}
}

func TestHandleResult_WithResult(t *testing.T) {
	t.Parallel()
	srv := &Server{result: &MirrorResult{
		TotalScanned:    100,
		TotalDuplicates: 5,
		TotalWasteBytes: 2048,
	}}
	req := httptest.NewRequest("GET", "/api/result", nil)
	w := httptest.NewRecorder()

	srv.handleResult(w, req)

	var body MirrorResult
	json.NewDecoder(w.Result().Body).Decode(&body)
	if body.TotalScanned != 100 {
		t.Errorf("TotalScanned = %d, want 100", body.TotalScanned)
	}
}

// ── handleScan ──────────────────────────────────────────────────────────

func TestHandleScan_BadRequest(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader("{invalid"))
	w := httptest.NewRecorder()

	srv.handleScan(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleScan_AlreadyScanning(t *testing.T) {
	t.Parallel()
	srv := &Server{scanning: true}
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(`{"paths":["/tmp"]}`))
	w := httptest.NewRecorder()

	srv.handleScan(w, req)

	var body map[string]string
	json.NewDecoder(w.Result().Body).Decode(&body)
	if body["status"] != "already_scanning" {
		t.Errorf("status = %q, want 'already_scanning'", body["status"])
	}
}

// ── handleUI ─────────────────────────────────────────────────────────────

func TestHandleUI(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	srv.handleUI(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Mirror") {
		t.Error("HTML should contain 'Mirror'")
	}
}

// ── mirrorHTML ────────────────────────────────────────────────────────────

func TestMirrorHTML(t *testing.T) {
	t.Parallel()
	html := mirrorHTML()
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("should contain DOCTYPE")
	}
	if !strings.Contains(html, "Mirror") {
		t.Error("should contain Mirror branding")
	}
}

// ── hashFilePartial ──────────────────────────────────────────────────────

func TestHashFilePartial_SmallFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "small.txt")
	os.WriteFile(path, []byte("tiny"), 0o644)

	hash, err := hashFilePartial(path, 4096)
	if err != nil {
		t.Fatalf("hashFilePartial error: %v", err)
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
}

func TestHashFilePartial_LargeFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")
	data := make([]byte, 16384) // 16KB > 2*4096
	for i := range data {
		data[i] = byte(i % 256)
	}
	os.WriteFile(path, data, 0o644)

	hash, err := hashFilePartial(path, 4096)
	if err != nil {
		t.Fatalf("hashFilePartial error: %v", err)
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
}

func TestHashFilePartial_DifferentFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Two files same size, different content
	data1 := make([]byte, 16384)
	data2 := make([]byte, 16384)
	for i := range data1 {
		data1[i] = byte(i % 256)
		data2[i] = byte((i + 1) % 256)
	}
	os.WriteFile(filepath.Join(dir, "a.bin"), data1, 0o644)
	os.WriteFile(filepath.Join(dir, "b.bin"), data2, 0o644)

	h1, _ := hashFilePartial(filepath.Join(dir, "a.bin"), 4096)
	h2, _ := hashFilePartial(filepath.Join(dir, "b.bin"), 4096)
	if h1 == h2 {
		t.Error("different files should have different partial hashes")
	}
}

func TestHashFilePartial_NonExistent(t *testing.T) {
	t.Parallel()
	_, err := hashFilePartial("/nonexistent/file.bin", 4096)
	if err == nil {
		t.Error("should error on nonexistent file")
	}
}

// ── sortByPriority ───────────────────────────────────────────────────────

func TestSortByPriority_ProtectedFirst(t *testing.T) {
	t.Parallel()
	files := []FileEntry{
		{Path: "/deep/nested/copy.txt", IsProtected: false},
		{Path: "/safe/original.txt", IsProtected: true},
	}
	sortByPriority(files)
	if !files[0].IsProtected {
		t.Error("protected file should be first")
	}
}

func TestSortByPriority_ShallowerFirst(t *testing.T) {
	t.Parallel()
	files := []FileEntry{
		{Path: "/a/b/c/d/deep.txt", ModTime: time.Now()},
		{Path: "/a/shallow.txt", ModTime: time.Now()},
	}
	sortByPriority(files)
	if strings.Count(files[0].Path, "/") > strings.Count(files[1].Path, "/") {
		t.Error("shallower path should be first")
	}
}

func TestSortByPriority_OlderFirst(t *testing.T) {
	t.Parallel()
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	files := []FileEntry{
		{Path: "/a/new.txt", ModTime: newer},
		{Path: "/a/old.txt", ModTime: old},
	}
	sortByPriority(files)
	if files[0].ModTime.After(files[1].ModTime) {
		t.Error("older file should be first")
	}
}

// ── Scan edge cases ──────────────────────────────────────────────────────

func TestScan_MaxSizeFilter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "small.txt"), "tiny file")
	bigData := strings.Repeat("x", 5000)
	writeFile(t, filepath.Join(dir, "big.txt"), bigData)

	result, err := Scan(ScanOptions{
		Paths:   []string{dir},
		MaxSize: 100,
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if result.TotalScanned != 1 {
		t.Errorf("TotalScanned = %d, want 1 (big file filtered)", result.TotalScanned)
	}
}

func TestScan_SkipsHiddenDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hiddenDir := filepath.Join(dir, ".hidden")
	os.MkdirAll(hiddenDir, 0o755)
	writeFile(t, filepath.Join(hiddenDir, "secret.txt"), "hidden content stuff")
	writeFile(t, filepath.Join(dir, "visible.txt"), "visible content stuff")

	result, err := Scan(ScanOptions{Paths: []string{dir}})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if result.TotalScanned != 1 {
		t.Errorf("TotalScanned = %d, want 1 (hidden dir skipped)", result.TotalScanned)
	}
}

func TestScan_NonExistentPath(t *testing.T) {
	t.Parallel()
	_, err := Scan(ScanOptions{Paths: []string{"/nonexistent/path/xyz"}})
	if err == nil {
		t.Error("should error on nonexistent path")
	}
}

// ── MirrorResult fields ──────────────────────────────────────────────────

func TestMirrorResult_Fields(t *testing.T) {
	t.Parallel()
	r := MirrorResult{
		UniqueFiles: 950,
		DirsScanned: []string{"/a", "/b"},
	}
	if r.UniqueFiles != 950 {
		t.Errorf("UniqueFiles = %d", r.UniqueFiles)
	}
	if len(r.DirsScanned) != 2 {
		t.Errorf("DirsScanned = %d", len(r.DirsScanned))
	}
}

// ── DuplicateGroup fields ────────────────────────────────────────────────

func TestDuplicateGroup_Fields(t *testing.T) {
	t.Parallel()
	g := DuplicateGroup{
		MatchType:  MatchPerceptual,
		Confidence: 0.95,
	}
	if g.MatchType != MatchPerceptual {
		t.Errorf("MatchType = %q", g.MatchType)
	}
	if g.Confidence != 0.95 {
		t.Errorf("Confidence = %f", g.Confidence)
	}
}

// ── FormatBytes edge cases ───────────────────────────────────────────────

func TestFormatBytes_TB(t *testing.T) {
	t.Parallel()
	got := FormatBytes(1099511627776) // 1 TB
	if !strings.Contains(got, "TB") {
		t.Errorf("FormatBytes(1TB) = %q, should contain TB", got)
	}
}
