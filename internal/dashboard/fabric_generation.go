package dashboard

import (
	"net/http"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
)

func fabricReadMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// FleetFromGeneration retains the existing fleet DTO, using producer facts and
// exact supervised enums. No task-status, heartbeat or local-age classification.
func FleetFromGeneration(g *ledger.FabricGeneration) (FleetSnapshot, error) {
	v, err := g.View()
	if err != nil {
		return FleetSnapshot{}, err
	}
	w := v.Board.Work
	out := FleetSnapshot{GeneratedAt: v.Board.GeneratedAt, Seeded: v.Board.ActivitySeeded, Activity: []FleetEvent{}, Lanes: []FleetLane{},
		Summary: FleetSummary{Total: w.Total, Done: w.Done, InFlight: w.Open, Active: w.InProgress, Assigned: w.AssignedNotStarted, Stalled: w.Stalled, Blocked: w.Blocked, LanesTotal: len(v.Board.Lanes), IdleLanes: ledger.IdleFabricLanes(v.Board.Lanes)}}
	if w.Total > 0 {
		out.Summary.PctDone = w.Done * 100 / w.Total
	}
	at, err := time.Parse(time.RFC3339Nano, v.Board.GeneratedAt)
	if err != nil {
		return FleetSnapshot{}, err
	}
	for _, lane := range v.Board.Lanes {
		f := v.Facts.Lanes[lane.Agent]
		row := FleetLane{Agent: lane.Agent, State: lane.State, Open: lane.Open, Active: lane.Active, Stalled: lane.Stalled, Blocked: lane.Blocked, Inbox: f.Inbox}
		if lane.Runtime != nil {
			row.Routable = lane.Runtime.WakeRoutable
		}
		if stamp, e := time.Parse(time.RFC3339Nano, lane.LastTouch); e == nil {
			row.TouchedAgo = ledger.FormatAge(at.Sub(stamp).Seconds())
		}
		if lane.State == LaneWorking {
			out.Summary.LanesWork++
		}
		out.Lanes = append(out.Lanes, row)
	}
	for _, event := range v.Events {
		out.Activity = append(out.Activity, FleetEvent{At: event.At, Agent: event.Agent, TaskID: event.TaskID, Subject: event.Subject, From: event.From, To: event.To})
	}
	return out, nil
}

// BindRouterFabric keeps the Ra-owned router status plane intact. Canonical
// task/message fields carry their own immutable generation/time; queue/consumer
// verdicts are never silently interpreted as tasks, workers or answer grants.
func BindRouterFabric(snapshot RouterSnapshot, g *ledger.FabricGeneration) (RouterSnapshot, error) {
	v, err := g.View()
	if err != nil {
		return RouterSnapshot{}, err
	}
	snapshot.Fabric = &v.Board
	snapshot.FabricGeneration = v.ID
	return snapshot, nil
}
