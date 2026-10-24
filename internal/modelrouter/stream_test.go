package modelrouter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

type streamProvider struct {
	name        string
	tier        provider.Tier
	available   bool
	streaming   bool
	streamKnown bool
	streamCall  int
	streamFn    func(context.Context, provider.Request) (<-chan provider.StreamChunk, error)
}

func (p *streamProvider) Name() string        { return p.name }
func (p *streamProvider) Tier() provider.Tier { return p.tier }
func (p *streamProvider) Caps() provider.Caps {
	return provider.Caps{Streaming: p.streaming, StreamingKnown: p.streamKnown, ContextTokens: 16384, Offline: p.tier == provider.TierLocal}
}
func (p *streamProvider) Available(context.Context) bool { return p.available }
func (p *streamProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{}, errors.New("buffered path must not be called")
}
func (p *streamProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.StreamChunk, error) {
	p.streamCall++
	if p.streamFn != nil {
		return p.streamFn(ctx, req)
	}
	chunks := make(chan provider.StreamChunk, 2)
	chunks <- provider.StreamChunk{Text: "answer", Model: "fixture-model"}
	chunks <- provider.StreamChunk{Done: true, FinishReason: "stop"}
	close(chunks)
	return chunks, nil
}

func TestStreamRoutesAndForwardsQualifiedProviderRequest(t *testing.T) {
	local := &streamProvider{name: "local-profile", tier: provider.TierLocal, available: true, streaming: true, streamFn: func(_ context.Context, req provider.Request) (<-chan provider.StreamChunk, error) {
		if req.Prompt != "fixture" || req.MaxTokens != 7 || req.Seed == nil || *req.Seed != 23 {
			t.Errorf("provider generation request was changed or dropped: %+v", req)
		}
		chunks := make(chan provider.StreamChunk, 2)
		chunks <- provider.StreamChunk{Text: "answer", Model: "fixture-model"}
		chunks <- provider.StreamChunk{Done: true}
		close(chunks)
		return chunks, nil
	}}
	sink := &memoryDecisionSink{}
	seed := int64(23)
	chunks, result, decision, err := New([]provider.Provider{local}).WithDecisionSink(sink).Stream(context.Background(), Request{Privacy: PrivacyLocalOnly}, provider.Request{Prompt: "fixture", MaxTokens: 7, Seed: &seed})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if decision.Provider != local || decision.Lane != LaneLocal || local.streamCall != 1 {
		t.Fatalf("decision/calls = %+v/%d", decision, local.streamCall)
	}
	var text strings.Builder
	for chunk := range chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error = %v", chunk.Err)
		}
		text.WriteString(chunk.Text)
	}
	if terminalErr, ok := <-result; !ok || terminalErr != nil || text.String() != "answer" || len(sink.records) != 1 || sink.records[0].Outcome != "completed" {
		t.Fatalf("text/result/evidence = %q/%v/%+v", text.String(), terminalErr, sink.records)
	}
}

func TestStreamRequiresExplicitCapabilityAndHonorsPrivacy(t *testing.T) {
	local := &streamProvider{name: "unprofiled-local", tier: provider.TierLocal, available: true}
	remote := &streamProvider{name: "remote", tier: provider.TierRemote, available: true, streaming: true}
	chunks, _, decision, err := New([]provider.Provider{local, remote}).WithDecisionSink(&memoryDecisionSink{}).Stream(
		context.Background(), Request{Privacy: PrivacyLocalOnly}, provider.Request{},
	)
	if err == nil || chunks != nil || decision.Provider != nil || local.streamCall != 0 || remote.streamCall != 0 {
		t.Fatalf("unknown local streaming capability was not refused: stream=%v decision=%+v err=%v calls=%d/%d", chunks, decision, err, local.streamCall, remote.streamCall)
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown capability refusal misreported evidence: %v", err)
	}
	local.streamKnown = true
	_, _, _, err = New([]provider.Provider{local}).WithDecisionSink(&memoryDecisionSink{}).Stream(
		context.Background(), Request{Privacy: PrivacyLocalOnly}, provider.Request{},
	)
	if err == nil || !strings.Contains(err.Error(), "declared unsupported") {
		t.Fatalf("explicit unsupported capability refusal misreported evidence: %v", err)
	}
}

func TestStreamRemoteExhaustionFallsDownOnceBeforeFirstChunk(t *testing.T) {
	for _, tc := range []struct {
		name, code, reason string
		status             int
	}{
		{name: "rate limit", status: 429, code: "rate_limit", reason: "remote_rate_exhausted"},
		{name: "recognized quota", status: 402, code: "insufficient_quota", reason: "remote_budget_exhausted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := &streamProvider{name: "remote", tier: provider.TierRemote, available: true, streaming: true, streamFn: func(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
				return nil, &provider.InferenceError{Status: tc.status, Code: tc.code, Message: "private backend response"}
			}}
			local := &streamProvider{name: "local-profile", tier: provider.TierLocal, available: true, streaming: true}
			sink := &memoryDecisionSink{}
			chunks, result, decision, err := New([]provider.Provider{local, remote}).WithDecisionSink(sink).Stream(
				context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable}, provider.Request{Prompt: "never journal this"},
			)
			if err != nil {
				t.Fatalf("Stream() error = %v", err)
			}
			if decision.Lane != LaneLocal || decision.InitialLane != LaneRemote || decision.FallbackReason != tc.reason || remote.streamCall != 1 || local.streamCall != 1 {
				t.Fatalf("decision/calls = %+v remote:%d local:%d", decision, remote.streamCall, local.streamCall)
			}
			for chunk := range chunks {
				if chunk.Err != nil {
					t.Fatalf("fallback chunk error = %v", chunk.Err)
				}
			}
			if terminalErr, ok := <-result; !ok || terminalErr != nil || len(sink.records) != 1 || sink.records[0].FinalLane != LaneLocal || sink.records[0].Outcome != "fallback_completed" {
				t.Fatalf("terminal result/decision evidence = %v/%+v", terminalErr, sink.records)
			}
			if strings.Contains(sink.records[0].Reason, "private backend response") || strings.Contains(sink.records[0].Reason, "never journal") {
				t.Fatalf("private content leaked into decision record: %+v", sink.records[0])
			}
		})
	}
}

func TestStreamDoesNotFallbackAfterExposingPartialOutput(t *testing.T) {
	remote := &streamProvider{name: "remote", tier: provider.TierRemote, available: true, streaming: true, streamFn: func(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
		chunks := make(chan provider.StreamChunk, 2)
		chunks <- provider.StreamChunk{Text: "partial", Model: "remote-model"}
		chunks <- provider.StreamChunk{Err: &provider.InferenceError{Status: 429, Code: "rate_limit"}}
		close(chunks)
		return chunks, nil
	}}
	local := &streamProvider{name: "local", tier: provider.TierLocal, available: true, streaming: true}
	chunks, result, decision, err := New([]provider.Provider{local, remote}).WithDecisionSink(&memoryDecisionSink{}).Stream(
		context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable}, provider.Request{},
	)
	if err != nil {
		t.Fatalf("Stream() error before first chunk = %v", err)
	}
	var got strings.Builder
	var streamErr error
	for chunk := range chunks {
		got.WriteString(chunk.Text)
		if chunk.Err != nil {
			streamErr = chunk.Err
		}
	}
	terminalResult, resultOpen := <-result
	var resultCause *provider.InferenceError
	if got.String() != "partial" || streamErr == nil || !resultOpen || !errors.As(terminalResult, &resultCause) || decision.Lane != LaneRemote || remote.streamCall != 1 || local.streamCall != 0 {
		t.Fatalf("partial stream replayed or error lost: text=%q chunkErr=%v result=%v decision=%+v calls=%d/%d", got.String(), streamErr, terminalResult, decision, remote.streamCall, local.streamCall)
	}
}

func TestStreamResultJoinsAlreadyDeliveredProviderCauseAndDecisionWriteFailure(t *testing.T) {
	providerCause := &provider.InferenceError{Status: 429, Code: "rate_limit", Message: "private provider body"}
	remote := &streamProvider{name: "remote", tier: provider.TierRemote, available: true, streaming: true, streamFn: func(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
		chunks := make(chan provider.StreamChunk, 2)
		chunks <- provider.StreamChunk{Text: "partial"}
		chunks <- provider.StreamChunk{Err: providerCause}
		close(chunks)
		return chunks, nil
	}}
	sink := &memoryDecisionSink{err: errors.New("fixture journal failure")}
	chunks, result, _, err := New([]provider.Provider{remote}).WithDecisionSink(sink).Stream(
		context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable}, provider.Request{},
	)
	if err != nil {
		t.Fatalf("Stream() error before first chunk = %v", err)
	}
	var text strings.Builder
	var deliveredProviderCause error
	chunkCauseCount := 0
	for chunk := range chunks {
		text.WriteString(chunk.Text)
		if chunk.Err != nil {
			chunkCauseCount++
			deliveredProviderCause = chunk.Err
		}
	}
	terminalErr, resultOpen := <-result
	var gotProviderCause *provider.InferenceError
	var gotRecordErr *DecisionRecordError
	if text.String() != "partial" || chunkCauseCount != 1 || !errors.As(deliveredProviderCause, &gotProviderCause) ||
		!resultOpen || !errors.As(terminalErr, &gotProviderCause) || !errors.As(terminalErr, &gotRecordErr) ||
		len(sink.records) != 1 || sink.records[0].Outcome != "failed" {
		t.Fatalf("terminal result lost/journaled a cause or duplicated the delivered chunk: text=%q chunk=%v result=%v count=%d records=%+v", text.String(), deliveredProviderCause, terminalErr, chunkCauseCount, sink.records)
	}
}

func TestStreamClosedAfterPartialOutputSurfacesTerminalCause(t *testing.T) {
	remote := &streamProvider{name: "remote", tier: provider.TierRemote, available: true, streaming: true, streamFn: func(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
		chunks := make(chan provider.StreamChunk, 1)
		chunks <- provider.StreamChunk{Text: "partial", Model: "remote-model"}
		close(chunks)
		return chunks, nil
	}}
	local := &streamProvider{name: "local", tier: provider.TierLocal, available: true, streaming: true}
	sink := &memoryDecisionSink{}
	chunks, result, decision, err := New([]provider.Provider{local, remote}).WithDecisionSink(sink).Stream(
		context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable}, provider.Request{},
	)
	if err != nil {
		t.Fatalf("Stream() error before first chunk = %v", err)
	}
	var got strings.Builder
	var terminalErr error
	for chunk := range chunks {
		got.WriteString(chunk.Text)
		if chunk.Err != nil {
			terminalErr = chunk.Err
		}
	}
	if got.String() != "partial" || terminalErr == nil || !strings.Contains(terminalErr.Error(), "closed without a terminal chunk") {
		t.Fatalf("unexpected EOF was not delivered after partial output: text=%q err=%v", got.String(), terminalErr)
	}
	if resultErr, ok := <-result; !ok || resultErr == nil || decision.Lane != LaneRemote || remote.streamCall != 1 || local.streamCall != 0 || len(sink.records) != 1 || sink.records[0].Outcome != "failed" {
		t.Fatalf("unexpected EOF terminal result or decision: result=%v decision=%+v calls=%d/%d records=%+v", resultErr, decision, remote.streamCall, local.streamCall, sink.records)
	}
}

func TestStreamTerminalCauseAndDecisionWriteFailureAreBothSurfaced(t *testing.T) {
	local := &streamProvider{name: "local", tier: provider.TierLocal, available: true, streaming: true, streamFn: func(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
		chunks := make(chan provider.StreamChunk, 1)
		chunks <- provider.StreamChunk{Text: "partial", Model: "remote-model"}
		close(chunks)
		return chunks, nil
	}}
	sink := &memoryDecisionSink{err: errors.New("fixture journal failure")}
	chunks, result, _, err := New([]provider.Provider{local}).WithDecisionSink(sink).Stream(
		context.Background(), Request{Privacy: PrivacyLocalOnly}, provider.Request{},
	)
	if err != nil {
		t.Fatalf("Stream() error before first chunk = %v", err)
	}
	var got strings.Builder
	var terminalErr error
	terminalCount := 0
	for chunk := range chunks {
		got.WriteString(chunk.Text)
		if chunk.Err != nil {
			terminalErr = chunk.Err
			terminalCount++
		}
	}
	var recordErr *DecisionRecordError
	resultErr, resultOpen := <-result
	if got.String() != "partial" || terminalCount != 1 || terminalErr == nil || !strings.Contains(terminalErr.Error(), "closed without a terminal chunk") || !errors.As(terminalErr, &recordErr) || !resultOpen || !errors.As(resultErr, &recordErr) {
		t.Fatalf("terminal cause or journal failure was lost/duplicated: text=%q chunk=%v result=%v count=%d", got.String(), terminalErr, resultErr, terminalCount)
	}
}

func TestStreamCancellationAfterPartialOutputSurfacesOnce(t *testing.T) {
	remote := &streamProvider{name: "remote", tier: provider.TierRemote, available: true, streaming: true, streamFn: func(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
		chunks := make(chan provider.StreamChunk, 1)
		chunks <- provider.StreamChunk{Text: "partial", Model: "remote-model"}
		return chunks, nil
	}}
	local := &streamProvider{name: "local", tier: provider.TierLocal, available: true, streaming: true}
	sink := &memoryDecisionSink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	chunks, result, decision, err := New([]provider.Provider{local, remote}).WithDecisionSink(sink).Stream(
		ctx, Request{Task: TaskJudgment, Privacy: PrivacyShareable}, provider.Request{},
	)
	if err != nil {
		t.Fatalf("Stream() error before first chunk = %v", err)
	}
	first, ok := <-chunks
	if !ok || first.Text != "partial" {
		t.Fatalf("first stream chunk = %+v, ok=%t", first, ok)
	}
	cancel()
	var terminalErr error
	for chunk := range chunks {
		if chunk.Err != nil {
			if terminalErr != nil {
				t.Fatalf("cancellation cause was delivered more than once: first=%v next=%v", terminalErr, chunk.Err)
			}
			terminalErr = chunk.Err
		}
	}
	resultErr, resultOpen := <-result
	if !errors.Is(terminalErr, context.Canceled) || !resultOpen || !errors.Is(resultErr, context.Canceled) || decision.Lane != LaneRemote || remote.streamCall != 1 || local.streamCall != 0 || len(sink.records) != 1 || sink.records[0].Outcome != "failed" {
		t.Fatalf("cancellation was lost or retried: chunkErr=%v result=%v decision=%+v calls=%d/%d records=%+v", terminalErr, resultErr, decision, remote.streamCall, local.streamCall, sink.records)
	}
}

func TestStreamAbandonedConsumerHasBoundedTerminalDelivery(t *testing.T) {
	local := &streamProvider{name: "local", tier: provider.TierLocal, available: true, streaming: true, streamFn: func(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
		chunks := make(chan provider.StreamChunk, 1)
		chunks <- provider.StreamChunk{Text: "partial"}
		close(chunks)
		return chunks, nil
	}}
	chunks, result, _, err := New([]provider.Provider{local}).WithDecisionSink(&memoryDecisionSink{}).Stream(
		context.Background(), Request{Privacy: PrivacyLocalOnly}, provider.Request{},
	)
	if err != nil {
		t.Fatalf("Stream() error before first chunk = %v", err)
	}
	if first, ok := <-chunks; !ok || first.Text != "partial" {
		t.Fatalf("first chunk = %+v, ok=%t", first, ok)
	}
	terminalErr, ok := <-result
	if !ok || terminalErr == nil || !strings.Contains(terminalErr.Error(), "closed without a terminal chunk") {
		t.Fatalf("abandoned consumer lost latched terminal result: result=%v open=%t", terminalErr, ok)
	}
	// Leave the chunk consumer abandoned past the terminal-chunk delivery bound.
	time.Sleep(1500 * time.Millisecond)
	select {
	case _, ok := <-chunks:
		if ok {
			t.Fatal("terminal chunk remained queued after the bounded abandoned-consumer wait")
		}
	case <-time.After(time.Second):
		t.Fatal("abandoned stream did not close after bounded terminal delivery")
	}
}

func TestStreamCancellationJoinedWithExhaustionIsTerminal(t *testing.T) {
	remote := &streamProvider{name: "remote", tier: provider.TierRemote, available: true, streaming: true, streamFn: func(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
		return nil, errors.Join(context.Canceled, &provider.InferenceError{Status: 429, Code: "rate_limit"})
	}}
	local := &streamProvider{name: "local", tier: provider.TierLocal, available: true, streaming: true}
	sink := &memoryDecisionSink{}
	_, _, decision, err := New([]provider.Provider{local, remote}).WithDecisionSink(sink).Stream(
		context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable}, provider.Request{},
	)
	if !errors.Is(err, context.Canceled) || remote.streamCall != 1 || local.streamCall != 0 || decision.FailureCategory != "canceled" {
		t.Fatalf("joined cancellation triggered fallback: decision=%+v err=%v calls=%d/%d", decision, err, remote.streamCall, local.streamCall)
	}
}

func TestStreamParentCancellationStopsBeforeAnyLocalFallback(t *testing.T) {
	started := make(chan struct{})
	remote := &streamProvider{name: "remote", tier: provider.TierRemote, available: true, streaming: true, streamFn: func(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
		close(started)
		return make(chan provider.StreamChunk), nil
	}}
	local := &streamProvider{name: "local", tier: provider.TierLocal, available: true, streaming: true}
	type result struct {
		err error
	}
	results := make(chan result, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_, _, _, err := New([]provider.Provider{local, remote}).WithDecisionSink(&memoryDecisionSink{}).Stream(
			ctx, Request{Task: TaskJudgment, Privacy: PrivacyShareable}, provider.Request{},
		)
		results <- result{err: err}
	}()
	<-started
	cancel()
	got := <-results
	if !errors.Is(got.err, context.Canceled) || remote.streamCall != 1 || local.streamCall != 0 {
		t.Fatalf("parent cancellation did not terminate stream: err=%v calls=%d/%d", got.err, remote.streamCall, local.streamCall)
	}
}

func TestCompleteRefusesStreamingRequest(t *testing.T) {
	local := &streamProvider{name: "local", tier: provider.TierLocal, available: true, streaming: true}
	_, decision, err := New([]provider.Provider{local}).WithDecisionSink(&memoryDecisionSink{}).Complete(
		context.Background(), Request{RequireStreaming: true}, provider.Request{},
	)
	if err == nil || decision.FailureCategory != "streaming_requires_stream_path" || local.streamCall != 0 {
		t.Fatalf("Complete() did not enforce the actual streaming path: decision=%+v err=%v", decision, err)
	}
}
