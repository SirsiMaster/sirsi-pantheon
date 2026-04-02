package seshat

import (
	"fmt"
	"time"
)

// SourceAdapter extracts knowledge from an external source.
type SourceAdapter interface {
	// Name returns the adapter identifier (e.g., "chrome-history", "gemini", "apple-notes").
	Name() string

	// Description returns a human-readable description of what this adapter ingests.
	Description() string

	// Ingest extracts knowledge items from the source.
	// The since parameter limits extraction to items after the given time.
	// If since is zero, all available items are extracted.
	Ingest(since time.Time) ([]KnowledgeItem, error)
}

// TargetAdapter distributes knowledge to an external target.
type TargetAdapter interface {
	// Name returns the adapter identifier (e.g., "notebooklm", "notion", "apple-notes").
	Name() string

	// Description returns a human-readable description of where this adapter sends knowledge.
	Description() string

	// Export sends a set of knowledge items to the target.
	Export(items []KnowledgeItem) error
}

// AdapterRegistry holds all registered source and target adapters.
type AdapterRegistry struct {
	Sources map[string]SourceAdapter
	Targets map[string]TargetAdapter
}

// NewRegistry creates an empty adapter registry.
func NewRegistry() *AdapterRegistry {
	return &AdapterRegistry{
		Sources: make(map[string]SourceAdapter),
		Targets: make(map[string]TargetAdapter),
	}
}

// RegisterSource adds a source adapter to the registry.
func (r *AdapterRegistry) RegisterSource(a SourceAdapter) {
	r.Sources[a.Name()] = a
}

// RegisterTarget adds a target adapter to the registry.
func (r *AdapterRegistry) RegisterTarget(a TargetAdapter) {
	r.Targets[a.Name()] = a
}

// DefaultRegistry returns a registry with all built-in adapters.
func DefaultRegistry() *AdapterRegistry {
	reg := NewRegistry()

	// Source adapters
	reg.RegisterSource(&ChromeHistoryAdapter{})
	reg.RegisterSource(&GeminiAdapter{})
	reg.RegisterSource(&ClaudeAdapter{})
	reg.RegisterSource(&AppleNotesSourceAdapter{})
	reg.RegisterSource(&GoogleWorkspaceAdapter{})

	// Target adapters
	reg.RegisterTarget(&AppleNotesTargetAdapter{})
	reg.RegisterTarget(&NotebookLMAdapter{})
	reg.RegisterTarget(&ThothAdapter{})

	return reg
}

// IngestAll runs all source adapters and returns combined results.
func (r *AdapterRegistry) IngestAll(since time.Time) ([]KnowledgeItem, error) {
	var all []KnowledgeItem
	for name, adapter := range r.Sources {
		items, err := adapter.Ingest(since)
		if err != nil {
			fmt.Printf("  ⚠️  %s: %v\n", name, err)
			continue
		}
		all = append(all, items...)
	}
	return all, nil
}
