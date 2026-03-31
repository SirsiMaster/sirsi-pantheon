// Package brain — spotlight.go provides macOS Spotlight-accelerated file classification.
//
// On Apple Silicon, Spotlight uses CoreML-backed content analysis that runs on
// the Neural Engine. By querying mdls (metadata listing), we get classification
// that's genuinely hardware-accelerated without shipping our own model.
//
// This classifier falls back to the heuristic StubClassifier on non-macOS
// platforms or when mdls is unavailable.
package brain

import (
	"os/exec"
	"runtime"
	"strings"
)

// SpotlightClassifier uses macOS Spotlight metadata for file classification.
// On Apple Silicon, Spotlight's content analysis runs on the ANE via CoreML.
type SpotlightClassifier struct {
	stub *StubClassifier // fallback for non-macOS or mdls failure
}

// NewSpotlightClassifier creates a classifier that uses Spotlight when available.
func NewSpotlightClassifier() *SpotlightClassifier {
	return &SpotlightClassifier{stub: NewStubClassifier()}
}

func (s *SpotlightClassifier) Name() string {
	if runtime.GOOS == "darwin" {
		return "spotlight-mdls"
	}
	return s.stub.Name()
}

func (s *SpotlightClassifier) Load(weightsDir string) error {
	return s.stub.Load(weightsDir)
}

func (s *SpotlightClassifier) Close() error {
	return s.stub.Close()
}

// Classify uses mdls on macOS to get the system content type, then maps
// that to a FileClass. Falls back to heuristic classification if mdls fails.
func (s *SpotlightClassifier) Classify(filePath string) (*Classification, error) {
	if runtime.GOOS != "darwin" {
		return s.stub.Classify(filePath)
	}

	contentType, err := queryMdls(filePath)
	if err != nil || contentType == "" {
		return s.stub.Classify(filePath)
	}

	class, confidence := mapContentType(contentType)
	if class == ClassUnknown {
		// Fall back to heuristic for unknown types
		return s.stub.Classify(filePath)
	}

	return &Classification{
		Path:       filePath,
		Class:      class,
		Confidence: confidence,
		ModelUsed:  "spotlight-mdls",
	}, nil
}

func (s *SpotlightClassifier) ClassifyBatch(filePaths []string, workers int) (*BatchResult, error) {
	if workers <= 0 {
		workers = 4
	}

	// On macOS, use batch mdls for system-accelerated classification
	if runtime.GOOS == "darwin" && len(filePaths) > 0 {
		return s.batchMdls(filePaths, workers)
	}

	return s.stub.ClassifyBatch(filePaths, workers)
}

// batchMdls classifies files using batch mdls queries.
func (s *SpotlightClassifier) batchMdls(filePaths []string, workers int) (*BatchResult, error) {
	result := &BatchResult{ModelUsed: "spotlight-mdls"}

	// Process in chunks to avoid argument length limits
	chunkSize := 50
	for i := 0; i < len(filePaths); i += chunkSize {
		end := i + chunkSize
		if end > len(filePaths) {
			end = len(filePaths)
		}
		chunk := filePaths[i:end]

		args := append([]string{"-name", "kMDItemContentType"}, chunk...)
		out, err := exec.Command("mdls", args...).Output()
		if err != nil {
			// Fall back to heuristic for this chunk
			for _, fp := range chunk {
				c, cerr := s.stub.Classify(fp)
				if cerr != nil {
					result.FilesSkipped++
					continue
				}
				result.Classifications = append(result.Classifications, *c)
				result.FilesProcessed++
			}
			continue
		}

		// Parse mdls output — each file gets a "kMDItemContentType = ..." line
		// or "(null)" if not indexed
		lines := strings.Split(string(out), "\n")
		lineIdx := 0
		for _, fp := range chunk {
			if lineIdx >= len(lines) {
				result.FilesSkipped++
				continue
			}
			line := strings.TrimSpace(lines[lineIdx])
			lineIdx++

			contentType := ""
			if strings.Contains(line, "=") && !strings.Contains(line, "(null)") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					contentType = strings.Trim(strings.TrimSpace(parts[1]), "\"")
				}
			}

			if contentType == "" {
				// Fall back to heuristic
				c, cerr := s.stub.Classify(fp)
				if cerr != nil {
					result.FilesSkipped++
					continue
				}
				result.Classifications = append(result.Classifications, *c)
			} else {
				class, confidence := mapContentType(contentType)
				if class == ClassUnknown {
					c, _ := s.stub.Classify(fp)
					if c != nil {
						result.Classifications = append(result.Classifications, *c)
					}
				} else {
					result.Classifications = append(result.Classifications, Classification{
						Path:       fp,
						Class:      class,
						Confidence: confidence,
						ModelUsed:  "spotlight-mdls",
					})
				}
			}
			result.FilesProcessed++
		}
	}

	return result, nil
}

// queryMdls runs mdls on a single file and returns the content type.
func queryMdls(path string) (string, error) {
	out, err := exec.Command("mdls", "-name", "kMDItemContentType", "-raw", path).Output()
	if err != nil {
		return "", err
	}
	result := strings.TrimSpace(string(out))
	if result == "(null)" {
		return "", nil
	}
	return result, nil
}

// mapContentType converts a macOS Uniform Type Identifier to a FileClass.
// UTIs are hierarchical: public.image conforms-to public.data conforms-to public.item
func mapContentType(uti string) (FileClass, float64) {
	switch {
	// Junk / temporary
	case strings.Contains(uti, "cache"),
		strings.Contains(uti, "log"),
		strings.Contains(uti, "temporary"):
		return ClassJunk, 0.85

	// Source code
	case uti == "public.source-code",
		uti == "public.c-source",
		uti == "public.c-header",
		uti == "public.objective-c-source",
		uti == "public.swift-source",
		uti == "public.python-script",
		uti == "public.ruby-script",
		uti == "public.perl-script",
		uti == "public.shell-script",
		uti == "public.script",
		strings.HasPrefix(uti, "com.sun.java"),
		strings.Contains(uti, "source-code"),
		strings.Contains(uti, "sourcecode"):
		return ClassProject, 0.90

	// Text/documentation
	case uti == "public.plain-text",
		uti == "public.utf8-plain-text",
		uti == "public.html",
		uti == "net.daringfireball.markdown",
		uti == "public.xml",
		uti == "public.json":
		return ClassProject, 0.70

	// Configuration
	case uti == "public.yaml",
		strings.Contains(uti, "property-list"),
		strings.Contains(uti, "preferences"):
		return ClassConfig, 0.85

	// Images
	case strings.HasPrefix(uti, "public.image"),
		uti == "public.jpeg",
		uti == "public.png",
		uti == "public.tiff",
		uti == "com.compuserve.gif",
		uti == "public.svg-image",
		uti == "public.heic",
		uti == "com.apple.icns":
		return ClassMedia, 0.90

	// Audio/Video
	case strings.HasPrefix(uti, "public.audio"),
		strings.HasPrefix(uti, "public.video"),
		strings.HasPrefix(uti, "public.movie"),
		uti == "public.mp3",
		uti == "public.mpeg-4",
		uti == "public.mpeg-4-audio",
		uti == "com.apple.quicktime-movie":
		return ClassMedia, 0.90

	// Archives
	case uti == "public.zip-archive",
		uti == "org.gnu.gnu-tar-archive",
		uti == "public.tar-archive",
		uti == "com.apple.disk-image",
		strings.Contains(uti, "archive"),
		strings.Contains(uti, "compressed"):
		return ClassArchive, 0.90

	// ML models
	case strings.Contains(uti, "mlmodel"),
		strings.Contains(uti, "coreml"):
		return ClassModel, 0.95

	// Data
	case uti == "public.database",
		uti == "public.comma-separated-values-text",
		strings.Contains(uti, "sqlite"),
		strings.Contains(uti, "database"):
		return ClassData, 0.85

	// Executables
	case uti == "public.executable",
		uti == "public.unix-executable",
		uti == "com.apple.mach-o-binary",
		uti == "com.microsoft.windows-executable":
		return ClassEssential, 0.80
	}

	return ClassUnknown, 0.0
}
