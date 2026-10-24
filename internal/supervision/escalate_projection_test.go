package supervision

import (
	"fmt"
	"strings"
	"testing"
)

// LaneInput carries observed availability, not wake configuration or a count of
// stale registrations. Neither a single notice nor a rollup may invent those.
func TestEscalationKeepsUnavailableCauseUnknown(t *testing.T) {
	for _, count := range []int{1, 2, 3, 6} {
		t.Run(fmt.Sprintf("lanes_%d", count), func(t *testing.T) {
			var lanes []LaneInput
			states := make(map[string]LaneState)
			for i := 0; i < count; i++ {
				id := fmt.Sprintf("declared-launchagent-%d", i)
				lanes = append(lanes, escLaneThreads(id, 2, 1, 1, 0, false))
				states[id] = StateUnroutable
			}
			got := Escalations(lanes, states)
			want := count
			if count >= rollupThreshold {
				want = 1
			}
			if len(got) != want {
				t.Fatalf("notice count=%d, want %d", len(got), want)
			}
			for _, notice := range got {
				for _, invented := range []string{
					"declares no automatic wake mechanism", "Each declares no automatic wake",
					"has no registered thread", "wake mechanism is none",
					"only thing that clears it", "waking harder will not",
				} {
					if strings.Contains(notice.Why, invented) {
						t.Errorf("invented cause %q: %s", invented, notice.Why)
					}
				}
				for _, observed := range []string{"no fresh usable thread", "wake configuration", "Opening a session"} {
					if !strings.Contains(notice.Why, observed) {
						t.Errorf("missing observation or supported recovery %q: %s", observed, notice.Why)
					}
				}
			}
			var total int
			for _, notice := range got {
				total += notice.OpenItems
			}
			if total != count*2 {
				t.Fatalf("backlog=%d, want %d", total, count*2)
			}
		})
	}
}
