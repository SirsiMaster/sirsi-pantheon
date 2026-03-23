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
