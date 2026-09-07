package routerboard

import "encoding/json"

// ControlSchema is the stable machine-facing envelope used by remote workers.
// The board payload remains the presentation contract; this envelope makes the
// authority and supported control verbs explicit so an M1 client cannot infer
// a second router or silently fall back to a local store.
const ControlSchema = "pantheon.worker-control/v1"

// ControlCapability describes one operation owned by the canonical router.
// Commands are documentation for constrained clients, not an invitation to
// execute arbitrary shell strings.
type ControlCapability struct {
	Verb        string `json:"verb"`
	Command     string `json:"command"`
	Mutates     bool   `json:"mutates"`
	Description string `json:"description"`
}

// ControlEnvelope is the single worker-facing read model. State is the same
// Payload used by the board and SSE stream, so remote inspection cannot drift
// from the local UI or menubar projection.
type ControlEnvelope struct {
	Schema       string              `json:"schema"`
	Authority    string              `json:"authority"`
	GeneratedAt  string              `json:"generated_at"`
	Capabilities []ControlCapability `json:"capabilities"`
	State        Payload             `json:"state"`
}

var controlCapabilities = []ControlCapability{
	{Verb: "inspect", Command: "sirsi router control --json", Description: "Read the canonical live worker, task, event, and evidence projection."},
	{Verb: "message", Command: "sirsi router send <agent>", Mutates: true, Description: "Send a durable message through the canonical router store."},
	{Verb: "review_request", Command: "sirsi router send <agent> --type review", Mutates: true, Description: "Create a review request as a durable router item."},
	{Verb: "delegate", Command: "sirsi router task add <agent> <task-id>", Mutates: true, Description: "Register durable work in the canonical task ledger."},
	{Verb: "claim", Command: "sirsi router task claim <agent>", Mutates: true, Description: "Atomically claim work with a fenced lease."},
	{Verb: "cancel_handback", Command: "sirsi router task release <agent> <task-id>", Mutates: true, Description: "Release leased work with an explicit handback reason."},
	{Verb: "result_return", Command: "sirsi router task complete <agent> <task-id>", Mutates: true, Description: "Return a result through the fenced task lease."},
}

func cloneCapabilities() []ControlCapability {
	return append([]ControlCapability(nil), controlCapabilities...)
}

// SnapshotControl wraps the latest successful board poll. A zero version is
// deliberately an error: a missing poll is not an empty fleet.
func (b *Board) SnapshotControl() ([]byte, uint64, error) {
	body, version := b.Snapshot()
	if version == 0 || len(body) == 0 {
		return nil, 0, nil
	}
	var state Payload
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, version, err
	}
	envelope := ControlEnvelope{
		Schema:       ControlSchema,
		Authority:    "canonical-routerstore",
		GeneratedAt:  state.GeneratedAt,
		Capabilities: cloneCapabilities(),
		State:        state,
	}
	out, err := json.Marshal(envelope)
	if err != nil {
		return nil, version, err
	}
	return out, version, nil
}
