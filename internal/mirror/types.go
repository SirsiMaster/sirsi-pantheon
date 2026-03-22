// Package mirror provides file deduplication with semantic importance ranking.
// Named after the sacred Egyptian copper mirrors — tools of truth that reveal
// what is real and what is merely a reflection.
//
// Free tier: SHA-256 hash dedup + perceptual hashing
// Pro tier: ANE-powered importance ranking + knowledge graph
//
// Rule A1: all dedup artifacts are self-deletable
// Rule A11: no telemetry — all analysis stays on-device
package mirror

import (
	"time"
)

// MediaType categorizes files for specialized handling.
type MediaType string

const (
	MediaPhoto    MediaType = "photo"
	MediaMusic    MediaType = "music"
	MediaVideo    MediaType = "video"
	MediaDocument MediaType = "document"
	MediaOther    MediaType = "other"
)

// MatchType describes how duplicates were identified.
type MatchType string

const (
	MatchExact      MatchType = "exact"      // SHA-256 identical
	MatchPerceptual MatchType = "perceptual" // pHash similar (images)
	MatchAudio      MatchType = "audio"      // Audio fingerprint match
	MatchSemantic   MatchType = "semantic"   // Neural embedding similarity (pro)
)

// FileEntry represents a single file in the dedup scan.
type FileEntry struct {
	Path        string            `json:"path"`
	Size        int64             `json:"size"`
	ModTime     time.Time         `json:"mod_time"`
	SHA256      string            `json:"sha256"`
	MediaType   MediaType         `json:"media_type"`
	Importance  float64           `json:"importance"`   // 0.0-1.0 (pro tier)
	IsProtected bool              `json:"is_protected"` // In safe-list dir
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// DuplicateGroup is a set of files that are duplicates of each other.
type DuplicateGroup struct {
	ID          string      `json:"id"`
	Files       []FileEntry `json:"files"`
	MatchType   MatchType   `json:"match_type"`
	Recommended int         `json:"recommended"` // Index of file to keep
	Confidence  float64     `json:"confidence"`
	WasteBytes  int64       `json:"waste_bytes"` // Recoverable bytes
}

// MirrorResult contains the full dedup scan results.
type MirrorResult struct {
	Groups          []DuplicateGroup `json:"groups"`
	TotalScanned    int              `json:"total_scanned"`
	TotalDuplicates int              `json:"total_duplicates"`
	TotalWasteBytes int64            `json:"total_waste_bytes"`
	UniqueFiles     int              `json:"unique_files"`
	ScanDuration    time.Duration    `json:"scan_duration"`
	DirsScanned     []string         `json:"dirs_scanned"`
}

// ScanOptions configures the dedup scan.
type ScanOptions struct {
	Paths       []string  // Directories to scan
	MinSize     int64     // Minimum file size (bytes) to consider
	MaxSize     int64     // Maximum file size (bytes), 0 = no limit
	MediaFilter MediaType // Filter to specific media type ("" = all)
	FollowLinks bool      // Follow symbolic links
	ProtectDirs []string  // Directories whose files should never be suggested for deletion
	DryRun      bool      // Preview only, don't track for cleaning
}

// mediaExtensions maps file extensions to media types.
var mediaExtensions = map[string]MediaType{
	// Photos
	".jpg": MediaPhoto, ".jpeg": MediaPhoto, ".png": MediaPhoto,
	".heic": MediaPhoto, ".heif": MediaPhoto, ".webp": MediaPhoto,
	".tiff": MediaPhoto, ".tif": MediaPhoto, ".bmp": MediaPhoto,
	".gif": MediaPhoto, ".raw": MediaPhoto, ".cr2": MediaPhoto,
	".nef": MediaPhoto, ".arw": MediaPhoto, ".dng": MediaPhoto,
	".svg": MediaPhoto,

	// Music
	".mp3": MediaMusic, ".m4a": MediaMusic, ".aac": MediaMusic,
	".flac": MediaMusic, ".wav": MediaMusic, ".ogg": MediaMusic,
	".wma": MediaMusic, ".aiff": MediaMusic, ".alac": MediaMusic,
	".opus": MediaMusic,

	// Video
	".mp4": MediaVideo, ".mov": MediaVideo, ".avi": MediaVideo,
	".mkv": MediaVideo, ".wmv": MediaVideo, ".flv": MediaVideo,
	".webm": MediaVideo, ".m4v": MediaVideo,

	// Documents
	".pdf": MediaDocument, ".doc": MediaDocument, ".docx": MediaDocument,
	".xls": MediaDocument, ".xlsx": MediaDocument, ".ppt": MediaDocument,
	".pptx": MediaDocument, ".txt": MediaDocument, ".rtf": MediaDocument,
	".pages": MediaDocument, ".numbers": MediaDocument, ".keynote": MediaDocument,
}

// ClassifyMedia determines the media type from a file extension.
func ClassifyMedia(ext string) MediaType {
	if mt, ok := mediaExtensions[ext]; ok {
		return mt
	}
	return MediaOther
}
