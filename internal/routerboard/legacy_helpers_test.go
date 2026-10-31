package routerboard

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Historical tiny contract fixtures adapt through the production fact owner.
func typeOf(cfg interface{}) string {
	if value, ok := cfg.(map[string]interface{}); ok {
		if kind, ok := value["type"].(string); ok {
			return kind
		}
	}
	return "?"
}
func wakeOf(cfg interface{}) (string, string) {
	if value, ok := cfg.(map[string]interface{}); ok {
		if wake, ok := value["wake"].(map[string]interface{}); ok {
			mechanism, _ := wake["mechanism"].(string)
			label, _ := wake["launch_agent_label"].(string)
			return mechanism, label
		}
	}
	return "", ""
}

// Compatibility contract fixtures use the same fact owner as production.
func summarize(fleet []Lane) BoardSummary {
	snapshot := ledger.Snapshot{GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	for _, lane := range fleet {
		agent := ledger.Agent{AgentID: lane.Agent}
		for _, raw := range lane.Tasks {
			var task routerstore.Task
			encoded, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			if err = json.Unmarshal(encoded, &task); err != nil {
				continue
			}
			agent.Tasks = append(agent.Tasks, task)
		}
		// Old tiny fixtures supplied only gauges. Materialize those inert rows
		// rather than retain an independent production counting algorithm.
		if len(lane.Tasks) == 0 {
			for status, count := range lane.Counts {
				for i := 0; i < count; i++ {
					agent.Tasks = append(agent.Tasks, routerstore.Task{TaskID: fmt.Sprint(status, i), Status: status})
				}
			}
		}
		for i := 0; i < lane.OpenItems; i++ {
			agent.Items = append(agent.Items, ledger.Item{ID: fmt.Sprint(i)})
		}
		snapshot.Agents = append(snapshot.Agents, agent)
	}
	producer := ledger.NewFabricGenerationProducer(func() (ledger.FabricRead, error) { return ledger.FabricRead{Snapshot: snapshot}, nil }, "fixture")
	generation, err := producer.Read(context.Background())
	if err != nil {
		return BoardSummary{}
	}
	payload, err := FromGeneration(generation, "fixture")
	if err != nil {
		return BoardSummary{}
	}
	return payload.Board
}

func indexAny(b []byte, a, c byte) int {
	for i, x := range b {
		if x == a || x == c {
			return i
		}
	}
	return -1
}

// ageSeconds returns seconds since an RFC3339 UTC router timestamp.
//
// Parsed as UTC explicitly. The Python original used a local-time parse at
// first, which shifted every age by the UTC offset: stalls under-reported by
// four hours and negative ages printed as "-1d20h".
func ageSeconds(ts string) (float64, bool) {
	if len(ts) < 19 {
		return 0, false
	}
	t, err := time.Parse("2006-01-02T15:04:05", ts[:19])
	if err != nil {
		return 0, false
	}
	return time.Since(t.UTC()).Seconds(), true
}

func ageStr(ts string) string {
	secs, ok := ageSeconds(ts)
	if !ok {
		return "?"
	}
	if secs < 0 {
		secs = 0 // clock skew must read 0m, never a negative duration
	}
	d := int(secs) / 86400
	h := (int(secs) % 86400) / 3600
	if d > 0 {
		return fmt.Sprintf("%dd%dh", d, h)
	}
	return fmt.Sprintf("%dh%dm", h, (int(secs)%3600)/60)
}
