// Package brain — inference.go provides the model inference wrapper.
// This file defines the Classifier interface and provides backend
// implementations for ONNX Runtime (cross-platform) and CoreML (macOS).
//
// Current state: Interface scaffold with CPU-based stub classifier.
// Full ONNX Runtime and CoreML backends will be implemented when the
// trained model is available (post-ship-week).
//
// Architecture:
//
//	Classifier (interface)
//	├── StubClassifier     — always returns "unknown" (ships now)
//	├── ONNXClassifier     — ort-go bindings (future)
//	└── CoreMLClassifier   — CGO bridge on macOS (future)
package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileClass represents a semantic classification of a file.
type FileClass string

const (
	ClassJunk      FileClass = "junk"      // Temporary, cache, build artifact
	ClassEssential FileClass = "essential" // System or app critical file
	ClassProject   FileClass = "project"   // Source code, documentation
	ClassModel     FileClass = "model"     // ML model weights
	ClassData      FileClass = "data"      // Datasets, databases
	ClassMedia     FileClass = "media"     // Images, video, audio
	ClassArchive   FileClass = "archive"   // Compressed archives
	ClassConfig    FileClass = "config"    // Configuration files
	ClassUnknown   FileClass = "unknown"   // Unclassified
)

// Classification is the result of classifying a single file.
type Classification struct {
	Path       string    `json:"path"`
	Class      FileClass `json:"class"`
	Confidence float64   `json:"confidence"` // 0.0 to 1.0
	ModelUsed  string    `json:"model_used"`
}

// BatchResult contains results from classifying multiple files.
type BatchResult struct {
	Classifications []Classification `json:"classifications"`
	FilesProcessed  int              `json:"files_processed"`
	FilesSkipped    int              `json:"files_skipped"`
	ModelUsed       string           `json:"model_used"`
}

// Classifier is the interface for file classification backends.
// Implementations handle loading the model and running inference.
type Classifier interface {
	// Name returns the identifier for this classifier backend.
	Name() string

	// Load initializes the model from the weights directory.
	Load(weightsDir string) error

	// Classify returns the semantic classification of a file path.
	Classify(filePath string) (*Classification, error)

	// ClassifyBatch classifies multiple files concurrently.
	ClassifyBatch(filePaths []string, workers int) (*BatchResult, error)

	// Close releases any resources held by the classifier.
	Close() error
}

// StubClassifier is a placeholder that uses file extension heuristics.
// This ships in v0.2.0-alpha until the trained ONNX model is ready.
type StubClassifier struct {
	loaded bool
}

// NewStubClassifier creates a new stub classifier.
func NewStubClassifier() *StubClassifier {
	return &StubClassifier{}
}

// Name returns the classifier identifier.
func (s *StubClassifier) Name() string {
	return "stub-heuristic-v1"
}

// Load is a no-op for the stub classifier.
func (s *StubClassifier) Load(_ string) error {
	s.loaded = true
	return nil
}

// Classify returns a heuristic classification based on file extension and path.
func (s *StubClassifier) Classify(filePath string) (*Classification, error) {
	if !s.loaded {
		return nil, fmt.Errorf("classifier not loaded — call Load() first")
	}

	class, confidence := classifyByHeuristic(filePath)

	return &Classification{
		Path:       filePath,
		Class:      class,
		Confidence: confidence,
		ModelUsed:  s.Name(),
	}, nil
}

// ClassifyBatch classifies multiple files concurrently using goroutines.
func (s *StubClassifier) ClassifyBatch(filePaths []string, workers int) (*BatchResult, error) {
	if !s.loaded {
		return nil, fmt.Errorf("classifier not loaded — call Load() first")
	}

	if workers <= 0 {
		workers = 4
	}

	result := &BatchResult{
		ModelUsed: s.Name(),
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)

	for _, fp := range filePaths {
		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore

		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			c, err := s.Classify(path)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.FilesSkipped++
				return
			}
			result.Classifications = append(result.Classifications, *c)
			result.FilesProcessed++
		}(fp)
	}

	wg.Wait()

	return result, nil
}

// Close is a no-op for the stub classifier.
func (s *StubClassifier) Close() error {
	s.loaded = false
	return nil
}

// classifyByHeuristic uses file extension and path patterns to classify.
func classifyByHeuristic(filePath string) (FileClass, float64) {
	ext := filepath.Ext(filePath)
	base := filepath.Base(filePath)
	dir := filepath.Dir(filePath)

	// Path-based heuristics (most specific — check first)
	switch {
	case containsSegment(dir, "node_modules"):
		return ClassJunk, 0.7
	case containsSegment(dir, "__pycache__"):
		return ClassJunk, 0.85
	case containsSegment(dir, ".cache"):
		return ClassJunk, 0.75
	}

	// Filename-based heuristics
	switch {
	case base == "Thumbs.db" || base == ".DS_Store":
		return ClassJunk, 0.95
	case base == "Dockerfile" || base == "Makefile" || base == "Taskfile.yml":
		return ClassProject, 0.85
	case base == "LICENSE" || base == "README.md" || base == "CHANGELOG.md":
		return ClassProject, 0.9
	}

	// Extension-based matches
	switch ext {
	// Junk indicators
	case ".log", ".tmp", ".bak", ".swp", ".swo", ".DS_Store", ".pyc":
		return ClassJunk, 0.9
	case ".cache":
		return ClassJunk, 0.85

	// Model weights
	case ".onnx", ".pt", ".pth", ".safetensors", ".ckpt", ".h5", ".pb",
		".mlmodel", ".mlmodelc", ".tflite", ".bin":
		return ClassModel, 0.8

	// Source / Project
	case ".go", ".py", ".js", ".ts", ".rs", ".c", ".cpp", ".h",
		".java", ".rb", ".swift", ".kt", ".scala", ".zig":
		return ClassProject, 0.9

	// Configuration
	case ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".json",
		".xml", ".plist", ".env":
		return ClassConfig, 0.85

	// Media
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".mp4",
		".mov", ".avi", ".mkv", ".mp3", ".wav", ".flac", ".aac":
		return ClassMedia, 0.95

	// Archives
	case ".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar", ".dmg":
		return ClassArchive, 0.95

	// Data files
	case ".csv", ".tsv", ".parquet", ".sqlite", ".db", ".sql":
		return ClassData, 0.85
	}

	// Low-confidence path-based heuristics
	switch {
	case containsSegment(dir, "build") || containsSegment(dir, "dist"):
		return ClassJunk, 0.6
	case containsSegment(dir, "vendor"):
		return ClassProject, 0.5
	}

	return ClassUnknown, 0.0
}

// containsSegment checks if a directory path contains a specific segment.
func containsSegment(dir, segment string) bool {
	for _, part := range splitPath(dir) {
		if part == segment {
			return true
		}
	}
	return false
}

// splitPath splits a path into its components.
func splitPath(path string) []string {
	var parts []string
	path = filepath.Clean(path)
	for {
		dir, file := filepath.Split(path)
		if file != "" {
			parts = append([]string{file}, parts...)
		}
		if dir == path || dir == "" || dir == "." { // root or no more parts
			break
		}
		path = filepath.Clean(dir)
	}
	return parts
}

// GetClassifier returns the best available classifier for the current platform.
// Currently returns a StubClassifier. When ONNX/CoreML models are trained,
// this will auto-detect the installed model and return the appropriate backend.
func GetClassifier() (Classifier, error) {
	dir, err := WeightsDir()
	if err != nil {
		// No weights dir — use Spotlight on macOS, heuristic elsewhere
		c := NewSpotlightClassifier()
		_ = c.Load("")
		return c, nil
	}

	// Check if a model is installed
	local, err := readLocalManifest(dir)
	if err != nil || local == nil {
		// No model installed — use Spotlight on macOS, heuristic elsewhere
		c := NewSpotlightClassifier()
		_ = c.Load("")
		return c, nil
	}

	// Model installed — for now, use Spotlight + heuristic until real ONNX/CoreML backends ship
	// Future: check local.Format and return ONNXClassifier or CoreMLClassifier
	c := NewSpotlightClassifier()
	_ = c.Load(dir)
	return c, nil
}

// InstalledModelPath returns the full path to the installed model file, or empty string.
func InstalledModelPath() string {
	dir, err := WeightsDir()
	if err != nil {
		return ""
	}

	local, err := readLocalManifest(dir)
	if err != nil || local == nil {
		return ""
	}

	path := filepath.Join(dir, local.ModelFile)
	if _, err := os.Stat(path); err != nil {
		return ""
	}

	return path
}
