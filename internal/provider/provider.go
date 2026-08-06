// Package provider makes intelligence swappable and makes the swap honest.
//
// Sirsi currently has no inference client at all: the only reference to
// api.anthropic.com in 525k lines of Go is a DNS reachability check. The agent
// loop lives in Claude Code, and Sirsi's installer, when it finds no AI
// present, offers to register Sirsi *inside someone else's agent*. That is the
// inversion this package begins to correct (SIRSI_V2_APPLICATION §1).
//
// The design is not new. sirsi-brain.sh already declares it:
//
//	"the PLUGGABLE orchestration brain for Sirsi Pantheon... It must be
//	 swappable painlessly — gemma is the zero-token DEFAULT, but a user can
//	 point it at Claude, Codex, Gemini, GLM, Kimi K2, or any OpenAI-compatible
//	 endpoint without changing anything else."
//
// That design was right and was never given a body: an unversioned bash script
// outside the repo, offering one verb — ask: one prompt, one answer, no tools,
// no loop, no conversation. This package is the body. It reads the SAME
// ~/.sirsi/orchestrator.conf so an existing configuration keeps working.
package provider

import (
	"context"
	"errors"
)

// Tier is the certainties ladder (SIRSI_V2_APPLICATION §2c): heuristic → local
// → remote, escalating only when the cheaper rung cannot answer.
//
// The tier IS the confidence, and it is carried on every response so a surface
// can tell the user whether they are reading a measurement, an inference, or a
// judgment. It is also the cost and availability model: heuristic and local
// work offline and free; remote is an enhancement, never a dependency.
type Tier int

const (
	// TierHeuristic answers from deterministic checks with no model at all.
	// Most diagnostic work lives here: the 2026-07-27 fork storm — 358
	// processes, lineage depth 5, swap 48.5/49 GB — is entirely heuristic.
	// This rung is why Sirsi can help a 16 GB machine, which is precisely the
	// machine with no headroom to load a model.
	TierHeuristic Tier = iota
	// TierLocal is on-device inference. Zero tokens, works offline, costs RAM.
	TierLocal
	// TierRemote is a frontier model. Costs tokens and needs the network.
	TierRemote
)

func (t Tier) String() string {
	switch t {
	case TierHeuristic:
		return "heuristic"
	case TierLocal:
		return "local"
	case TierRemote:
		return "remote"
	}
	return "unknown"
}

// Caps describes what a backend can actually do. Backends are NOT equivalent,
// and pretending otherwise is how a local-first product silently becomes a
// cloud product: the loop must be able to see that a provider cannot call
// tools and degrade to a constrained path deliberately, saying so in the
// transcript, rather than failing in a way that looks like the model being bad.
type Caps struct {
	Tools         bool // can the model call tools?
	Streaming     bool
	ContextTokens int
	Deterministic bool
	JSONMode      bool
	Offline       bool // usable with no network
}

// Request is one turn of work.
type Request struct {
	System    string
	Prompt    string
	MaxTokens int
	// Tools offered this turn. A provider without Caps.Tools must ignore these
	// and the caller must notice — see Response.ToolsHonored.
	Tools []ToolSpec
}

// ToolSpec is a tool offered to the model. Deliberately minimal for now: the
// loop's tool tiers (observe / repair / destructive) are enforced by the caller,
// never by the provider, so a backend can never widen its own permissions.
type ToolSpec struct {
	Name        string
	Description string
	Schema      map[string]any
}

// ToolCall is a model's request to run a tool. Executing it is the caller's
// decision, gated by tier.
type ToolCall struct {
	Name string
	Args map[string]any
}

// Response carries the answer AND the provenance needed to judge it.
type Response struct {
	Text      string
	ToolCalls []ToolCall
	// Tier that actually produced this answer — not the tier that was asked
	// for. Escalation must be visible.
	Tier Tier
	// Provider and Model that served it, so "which brain said this" is never a
	// guess.
	Provider string
	Model    string
	// ToolsHonored is false when tools were offered to a backend that cannot
	// call them. The loop uses this to degrade explicitly instead of concluding
	// the model simply chose not to act.
	ToolsHonored bool
	// FinishReason as reported by the backend. Asserting on this rather than on
	// non-empty content is what distinguishes a healthy reasoning model that hit
	// a token cap from a wedged one — a false-"wedged" this fabric has recorded.
	FinishReason string
	PromptTokens int
	OutputTokens int
}

// Provider is one swappable backend.
type Provider interface {
	Name() string
	Tier() Tier
	Caps() Caps
	// Available reports whether this backend can serve right now — a local
	// broker that is down and a cloud key that is absent are both "not
	// available", and the ladder must be able to skip a rung without failing.
	Available(ctx context.Context) bool
	Complete(ctx context.Context, req Request) (Response, error)
}

// ErrUnavailable is returned when a backend cannot serve. Callers escalate to
// the next rung rather than surfacing an error, but the escalation is recorded.
var ErrUnavailable = errors.New("provider unavailable")

var (
	ErrRateLimited     = errors.New("provider rate limited")
	ErrBudgetExhausted = errors.New("provider budget exhausted")
)
