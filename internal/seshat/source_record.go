package seshat

import "time"

// SourceRecord is the lossless, source-scoped transport record used by Ma'at's
// archive. It is deliberately not a Seshat store or projection contract.
type SourceRecord struct {
	Source       string
	InstanceID   string
	NativeItemID string
	Schema       string
	ExtractedAt  time.Time
	Title        string
	Summary      string
	Content      string
	References   []KIReference
}

// ArchiveSourceAdapter is an optional richer path for adapters which can
// provide stable source identity and full content. The legacy SourceAdapter
// remains available for compatibility callers.
type ArchiveSourceAdapter interface {
	IngestArchive(since time.Time) ([]SourceRecord, error)
}
