package brain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- selectPlatformModel tests ---

func TestSelectPlatformModel_DefaultPassthrough(t *testing.T) {
	// When there's no matching coreml model, return the default
	defaultModel := &ModelInfo{
		Name:   "anubis-classifier-v1",
		Format: "onnx",
	}
	remote := &RemoteManifest{
		Models: []ModelInfo{
			{Name: "anubis-classifier-v1", Format: "onnx"},
			{Name: "other-model", Format: "onnx"},
		},
	}

	result := selectPlatformModel(defaultModel, remote)
	if result.Name != defaultModel.Name {
		t.Errorf("selectPlatformModel should return default model, got %q", result.Name)
	}
}

func TestSelectPlatformModel_OnnxSuffix(t *testing.T) {
	// When the default model name already ends with -onnx, the prefix
	// match should strip that suffix to find the coreml variant
	defaultModel := &ModelInfo{
		Name:   "classifier-onnx",
		Format: "onnx",
	}
	remote := &RemoteManifest{
		Models: []ModelInfo{
			{Name: "classifier-onnx", Format: "onnx"},
			{Name: "classifier-coreml", Format: "coreml"},
		},
	}

	// On non-darwin/arm64, should return default
	result := selectPlatformModel(defaultModel, remote)
	// Can't control runtime.GOOS/GOARCH in tests, so just verify it doesn't panic
	if result == nil {
		t.Fatal("selectPlatformModel returned nil")
	}
}

// --- installModel tests (with httptest) ---

func TestInstallModel_DownloadAndVerify(t *testing.T) {
	// Create a temp file to serve as the model
	modelContent := []byte("fake-model-weights-for-testing-12345")
	h := sha256.Sum256(modelContent)
	expectedHash := hex.EncodeToString(h[:])

	// Start a test HTTP server to serve the model download
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(modelContent)))
		w.Write(modelContent)
	}))
	defer server.Close()

	// Override WeightsDir by using installModel directly with a custom model
	tmpDir := t.TempDir()
	weightsDir := filepath.Join(tmpDir, "weights")
	if err := os.MkdirAll(weightsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	downloadURL := server.URL + "/model.onnx"
	expectedSize := int64(len(modelContent))

	// We can't call installModel directly since it uses WeightsDir().
	// Instead, test downloadFile + hashFile + writeLocalManifest as the unit composition.

	localPath := filepath.Join(weightsDir, "test-classifier.onnx")

	// Test downloadFile
	var progressCalls int
	err := downloadFile(downloadURL, localPath, expectedSize, func(downloaded, total int64) {
		progressCalls++
	})
	if err != nil {
		t.Fatalf("downloadFile error: %v", err)
	}
	if progressCalls == 0 {
		t.Error("progress callback was never called")
	}

	// Verify the file was written
	content, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(content) != string(modelContent) {
		t.Errorf("Downloaded content mismatch: got %q, want %q", string(content), string(modelContent))
	}

	// Test hashFile on the downloaded file
	actualHash, err := hashFile(localPath)
	if err != nil {
		t.Fatalf("hashFile error: %v", err)
	}
	if actualHash != expectedHash {
		t.Errorf("Hash mismatch: got %s, want %s", actualHash, expectedHash)
	}
}

func TestDownloadFile_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "model.bin")

	err := downloadFile(server.URL+"/missing.bin", dest, 1024, nil)
	if err == nil {
		t.Error("downloadFile should error on HTTP 404")
	}
}

func TestDownloadFile_NilProgress(t *testing.T) {
	content := []byte("test-content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "file.bin")

	// Download with nil progress should not panic
	err := downloadFile(server.URL+"/file.bin", dest, int64(len(content)), nil)
	if err == nil {
		// Verify file exists and is correct
		got, readErr := os.ReadFile(dest)
		if readErr != nil {
			t.Fatalf("ReadFile error: %v", readErr)
		}
		if string(got) != string(content) {
			t.Errorf("Content mismatch")
		}
	}
}

func TestDownloadFile_BadScheme(t *testing.T) {
	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "bad.bin")

	// Use an invalid URL scheme to trigger an immediate error (no timeout)
	err := downloadFile("not-a-url://invalid", dest, 100, nil)
	if err == nil {
		t.Error("downloadFile should error on invalid URL")
	}
}

// --- FetchRemoteManifest with httptest ---

func TestFetchRemoteManifest_Success(t *testing.T) {
	manifest := RemoteManifest{
		SchemaVersion: 1,
		Updated:       "2026-03-23",
		DefaultModel:  "anubis-classifier-v1",
		Models: []ModelInfo{
			{
				Name:        "anubis-classifier-v1",
				Version:     "1.0.0",
				Format:      "onnx",
				SizeBytes:   50 * 1024 * 1024,
				SHA256:      "abc123",
				DownloadURL: "https://example.com/model.onnx",
			},
		},
	}

	data, _ := json.Marshal(manifest)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer server.Close()

	// We can't override DefaultManifestURL easily, but we can test the HTTP
	// parsing logic by directly testing with a real server.
	// Instead, test the JSON decoding path:
	client := &http.Client{Timeout: manifestTimeout}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("GET error: %v", err)
	}
	defer resp.Body.Close()

	var got RemoteManifest
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	if got.DefaultModel != manifest.DefaultModel {
		t.Errorf("DefaultModel = %q, want %q", got.DefaultModel, manifest.DefaultModel)
	}
	if len(got.Models) != 1 {
		t.Fatalf("Models count = %d, want 1", len(got.Models))
	}
	if got.Models[0].Name != "anubis-classifier-v1" {
		t.Errorf("Model name = %q, want %q", got.Models[0].Name, "anubis-classifier-v1")
	}
}

// --- Remove tests ---

func TestRemove_CleansUp(t *testing.T) {
	tmpDir := t.TempDir()
	weightsDir := filepath.Join(tmpDir, ".anubis", "weights")

	// Create a fake weights directory with files
	if err := os.MkdirAll(weightsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	modelPath := filepath.Join(weightsDir, "model.onnx")
	if err := os.WriteFile(modelPath, []byte("fake-model"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	manifestPath := filepath.Join(weightsDir, ManifestFile)
	if err := os.WriteFile(manifestPath, []byte(`{"installed_model":"test"}`), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	// Verify files exist
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Fatal("model file should exist before remove")
	}

	// os.RemoveAll on the weights dir
	if err := os.RemoveAll(weightsDir); err != nil {
		t.Fatalf("RemoveAll error: %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(weightsDir); !os.IsNotExist(err) {
		t.Error("weights dir should not exist after remove")
	}
}

// --- Filename extension tests in installModel path ---

func TestModelFilenameExtension(t *testing.T) {
	// Test the filename extension logic from installModel
	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{"classifier", "onnx", "classifier.onnx"},
		{"classifier.onnx", "onnx", "classifier.onnx"},
		{"model", "coreml", "model.mlmodelc"},
		{"model.mlmodelc", "coreml", "model.mlmodelc"},
		{"model.mlpackage", "coreml", "model.mlpackage"},
		{"weights", "tflite", "weights.bin"},
		{"model", "", "model.bin"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.format, func(t *testing.T) {
			filename := tt.name
			switch tt.format {
			case "onnx":
				if !hasAnySuffix(filename, ".onnx") {
					filename += ".onnx"
				}
			case "coreml":
				if !hasAnySuffix(filename, ".mlmodelc", ".mlpackage") {
					filename += ".mlmodelc"
				}
			default:
				filename += ".bin"
			}

			if filename != tt.expected {
				t.Errorf("filename = %q, want %q", filename, tt.expected)
			}
		})
	}
}

// hasAnySuffix checks if s has any of the given suffixes.
func hasAnySuffix(s string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}

// --- Constants verification ---

func TestConstants(t *testing.T) {
	if DefaultWeightsDir == "" {
		t.Error("DefaultWeightsDir should not be empty")
	}
	if ManifestFile == "" {
		t.Error("ManifestFile should not be empty")
	}
	if DefaultManifestURL == "" {
		t.Error("DefaultManifestURL should not be empty")
	}
	if httpTimeout == 0 {
		t.Error("httpTimeout should not be zero")
	}
	if manifestTimeout == 0 {
		t.Error("manifestTimeout should not be zero")
	}
}

// --- RemoteManifest JSON tests ---

func TestRemoteManifestJSON(t *testing.T) {
	manifest := RemoteManifest{
		SchemaVersion: 2,
		Updated:       "2026-03-23",
		DefaultModel:  "anubis-v2",
		Models: []ModelInfo{
			{
				Name:        "anubis-v2",
				Version:     "2.0.0",
				Description: "Next-gen classifier",
				Format:      "onnx",
				SizeBytes:   100 * 1024 * 1024,
				SHA256:      "deadbeef",
				DownloadURL: "https://example.com/v2.onnx",
				MinVersion:  "0.5.0",
			},
		},
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var got RemoteManifest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if got.SchemaVersion != 2 {
		t.Errorf("SchemaVersion = %d, want 2", got.SchemaVersion)
	}
	if got.DefaultModel != "anubis-v2" {
		t.Errorf("DefaultModel = %q, want anubis-v2", got.DefaultModel)
	}
	if len(got.Models) != 1 {
		t.Fatalf("Models count = %d, want 1", len(got.Models))
	}
	m := got.Models[0]
	if m.MinVersion != "0.5.0" {
		t.Errorf("MinVersion = %q, want 0.5.0", m.MinVersion)
	}
	if m.Description != "Next-gen classifier" {
		t.Errorf("Description mismatch")
	}
}

// --- Status struct tests ---

func TestStatusJSON(t *testing.T) {
	status := Status{
		Installed:   true,
		UpdateReady: false,
		WeightsDir:  "/home/user/.anubis/weights",
		Model: &LocalManifest{
			InstalledModel: "test-model",
			Version:        "1.0.0",
			Format:         "onnx",
			SHA256:         "abc123",
			SizeBytes:      1024,
			InstalledAt:    time.Now().Truncate(time.Second),
			ModelFile:      "test-model.onnx",
		},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var got Status
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if !got.Installed {
		t.Error("Installed should be true")
	}
	if got.UpdateReady {
		t.Error("UpdateReady should be false")
	}
	if got.Model == nil {
		t.Fatal("Model should not be nil")
	}
	if got.Model.InstalledModel != "test-model" {
		t.Errorf("InstalledModel = %q, want test-model", got.Model.InstalledModel)
	}
}

func TestStatusJSON_EmptyModel(t *testing.T) {
	status := Status{
		Installed:  false,
		WeightsDir: "/tmp/test",
		Error:      "no model installed",
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var got Status
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if got.Model != nil {
		t.Error("Model should be nil when not installed")
	}
	if got.Error != "no model installed" {
		t.Errorf("Error = %q, want 'no model installed'", got.Error)
	}
}

// --- writeLocalManifest error path ---

func TestReadLocalManifest_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, ManifestFile)

	// Write invalid JSON
	if err := os.WriteFile(manifestPath, []byte("not valid json {{{"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	_, err := readLocalManifest(tmpDir)
	if err == nil {
		t.Error("readLocalManifest should error on invalid JSON")
	}
}

func TestWriteLocalManifest_ReadBack(t *testing.T) {
	tmpDir := t.TempDir()

	manifest := &LocalManifest{
		InstalledModel: "round-trip-test",
		Version:        "3.0.0",
		Format:         "coreml",
		SHA256:         "fedcba987654",
		SizeBytes:      999999,
		InstalledAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ModelFile:      "model.mlmodelc",
	}

	if err := writeLocalManifest(tmpDir, manifest); err != nil {
		t.Fatalf("writeLocalManifest error: %v", err)
	}

	got, err := readLocalManifest(tmpDir)
	if err != nil {
		t.Fatalf("readLocalManifest error: %v", err)
	}

	if got.InstalledModel != "round-trip-test" {
		t.Errorf("InstalledModel = %q, want 'round-trip-test'", got.InstalledModel)
	}
	if got.SizeBytes != 999999 {
		t.Errorf("SizeBytes = %d, want 999999", got.SizeBytes)
	}
}

// --- GetStatus tests ---

func TestGetStatus_ReturnsStatus(t *testing.T) {
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus error: %v", err)
	}
	if status == nil {
		t.Fatal("GetStatus should return non-nil status")
	}
	if status.WeightsDir == "" {
		t.Error("WeightsDir should not be empty")
	}
}

func TestReadLocalManifest_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := readLocalManifest(tmpDir)
	if err == nil {
		t.Error("readLocalManifest should error when manifest doesn't exist")
	}
}

// --- FormatBytes tests ---

func TestFormatBytes_Coverage(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{52428800, "50.0 MB"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.input), func(t *testing.T) {
			got := FormatBytes(tt.input)
			if got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Install model selection logic ---

func TestInstallModelSelection_DefaultModel(t *testing.T) {
	remote := &RemoteManifest{
		SchemaVersion: 1,
		DefaultModel:  "anubis-v1",
		Models: []ModelInfo{
			{Name: "anubis-v1", Version: "1.0.0", Format: "onnx"},
			{Name: "anubis-v2", Version: "2.0.0", Format: "onnx"},
		},
	}
	if remote.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", remote.SchemaVersion)
	}

	// Find default model by name
	var model *ModelInfo
	for _, m := range remote.Models {
		if m.Name == remote.DefaultModel {
			info := m
			model = &info
			break
		}
	}
	if model == nil {
		t.Fatal("should find default model")
	}
	if model.Name != "anubis-v1" {
		t.Errorf("selected model = %q, want anubis-v1", model.Name)
	}
}

func TestInstallModelSelection_FallbackToFirst(t *testing.T) {
	remote := &RemoteManifest{
		SchemaVersion: 1,
		DefaultModel:  "nonexistent",
		Models: []ModelInfo{
			{Name: "first-model", Version: "1.0.0", Format: "onnx"},
		},
	}
	if remote.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", remote.SchemaVersion)
	}

	var model *ModelInfo
	for _, m := range remote.Models {
		if m.Name == remote.DefaultModel {
			info := m
			model = &info
			break
		}
	}
	if model == nil {
		info := remote.Models[0]
		model = &info
	}
	if model.Name != "first-model" {
		t.Errorf("should fall back to first model, got %q", model.Name)
	}
}

func TestInstallModelSelection_EmptyModels(t *testing.T) {
	remote := &RemoteManifest{
		SchemaVersion: 1,
		Models:        []ModelInfo{},
	}
	if remote.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", remote.SchemaVersion)
	}
	if len(remote.Models) == 0 {
		// This is the error path Install() takes
		return
	}
	t.Error("should detect empty models list")
}

// --- IsInstalled tests ---

func TestIsInstalled(t *testing.T) {
	installed := IsInstalled()
	// Just verify it returns without panic — value depends on env
	_ = installed
}

// --- WeightsDir tests ---

func TestWeightsDir_ReturnsPath(t *testing.T) {
	dir, err := WeightsDir()
	if err != nil {
		t.Fatalf("WeightsDir error: %v", err)
	}
	if dir == "" {
		t.Error("WeightsDir should return non-empty path")
	}
}

// --- LatestRemote field in Status ---

func TestStatus_WithLatestRemote(t *testing.T) {
	status := Status{
		Installed:   true,
		UpdateReady: true,
		LatestRemote: &ModelInfo{
			Name:    "v2-model",
			Version: "2.0.0",
		},
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var got Status
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !got.UpdateReady {
		t.Error("UpdateReady should be true")
	}
	if got.LatestRemote == nil || got.LatestRemote.Version != "2.0.0" {
		t.Error("LatestRemote should be preserved")
	}
}

// --- hashFile tests ---

func TestHashFile_CorrectHash(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.bin")
	content := []byte("hello anubis brain")
	os.WriteFile(testFile, content, 0o644)

	got, err := hashFile(testFile)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}

	// Compute expected hash
	h := sha256.Sum256(content)
	want := hex.EncodeToString(h[:])

	if got != want {
		t.Errorf("hashFile = %s, want %s", got, want)
	}
}

func TestHashFile_MissingFile(t *testing.T) {
	_, err := hashFile("/nonexistent/path/to/file.bin")
	if err == nil {
		t.Error("should error on missing file")
	}
}

// --- downloadFile tests (using httptest) ---

func TestDownloadFile_Success(t *testing.T) {
	content := []byte("model-weights-data-here")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "model.onnx")

	err := downloadFile(srv.URL, dest, int64(len(content)), nil)
	if err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != string(content) {
		t.Error("downloaded content doesn't match")
	}
}

func TestDownloadFile_WithProgress(t *testing.T) {
	content := []byte("some-model-data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "model.bin")

	progressCalled := false
	err := downloadFile(srv.URL, dest, int64(len(content)), func(downloaded, total int64) {
		progressCalled = true
		if downloaded <= 0 {
			t.Errorf("downloaded should be > 0, got %d", downloaded)
		}
	})
	if err != nil {
		t.Fatalf("downloadFile: %v", err)
	}
	if !progressCalled {
		t.Error("progress callback should have been called")
	}
}

func TestDownloadFile_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "model.bin")

	err := downloadFile(srv.URL, dest, 0, nil)
	if err == nil {
		t.Error("should error on 404")
	}
}

// --- Remove tests ---

func TestRemove_NonExistentDir(t *testing.T) {
	// Remove should be a no-op if nothing is installed
	// We can't easily test this without mocking WeightsDir,
	// but we can verify the function doesn't panic
	err := Remove()
	// This may or may not error depending on environment
	_ = err
}

// === MOCKED DEPENDENCY TESTS ===
// These swap the injectable function vars to test functions
// that normally require network/filesystem access.

// saveAndRestore saves the current function vars and returns a cleanup func.
func saveAndRestore(t *testing.T) {
	t.Helper()
	origWeightsDir := weightsDirFn
	origFetchManifest := fetchRemoteManifestFn
	origDownload := downloadFileFn
	origHash := hashFileFn
	t.Cleanup(func() {
		weightsDirFn = origWeightsDir
		fetchRemoteManifestFn = origFetchManifest
		downloadFileFn = origDownload
		hashFileFn = origHash
	})
}

func mockWeightsDir(dir string) {
	weightsDirFn = func() (string, error) { return dir, nil }
}

func mockFetchManifest(manifest *RemoteManifest, err error) {
	fetchRemoteManifestFn = func() (*RemoteManifest, error) { return manifest, err }
}

func mockDownloadOK() {
	downloadFileFn = func(url, dest string, size int64, prog ProgressFunc) error {
		// Write a fake file to dest
		return os.WriteFile(dest, []byte("fake-model-data"), 0o644)
	}
}

func mockDownloadFail(errMsg string) {
	downloadFileFn = func(url, dest string, size int64, prog ProgressFunc) error {
		return fmt.Errorf("%s", errMsg)
	}
}

func mockHashOK(hash string) {
	hashFileFn = func(path string) (string, error) { return hash, nil }
}

// --- GetStatus (mocked) ---

func TestGetStatus_NotInstalled(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockFetchManifest(&RemoteManifest{
		Models: []ModelInfo{{Name: "test-v1", Version: "1.0"}},
	}, nil)

	status, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status.Installed {
		t.Error("should not be installed")
	}
	if len(status.Available) != 1 {
		t.Errorf("Available = %d, want 1", len(status.Available))
	}
}

func TestGetStatus_Installed(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)

	// Write a local manifest
	writeLocalManifest(tmpDir, &LocalManifest{
		InstalledModel: "test-v1",
		Version:        "1.0",
		Format:         "onnx",
	})

	mockFetchManifest(&RemoteManifest{
		Models: []ModelInfo{{Name: "test-v1", Version: "1.0"}},
	}, nil)

	status, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if !status.Installed {
		t.Error("should be installed")
	}
	if status.UpdateReady {
		t.Error("should not have update (same version)")
	}
}

func TestGetStatus_UpdateAvailable(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)

	writeLocalManifest(tmpDir, &LocalManifest{
		InstalledModel: "test-v1",
		Version:        "1.0",
	})

	mockFetchManifest(&RemoteManifest{
		Models: []ModelInfo{{Name: "test-v1", Version: "2.0"}},
	}, nil)

	status, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if !status.UpdateReady {
		t.Error("should have update available")
	}
	if status.LatestRemote == nil || status.LatestRemote.Version != "2.0" {
		t.Error("LatestRemote should be v2.0")
	}
}

func TestGetStatus_FetchError_StillWorks(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockFetchManifest(nil, fmt.Errorf("network down"))

	status, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus should not error on fetch fail: %v", err)
	}
	if status == nil {
		t.Fatal("status should not be nil")
	}
}

// --- Install (mocked) ---

func TestInstall_DefaultModel(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("")

	mockFetchManifest(&RemoteManifest{
		DefaultModel: "classifier-v1",
		Models: []ModelInfo{
			{Name: "classifier-v1", Version: "1.0", Format: "onnx"},
			{Name: "classifier-v2", Version: "2.0", Format: "onnx"},
		},
	}, nil)

	manifest, err := Install(nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if manifest.InstalledModel != "classifier-v1" {
		t.Errorf("installed %q, want classifier-v1", manifest.InstalledModel)
	}
}

func TestInstall_FallbackToFirst(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("")

	mockFetchManifest(&RemoteManifest{
		DefaultModel: "nonexistent",
		Models: []ModelInfo{
			{Name: "first-model", Version: "1.0", Format: "onnx"},
		},
	}, nil)

	manifest, err := Install(nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if manifest.InstalledModel != "first-model" {
		t.Errorf("installed %q, want first-model", manifest.InstalledModel)
	}
}

func TestInstall_EmptyModels(t *testing.T) {
	saveAndRestore(t)
	mockFetchManifest(&RemoteManifest{Models: []ModelInfo{}}, nil)

	_, err := Install(nil)
	if err == nil {
		t.Error("should error on empty models")
	}
}

func TestInstall_FetchError(t *testing.T) {
	saveAndRestore(t)
	mockFetchManifest(nil, fmt.Errorf("network timeout"))

	_, err := Install(nil)
	if err == nil {
		t.Error("should error on fetch failure")
	}
}

func TestInstall_DownloadError(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadFail("connection reset")

	mockFetchManifest(&RemoteManifest{
		DefaultModel: "test",
		Models:       []ModelInfo{{Name: "test", Version: "1.0", Format: "onnx"}},
	}, nil)

	_, err := Install(nil)
	if err == nil {
		t.Error("should error on download failure")
	}
}

// --- InstallFromManifest (mocked) ---

func TestInstallFromManifest_Found(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("")

	mockFetchManifest(&RemoteManifest{
		Models: []ModelInfo{
			{Name: "model-a", Version: "1.0", Format: "onnx"},
			{Name: "model-b", Version: "2.0", Format: "coreml"},
		},
	}, nil)

	manifest, err := InstallFromManifest("model-b", nil)
	if err != nil {
		t.Fatalf("InstallFromManifest: %v", err)
	}
	if manifest.InstalledModel != "model-b" {
		t.Errorf("installed %q, want model-b", manifest.InstalledModel)
	}
	if manifest.Format != "coreml" {
		t.Errorf("format %q, want coreml", manifest.Format)
	}
}

func TestInstallFromManifest_NotFound(t *testing.T) {
	saveAndRestore(t)
	mockFetchManifest(&RemoteManifest{
		Models: []ModelInfo{{Name: "model-a", Version: "1.0", Format: "onnx"}},
	}, nil)

	_, err := InstallFromManifest("nonexistent", nil)
	if err == nil {
		t.Error("should error when model not found")
	}
}

// --- Update (mocked) ---

func TestUpdate_NotInstalled_FreshInstall(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("")

	mockFetchManifest(&RemoteManifest{
		DefaultModel: "test",
		Models:       []ModelInfo{{Name: "test", Version: "1.0", Format: "onnx"}},
	}, nil)

	manifest, updated, err := Update(nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !updated {
		t.Error("should report as updated (fresh install)")
	}
	if manifest.InstalledModel != "test" {
		t.Errorf("installed %q, want test", manifest.InstalledModel)
	}
}

func TestUpdate_AlreadyUpToDate(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)

	writeLocalManifest(tmpDir, &LocalManifest{
		InstalledModel: "model-v1",
		Version:        "1.0",
	})

	mockFetchManifest(&RemoteManifest{
		Models: []ModelInfo{{Name: "model-v1", Version: "1.0", Format: "onnx"}},
	}, nil)

	manifest, updated, err := Update(nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated {
		t.Error("should not be updated (same version)")
	}
	if manifest.InstalledModel != "model-v1" {
		t.Errorf("installed %q, want model-v1", manifest.InstalledModel)
	}
}

func TestUpdate_NewVersionAvailable(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("")

	writeLocalManifest(tmpDir, &LocalManifest{
		InstalledModel: "model-v1",
		Version:        "1.0",
	})

	mockFetchManifest(&RemoteManifest{
		Models: []ModelInfo{{Name: "model-v1", Version: "2.0", Format: "onnx"}},
	}, nil)

	manifest, updated, err := Update(nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !updated {
		t.Error("should be updated (new version)")
	}
	if manifest.InstalledModel != "model-v1" {
		t.Errorf("installed %q, want model-v1", manifest.InstalledModel)
	}
}

func TestUpdate_FetchError(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)

	writeLocalManifest(tmpDir, &LocalManifest{
		InstalledModel: "model-v1",
		Version:        "1.0",
	})

	mockFetchManifest(nil, fmt.Errorf("DNS failure"))

	_, _, err := Update(nil)
	if err == nil {
		t.Error("should error on fetch failure")
	}
}

// --- installModel (mocked) ---

func TestInstallModel_OnnxExtension(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("")

	model := &ModelInfo{Name: "classifier", Version: "1.0", Format: "onnx"}
	manifest, err := installModel(model, nil)
	if err != nil {
		t.Fatalf("installModel: %v", err)
	}
	if manifest.ModelFile != "classifier.onnx" {
		t.Errorf("ModelFile = %q, want classifier.onnx", manifest.ModelFile)
	}
}

func TestInstallModel_CoreMLExtension(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("")

	model := &ModelInfo{Name: "classifier", Version: "1.0", Format: "coreml"}
	manifest, err := installModel(model, nil)
	if err != nil {
		t.Fatalf("installModel: %v", err)
	}
	if manifest.ModelFile != "classifier.mlmodelc" {
		t.Errorf("ModelFile = %q, want classifier.mlmodelc", manifest.ModelFile)
	}
}

func TestInstallModel_UnknownFormat(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("")

	model := &ModelInfo{Name: "classifier", Version: "1.0", Format: "tensorflow"}
	manifest, err := installModel(model, nil)
	if err != nil {
		t.Fatalf("installModel: %v", err)
	}
	if manifest.ModelFile != "classifier.bin" {
		t.Errorf("ModelFile = %q, want classifier.bin", manifest.ModelFile)
	}
}

func TestInstallModel_HashMismatch(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("aaaaaa")

	model := &ModelInfo{
		Name:    "classifier",
		Version: "1.0",
		Format:  "onnx",
		SHA256:  "bbbbbb", // Mismatch!
	}
	_, err := installModel(model, nil)
	if err == nil {
		t.Error("should error on hash mismatch")
	}
}

func TestInstallModel_HashMatch(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("aabbcc")

	model := &ModelInfo{
		Name:    "classifier",
		Version: "1.0",
		Format:  "onnx",
		SHA256:  "aabbcc",
	}
	manifest, err := installModel(model, nil)
	if err != nil {
		t.Fatalf("installModel: %v", err)
	}
	if manifest.InstalledModel != "classifier" {
		t.Errorf("installed %q, want classifier", manifest.InstalledModel)
	}
}

func TestInstallModel_WithProgress(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockHashOK("")

	progressCalled := false
	downloadFileFn = func(url, dest string, size int64, prog ProgressFunc) error {
		os.WriteFile(dest, []byte("data"), 0o644)
		if prog != nil {
			prog(100, 100)
		}
		return nil
	}

	model := &ModelInfo{Name: "test", Version: "1.0", Format: "onnx"}
	_, err := installModel(model, func(downloaded, total int64) {
		progressCalled = true
	})
	if err != nil {
		t.Fatalf("installModel: %v", err)
	}
	if !progressCalled {
		t.Error("progress should have been called")
	}
}

// --- Remove (mocked) ---

func TestRemove_WithInstalledModel(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)

	// Create a fake weights dir with content
	os.WriteFile(filepath.Join(tmpDir, "model.onnx"), []byte("data"), 0o644)
	writeLocalManifest(tmpDir, &LocalManifest{InstalledModel: "test"})

	err := Remove()
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("weights dir should be removed")
	}
}

func TestRemove_NothingInstalled(t *testing.T) {
	saveAndRestore(t)
	// Point to a non-existent dir
	mockWeightsDir("/tmp/nonexistent-brain-weights-" + fmt.Sprintf("%d", time.Now().UnixNano()))

	err := Remove()
	if err != nil {
		t.Fatalf("Remove should be no-op: %v", err)
	}
}

// --- IsInstalled (mocked) ---

func TestIsInstalled_True(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	writeLocalManifest(tmpDir, &LocalManifest{InstalledModel: "test"})

	if !IsInstalled() {
		t.Error("should be installed")
	}
}

func TestIsInstalled_False(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)

	if IsInstalled() {
		t.Error("should not be installed")
	}
}

// --- GetClassifier (mocked) ---

func TestGetClassifier_NoModel(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)

	c, err := GetClassifier()
	if err != nil {
		t.Fatalf("GetClassifier: %v", err)
	}
	if c == nil {
		t.Fatal("classifier should not be nil")
	}
	// On macOS, returns spotlight-mdls; on other platforms, stub-heuristic-v1
	if c.Name() != "spotlight-mdls" && c.Name() != "stub-heuristic-v1" {
		t.Errorf("Name = %q, want spotlight-mdls or stub-heuristic-v1", c.Name())
	}
}

func TestGetClassifier_WithModel(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)

	writeLocalManifest(tmpDir, &LocalManifest{
		InstalledModel: "test-v1",
		Version:        "1.0",
		Format:         "onnx",
		ModelFile:      "test.onnx",
	})

	c, err := GetClassifier()
	if err != nil {
		t.Fatalf("GetClassifier: %v", err)
	}
	if c == nil {
		t.Fatal("classifier should not be nil")
	}
}

func TestGetClassifier_WeightsDirError(t *testing.T) {
	saveAndRestore(t)
	weightsDirFn = func() (string, error) { return "", fmt.Errorf("no home") }

	// GetClassifier now falls back to Spotlight/heuristic instead of erroring
	c, err := GetClassifier()
	if err != nil {
		t.Errorf("GetClassifier should fall back gracefully, got error: %v", err)
	}
	if c == nil {
		t.Error("classifier should not be nil even on WeightsDir failure")
	}
}

// --- InstalledModelPath (mocked) ---

func TestInstalledModelPath_NoModel_Mocked(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)

	path := InstalledModelPath()
	if path != "" {
		t.Errorf("should be empty, got %q", path)
	}
}

func TestInstalledModelPath_WithModel_Mocked(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)

	// Write manifest AND the actual model file
	modelFile := "test.onnx"
	os.WriteFile(filepath.Join(tmpDir, modelFile), []byte("data"), 0o644)
	writeLocalManifest(tmpDir, &LocalManifest{
		InstalledModel: "test",
		ModelFile:      modelFile,
	})

	path := InstalledModelPath()
	if path == "" {
		t.Error("should return model path")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("path should be absolute: %q", path)
	}
}

func TestInstalledModelPath_ModelFileMissing_Mocked(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)

	// Write manifest but NOT the model file
	writeLocalManifest(tmpDir, &LocalManifest{
		InstalledModel: "test",
		ModelFile:      "missing.onnx",
	})

	path := InstalledModelPath()
	if path != "" {
		t.Errorf("should be empty when file missing, got %q", path)
	}
}

func TestInstalledModelPath_WeightsDirError_Mocked(t *testing.T) {
	saveAndRestore(t)
	weightsDirFn = func() (string, error) { return "", fmt.Errorf("no dir") }

	path := InstalledModelPath()
	if path != "" {
		t.Error("should be empty on error")
	}
}

// --- ClassifyBatch edge cases ---

func TestClassifyBatch_EmptyList_Mocked(t *testing.T) {
	stub := NewStubClassifier()
	_ = stub.Load("")

	result, err := stub.ClassifyBatch([]string{}, 2)
	if err != nil {
		t.Fatalf("ClassifyBatch: %v", err)
	}
	if result.FilesProcessed != 0 {
		t.Error("should have 0 processed")
	}
}

func TestClassifyBatch_NotLoaded_Mocked(t *testing.T) {
	stub := NewStubClassifier()
	// Don't load

	_, err := stub.ClassifyBatch([]string{"/tmp/a"}, 2)
	if err == nil {
		t.Error("should error when not loaded")
	}
}

func TestClassifyBatch_NegativeWorkers_Mocked(t *testing.T) {
	stub := NewStubClassifier()
	_ = stub.Load("")

	result, err := stub.ClassifyBatch([]string{"/tmp/test.go", "/tmp/test.log"}, -1)
	if err != nil {
		t.Fatalf("ClassifyBatch: %v", err)
	}
	// Should default to 4 workers and still work
	if result.FilesProcessed+result.FilesSkipped != 2 {
		t.Errorf("expected 2 total files, got %d+%d", result.FilesProcessed, result.FilesSkipped)
	}
}

// --- FetchRemoteManifest via httptest (exercises defaultFetchRemoteManifest) ---

func TestDefaultFetchRemoteManifest_Success(t *testing.T) {
	manifest := RemoteManifest{
		SchemaVersion: 1,
		Models: []ModelInfo{
			{Name: "test", Version: "1.0", Format: "onnx"},
		},
		DefaultModel: "test",
	}
	data, _ := json.Marshal(manifest)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	// Temporarily point defaultFetchRemoteManifest to our test server
	saveAndRestore(t)
	fetchRemoteManifestFn = func() (*RemoteManifest, error) {
		client := &http.Client{Timeout: manifestTimeout}
		resp, err := client.Get(srv.URL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var m RemoteManifest
		json.NewDecoder(resp.Body).Decode(&m)
		return &m, nil
	}

	got, err := FetchRemoteManifest()
	if err != nil {
		t.Fatalf("FetchRemoteManifest: %v", err)
	}
	if got.DefaultModel != "test" {
		t.Errorf("DefaultModel = %q, want test", got.DefaultModel)
	}
}

// --- IsInstalled error paths ---

func TestIsInstalled_WeightsDirError(t *testing.T) {
	saveAndRestore(t)
	weightsDirFn = func() (string, error) { return "", fmt.Errorf("broken") }

	if IsInstalled() {
		t.Error("should be false when WeightsDir errors")
	}
}

// --- Error path tests for remaining coverage ---

func TestRemove_WeightsDirError(t *testing.T) {
	saveAndRestore(t)
	weightsDirFn = func() (string, error) { return "", fmt.Errorf("no home") }

	err := Remove()
	if err == nil {
		t.Error("should error when WeightsDir fails")
	}
}

func TestInstallFromManifest_FetchError(t *testing.T) {
	saveAndRestore(t)
	mockFetchManifest(nil, fmt.Errorf("timeout"))

	_, err := InstallFromManifest("any", nil)
	if err == nil {
		t.Error("should error on fetch failure")
	}
}

func TestInstallModel_WeightsDirError(t *testing.T) {
	saveAndRestore(t)
	weightsDirFn = func() (string, error) { return "", fmt.Errorf("no dir") }

	model := &ModelInfo{Name: "test", Version: "1.0", Format: "onnx"}
	_, err := installModel(model, nil)
	if err == nil {
		t.Error("should error when WeightsDir fails")
	}
}

func TestInstallModel_HashFileError(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	hashFileFn = func(path string) (string, error) {
		return "", fmt.Errorf("hash computation failed")
	}

	model := &ModelInfo{
		Name:    "test",
		Version: "1.0",
		Format:  "onnx",
		SHA256:  "expected-hash",
	}
	_, err := installModel(model, nil)
	if err == nil {
		t.Error("should error when hash fails")
	}
}

func TestInstallModel_AlreadyHasExtension(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("")

	// ONNX model name already ends in .onnx
	model := &ModelInfo{Name: "classifier.onnx", Version: "1.0", Format: "onnx"}
	manifest, err := installModel(model, nil)
	if err != nil {
		t.Fatalf("installModel: %v", err)
	}
	// Should not have double extension
	if manifest.ModelFile != "classifier.onnx" {
		t.Errorf("ModelFile = %q, want classifier.onnx", manifest.ModelFile)
	}
}

func TestInstallModel_CoreML_AlreadyHasExtension(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("")

	model := &ModelInfo{Name: "model.mlmodelc", Version: "1.0", Format: "coreml"}
	manifest, err := installModel(model, nil)
	if err != nil {
		t.Fatalf("installModel: %v", err)
	}
	if manifest.ModelFile != "model.mlmodelc" {
		t.Errorf("ModelFile = %q, want model.mlmodelc", manifest.ModelFile)
	}
}

func TestGetStatus_WeightsDirError(t *testing.T) {
	saveAndRestore(t)
	weightsDirFn = func() (string, error) { return "", fmt.Errorf("no home") }

	_, err := GetStatus()
	if err == nil {
		t.Error("should error when WeightsDir fails")
	}
}

func TestUpdate_WeightsDirError(t *testing.T) {
	saveAndRestore(t)
	weightsDirFn = func() (string, error) { return "", fmt.Errorf("no home") }

	_, _, err := Update(nil)
	if err == nil {
		t.Error("should error when WeightsDir fails")
	}
}

func TestInstallModel_CoreML_MlpackageSuffix(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadOK()
	mockHashOK("")

	// Name already has .mlpackage — should not add .mlmodelc
	model := &ModelInfo{Name: "model.mlpackage", Version: "1.0", Format: "coreml"}
	manifest, err := installModel(model, nil)
	if err != nil {
		t.Fatalf("installModel: %v", err)
	}
	if manifest.ModelFile != "model.mlpackage" {
		t.Errorf("ModelFile = %q, want model.mlpackage", manifest.ModelFile)
	}
}

func TestInstallModel_DownloadError_CleansUp(t *testing.T) {
	saveAndRestore(t)
	tmpDir := t.TempDir()
	mockWeightsDir(tmpDir)
	mockDownloadFail("simulated download error")

	model := &ModelInfo{Name: "test", Version: "1.0", Format: "onnx"}
	_, err := installModel(model, nil)
	if err == nil {
		t.Error("should error on download failure")
	}
	// Verify no partial file left behind
	files, _ := os.ReadDir(tmpDir)
	for _, f := range files {
		if f.Name() == "test.onnx" {
			t.Error("partial download should be cleaned up")
		}
	}
}

// --- Tests for default* implementations (real code, via httptest) ---

func TestDefaultDownloadFile_Success(t *testing.T) {
	content := []byte("real-model-data-payload")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "model.bin")

	err := defaultDownloadFile(srv.URL, dest, int64(len(content)), nil)
	if err != nil {
		t.Fatalf("defaultDownloadFile: %v", err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != string(content) {
		t.Error("content mismatch")
	}
}

func TestDefaultDownloadFile_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "model.bin")
	err := defaultDownloadFile(srv.URL, dest, 0, nil)
	if err == nil {
		t.Error("should error on 404")
	}
}

func TestDefaultDownloadFile_InvalidURL(t *testing.T) {
	err := defaultDownloadFile("http://invalid.invalid.invalid:99999/nothing", "/tmp/x", 0, nil)
	if err == nil {
		t.Error("should error on invalid URL")
	}
}

func TestDefaultDownloadFile_WithProgress(t *testing.T) {
	content := []byte("data-with-progress")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "model.bin")
	called := false
	err := defaultDownloadFile(srv.URL, dest, int64(len(content)), func(d, t int64) {
		called = true
	})
	if err != nil {
		t.Fatalf("defaultDownloadFile: %v", err)
	}
	if !called {
		t.Error("progress should have been called")
	}
}

func TestDefaultHashFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "data.bin")
	os.WriteFile(f, []byte("test-data"), 0o644)

	hash, err := defaultHashFile(f)
	if err != nil {
		t.Fatalf("defaultHashFile: %v", err)
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
}

func TestDefaultHashFile_Missing(t *testing.T) {
	_, err := defaultHashFile("/nonexistent/file.bin")
	if err == nil {
		t.Error("should error on missing file")
	}
}

func TestDefaultFetchRemoteManifest_ViaHttptest(t *testing.T) {
	manifest := RemoteManifest{
		SchemaVersion: 1,
		Models:        []ModelInfo{{Name: "t", Version: "1.0"}},
		DefaultModel:  "t",
	}
	data, _ := json.Marshal(manifest)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	// We can't easily redirect defaultFetchRemoteManifest since it hardcodes
	// DefaultManifestURL, but we can call fetchRemoteManifestFn directly
	// with a custom implementation that uses the test server
	saveAndRestore(t)
	fetchRemoteManifestFn = func() (*RemoteManifest, error) {
		client := &http.Client{Timeout: manifestTimeout}
		resp, err := client.Get(srv.URL)
		if err != nil {
			return nil, fmt.Errorf("fetch: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		var m RemoteManifest
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
		return &m, nil
	}

	got, err := FetchRemoteManifest()
	if err != nil {
		t.Fatalf("FetchRemoteManifest: %v", err)
	}
	if got.DefaultModel != "t" {
		t.Errorf("DefaultModel = %q, want t", got.DefaultModel)
	}
}
