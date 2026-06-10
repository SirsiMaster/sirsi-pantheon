package rules

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// TestAIModelCachesAreCaution locks the safety rule: AI/ML model caches (large
// downloaded weights) must surface as CAUTION, never one-click "safe". Regression
// guard for the menubar Clean Waste flow trying to one-click trash 30 GB of
// HuggingFace weights.
func TestAIModelCachesAreCaution(t *testing.T) {
	t.Parallel()
	ai := &baseScanRule{name: "huggingface_cache", category: jackal.CategoryAI}
	if got := ai.effectiveSeverity(); got != jackal.SeverityCaution {
		t.Errorf("AI cache severity = %q, want caution (model weights are never one-click safe)", got)
	}
	// A non-AI rule with no explicit severity stays safe (node_modules, build caches).
	dev := &baseScanRule{name: "node_modules", category: jackal.CategoryDev}
	if got := dev.effectiveSeverity(); got != jackal.SeveritySafe {
		t.Errorf("non-AI default severity = %q, want safe", got)
	}
	// An explicit severity always wins over the category default.
	explicit := &baseScanRule{name: "x", category: jackal.CategoryAI, severity: jackal.SeveritySafe}
	if got := explicit.effectiveSeverity(); got != jackal.SeveritySafe {
		t.Errorf("explicit severity should win, got %q", got)
	}
}
