package ledger

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestFabricDeliveryHistoryLabelPreservesCanonicalTruth(t *testing.T) {
	for _, tc := range []struct {
		name       string
		state      string
		route      bool
		unresolved int
		missing    bool
		foreign    bool
		wantState  string
		wantLabel  string
	}{
		{"current route historical failure", routerstore.LaneUnroutable, true, 1, false, false, routerstore.LaneUnroutable, "DELIVERY NEEDS REVIEW"},
		{"route unavailable eight failures", routerstore.LaneUnroutable, false, 8, false, false, routerstore.LaneUnroutable, routerstore.LaneUnroutable},
		{"no unresolved failure", routerstore.LaneUnroutable, true, 0, false, false, routerstore.LaneUnroutable, routerstore.LaneUnroutable},
		{"negative count not qualified", routerstore.LaneUnroutable, true, -1, false, false, routerstore.LaneUnroutable, routerstore.LaneUnroutable},
		{"working stays working", routerstore.LaneWorking, true, 1, false, false, routerstore.LaneWorking, routerstore.LaneWorking},
		{"blocked stays blocked", routerstore.LaneBlocked, true, 1, false, false, routerstore.LaneBlocked, routerstore.LaneBlocked},
		{"idle label unchanged", routerstore.LaneIdleWithWork, true, 1, false, false, routerstore.LaneIdleWithWork, "IDLE with work"},
		{"absent runtime stays unknown", routerstore.LaneUnroutable, true, 1, true, false, "UNKNOWN", "UNKNOWN"},
		{"foreign runtime stays unknown", routerstore.LaneUnroutable, true, 1, false, true, "UNKNOWN", "UNKNOWN"},
		{"invalid enum stays unknown", "unqualified", true, 1, false, false, "UNKNOWN", "UNKNOWN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := routerstore.LaneOperationalState{Agent: "lane", Classification: tc.state,
				WakeRoutable: tc.route, UnresolvedWakeFailures: tc.unresolved,
				TerminalWakeFailures: 8, CompletionIntegrityFailures: 2, BlockedWork: 3, LiveLeases: 0}
			if tc.foreign {
				state.Agent = "another-lane"
			}
			evidence := FabricEvidence{Operational: map[string]routerstore.LaneOperationalState{"lane": state}}
			if tc.missing {
				evidence.Operational = nil
			}
			snapshot := Snapshot{GeneratedAt: "2026-11-10T19:04:42Z", Agents: []Agent{{AgentID: "lane",
				Tasks: []routerstore.Task{{TaskID: "private-inert-task", Subject: "inert label fixture", Status: "pending"}}}}}
			before, err := json.Marshal([]any{snapshot, evidence})
			if err != nil {
				t.Fatal(err)
			}
			board := FabricFrom(snapshot, nil, "private-label-fixture", evidence)
			if len(board.Lanes) != 1 {
				t.Fatal("lane disappeared")
			}
			lane := board.Lanes[0]
			if lane.State != tc.wantState || lane.StateLabel != tc.wantLabel || lane.Role != fabricLaneRole(tc.wantState) {
				t.Fatalf("presentation changed canonical state/attention role: %+v", lane)
			}
			if tc.wantState == "UNKNOWN" {
				if lane.Runtime != nil || board.RuntimeAvailable {
					t.Fatal("unqualified runtime became current evidence")
				}
			} else if lane.Runtime == nil || !reflect.DeepEqual(*lane.Runtime, state) {
				t.Fatal("label erased or altered canonical route/history/gate fields")
			}
			if board.Work.Done != 0 || board.Work.Open != 1 || board.Work.Total != 1 {
				t.Fatal("attention label fabricated completion or discarded work")
			}
			after, err := json.Marshal([]any{snapshot, evidence})
			if err != nil || string(before) != string(after) {
				t.Fatal("pure label projection mutated source records/history", err)
			}
		})
	}
}
