package modelrouter

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

type memoryDecisionSink struct {
	records []DecisionRecord
	err     error
}

func (s *memoryDecisionSink) Record(r DecisionRecord) error {
	s.records = append(s.records, r)
	return s.err
}

func TestCompleteFallsDownOnceForStructuredRemoteExhaustion(t *testing.T) {
	for _, tc := range []struct {
		name, code, wantReason string
		status                 int
	}{
		{name: "rate limit", status: 429, code: "rate_limit", wantReason: "remote_rate_exhausted"},
		{name: "recognized quota", status: 402, code: "insufficient_quota", wantReason: "remote_budget_exhausted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := &stubProvider{name: "remote-api", tier: provider.TierRemote, ctx: 8192, avail: true}
			remote.completeFunc = func(context.Context, provider.Request) (provider.Response, error) {
				return provider.Response{}, &provider.InferenceError{Status: tc.status, Code: tc.code, Message: "secret backend error body"}
			}
			local := &stubProvider{name: "local-profile", tier: provider.TierLocal, ctx: 8192, avail: true}
			local.completeFunc = func(context.Context, provider.Request) (provider.Response, error) {
				return provider.Response{Text: "local answer", Provider: "local-profile", Model: "local-model", Tier: provider.TierLocal}, nil
			}
			sink := &memoryDecisionSink{}
			rt := New([]provider.Provider{local, remote}).WithDecisionSink(sink)
			resp, decision, err := rt.Complete(context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable}, provider.Request{Prompt: "private prompt"})
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if resp.Text != "local answer" || decision.Lane != LaneLocal || decision.InitialLane != LaneRemote || decision.FallbackReason != tc.wantReason {
				t.Fatalf("response/decision = %q/%+v", resp.Text, decision)
			}
			if decision.ProviderName != "local-profile" || decision.Model != "local-model" || !decision.Escalated {
				t.Fatalf("final route provenance = %+v", decision)
			}
			if remote.completeCall != 1 || local.completeCall != 1 || len(sink.records) != 1 {
				t.Fatalf("calls/log records = remote:%d local:%d records:%d", remote.completeCall, local.completeCall, len(sink.records))
			}
			rec := sink.records[0]
			if rec.Outcome != "fallback_completed" || rec.InitialLane != LaneRemote || rec.FinalLane != LaneLocal || rec.FailureCategory != "" {
				t.Fatalf("decision record = %+v", rec)
			}
			if strings.Contains(strings.Join([]string{rec.Provider, rec.Model, rec.Reason, rec.FailureCategory}, " "), "secret") {
				t.Fatal("raw provider error body leaked into decision evidence")
			}
		})
	}
}

func TestCompleteDoesNotFallbackWithoutStructuredEligibleExhaustion(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "unrecognized 402", err: &provider.InferenceError{Status: 402, Code: "payment_declined"}},
		{name: "authentication", err: &provider.InferenceError{Status: 401, Code: "invalid_api_key", Permanent: true}},
		{name: "ordinary error", err: errors.New("transport failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local := &stubProvider{name: "local", tier: provider.TierLocal, ctx: 8192, avail: true}
			remote := &stubProvider{name: "remote", tier: provider.TierRemote, ctx: 8192, avail: true, completeFunc: func(context.Context, provider.Request) (provider.Response, error) { return provider.Response{}, tc.err }}
			rt := testRouter([]provider.Provider{local, remote})
			_, _, err := rt.Complete(context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable}, provider.Request{})
			if err == nil || remote.completeCall != 1 || local.completeCall != 0 {
				t.Fatalf("error=%v remote calls=%d local calls=%d", err, remote.completeCall, local.completeCall)
			}
		})
	}
}

func TestCompletePreservesBothProviderCausesWhenFallbackFails(t *testing.T) {
	remoteCause, localCause := errors.New("remote cause"), errors.New("local cause")
	remote := &stubProvider{name: "remote", tier: provider.TierRemote, ctx: 8192, avail: true, completeFunc: func(context.Context, provider.Request) (provider.Response, error) {
		return provider.Response{}, errors.Join(remoteCause, &provider.InferenceError{Status: 429, Code: "rate_limit"})
	}}
	local := &stubProvider{name: "local", tier: provider.TierLocal, ctx: 8192, avail: true, completeFunc: func(context.Context, provider.Request) (provider.Response, error) {
		return provider.Response{}, localCause
	}}
	sink := &memoryDecisionSink{}
	_, d, err := New([]provider.Provider{local, remote}).WithDecisionSink(sink).Complete(context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable}, provider.Request{})
	if !errors.Is(err, remoteCause) || !errors.Is(err, localCause) {
		t.Fatalf("joined error lost a cause: %v", err)
	}
	if d.Lane != LaneLocal || d.FailureCategory != "local_fallback_failed" || remote.completeCall != 1 || local.completeCall != 1 {
		t.Fatalf("decision/calls = %+v remote:%d local:%d", d, remote.completeCall, local.completeCall)
	}
	if len(sink.records) != 1 || sink.records[0].Outcome != "failed" || sink.records[0].FinalLane != LaneLocal {
		t.Fatalf("decision evidence = %+v", sink.records)
	}
}

func TestCompleteRetainsRemoteCauseWhenQualifiedLocalFallbackIsUnavailable(t *testing.T) {
	remote := &stubProvider{name: "remote", tier: provider.TierRemote, ctx: 16384, avail: true, completeFunc: func(context.Context, provider.Request) (provider.Response, error) {
		return provider.Response{}, &provider.InferenceError{Status: 429, Code: "rate_limit"}
	}}
	local := &stubProvider{name: "short-local", tier: provider.TierLocal, ctx: 4096, avail: true}
	sink := &memoryDecisionSink{}
	_, d, err := New([]provider.Provider{local, remote}).WithDecisionSink(sink).Complete(context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable, MinContextTokens: 8192}, provider.Request{})
	var inference *provider.InferenceError
	if !errors.As(err, &inference) || inference.Status != 429 {
		t.Fatalf("remote structured cause lost: %v", err)
	}
	if d.Lane != LaneRemote || d.FailureCategory != "remote_exhausted_local_unavailable" || local.completeCall != 0 || remote.completeCall != 1 {
		t.Fatalf("decision/calls = %+v local:%d remote:%d", d, local.completeCall, remote.completeCall)
	}
	if len(sink.records) != 1 || sink.records[0].InitialLane != LaneRemote || sink.records[0].FinalLane != LaneRemote {
		t.Fatalf("final refusal evidence = %+v", sink.records)
	}
}

func TestJudgmentAvailabilityFailureIsRecordedAsFallbackReason(t *testing.T) {
	probeRefusal := &provider.InferenceError{Status: 402, Code: "quota_exceeded"}
	remote := &stubProvider{name: "remote", tier: provider.TierRemote, ctx: 8192, avail: false, availabilityErr: probeRefusal}
	local := &stubProvider{name: "local", tier: provider.TierLocal, ctx: 8192, avail: true}
	sink := &memoryDecisionSink{}
	d, err := New([]provider.Provider{remote, local}).WithDecisionSink(sink).Route(context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable})
	if err != nil || d.Lane != LaneLocal || d.FallbackReason != "remote_budget_exhausted" || d.FailureCategory != "remote_budget_exhausted" {
		t.Fatalf("availability fallback = %+v, %v", d, err)
	}
	if len(sink.records) != 1 || sink.records[0].FailureCategory != "remote_budget_exhausted" {
		t.Fatalf("availability cause not recorded: %+v", sink.records)
	}
}

func TestLocalOverrideNeverEscalatesAfterCompletionFailure(t *testing.T) {
	local := &stubProvider{name: "local", tier: provider.TierLocal, ctx: 8192, avail: true, completeFunc: func(context.Context, provider.Request) (provider.Response, error) {
		return provider.Response{}, &provider.InferenceError{Status: 429}
	}}
	remote := &stubProvider{name: "remote", tier: provider.TierRemote, ctx: 8192, avail: true}
	_, d, err := testRouter([]provider.Provider{remote, local}).Complete(context.Background(), Request{Privacy: PrivacyShareable, Override: LaneLocal}, provider.Request{})
	if err == nil || d.Lane != LaneLocal || local.completeCall != 1 || remote.availableCall != 0 || remote.completeCall != 0 {
		t.Fatalf("local override escaped its lane: decision=%+v err=%v calls local=%d remote probe=%d complete=%d", d, err, local.completeCall, remote.availableCall, remote.completeCall)
	}
}

func TestCompleteDoesNotRetryCanceledOrLocalOnlyRequests(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	remote := &stubProvider{name: "remote", tier: provider.TierRemote, ctx: 8192, avail: true}
	remote.completeFunc = func(context.Context, provider.Request) (provider.Response, error) {
		cancel()
		return provider.Response{}, context.Canceled
	}
	local := &stubProvider{name: "local", tier: provider.TierLocal, ctx: 8192, avail: true}
	_, _, _ = testRouter([]provider.Provider{local, remote}).Complete(canceled, Request{Task: TaskJudgment, Privacy: PrivacyShareable}, provider.Request{})
	if remote.completeCall != 1 || local.completeCall != 0 {
		t.Fatalf("cancellation retried: remote:%d local:%d", remote.completeCall, local.completeCall)
	}

	localOnly := &stubProvider{name: "private-local", tier: provider.TierLocal, ctx: 8192, avail: false}
	privateRemote := &stubProvider{name: "must-not-probe", tier: provider.TierRemote, ctx: 8192, avail: true}
	_, _, _ = testRouter([]provider.Provider{localOnly, privateRemote}).Complete(context.Background(), Request{Task: TaskGeneration, Privacy: PrivacyLocalOnly}, provider.Request{})
	if privateRemote.availableCall != 0 || privateRemote.completeCall != 0 {
		t.Fatalf("local-only request touched remote: probe=%d completion=%d", privateRemote.availableCall, privateRemote.completeCall)
	}
}

func TestOverridesAreValidatedPrivacyFirstAndRecorded(t *testing.T) {
	for _, tc := range []struct {
		name       string
		req        Request
		wantLane   Lane
		wantErr    bool
		wantProbes []int
	}{
		{name: "local override", req: Request{Privacy: PrivacyShareable, Override: LaneLocal}, wantLane: LaneLocal, wantProbes: []int{1, 0}},
		{name: "remote override", req: Request{Privacy: PrivacyShareable, Override: LaneRemote}, wantLane: LaneRemote, wantProbes: []int{0, 1}},
		{name: "privacy wins", req: Request{Privacy: PrivacyLocalOnly, Override: LaneRemote}, wantErr: true, wantProbes: []int{0, 0}},
		{name: "unknown override fails before probing", req: Request{Privacy: PrivacyShareable, Override: Lane("cloud")}, wantErr: true, wantProbes: []int{0, 0}},
		{name: "unavailable requested lane refuses", req: Request{Privacy: PrivacyShareable, Override: LaneRemote}, wantErr: true, wantProbes: []int{0, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local := &stubProvider{name: "local", tier: provider.TierLocal, ctx: 8192, avail: true}
			remoteAvailable := tc.name != "unavailable requested lane refuses"
			remote := &stubProvider{name: "remote", tier: provider.TierRemote, ctx: 8192, avail: remoteAvailable}
			sink := &memoryDecisionSink{}
			d, err := New([]provider.Provider{local, remote}).WithDecisionSink(sink).Route(context.Background(), tc.req)
			if (err != nil) != tc.wantErr || d.Lane != tc.wantLane {
				t.Fatalf("decision=%+v err=%v", d, err)
			}
			if local.availableCall != tc.wantProbes[0] || remote.availableCall != tc.wantProbes[1] {
				t.Fatalf("probe calls=%d,%d want=%v", local.availableCall, remote.availableCall, tc.wantProbes)
			}
			wantOverride := tc.req.Override
			if wantOverride != LaneLocal && wantOverride != LaneRemote {
				wantOverride = ""
			}
			if len(sink.records) != 1 || sink.records[0].RequestedOverride != wantOverride {
				t.Fatalf("override not recorded: %+v", sink.records)
			}
		})
	}
}

func TestCompleteDecisionSinkFailureDoesNotRepeatInference(t *testing.T) {
	local := &stubProvider{name: "local", tier: provider.TierLocal, ctx: 8192, avail: true}
	sink := &memoryDecisionSink{err: errors.New("disk full")}
	resp, d, err := New([]provider.Provider{local}).WithDecisionSink(sink).Complete(context.Background(), Request{}, provider.Request{Prompt: "must-not-be-recorded"})
	var recordErr *DecisionRecordError
	if !errors.As(err, &recordErr) || resp.Text != "stub:local" || d.Lane != LaneLocal {
		t.Fatalf("result=%q decision=%+v err=%v", resp.Text, d, err)
	}
	if local.completeCall != 1 || len(sink.records) != 1 {
		t.Fatalf("completion repeated after log failure: calls=%d records=%d", local.completeCall, len(sink.records))
	}
	if sink.records[0].Task != TaskGeneration || sink.records[0].Privacy != PrivacyLocalOnly || sink.records[0].Latency != "" {
		t.Fatalf("decision journal changed request defaults: %+v", sink.records[0])
	}
	if strings.Contains(strings.Join([]string{sink.records[0].Reason, sink.records[0].Provider, sink.records[0].Model}, " "), "must-not-be-recorded") {
		t.Fatal("prompt leaked into decision record")
	}
}

func TestDecisionSinkReceivesRedactedProviderMetadata(t *testing.T) {
	local := &stubProvider{name: "profile@example.com", tier: provider.TierLocal, ctx: 8192, avail: true, completeFunc: func(context.Context, provider.Request) (provider.Response, error) {
		return provider.Response{Text: "private answer", Provider: "profile@example.com", Model: "Bearer secret-token"}, nil
	}}
	sink := &memoryDecisionSink{}
	_, _, err := New([]provider.Provider{local}).WithDecisionSink(sink).Complete(context.Background(), Request{}, provider.Request{Prompt: "private prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.records) != 1 {
		t.Fatalf("records = %d, want one", len(sink.records))
	}
	joined := strings.Join([]string{sink.records[0].Provider, sink.records[0].Model, sink.records[0].Reason}, " ")
	if strings.Contains(joined, "profile@example.com") || strings.Contains(joined, "Bearer") || strings.Contains(joined, "secret-token") || strings.Contains(joined, "private prompt") || strings.Contains(joined, "private answer") {
		t.Fatalf("decision sink received sensitive content: %+v", sink.records[0])
	}
	if sink.records[0].Provider != "redacted" || sink.records[0].Model != "redacted" {
		t.Fatalf("unsafe provider metadata not replaced: %+v", sink.records[0])
	}
}
