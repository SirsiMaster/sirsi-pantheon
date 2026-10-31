package routerboard

import (
	"context"
	"encoding/json"
	"os"
)

// Poll acquires exactly one immutable generation. No shape adapter executes a
// command, reopens a registry or derives task liveness from timestamps.
func (b *Board) Poll(ctx context.Context) {
	b.pollMu.Lock()
	defer b.pollMu.Unlock()
	generation, err := b.read(ctx)
	var body []byte
	if err == nil {
		var payload Payload
		payload, err = FromGeneration(generation, b.buildID)
		if err == nil {
			body, err = json.Marshal(payload)
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err != nil {
		if b.valid {
			b.version++
		}
		b.valid = false // retain last bytes, never publish false zeroes
		return
	}
	changed := !b.valid || string(body) != string(b.payload)
	b.valid = true
	if b.version == 0 || changed {
		b.payload = body
		b.version++
	}
}

// The existing arm mutation has its own whitelist; never attach it through a
// read-only generation adapter.
func (b *Board) registeredAgents(errs *[]string) map[string]interface{} {
	raw, err := os.ReadFile(b.agentsJSON)
	if err != nil {
		*errs = append(*errs, "agents.json: "+err.Error())
		return map[string]interface{}{}
	}
	var value map[string]interface{}
	if err = json.Unmarshal(raw, &value); err != nil {
		*errs = append(*errs, "agents.json: "+err.Error())
		return map[string]interface{}{}
	}
	if agents, ok := value["agents"].(map[string]interface{}); ok {
		return agents
	}
	return value
}

func pct(n, d int) float64 {
	if d < 1 {
		d = 1
	}
	return float64(int(1000.0*float64(n)/float64(d)+0.5)) / 10.0
}
func sliceOf(value interface{}) []interface{} {
	if values, ok := value.([]interface{}); ok {
		return values
	}
	return []interface{}{}
}
func strOr(value interface{}, fallback string) string {
	if text, ok := value.(string); ok && text != "" {
		return text
	}
	return fallback
}
func intOr(value interface{}) int {
	if n, ok := value.(float64); ok {
		return int(n)
	}
	return 0
}
