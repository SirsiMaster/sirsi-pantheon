package provider

import (
	"errors"
	"fmt"
	"strings"
)

// TaskClass describes the kind of model work being requested.
type TaskClass string

const (
	TaskGeneration TaskClass = "generation"
	TaskJudgment   TaskClass = "judgment"
	TaskExtraction TaskClass = "extraction"
	TaskEmbedding  TaskClass = "embedding"
)

// PrivacyClass controls whether request content may leave the machine.
type PrivacyClass string

const (
	PrivacyLocalOnly PrivacyClass = "local-only"
	PrivacyShareable PrivacyClass = "shareable"
)

// LatencyClass records the caller's latency expectation for diagnostics and
// future qualified policies. V1 does not race lanes or make speed claims.
type LatencyClass string

const (
	LatencyInteractive LatencyClass = "interactive"
	LatencyBackground  LatencyClass = "background"
)

// Lane is a policy destination, not a provider or model identity.
type Lane string

const (
	LaneLocal  Lane = "local"
	LaneRemote Lane = "remote"
	LaneHybrid Lane = "hybrid"
)

// Availability is the router's current qualified view of a lane.
type Availability string

const (
	Available Availability = "available"
	Degraded  Availability = "degraded"
	Offline   Availability = "offline"
	Budgeted  Availability = "budget-exhausted"
	RateLimit Availability = "rate-exhausted"
)

// CapabilityNeeds are minimum requirements, never comparative preferences.
type CapabilityNeeds struct {
	ContextTokens int  `json:"context_tokens,omitempty"`
	Streaming     bool `json:"streaming,omitempty"`
	Deterministic bool `json:"deterministic,omitempty"`
	JSONMode      bool `json:"json_mode,omitempty"`
}

// LaneCaps are capabilities discovered from a lane's authoritative endpoint.
type LaneCaps struct {
	ContextTokens int
	Streaming     bool
	Deterministic bool
	JSONMode      bool
}

func (c LaneCaps) satisfies(n CapabilityNeeds) bool {
	return (n.ContextTokens <= 0 || c.ContextTokens >= n.ContextTokens) &&
		(!n.Streaming || c.Streaming) &&
		(!n.Deterministic || c.Deterministic) &&
		(!n.JSONMode || c.JSONMode)
}

// LaneState is the probe and budget state used for one decision.
type LaneState struct {
	Availability Availability
	Caps         LaneCaps
	Provider     string
	Model        string
}

func (s LaneState) qualified(n CapabilityNeeds) bool {
	return (s.Availability == Available || s.Availability == Degraded) && s.Caps.satisfies(n)
}

// PolicyRequest contains only caller-declared policy inputs.
type PolicyRequest struct {
	Task     TaskClass       `json:"task"`
	Privacy  PrivacyClass    `json:"privacy"`
	Needs    CapabilityNeeds `json:"needs"`
	Latency  LatencyClass    `json:"latency"`
	Override Lane            `json:"override,omitempty"`
}

// PolicyState is the router's current view of available lanes.
type PolicyState struct {
	Local  LaneState
	Remote LaneState
}

// Decision is suitable for an observable decision log. Fallback is empty when
// policy forbids or cannot qualify another lane.
type Decision struct {
	Lane     Lane   `json:"lane"`
	Fallback Lane   `json:"fallback,omitempty"`
	Reason   string `json:"reason"`
	Override bool   `json:"override"`
}

var ErrNoQualifiedLane = errors.New("no qualified model lane")

// Decide applies the owner-ratified default policy. It performs no probes and
// no I/O, making the same decision reusable by every Pantheon surface.
func Decide(req PolicyRequest, state PolicyState) (Decision, error) {
	if err := validatePolicyRequest(req); err != nil {
		return Decision{}, err
	}
	if req.Privacy == PrivacyLocalOnly {
		if req.Override == LaneRemote {
			return Decision{}, fmt.Errorf("%w: remote override conflicts with local-only privacy", ErrNoQualifiedLane)
		}
		if state.Local.Availability != Available && state.Local.Availability != Degraded {
			return Decision{}, fmt.Errorf("%w: local-only request requires local inference, but the local lane is %s", ErrNoQualifiedLane, state.Local.Availability)
		}
		if !state.Local.qualified(req.Needs) {
			return Decision{}, fmt.Errorf("%w: local-only request exceeds the available local lane capabilities", ErrNoQualifiedLane)
		}
		return Decision{Lane: LaneLocal, Reason: "local-only privacy requires the local lane", Override: req.Override != ""}, nil
	}

	if req.Override != "" {
		return decideOverride(req.Override, req.Needs, state)
	}

	localOK := state.Local.qualified(req.Needs)
	remoteOK := state.Remote.qualified(req.Needs) && qualifiedRemoteModel(state.Remote)
	if req.Task == TaskJudgment && remoteOK {
		d := Decision{Lane: LaneRemote, Reason: "shareable judgment uses the best qualified remote lane"}
		if localOK {
			d.Fallback = LaneLocal
		}
		return d, nil
	}
	if localOK {
		d := Decision{Lane: LaneLocal, Reason: "local lane satisfies the request capabilities"}
		if remoteOK {
			d.Fallback = LaneRemote
		}
		return d, nil
	}
	if remoteOK {
		return Decision{Lane: LaneRemote, Reason: "request capabilities exceed the local envelope"}, nil
	}
	return Decision{}, fmt.Errorf("%w: neither local nor remote lane satisfies the request", ErrNoQualifiedLane)
}

func decideOverride(lane Lane, needs CapabilityNeeds, state PolicyState) (Decision, error) {
	qualified := false
	switch lane {
	case LaneLocal:
		qualified = state.Local.qualified(needs)
	case LaneRemote:
		qualified = state.Remote.qualified(needs) && qualifiedRemoteModel(state.Remote)
	case LaneHybrid:
		if state.Local.qualified(needs) && state.Remote.qualified(needs) && qualifiedRemoteModel(state.Remote) {
			return Decision{Lane: LaneHybrid, Fallback: LaneRemote, Reason: "explicit hybrid override", Override: true}, nil
		}
	default:
		return Decision{}, fmt.Errorf("invalid lane override %q", lane)
	}
	if !qualified {
		return Decision{}, fmt.Errorf("%w: overridden %s lane is not qualified", ErrNoQualifiedLane, lane)
	}
	return Decision{Lane: lane, Reason: "explicit per-request override", Override: true}, nil
}

func validatePolicyRequest(req PolicyRequest) error {
	switch req.Task {
	case TaskGeneration, TaskJudgment, TaskExtraction, TaskEmbedding:
	default:
		return fmt.Errorf("invalid task class %q", req.Task)
	}
	switch req.Privacy {
	case PrivacyLocalOnly, PrivacyShareable:
	default:
		return fmt.Errorf("invalid privacy class %q", req.Privacy)
	}
	if req.Needs.ContextTokens < 0 {
		return errors.New("minimum context tokens cannot be negative")
	}
	return nil
}

// qualifiedRemoteModel enforces the ratified Gemini floor without pretending
// to rank providers. Other remote families are qualified by their adapters.
func qualifiedRemoteModel(s LaneState) bool {
	if !strings.EqualFold(s.Provider, "gemini") {
		return true
	}
	m := strings.ToLower(strings.TrimSpace(s.Model))
	return strings.Contains(m, "omni") || strings.HasPrefix(m, "gemini-3") || strings.HasPrefix(m, "3")
}
