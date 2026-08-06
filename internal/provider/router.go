package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DecisionRecord is the durable, operator-readable explanation for one route.
type DecisionRecord struct {
	At       time.Time     `json:"at"`
	Request  PolicyRequest `json:"request"`
	Decision Decision      `json:"decision"`
	Provider string        `json:"provider,omitempty"`
	Model    string        `json:"model,omitempty"`
	Outcome  string        `json:"outcome"`
	Error    string        `json:"error,omitempty"`
}

// DecisionLogger keeps routing observable without coupling policy to storage.
type DecisionLogger interface {
	Append(DecisionRecord) error
}

// JSONLDecisionLogger appends records under ~/.sirsi by default.
type JSONLDecisionLogger struct{ Path string }

func (l JSONLDecisionLogger) Append(rec DecisionRecord) error {
	if strings.TrimSpace(l.Path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o700); err != nil {
		return fmt.Errorf("create decision log directory: %w", err)
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // caller-selected local audit path
	if err != nil {
		return fmt.Errorf("open decision log: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := json.NewEncoder(f).Encode(rec); err != nil {
		return fmt.Errorf("append decision log: %w", err)
	}
	return nil
}

// ModelRouter qualifies lanes, chooses policy, executes, and records fallback.
type ModelRouter struct {
	Local  Provider
	Remote Provider
	Log    DecisionLogger
	Now    func() time.Time
}

// NewModelRouter resolves the existing local and configured remote adapters.
func NewModelRouter(ctx context.Context, home string) *ModelRouter {
	r := &ModelRouter{Now: time.Now, Log: JSONLDecisionLogger{Path: filepath.Join(home, ".sirsi", "model-router-decisions.jsonl")}}
	for _, p := range Ladder(ctx, home) {
		switch p.Tier() {
		case TierLocal:
			r.Local = p
		case TierRemote:
			r.Remote = p
		}
	}
	return r
}

// Run routes and completes one request. Remote execution failures fail down to
// a qualified local lane and remain visible in the returned decision reason.
func (r *ModelRouter) Run(ctx context.Context, policyReq PolicyRequest, req Request) (Response, Decision, error) {
	state := PolicyState{Local: r.laneState(ctx, r.Local), Remote: r.laneState(ctx, r.Remote)}
	decision, err := Decide(policyReq, state)
	if err != nil {
		if logErr := r.record(policyReq, decision, Response{}, "rejected", err); logErr != nil {
			return Response{}, decision, errors.Join(err, logErr)
		}
		return Response{}, decision, err
	}

	resp, runErr := r.completeLane(ctx, decision.Lane, req)
	if runErr == nil {
		if logErr := r.record(policyReq, decision, resp, "completed", nil); logErr != nil {
			return resp, decision, logErr
		}
		return resp, decision, nil
	}
	if decision.Fallback != "" {
		fallback := decision.Fallback
		fallbackResp, fallbackErr := r.completeLane(ctx, fallback, req)
		if fallbackErr == nil {
			decision.Reason += fmt.Sprintf("; %s failed (%v), fell back to %s", decision.Lane, runErr, fallback)
			decision.Lane = fallback
			decision.Fallback = ""
			if logErr := r.record(policyReq, decision, fallbackResp, "fallback-completed", nil); logErr != nil {
				return fallbackResp, decision, logErr
			}
			return fallbackResp, decision, nil
		}
		runErr = errors.Join(runErr, fallbackErr)
	}
	if logErr := r.record(policyReq, decision, Response{}, "failed", runErr); logErr != nil {
		return Response{}, decision, errors.Join(runErr, logErr)
	}
	return Response{}, decision, runErr
}

func (r *ModelRouter) laneState(ctx context.Context, p Provider) LaneState {
	if p == nil {
		return LaneState{Availability: Offline}
	}
	caps := p.Caps()
	discoveredModel := ""
	if compat, ok := p.(*OpenAICompat); ok {
		discovered, model, err := compat.DiscoverCapabilities(ctx)
		if err != nil {
			return LaneState{Availability: compat.Availability(ctx), Provider: p.Name()}
		}
		caps = discovered
		discoveredModel = model
	}
	state := LaneState{Availability: Offline, Provider: p.Name(), Caps: LaneCaps{
		ContextTokens: caps.ContextTokens, Streaming: caps.Streaming,
		Deterministic: caps.Deterministic, JSONMode: caps.JSONMode,
	}}
	if local, ok := p.(*OpenAICompat); ok && p.Tier() == TierLocal {
		if err := local.ProbeCompletion(ctx); err != nil {
			return state
		}
		model, err := local.ProbeServedModel(ctx)
		if discoveredModel != "" {
			state.Model = discoveredModel
		} else if err == nil {
			state.Model = model
		}
		state.Availability = Available
		return state
	}
	availability := Offline
	if compat, ok := p.(*OpenAICompat); ok {
		availability = compat.Availability(ctx)
	} else if p.Available(ctx) {
		availability = Available
	}
	state.Availability = availability
	if availability == Available {
		if compat, ok := p.(*OpenAICompat); ok {
			if discoveredModel != "" {
				state.Model = discoveredModel
			} else {
				state.Model = compat.Model
			}
		}
	}
	return state
}

func (r *ModelRouter) completeLane(ctx context.Context, lane Lane, req Request) (Response, error) {
	var p Provider
	switch lane {
	case LaneLocal:
		p = r.Local
	case LaneRemote:
		p = r.Remote
	default:
		return Response{}, fmt.Errorf("unsupported execution lane %q", lane)
	}
	if p == nil {
		return Response{}, fmt.Errorf("%w: %s lane is not configured", ErrUnavailable, lane)
	}
	return p.Complete(ctx, req)
}

func (r *ModelRouter) record(req PolicyRequest, decision Decision, resp Response, outcome string, runErr error) error {
	if r.Log == nil {
		return nil
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	rec := DecisionRecord{At: now().UTC(), Request: req, Decision: decision, Provider: resp.Provider, Model: resp.Model, Outcome: outcome}
	if runErr != nil {
		rec.Error = runErr.Error()
	}
	if err := r.Log.Append(rec); err != nil {
		return fmt.Errorf("record model route decision: %w", err)
	}
	return nil
}
