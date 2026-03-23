package brain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- InstalledModelPath tests ---

func TestInstalledModelPath_NoManifest(t *testing.T) {
	// When no model is installed, should return empty string
	path := InstalledModelPath()
	// We can't guarantee the state is empty on CI, but verify it doesn't panic
	_ = path
}

func TestInstalledModelPath_WithManifest(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a manifest pointing to a model file
	manifest := &LocalManifest{
		InstalledModel: "test-model",
		Version:        "1.0.0",
		Format:         "onnx",
		SHA256:         "abc123",
		SizeBytes:      1024,
		ModelFile:      "test-model.onnx",
	}

	if err := writeLocalManifest(tmpDir, manifest); err != nil {
		t.Fatalf("writeLocalManifest error: %v", err)
	}

	// Create the model file
	modelPath := filepath.Join(tmpDir, "test-model.onnx")
	if err := os.WriteFile(modelPath, []byte("fake-model-data"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	// Read back the manifest and verify the model path resolution
	got, err := readLocalManifest(tmpDir)
	if err != nil {
		t.Fatalf("readLocalManifest error: %v", err)
	}

	resolvedPath := filepath.Join(tmpDir, got.ModelFile)
	if _, statErr := os.Stat(resolvedPath); os.IsNotExist(statErr) {
		t.Error("Resolved model path should exist")
	}
}

func TestInstalledModelPath_ModelFileMissing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create manifest but NOT the model file
	manifest := &LocalManifest{
		InstalledModel: "ghost-model",
		Version:        "1.0.0",
		Format:         "onnx",
		ModelFile:      "ghost-model.onnx",
	}
	if err := writeLocalManifest(tmpDir, manifest); err != nil {
		t.Fatalf("writeLocalManifest error: %v", err)
	}

	// Read manifest and verify the model file doesn't exist
	got, err := readLocalManifest(tmpDir)
	if err != nil {
		t.Fatalf("readLocalManifest error: %v", err)
	}

	resolvedPath := filepath.Join(tmpDir, got.ModelFile)
	if _, statErr := os.Stat(resolvedPath); !os.IsNotExist(statErr) {
		t.Error("Model file should NOT exist when only manifest is present")
	}
}

// --- classifyByHeuristic coverage for remaining branches ---

func TestClassifyByHeuristic_CachePath(t *testing.T) {
	class, confidence := classifyByHeuristic("/home/user/.cache/something/file.bin")
	if class != ClassJunk {
		t.Errorf("Expected ClassJunk for .cache path, got %q", class)
	}
	if confidence <= 0 {
		t.Error("Confidence should be > 0")
	}
}

func TestClassifyByHeuristic_BuildPath(t *testing.T) {
	class, confidence := classifyByHeuristic("/project/build/output/binary")
	if class != ClassJunk {
		t.Errorf("Expected ClassJunk for build path, got %q", class)
	}
	if confidence <= 0 {
		t.Error("Confidence should be > 0")
	}
}

func TestClassifyByHeuristic_DistPath(t *testing.T) {
	class, confidence := classifyByHeuristic("/project/dist/bundle.js.map")
	// The dist path heuristic should fire (low confidence junk)
	if class != ClassJunk {
		t.Errorf("Expected ClassJunk for dist path, got %q", class)
	}
	if confidence <= 0 {
		t.Error("Confidence should be > 0")
	}
}

func TestClassifyByHeuristic_VendorPath(t *testing.T) {
	class, _ := classifyByHeuristic("/project/vendor/github.com/pkg/errors/errors")
	if class != ClassProject {
		t.Errorf("Expected ClassProject for vendor path, got %q", class)
	}
}

func TestClassifyByHeuristic_ThumbsDb(t *testing.T) {
	class, confidence := classifyByHeuristic("/some/dir/Thumbs.db")
	if class != ClassJunk {
		t.Errorf("Expected ClassJunk for Thumbs.db, got %q", class)
	}
	if confidence < 0.9 {
		t.Errorf("Thumbs.db confidence should be >= 0.9, got %f", confidence)
	}
}

func TestClassifyByHeuristic_Makefile(t *testing.T) {
	class, _ := classifyByHeuristic("/project/Makefile")
	if class != ClassProject {
		t.Errorf("Expected ClassProject for Makefile, got %q", class)
	}
}

func TestClassifyByHeuristic_TaskfileYml(t *testing.T) {
	// Taskfile.yml should match filename heuristic before extension heuristic
	class, _ := classifyByHeuristic("/project/Taskfile.yml")
	if class != ClassProject {
		t.Errorf("Expected ClassProject for Taskfile.yml, got %q", class)
	}
}

func TestClassifyByHeuristic_CHANGELOG(t *testing.T) {
	class, _ := classifyByHeuristic("/project/CHANGELOG.md")
	if class != ClassProject {
		t.Errorf("Expected ClassProject for CHANGELOG.md, got %q", class)
	}
}

func TestClassifyByHeuristic_CacheExtension(t *testing.T) {
	class, _ := classifyByHeuristic("/tmp/something.cache")
	if class != ClassJunk {
		t.Errorf("Expected ClassJunk for .cache extension, got %q", class)
	}
}

func TestClassifyByHeuristic_ModelWeights(t *testing.T) {
	extensions := []string{".pt", ".pth", ".safetensors", ".ckpt", ".h5", ".pb",
		".mlmodel", ".mlmodelc", ".tflite", ".bin"}

	for _, ext := range extensions {
		t.Run(ext, func(t *testing.T) {
			class, _ := classifyByHeuristic("/models/model" + ext)
			if class != ClassModel {
				t.Errorf("Expected ClassModel for %s, got %q", ext, class)
			}
		})
	}
}

func TestClassifyByHeuristic_SourceLanguages(t *testing.T) {
	extensions := []string{".c", ".cpp", ".h", ".java", ".rb", ".swift", ".kt", ".scala", ".zig"}

	for _, ext := range extensions {
		t.Run(ext, func(t *testing.T) {
			class, _ := classifyByHeuristic("/src/code" + ext)
			if class != ClassProject {
				t.Errorf("Expected ClassProject for %s, got %q", ext, class)
			}
		})
	}
}

func TestClassifyByHeuristic_ConfigFormats(t *testing.T) {
	files := map[string]string{
		"config.ini":    ".ini",
		"settings.cfg":  ".cfg",
		"app.conf":      ".conf",
		"schema.xml":    ".xml",
		"Info.plist":    ".plist",
		"variables.env": ".env",
	}

	for name, ext := range files {
		t.Run(ext, func(t *testing.T) {
			class, _ := classifyByHeuristic("/etc/" + name)
			if class != ClassConfig {
				t.Errorf("Expected ClassConfig for %s, got %q", name, class)
			}
		})
	}
}

func TestClassifyByHeuristic_MediaFormats(t *testing.T) {
	extensions := []string{".jpeg", ".png", ".gif", ".webp", ".svg", ".mov",
		".avi", ".mkv", ".wav", ".flac", ".aac"}

	for _, ext := range extensions {
		t.Run(ext, func(t *testing.T) {
			class, _ := classifyByHeuristic("/media/file" + ext)
			if class != ClassMedia {
				t.Errorf("Expected ClassMedia for %s, got %q", ext, class)
			}
		})
	}
}

func TestClassifyByHeuristic_ArchiveFormats(t *testing.T) {
	extensions := []string{".gz", ".bz2", ".xz", ".7z", ".rar"}

	for _, ext := range extensions {
		t.Run(ext, func(t *testing.T) {
			class, _ := classifyByHeuristic("/downloads/archive" + ext)
			if class != ClassArchive {
				t.Errorf("Expected ClassArchive for %s, got %q", ext, class)
			}
		})
	}
}

func TestClassifyByHeuristic_DataFormats(t *testing.T) {
	extensions := []string{".tsv", ".parquet", ".db", ".sql"}

	for _, ext := range extensions {
		t.Run(ext, func(t *testing.T) {
			class, _ := classifyByHeuristic("/data/dataset" + ext)
			if class != ClassData {
				t.Errorf("Expected ClassData for %s, got %q", ext, class)
			}
		})
	}
}

func TestClassifyByHeuristic_JunkExtensions(t *testing.T) {
	extensions := []string{".swp", ".swo", ".pyc"}

	for _, ext := range extensions {
		t.Run(ext, func(t *testing.T) {
			class, _ := classifyByHeuristic("/tmp/file" + ext)
			if class != ClassJunk {
				t.Errorf("Expected ClassJunk for %s, got %q", ext, class)
			}
		})
	}
}

// --- FileClass and Classification struct tests ---

func TestFileClassConstants(t *testing.T) {
	classes := []FileClass{
		ClassJunk, ClassEssential, ClassProject, ClassModel,
		ClassData, ClassMedia, ClassArchive, ClassConfig, ClassUnknown,
	}

	seen := make(map[FileClass]bool)
	for _, c := range classes {
		if c == "" {
			t.Error("FileClass constant should not be empty")
		}
		if seen[c] {
			t.Errorf("Duplicate FileClass constant: %q", c)
		}
		seen[c] = true
	}

	if len(classes) != 9 {
		t.Errorf("Expected 9 FileClass constants, got %d", len(classes))
	}
}

func TestClassificationJSON(t *testing.T) {
	c := Classification{
		Path:       "/test/file.go",
		Class:      ClassProject,
		Confidence: 0.9,
		ModelUsed:  "stub-heuristic-v1",
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var got Classification
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if got.Path != c.Path || got.Class != c.Class {
		t.Error("Classification JSON round-trip mismatch")
	}
}

func TestBatchResultJSON(t *testing.T) {
	br := BatchResult{
		Classifications: []Classification{
			{Path: "/a.go", Class: ClassProject, Confidence: 0.9, ModelUsed: "stub"},
			{Path: "/b.log", Class: ClassJunk, Confidence: 0.8, ModelUsed: "stub"},
		},
		FilesProcessed: 2,
		FilesSkipped:   0,
		ModelUsed:      "stub",
	}

	data, err := json.Marshal(br)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var got BatchResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if got.FilesProcessed != 2 {
		t.Errorf("FilesProcessed = %d, want 2", got.FilesProcessed)
	}
	if len(got.Classifications) != 2 {
		t.Errorf("Classifications count = %d, want 2", len(got.Classifications))
	}
}

// --- containsSegment tests ---

func TestContainsSegment_EdgeCases(t *testing.T) {
	tests := []struct {
		dir      string
		segment  string
		expected bool
	}{
		{"/a/b/c", "b", true},
		{"/a/b/c", "d", false},
		{"/", "root", false},
		{"/node_modules/pkg", "node_modules", true},
		{"/a/node_modules_extra/b", "node_modules", false},
	}

	for _, tt := range tests {
		t.Run(tt.dir+"_"+tt.segment, func(t *testing.T) {
			got := containsSegment(tt.dir, tt.segment)
			if got != tt.expected {
				t.Errorf("containsSegment(%q, %q) = %v, want %v",
					tt.dir, tt.segment, got, tt.expected)
			}
		})
	}
}

// --- ClassifyBatch with empty input ---

func TestStubClassifier_ClassifyBatch_Empty(t *testing.T) {
	c := NewStubClassifier()
	_ = c.Load("")
	defer c.Close()

	result, err := c.ClassifyBatch([]string{}, 2)
	if err != nil {
		t.Fatalf("ClassifyBatch error: %v", err)
	}
	if result.FilesProcessed != 0 {
		t.Errorf("FilesProcessed = %d, want 0", result.FilesProcessed)
	}
	if len(result.Classifications) != 0 {
		t.Errorf("Classifications should be empty, got %d", len(result.Classifications))
	}
}

// --- ClassifyBatch with negative workers ---

func TestStubClassifier_ClassifyBatch_NegativeWorkers(t *testing.T) {
	c := NewStubClassifier()
	_ = c.Load("")
	defer c.Close()

	result, err := c.ClassifyBatch([]string{"/test.go"}, -1)
	if err != nil {
		t.Fatalf("ClassifyBatch error: %v", err)
	}
	if result.FilesProcessed != 1 {
		t.Errorf("FilesProcessed = %d, want 1", result.FilesProcessed)
	}
}
