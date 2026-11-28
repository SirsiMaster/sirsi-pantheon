package dashboard

import (
	"encoding/json"
	"strings"
	"testing"
)

// The board must say WHY a lane is unroutable, and must not invent a reason for
// a lane that is not.
func TestFleetFromGenerationCarriesReasonAndVerdict(t *testing.T) {
	g, _ := mixedFabricGeneration(t)
	fleet, err := FleetFromGeneration(g)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]FleetLane{}
	for _, l := range fleet.Lanes {
		got[l.Agent] = l
	}
	if r := got["orphan"].Reason; r != "retired lane (not in registry); open rows left over" {
		t.Fatalf("orphan reason = %q", r)
	}
	// Negative control: an IDLE lane keeps its verdict but gets no reason, and its
	// state is untouched by the explanation.
	w := got["worker"]
	if w.Verdict != "HELD" || w.Reason != "" || w.State != LaneIdleWithWork {
		t.Fatalf("worker lane = %+v", w)
	}
	raw, err := json.Marshal(got["orphan"])
	if err != nil || !strings.Contains(string(raw), `"reason":"retired lane`) {
		t.Fatalf("reason missing from /api/fleet JSON: %s %v", raw, err)
	}
}
