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
	"errors"
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
// their work; unset fields apply the safe defaults. A requirement is not
// evidence that any configured provider can satisfy it.
type Request struct {
	ResponseFormat provider.ResponseFormat
	Determinism    provider.DeterminismMode
	// Task class: what kind of work is this? Default: generation.
	Task TaskClass
	// Privacy: may the content leave the machine? Default: local-only.
	Privacy PrivacyClass
	// Latency: how urgent is the response? Default: interactive.
	Latency LatencyClass
	// MinContextTokens: minimum context window needed. 0 = any; negative is invalid.
	MinContextTokens int
	// RequireStreaming selects only providers with explicitly declared streaming
	// support. Use Router.Stream to execute such a request.
	RequireStreaming bool
	// Override requests an exact lane. Privacy constraints are evaluated first.
	Override Lane
}

// Lane is the selected routing lane.
type Lane string

const (
	LaneLocal  Lane = "local"
	LaneRemote Lane = "remote"
)

// Decision records which lane was selected and why.
type Decision struct {
	Lane              Lane
	Provider          provider.Provider
	Rationale         string
	RequestedOverride Lane
	OverrideApplied   bool
	InitialLane       Lane
	ProviderName      string
	Model             string
	FallbackReason    string
	FailureCategory   string
	// Escalated is true when the preferred lane was unavailable and a fallback was used.
	Escalated bool
}

// Router applies the owner-ratified routing policy over a provider ladder.
// The zero value is NOT usable; use New.
type Router struct {
	ladder       []provider.Provider
	probeTimeout time.Duration
	decisionSink DecisionSink
}

// New builds a Router from the given provider ladder. If ladder is empty,
// every routing call will fail (no providers = no route).
func New(ladder []provider.Provider) *Router {
	return &Router{
		ladder:       ladder,
		probeTimeout: 5 * time.Second,
		decisionSink: NewPrivateDecisionSink(""),
	}
}

// WithDecisionSink injects the bounded decision evidence destination.
func (rt *Router) WithDecisionSink(sink DecisionSink) *Router {
	rt.decisionSink = sink
	return rt
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
//  5. Explicit per-request lane override applies only after privacy validation.
func (rt *Router) Route(ctx context.Context, req Request) (Decision, error) {
	req, err := normalizeRequest(req)
	if err != nil {
		d := Decision{RequestedOverride: req.Override, FailureCategory: failureCategory(err)}
		return d, errors.Join(err, rt.recordDecision(req, d, "rejected", err))
	}
	d, err := rt.route(ctx, req)
	outcome := "selected"
	if err != nil {
		outcome = "rejected"
	}
	return d, errors.Join(err, rt.recordDecision(req, d, outcome, err))
}

func normalizeRequest(req Request) (Request, error) {
	if req.Task == "" {
		req.Task = TaskGeneration
	}
	if req.Privacy == "" {
		req.Privacy = PrivacyLocalOnly
	}
	if err := validateRequest(req); err != nil {
		return req, err
	}
	return req, nil
}

func (rt *Router) route(ctx context.Context, req Request) (Decision, error) {
	d := Decision{RequestedOverride: req.Override}
	if err := ctx.Err(); err != nil {
		d.FailureCategory = "canceled"
		return d, fmt.Errorf("model-router: request canceled: %w", err)
	}

	local, remote := rt.splitLadder()
	if req.Override != "" {
		if req.Override == LaneRemote && req.Privacy == PrivacyLocalOnly {
			d.FailureCategory = "privacy_refusal"
			return d, fmt.Errorf("model-router: remote override refused by privacy=local-only")
		}
		candidates := local
		if req.Override == LaneRemote {
			candidates = remote
		}
		selection, err := rt.firstQualified(ctx, candidates, req)
		if err != nil {
			d.FailureCategory = failureCategory(err)
			return d, err
		}
		if selection.provider == nil {
			d.FailureCategory = "override_unavailable_or_unqualified"
			return d, overrideUnavailableError(req.Override, req, selection)
		}
		d.Lane, d.InitialLane, d.Provider = req.Override, req.Override, selection.provider
		d.OverrideApplied = true
		d.Rationale = "explicit lane override selected"
		return d, nil
	}

	switch {
	case req.Privacy == PrivacyLocalOnly:
		// Rule 1: local-only content never leaves the machine.
		selection, err := rt.firstQualified(ctx, local, req)
		if err != nil {
			d.FailureCategory = failureCategory(err)
			return d, err
		}
		if selection.provider == nil && !selection.available && selection.generationErr == nil {
			d.FailureCategory = "no_local_provider"
			return d, fmt.Errorf("model-router: privacy=local-only but no local provider is available")
		}
		if selection.provider == nil {
			d.FailureCategory = "capability_insufficient"
			return d, selectionRequirementError("local-only", req, selection)
		}
		d.Lane, d.InitialLane, d.Provider = LaneLocal, LaneLocal, selection.provider
		d.Rationale = "privacy=local-only; local provider selected"
		return d, nil

	case req.Task == TaskJudgment && req.Privacy == PrivacyShareable:
		// Rule 2: judgment work benefits from the best model; prefer remote.
		remoteSelection, err := rt.firstQualified(ctx, remote, req)
		if err != nil {
			d.FailureCategory = failureCategory(err)
			return d, err
		}
		if remoteSelection.provider != nil {
			d.Lane, d.InitialLane, d.Provider = LaneRemote, LaneRemote, remoteSelection.provider
			d.Rationale = "task=judgment + shareable; best remote selected"
			return d, nil
		}
		// Sovereignty: if remote cannot satisfy the request, fail-down to local.
		localSelection, err := rt.firstQualified(ctx, local, req)
		if err != nil {
			d.FailureCategory = failureCategory(err)
			return d, err
		}
		if localSelection.provider == nil && !localSelection.available && !remoteSelection.available && localSelection.generationErr == nil && remoteSelection.generationErr == nil {
			d.FailureCategory = "no_provider_available"
			return d, fmt.Errorf("model-router: judgment task — no provider available (remote down, no local)")
		}
		if localSelection.provider == nil {
			d.FailureCategory = "capability_insufficient"
			combined := mergeProviderSelections(remoteSelection, localSelection)
			return d, selectionRequirementError("available", req, combined)
		}
		d.Lane, d.InitialLane, d.Provider = LaneLocal, LaneRemote, localSelection.provider
		d.Rationale = "task=judgment + remote unavailable or insufficient; local fallback (sovereignty rule)"
		d.Escalated = true
		d.FallbackReason = availabilityFallbackReason(remoteSelection)
		d.FailureCategory = remoteFailureCategory(remoteSelection)
		return d, nil

	default:
		// Rule 3: generation/extraction defaults to local; remote only if capability gap.
		localSelection, err := rt.firstQualified(ctx, local, req)
		if err != nil {
			d.FailureCategory = failureCategory(err)
			return d, err
		}
		if localSelection.provider != nil {
			d.Lane, d.InitialLane, d.Provider = LaneLocal, LaneLocal, localSelection.provider
			d.Rationale = fmt.Sprintf("task=%s; local preferred", req.Task)
			return d, nil
		}
		// Local cannot satisfy capability needs — try remote (only if shareable).
		if req.Privacy == PrivacyShareable {
			remoteSelection, err := rt.firstQualified(ctx, remote, req)
			if err != nil {
				d.FailureCategory = failureCategory(err)
				return d, err
			}
			if remoteSelection.provider != nil {
				d.Lane, d.InitialLane, d.Provider = LaneRemote, LaneLocal, remoteSelection.provider
				d.Rationale = fmt.Sprintf("task=%s; local capability insufficient, escalating to remote", req.Task)
				d.Escalated = true
				d.FallbackReason = "local_capability_insufficient"
				return d, nil
			}
			if localSelection.available || remoteSelection.available || localSelection.generationErr != nil || remoteSelection.generationErr != nil {
				d.FailureCategory = "capability_insufficient"
				combined := mergeProviderSelections(localSelection, remoteSelection)
				return d, selectionRequirementError("available", req, combined)
			}
		}
		if localSelection.available || localSelection.generationErr != nil {
			d.FailureCategory = "capability_insufficient"
			return d, selectionRequirementError("local-only", req, localSelection)
		}
		d.FailureCategory = "no_provider_available"
		return d, fmt.Errorf("model-router: no provider available for task=%s", req.Task)
	}
}

func validateRequest(req Request) error {
	switch req.Task {
	case TaskGeneration, TaskJudgment, TaskExtraction, TaskEmbedding:
	default:
		return fmt.Errorf("%w: unknown task class %q", errInvalidRequest, req.Task)
	}
	switch req.Privacy {
	case PrivacyLocalOnly, PrivacyShareable:
	default:
		return fmt.Errorf("%w: unknown privacy class %q", errInvalidRequest, req.Privacy)
	}
	switch req.Latency {
	case "", LatencyInteractive, LatencyBackground:
	default:
		return fmt.Errorf("%w: unknown latency class %q", errInvalidRequest, req.Latency)
	}
	if req.MinContextTokens < 0 {
		return fmt.Errorf("%w: minimum context tokens must be nonnegative, got %d", errInvalidRequest, req.MinContextTokens)
	}
	switch req.Override {
	case "", LaneLocal, LaneRemote:
	default:
		return fmt.Errorf("%w: unknown lane override %q", errInvalidRequest, req.Override)
	}
	if err := provider.ValidateRequirements(generationRequest(req), req.RequireStreaming); err != nil {
		return fmt.Errorf("%w: %w", errInvalidRequest, err)
	}
	return nil
}

func generationRequest(req Request) provider.Request {
	return provider.Request{ResponseFormat: req.ResponseFormat, Determinism: req.Determinism}
}

// effectiveRequest merges optional requirements without allowing either layer
// to weaken the other. Explicit conflicts refuse before availability/probing.
func effectiveRequest(route Request, direct provider.Request) (Request, provider.Request, error) {
	if route.ResponseFormat != "" && direct.ResponseFormat != "" && route.ResponseFormat != direct.ResponseFormat {
		return route, direct, fmt.Errorf("%w: conflicting response formats", errInvalidRequest)
	}
	if route.Determinism != "" && direct.Determinism != "" && route.Determinism != direct.Determinism {
		return route, direct, fmt.Errorf("%w: conflicting determinism contracts", errInvalidRequest)
	}
	if route.ResponseFormat == "" {
		route.ResponseFormat = direct.ResponseFormat
	}
	if route.Determinism == "" {
		route.Determinism = direct.Determinism
	}
	direct.ResponseFormat, direct.Determinism = route.ResponseFormat, route.Determinism
	route, err := normalizeRequest(route)
	if err == nil {
		if e := provider.ValidateRequirements(direct, route.RequireStreaming); e != nil {
			err = fmt.Errorf("%w: %w", errInvalidRequest, e)
		}
	}
	return route, direct, err
}

// Stream routes and executes a streaming request through the same policy and
// qualification path as buffered completion. Only a structured remote
// exhaustion before the first provider chunk can fall down to local; once any
// chunk is exposed, an error is terminal and the response is never replayed.
// The result channel yields exactly one terminal value (nil on success) and
// closes independently of chunk-consumer speed.
func (rt *Router) Stream(ctx context.Context, routeReq Request, provReq provider.Request) (<-chan provider.StreamChunk, <-chan error, Decision, error) {
	routeReq.RequireStreaming = true
	req, provReq, err := effectiveRequest(routeReq, provReq)
	if err != nil {
		d := Decision{RequestedOverride: routeReq.Override, FailureCategory: failureCategory(err)}
		return nil, nil, d, errors.Join(err, rt.recordDecision(req, d, "rejected", err))
	}
	d, err := rt.route(ctx, req)
	if err != nil {
		return nil, nil, d, errors.Join(err, rt.recordDecision(req, d, "rejected", err))
	}
	if err := ctx.Err(); err != nil {
		d.FailureCategory = "canceled"
		cause := fmt.Errorf("model-router: request canceled before streaming: %w", err)
		return nil, nil, d, errors.Join(cause, rt.recordDecision(req, d, "failed", cause))
	}

	streamCtx, cancel := context.WithCancel(ctx)
	chunks, err := openProviderStream(streamCtx, d.Provider, provReq)
	if err != nil {
		cancel()
		return rt.streamExhaustionFallback(ctx, req, provReq, d, err)
	}
	first, ok, err := firstStreamChunk(ctx, chunks)
	if err != nil {
		cancel()
		return rt.streamExhaustionFallback(ctx, req, provReq, d, err)
	}
	if !ok {
		cancel()
		cause := errors.New("model-router: provider stream ended before its first chunk")
		d.FailureCategory = "provider_failure"
		return nil, nil, d, errors.Join(providerFailure(d, cause), rt.recordDecision(req, d, "failed", cause))
	}
	if first.Err != nil && first.Text == "" {
		cancel()
		return rt.streamExhaustionFallback(ctx, req, provReq, d, first.Err)
	}
	d.Model = first.Model
	d.ProviderName = d.Provider.Name()
	forwarded, result := rt.forwardStream(ctx, cancel, req, d, first, chunks)
	return forwarded, result, d, nil
}

func openProviderStream(ctx context.Context, selected provider.Provider, req provider.Request) (<-chan provider.StreamChunk, error) {
	streamer, ok := selected.(provider.StreamingProvider)
	if !ok {
		return nil, errors.New("model-router: selected provider has no streaming execution interface")
	}
	chunks, err := streamer.Stream(ctx, req)
	if err != nil {
		return nil, err
	}
	if chunks == nil {
		return nil, errors.New("model-router: provider returned no stream")
	}
	return chunks, nil
}

func firstStreamChunk(ctx context.Context, chunks <-chan provider.StreamChunk) (provider.StreamChunk, bool, error) {
	select {
	case <-ctx.Done():
		return provider.StreamChunk{}, false, ctx.Err()
	case chunk, ok := <-chunks:
		return chunk, ok, nil
	}
}

func (rt *Router) streamExhaustionFallback(ctx context.Context, req Request, provReq provider.Request, d Decision, remoteCause error) (<-chan provider.StreamChunk, <-chan error, Decision, error) {
	category := providerExhaustionCategory(d, remoteCause)
	if ctxErr := ctx.Err(); ctxErr != nil || errors.Is(remoteCause, context.Canceled) || errors.Is(remoteCause, context.DeadlineExceeded) {
		if ctxErr == nil {
			ctxErr = remoteCause
		}
		d.FailureCategory = "canceled"
		combined := errors.Join(providerFailure(d, remoteCause), ctxErr)
		return nil, nil, d, errors.Join(combined, rt.recordDecision(req, d, "failed", combined))
	}
	if d.Lane != LaneRemote || req.Privacy != PrivacyShareable || req.Override == LaneLocal || category == "" {
		if category == "" {
			d.FailureCategory = failureCategory(remoteCause)
		} else {
			d.FailureCategory = category
		}
		cause := providerFailure(d, remoteCause)
		return nil, nil, d, errors.Join(cause, rt.recordDecision(req, d, "failed", cause))
	}

	remoteDecision := d
	local, _ := rt.splitLadder()
	selection, selectErr := rt.firstQualified(ctx, local, req)
	if selectErr != nil || selection.provider == nil {
		d.FallbackReason = category
		d.FailureCategory = "remote_exhausted_local_unavailable"
		if selectErr != nil && (errors.Is(selectErr, context.Canceled) || errors.Is(selectErr, context.DeadlineExceeded)) {
			d.FailureCategory = "canceled"
			d.Rationale = "remote streaming exhausted; local fallback selection canceled"
		}
		if selectErr == nil {
			selectErr = overrideUnavailableError(LaneLocal, req, selection)
		}
		combined := errors.Join(providerFailure(remoteDecision, remoteCause), fmt.Errorf("model-router: local streaming fallback unavailable: %w", selectErr))
		return nil, nil, d, errors.Join(combined, rt.recordDecision(req, d, "failed", combined))
	}

	d.InitialLane = LaneRemote
	d.Lane, d.Provider = LaneLocal, selection.provider
	d.Escalated, d.FallbackReason = true, category
	d.Rationale = "remote streaming exhausted before first chunk; one qualified local fallback"
	localCtx, cancel := context.WithCancel(ctx)
	localChunks, localErr := openProviderStream(localCtx, d.Provider, provReq)
	if localErr != nil {
		cancel()
		d.FailureCategory = "local_fallback_failed"
		if errors.Is(localErr, context.Canceled) || errors.Is(localErr, context.DeadlineExceeded) {
			d.FailureCategory = "canceled"
		}
		combined := errors.Join(providerFailure(remoteDecision, remoteCause), providerFailure(d, localErr))
		return nil, nil, d, errors.Join(combined, rt.recordDecision(req, d, "failed", combined))
	}
	first, ok, firstErr := firstStreamChunk(ctx, localChunks)
	if firstErr != nil || !ok || (first.Err != nil && first.Text == "") {
		cancel()
		d.FailureCategory = "local_fallback_failed"
		if firstErr == nil {
			if !ok {
				firstErr = errors.New("local stream ended before its first chunk")
			} else {
				firstErr = first.Err
			}
		}
		if errors.Is(firstErr, context.Canceled) || errors.Is(firstErr, context.DeadlineExceeded) {
			d.FailureCategory = "canceled"
		}
		combined := errors.Join(providerFailure(remoteDecision, remoteCause), providerFailure(d, firstErr))
		return nil, nil, d, errors.Join(combined, rt.recordDecision(req, d, "failed", combined))
	}
	d.Model = first.Model
	d.ProviderName = d.Provider.Name()
	forwarded, result := rt.forwardStream(ctx, cancel, req, d, first, localChunks)
	return forwarded, result, d, nil
}

func (rt *Router) forwardStream(ctx context.Context, cancel context.CancelFunc, req Request, d Decision, first provider.StreamChunk, source <-chan provider.StreamChunk) (<-chan provider.StreamChunk, <-chan error) {
	out := make(chan provider.StreamChunk)
	result := make(chan error, 1)
	go func() {
		defer close(out)
		defer cancel()
		outcome := "completed"
		if d.Escalated {
			outcome = "fallback_completed"
		}
		var terminalErr error
		send := func(chunk provider.StreamChunk) bool {
			select {
			case out <- chunk:
				return true
			case <-ctx.Done():
				terminalErr = errors.Join(terminalErr, ctx.Err())
				outcome = "failed"
				return false
			}
		}
		sendTerminal := func(chunk provider.StreamChunk) bool {
			// Terminal errors must remain observable after cancellation, but a
			// consumer that abandoned the stream must not strand this goroutine.
			timer := time.NewTimer(time.Second)
			defer timer.Stop()
			select {
			case out <- chunk:
				return true
			case <-timer.C:
				return false
			}
		}
		if first.Err != nil {
			terminalErr, outcome = first.Err, "failed"
		}
		if !send(first) {
			rt.finishStreamRecord(req, d, outcome, terminalErr, false, result, sendTerminal)
			return
		}
		if first.Done || first.Err != nil {
			rt.finishStreamRecord(req, d, outcome, terminalErr, first.Err != nil, result, sendTerminal)
			return
		}
		for {
			select {
			case <-ctx.Done():
				terminalErr, outcome = ctx.Err(), "failed"
				rt.finishStreamRecord(req, d, outcome, terminalErr, false, result, sendTerminal)
				return
			case chunk, ok := <-source:
				if !ok {
					terminalErr, outcome = errors.New("model-router: provider stream closed without a terminal chunk"), "failed"
					rt.finishStreamRecord(req, d, outcome, terminalErr, false, result, sendTerminal)
					return
				}
				if chunk.Model != "" && d.Model == "" {
					d.Model = chunk.Model
					d.ProviderName = d.Provider.Name()
				}
				if chunk.Err != nil {
					terminalErr, outcome = chunk.Err, "failed"
				}
				if !send(chunk) {
					rt.finishStreamRecord(req, d, "failed", terminalErr, false, result, sendTerminal)
					return
				}
				if chunk.Done || chunk.Err != nil {
					rt.finishStreamRecord(req, d, outcome, terminalErr, chunk.Err != nil, result, sendTerminal)
					return
				}
			}
		}
	}()
	return out, result
}

func (rt *Router) finishStreamRecord(req Request, d Decision, outcome string, cause error, causeDelivered bool, result chan<- error, sendTerminal func(provider.StreamChunk) bool) {
	terminalErr := errors.Join(cause, rt.recordDecision(req, d, outcome, cause))
	result <- terminalErr
	close(result)
	// causeDelivered only suppresses a duplicate chunk. The result channel is
	// the authoritative terminal outcome and always retains every cause.
	if terminalErr != nil && !causeDelivered {
		_ = sendTerminal(provider.StreamChunk{Err: terminalErr})
	}
}

// Complete routes and executes once, with a single qualified-local fall-down
// only for structured remote rate or recognized budget exhaustion.
func (rt *Router) Complete(ctx context.Context, routeReq Request, provReq provider.Request) (provider.Response, Decision, error) {
	req, provReq, err := effectiveRequest(routeReq, provReq)
	if err != nil {
		d := Decision{RequestedOverride: routeReq.Override, FailureCategory: failureCategory(err)}
		return provider.Response{}, d, errors.Join(err, rt.recordDecision(routeReq, d, "rejected", err))
	}
	if req.RequireStreaming {
		d := Decision{RequestedOverride: req.Override, FailureCategory: "streaming_requires_stream_path"}
		cause := errors.New("model-router: streaming request must use Router.Stream")
		return provider.Response{}, d, errors.Join(cause, rt.recordDecision(req, d, "rejected", cause))
	}
	d, err := rt.route(ctx, req)
	if err != nil {
		return provider.Response{}, d, errors.Join(err, rt.recordDecision(req, d, "rejected", err))
	}
	if err := ctx.Err(); err != nil {
		d.FailureCategory = "canceled"
		cancelErr := fmt.Errorf("model-router: request canceled before completion: %w", err)
		return provider.Response{}, d, errors.Join(cancelErr, rt.recordDecision(req, d, "failed", cancelErr))
	}
	resp, err := d.Provider.Complete(ctx, provReq)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = provider.ValidateResponse(provReq, resp)
	}
	if err == nil {
		d.Model = resp.Model
		d.ProviderName = responseProvider(resp, d.Provider)
		d.FailureCategory = ""
		return resp, d, rt.recordDecision(req, d, "completed", nil)
	}
	remoteErr := providerFailure(d, err)
	category := providerExhaustionCategory(d, err)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		d.FailureCategory = "canceled"
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		d.FailureCategory = "canceled"
		combined := errors.Join(remoteErr, ctxErr)
		return provider.Response{}, d, errors.Join(combined, rt.recordDecision(req, d, "failed", combined))
	}
	// A provider can return a joined cancellation/deadline and structured
	// exhaustion while the caller's parent context is still live. Cancellation
	// in either error chain is terminal; never begin a local completion then.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		d.FailureCategory = "canceled"
		return provider.Response{}, d, errors.Join(remoteErr, rt.recordDecision(req, d, "failed", remoteErr))
	}
	if d.Lane != LaneRemote || req.Privacy != PrivacyShareable || req.Override == LaneLocal || category == "" {
		if d.FailureCategory == "" {
			d.FailureCategory = category
		}
		if d.FailureCategory == "" {
			d.FailureCategory = "provider_failure"
			if errors.Is(err, provider.ErrInvalidJSONObject) {
				d.FailureCategory = "invalid_json_object"
			}
		}
		return provider.Response{}, d, errors.Join(remoteErr, rt.recordDecision(req, d, "failed", remoteErr))
	}
	local, _ := rt.splitLadder()
	selection, selectErr := rt.firstQualified(ctx, local, req)
	if selectErr != nil || selection.provider == nil {
		d.FallbackReason = category
		d.FailureCategory = "remote_exhausted_local_unavailable"
		if selectErr != nil && (errors.Is(selectErr, context.Canceled) || errors.Is(selectErr, context.DeadlineExceeded)) {
			d.FailureCategory = "canceled"
			d.Rationale = "remote execution exhausted; local fallback selection canceled"
		} else if selectErr == nil && selection.availabilityErr != nil {
			d.Rationale = "remote execution exhausted; local fallback unavailable"
			selectErr = fmt.Errorf("local availability refusal: %w", selection.availabilityErr)
		} else if selectErr == nil {
			d.Rationale = "remote execution exhausted; no qualified local fallback"
			selectErr = overrideUnavailableError(LaneLocal, req, selection)
		}
		combined := errors.Join(remoteErr, fmt.Errorf("model-router: local fallback unavailable: %w", selectErr))
		return provider.Response{}, d, errors.Join(combined, rt.recordDecision(req, d, "failed", combined))
	}

	d.InitialLane = LaneRemote
	d.Lane, d.Provider = LaneLocal, selection.provider
	d.Escalated, d.FallbackReason = true, category
	d.Rationale = "remote execution exhausted; one qualified local fallback"
	localResp, localErr := d.Provider.Complete(ctx, provReq)
	if localErr == nil {
		localErr = ctx.Err()
	}
	if localErr == nil {
		localErr = provider.ValidateResponse(provReq, localResp)
	}
	if localErr != nil {
		d.FailureCategory = "local_fallback_failed"
		if errors.Is(localErr, context.Canceled) || errors.Is(localErr, context.DeadlineExceeded) {
			d.FailureCategory = "canceled"
		}
		combined := errors.Join(remoteErr, providerFailure(d, localErr))
		return provider.Response{}, d, errors.Join(combined, rt.recordDecision(req, d, "failed", combined))
	}
	d.Model = localResp.Model
	d.ProviderName = responseProvider(localResp, d.Provider)
	d.FailureCategory = ""
	return localResp, d, rt.recordDecision(req, d, "fallback_completed", nil)
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

type providerSelection struct {
	generationErr                error
	provider                     provider.Provider
	available                    bool
	maxContextTokens             int
	streamingSupported           bool
	streamingUnknown             bool
	streamingDeclaredUnsupported bool
	streamingInterfaceMissing    bool
	availabilityErr              error
}

func mergeProviderSelections(a, b providerSelection) providerSelection {
	return providerSelection{
		generationErr:                errors.Join(a.generationErr, b.generationErr),
		available:                    a.available || b.available,
		maxContextTokens:             maxContext(a.maxContextTokens, b.maxContextTokens),
		streamingSupported:           a.streamingSupported || b.streamingSupported,
		streamingUnknown:             a.streamingUnknown || b.streamingUnknown,
		streamingDeclaredUnsupported: a.streamingDeclaredUnsupported || b.streamingDeclaredUnsupported,
		streamingInterfaceMissing:    a.streamingInterfaceMissing || b.streamingInterfaceMissing,
	}
}

// firstQualified returns the first available provider satisfying every
// capability currently expressible by the routing request.
func (rt *Router) firstQualified(ctx context.Context, providers []provider.Provider, req Request) (providerSelection, error) {
	var selection providerSelection
	pCtx, cancel := context.WithTimeout(ctx, rt.probeTimeout)
	defer cancel()
	for _, p := range providers {
		if err := ctx.Err(); err != nil {
			return providerSelection{}, fmt.Errorf("model-router: availability probe canceled: %w", err)
		}
		// Generation evidence is local metadata. Reject before a readiness probe
		// (which may itself perform a real completion) whenever it cannot fit.
		if err := provider.QualifyRequirements(generationRequest(req), p.Caps()); err != nil {
			selection.generationErr = err
			continue
		}
		if p.Available(pCtx) {
			if err := ctx.Err(); err != nil {
				return providerSelection{}, fmt.Errorf("model-router: availability probe canceled: %w", err)
			}
			selection.available = true
			caps := p.Caps()
			if err := provider.QualifyRequirements(generationRequest(req), caps); err != nil {
				selection.generationErr = err
				continue
			}
			contextTokens := caps.ContextTokens
			if err := ctx.Err(); err != nil {
				return providerSelection{}, fmt.Errorf("model-router: capability check canceled: %w", err)
			}
			if contextTokens > selection.maxContextTokens {
				selection.maxContextTokens = contextTokens
			}
			_, streamingInterface := p.(provider.StreamingProvider)
			streamingKnown := caps.StreamingKnown || caps.Streaming
			streamingSupported := streamingKnown && caps.Streaming && streamingInterface
			selection.streamingSupported = selection.streamingSupported || streamingSupported
			selection.streamingUnknown = selection.streamingUnknown || !streamingKnown
			selection.streamingDeclaredUnsupported = selection.streamingDeclaredUnsupported || (streamingKnown && !caps.Streaming)
			selection.streamingInterfaceMissing = selection.streamingInterfaceMissing || (streamingKnown && caps.Streaming && !streamingInterface)
			contextQualified := req.MinContextTokens == 0 || contextTokens >= req.MinContextTokens
			streamQualified := !req.RequireStreaming || streamingSupported
			if contextQualified && streamQualified {
				selection.provider = p
				return selection, nil
			}
		} else if detailed, ok := p.(interface{ AvailabilityError() error }); ok && selection.availabilityErr == nil {
			selection.availabilityErr = detailed.AvailabilityError()
		}
	}
	if err := ctx.Err(); err != nil {
		return providerSelection{}, fmt.Errorf("model-router: availability probe canceled: %w", err)
	}
	return selection, nil
}

func overrideUnavailableError(lane Lane, req Request, selection providerSelection) error {
	if selection.generationErr != nil || (selection.available && (req.MinContextTokens > 0 || req.RequireStreaming)) {
		return selectionRequirementError(string(lane), req, selection)
	}
	return fmt.Errorf("model-router: requested %s lane is unavailable", lane)
}

func selectionRequirementError(scope string, req Request, selection providerSelection) error {
	// Preserve the released context-only diagnostic while richer requirements
	// retain the donor's combined generation/streaming refusal below.
	if selection.generationErr == nil && !req.RequireStreaming && req.MinContextTokens > 0 && selection.maxContextTokens < req.MinContextTokens {
		return contextRequirementError(scope, selection.maxContextTokens, req.MinContextTokens)
	}
	var unmet []string
	if selection.generationErr != nil {
		unmet = append(unmet, selection.generationErr.Error())
	}
	if req.MinContextTokens > 0 && selection.maxContextTokens < req.MinContextTokens {
		unmet = append(unmet, fmt.Sprintf("context window %d tokens (best available: %d tokens)", req.MinContextTokens, selection.maxContextTokens))
	}
	if req.RequireStreaming && !selection.streamingSupported {
		state := ""
		if selection.streamingUnknown {
			state = "unknown"
		}
		if selection.streamingDeclaredUnsupported {
			if state != "" {
				state += " or "
			}
			state += "declared unsupported"
		}
		if selection.streamingInterfaceMissing {
			if state != "" {
				state += " or "
			}
			state += "missing the streaming execution interface"
		}
		if state == "" {
			state = "not explicitly supported"
		}
		unmet = append(unmet, "streaming capability is "+state)
	}
	if len(unmet) == 0 {
		return fmt.Errorf("model-router: no %s provider satisfies the request capabilities", scope)
	}
	message := strings.Join(unmet, "; ")
	if scope == "local-only" {
		return fmt.Errorf("model-router: no local provider satisfies required %s (fail-closed: content is local-only)", message)
	}
	return fmt.Errorf("model-router: no %s provider satisfies required %s", scope, message)
}

func availabilityFallbackReason(selection providerSelection) string {
	if selection.available && selection.provider == nil {
		return "remote_capability_insufficient"
	}
	if selection.availabilityErr != nil {
		if category := failureCategory(selection.availabilityErr); category != "provider_failure" {
			return category
		}
	}
	return "remote_unavailable_or_unqualified"
}

func remoteFailureCategory(selection providerSelection) string {
	if selection.available && selection.provider == nil {
		return "remote_capability_insufficient"
	}
	if selection.availabilityErr != nil {
		return failureCategory(selection.availabilityErr)
	}
	return "remote_unavailable"
}

func providerExhaustionCategory(d Decision, err error) string {
	if d.Lane != LaneRemote {
		return ""
	}
	var inference *provider.InferenceError
	if !errors.As(err, &inference) {
		return ""
	}
	return inferenceExhaustion(inference)
}

func providerFailure(d Decision, err error) error {
	if d.Provider == nil {
		return err
	}
	return fmt.Errorf("model-router: provider %s (%s): %w", d.Provider.Name(), d.Lane, err)
}

func responseProvider(resp provider.Response, selected provider.Provider) string {
	if resp.Provider != "" {
		return resp.Provider
	}
	if selected != nil {
		return selected.Name()
	}
	return ""
}

func failureCategory(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errInvalidRequest) {
		return "invalid_request"
	}
	if errors.Is(err, provider.ErrInvalidJSONObject) {
		return "invalid_json_object"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	var inference *provider.InferenceError
	if errors.As(err, &inference) {
		if category := inferenceExhaustion(inference); category != "" {
			return category
		}
		if inference.Permanent {
			return "provider_permanent_refusal"
		}
		return "provider_failure"
	}
	return "routing_or_provider_failure"
}

var errInvalidRequest = errors.New("model-router: invalid request")

func inferenceExhaustion(inference *provider.InferenceError) string {
	if inference.Status == 429 {
		return "remote_rate_exhausted"
	}
	if inference.Status != 402 {
		return ""
	}
	structured := strings.ToLower(inference.Code + " " + inference.Type)
	for _, marker := range []string{"quota", "budget", "billing", "payment_required"} {
		if strings.Contains(structured, marker) {
			return "remote_budget_exhausted"
		}
	}
	return ""
}

func contextRequirementError(scope string, availableContextTokens, requiredContextTokens int) error {
	if scope == "local-only" {
		return fmt.Errorf("model-router: no local provider satisfies required context window %d tokens (best available: %d tokens; fail-closed: content is local-only)", requiredContextTokens, availableContextTokens)
	}
	return fmt.Errorf("model-router: no %s provider satisfies required context window %d tokens (best available: %d tokens)", scope, requiredContextTokens, availableContextTokens)
}

func maxContext(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// DecisionSummary returns a compact one-line string for diagnostic output.
func (d Decision) DecisionSummary() string {
	if d.Provider == nil {
		if d.FailureCategory != "" {
			return "no-route (" + d.FailureCategory + ")"
		}
		return "no-route"
	}
	var sb strings.Builder
	sb.WriteString(string(d.Lane))
	sb.WriteByte('/')
	if d.ProviderName != "" {
		sb.WriteString(d.ProviderName)
	} else {
		sb.WriteString(d.Provider.Name())
	}
	if d.Model != "" {
		sb.WriteByte('/')
		sb.WriteString(d.Model)
	}
	if d.Escalated {
		sb.WriteString(" (escalated)")
	}
	if d.FallbackReason != "" {
		sb.WriteString(" [fallback: ")
		sb.WriteString(d.FallbackReason)
		sb.WriteByte(']')
	}
	sb.WriteString(": ")
	sb.WriteString(d.Rationale)
	return sb.String()
}
