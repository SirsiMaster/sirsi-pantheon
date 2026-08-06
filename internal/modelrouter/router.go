// Package modelrouter implements the Pantheon model-routing policy engine.
//
// One policy engine decides per-request which backend serves an LLM call:
// local (SNE broker), remote (frontier APIs), or fail-closed. Every surface
// routes through it so no surface hardcodes a backend (MODEL-ROUTER-DESIGN.md).
//
// Routing is qualified, not comparative: the router selects the cheapest lane
// that is QUALIFIED for the request's needs. It makes no latency claims.
package modelrouter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

// TaskClass is the caller-declared nature of the work.
type TaskClass string

const (
	// TaskGeneration is text/code/content generation work. Default: local.
	TaskGeneration TaskClass = "generation"
	// TaskJudgment is review, verdict, or canon decision work. Default: best remote (if shareable).
	TaskJudgment TaskClass = "judgment"
	// TaskExtraction is structured extraction from existing text. Default: local.
	TaskExtraction TaskClass = "extraction"
	// TaskEmbedding is embedding generation. Default: local.
	TaskEmbedding TaskClass = "embedding"
)

// PrivacyClass is the caller-declared privacy constraint on the content.
type PrivacyClass string

const (
	// PrivacyLocalOnly means content MUST NOT leave the machine. Violation is
	// a hard error, never a silent escalation.
	PrivacyLocalOnly PrivacyClass = "local-only"
	// PrivacyShareable means the content may be sent to a remote backend.
	PrivacyShareable PrivacyClass = "shareable"
)

// LatencyClass is the caller-declared urgency.
type LatencyClass string

const (
	LatencyInteractive LatencyClass = "interactive"
	LatencyBackground  LatencyClass = "background"
)

// Request is the routing input. Callers fill only the fields relevant to
// their work; unset fields apply the safe defaults.
type Request struct {
	// Task class: what kind of work is this? Default: generation.
	Task TaskClass
	// Privacy: may the content leave the machine? Default: local-only.
	Privacy PrivacyClass
	// Latency: how urgent is the response? Default: interactive.
	Latency LatencyClass
	// MinContextTokens: minimum context window needed. 0 = any.
	MinContextTokens int
}

// Lane is the selected routing lane.
type Lane string

const (
	LaneLocal  Lane = "local"
	LaneRemote Lane = "remote"
)

// Decision records which lane was selected and why.
type Decision struct {
	Lane      Lane
	Provider  provider.Provider
	Rationale string
	// Escalated is true when the preferred lane was unavailable and a fallback was used.
	Escalated bool
}

// Router applies the owner-ratified routing policy over a provider ladder.
// The zero value is NOT usable; use New.
type Router struct {
	ladder       []provider.Provider
	probeTimeout time.Duration
}

// New builds a Router from the given provider ladder. If ladder is empty,
// every routing call will fail (no providers = no route).
func New(ladder []provider.Provider) *Router {
	return &Router{
		ladder:       ladder,
		probeTimeout: 5 * time.Second,
	}
}

// WithProbeTimeout overrides the per-provider availability probe timeout.
func (rt *Router) WithProbeTimeout(d time.Duration) *Router {
	rt.probeTimeout = d
	return rt
}

// Route applies the policy and returns the selected Decision.
//
// Policy summary (matches MODEL-ROUTER-DESIGN.md):
//  1. privacy=local-only → local only; fail if local cannot satisfy capability needs.
//  2. task=judgment AND shareable AND remote available → best remote; local fallback.
//  3. task=generation/extraction → local unless capability not satisfied.
//  4. Budget/rate exhaustion → fail-down to local, surfaced in diagnostics.
//  5. Explicit per-request override always wins (via Privacy override on caller side).
func (rt *Router) Route(ctx context.Context, req Request) (Decision, error) {
	// Apply defaults.
	if req.Task == "" {
		req.Task = TaskGeneration
	}
	if req.Privacy == "" {
		req.Privacy = PrivacyLocalOnly
	}

	local, remote := rt.splitLadder()

	switch {
	case req.Privacy == PrivacyLocalOnly:
		// Rule 1: local-only content never leaves the machine.
		p, ok := rt.firstAvailable(ctx, local)
		if !ok {
			return Decision{}, fmt.Errorf("model-router: privacy=local-only but no local provider is available")
		}
		if req.MinContextTokens > 0 && p.Caps().ContextTokens < req.MinContextTokens {
			return Decision{}, fmt.Errorf("model-router: local provider context window %d < required %d (fail-closed: content is local-only)",
				p.Caps().ContextTokens, req.MinContextTokens)
		}
		return Decision{Lane: LaneLocal, Provider: p, Rationale: "privacy=local-only; local provider selected"}, nil

	case req.Task == TaskJudgment && req.Privacy == PrivacyShareable:
		// Rule 2: judgment work benefits from the best model; prefer remote.
		p, ok := rt.firstAvailable(ctx, remote)
		if ok {
			return Decision{Lane: LaneRemote, Provider: p, Rationale: "task=judgment + shareable; best remote selected"}, nil
		}
		// Sovereignty: if remote is down, fail-down to local.
		p, ok = rt.firstAvailable(ctx, local)
		if !ok {
			return Decision{}, fmt.Errorf("model-router: judgment task — no provider available (remote down, no local)")
		}
		return Decision{Lane: LaneLocal, Provider: p, Rationale: "task=judgment + remote unavailable; local fallback (sovereignty rule)", Escalated: true}, nil

	default:
		// Rule 3: generation/extraction defaults to local; remote only if capability gap.
		p, ok := rt.firstAvailable(ctx, local)
		if ok && (req.MinContextTokens == 0 || p.Caps().ContextTokens >= req.MinContextTokens) {
			return Decision{Lane: LaneLocal, Provider: p, Rationale: fmt.Sprintf("task=%s; local preferred", req.Task)}, nil
		}
		// Local cannot satisfy capability needs — try remote (only if shareable).
		if req.Privacy == PrivacyShareable {
			p, ok = rt.firstAvailable(ctx, remote)
			if ok {
				return Decision{Lane: LaneRemote, Provider: p, Rationale: fmt.Sprintf("task=%s; local capability insufficient, escalating to remote", req.Task), Escalated: true}, nil
			}
		}
		if ok {
			// local available but context window too small AND not shareable
			return Decision{}, fmt.Errorf("model-router: content is local-only and local context window %d < required %d",
				p.Caps().ContextTokens, req.MinContextTokens)
		}
		return Decision{}, fmt.Errorf("model-router: no provider available for task=%s", req.Task)
	}
}

// Complete routes the request and calls the selected provider.
func (rt *Router) Complete(ctx context.Context, routeReq Request, provReq provider.Request) (provider.Response, Decision, error) {
	d, err := rt.Route(ctx, routeReq)
	if err != nil {
		return provider.Response{}, Decision{}, err
	}
	resp, err := d.Provider.Complete(ctx, provReq)
	if err != nil {
		return provider.Response{}, d, fmt.Errorf("model-router: provider %s (%s): %w", d.Provider.Name(), d.Lane, err)
	}
	return resp, d, nil
}

// splitLadder partitions the ladder into local and remote slices, preserving order.
func (rt *Router) splitLadder() (local, remote []provider.Provider) {
	for _, p := range rt.ladder {
		if p.Tier() == provider.TierLocal {
			local = append(local, p)
		} else {
			remote = append(remote, p)
		}
	}
	return
}

// firstAvailable returns the first provider whose Available() probe passes.
func (rt *Router) firstAvailable(ctx context.Context, providers []provider.Provider) (provider.Provider, bool) {
	pCtx, cancel := context.WithTimeout(ctx, rt.probeTimeout)
	defer cancel()
	for _, p := range providers {
		if p.Available(pCtx) {
			return p, true
		}
	}
	return nil, false
}

// DecisionSummary returns a compact one-line string for diagnostic output.
func (d Decision) DecisionSummary() string {
	if d.Provider == nil {
		return "no-route"
	}
	var sb strings.Builder
	sb.WriteString(string(d.Lane))
	sb.WriteByte('/')
	sb.WriteString(d.Provider.Name())
	if d.Escalated {
		sb.WriteString(" (escalated)")
	}
	sb.WriteString(": ")
	sb.WriteString(d.Rationale)
	return sb.String()
}
