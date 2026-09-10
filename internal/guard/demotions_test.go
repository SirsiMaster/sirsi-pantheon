package guard

import (
	"strings"
	"testing"
	"time"
)

func TestDemotionSummaryString(t *testing.T) {
	d := DemotionSummary{Reniced: map[string]int{"Python": 3, "codex": 5}, Held: map[string]int{"clang": 1}, Since: time.Now().Add(-24 * time.Hour)}
	s := d.String()
	for _, want := range []string{"codex×5, Python×3", "held clang×1", "sirsi guard undo"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary %q lacks %q", s, want)
		}
	}
	if (DemotionSummary{}).String() != "" {
		t.Error("empty summary must render as nothing")
	}
}
