package modelrouter

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

type generationProvider struct {
	*stubProvider
	caps provider.Caps
}

func (p *generationProvider) Caps() provider.Caps { return p.caps }

func jsonProvider(name string, tier provider.Tier, state provider.CapabilityState) *generationProvider {
	return &generationProvider{
		stubProvider: &stubProvider{name: name, tier: tier, avail: true, completeFunc: func(_ context.Context, req provider.Request) (provider.Response, error) {
			return provider.Response{Text: `{"ok":true}`, FinishReason: "stop", Provider: name, Model: "fixture-model", Tier: tier}, nil
		}},
		caps: provider.Caps{JSONObject: state, ContextTokens: 8192},
	}
}

func TestGenerationRequirementsRouteThreeStates(t *testing.T) {
	for _, state := range []provider.CapabilityState{provider.CapabilityUnknown, provider.CapabilityUnsupported, provider.CapabilitySupported} {
		t.Run("state-"+string(state), func(t *testing.T) {
			p := jsonProvider("local", provider.TierLocal, state)
			d, err := testRouter([]provider.Provider{p}).Route(context.Background(), Request{ResponseFormat: provider.ResponseFormatJSONObject})
			if state == provider.CapabilitySupported {
				if err != nil || d.Provider != p || p.availableCall != 1 {
					t.Fatalf("supported route=%+v err=%v probes=%d", d, err, p.availableCall)
				}
			} else {
				want := "unknown"
				if state == provider.CapabilityUnsupported {
					want = "declared unsupported"
				}
				if err == nil || !strings.Contains(err.Error(), want) || p.availableCall != 0 || p.completeCall != 0 {
					t.Fatalf("unqualified provider was probed/executed: %v probes=%d complete=%d", err, p.availableCall, p.completeCall)
				}
			}
		})
	}
}

func TestGenerationRequirementsChangedEvidenceAfterProbeRefuses(t *testing.T) {
	p := jsonProvider("local", provider.TierLocal, provider.CapabilitySupported)
	p.onAvailable = func() { p.caps.JSONObject = provider.CapabilityUnsupported }
	_, _, err := testRouter([]provider.Provider{p}).Complete(context.Background(), Request{ResponseFormat: provider.ResponseFormatJSONObject}, provider.Request{})
	if err == nil || p.availableCall != 1 || p.completeCall != 0 {
		t.Fatalf("changed evidence was consumed: err=%v probes=%d calls=%d", err, p.availableCall, p.completeCall)
	}
}

func TestGenerationRequirementsDefaultRemainsText(t *testing.T) {
	p := jsonProvider("local", provider.TierLocal, provider.CapabilityUnknown)
	p.completeFunc = func(_ context.Context, req provider.Request) (provider.Response, error) {
		if req.ResponseFormat != "" || req.Determinism != "" {
			t.Error("unset defaults changed")
		}
		return provider.Response{Text: "ordinary text"}, nil
	}
	resp, _, err := testRouter([]provider.Provider{p}).Complete(context.Background(), Request{}, provider.Request{})
	if err != nil || resp.Text != "ordinary text" || p.completeCall != 1 {
		t.Fatalf("default text regressed: %+v %v", resp, err)
	}
}

func TestGenerationRequirementsEffectiveRequestAndConflicts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		route  Request
		direct provider.Request
		refuse bool
	}{
		{"route-only", Request{ResponseFormat: provider.ResponseFormatJSONObject}, provider.Request{}, false},
		{"provider-only", Request{}, provider.Request{ResponseFormat: provider.ResponseFormatJSONObject}, false},
		{"matching", Request{ResponseFormat: provider.ResponseFormatJSONObject}, provider.Request{ResponseFormat: provider.ResponseFormatJSONObject}, false},
		{"format-conflict", Request{ResponseFormat: provider.ResponseFormatJSONObject}, provider.Request{ResponseFormat: provider.ResponseFormatText}, true},
		{"format-invalid", Request{ResponseFormat: "json_schema"}, provider.Request{}, true},
		{"determinism-invalid", Request{Determinism: "seed"}, provider.Request{}, true},
		{"determinism-conflict", Request{Determinism: provider.DeterminismBatchInvariant}, provider.Request{Determinism: "other"}, true},
		{"tools-conflict", Request{ResponseFormat: provider.ResponseFormatJSONObject}, provider.Request{RequireTool: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := jsonProvider("local", provider.TierLocal, provider.CapabilitySupported)
			p.completeFunc = func(_ context.Context, req provider.Request) (provider.Response, error) {
				if req.ResponseFormat != provider.ResponseFormatJSONObject {
					t.Error("effective requirement was dropped before execution")
				}
				return provider.Response{Text: `{}`, FinishReason: "stop"}, nil
			}
			sink := &memoryDecisionSink{}
			_, _, err := New([]provider.Provider{p}).WithDecisionSink(sink).Complete(context.Background(), tc.route, tc.direct)
			if tc.refuse {
				if !errors.Is(err, errInvalidRequest) || p.availableCall != 0 || p.completeCall != 0 {
					t.Fatalf("conflict reached provider: err=%v probes=%d complete=%d", err, p.availableCall, p.completeCall)
				}
			} else if err != nil || p.completeCall != 1 || sink.records[0].ResponseFormat != provider.ResponseFormatJSONObject {
				t.Fatalf("effective request error=%v calls=%d records=%+v", err, p.completeCall, sink.records)
			}
			if len(sink.records) != 1 {
				t.Fatalf("records=%d", len(sink.records))
			}
		})
	}
}

func TestGenerationRequirementsPrivacyOverrideAndStream(t *testing.T) {
	for _, tc := range []struct {
		name   string
		route  Request
		stream bool
		direct provider.Request
	}{
		{"privacy-first", Request{Override: LaneRemote, ResponseFormat: provider.ResponseFormatJSONObject}, false, provider.Request{}},
		{"route-stream", Request{ResponseFormat: provider.ResponseFormatJSONObject}, true, provider.Request{}},
		{"direct-stream", Request{}, true, provider.Request{ResponseFormat: provider.ResponseFormatJSONObject}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local := jsonProvider("local", provider.TierLocal, provider.CapabilitySupported)
			remote := jsonProvider("remote", provider.TierRemote, provider.CapabilitySupported)
			sink := &memoryDecisionSink{}
			rt := New([]provider.Provider{local, remote}).WithDecisionSink(sink)
			var err error
			var d Decision
			if tc.stream {
				_, _, d, err = rt.Stream(context.Background(), tc.route, tc.direct)
			} else {
				_, d, err = rt.Complete(context.Background(), tc.route, tc.direct)
			}
			if err == nil || local.availableCall+remote.availableCall+local.completeCall+remote.completeCall != 0 || len(sink.records) != 1 {
				t.Fatalf("refusal escaped: err=%v decision=%+v records=%d", err, d, len(sink.records))
			}
			if tc.name == "privacy-first" && d.FailureCategory != "privacy_refusal" {
				t.Fatalf("privacy lost precedence: %+v", d)
			}
		})
	}
}

func TestGenerationRequirementsFallbackPreservesAllRequirements(t *testing.T) {
	for _, localState := range []provider.CapabilityState{provider.CapabilitySupported, provider.CapabilityUnknown, provider.CapabilityUnsupported} {
		t.Run("local-"+string(localState), func(t *testing.T) {
			remote := jsonProvider("remote", provider.TierRemote, provider.CapabilitySupported)
			remote.completeFunc = func(_ context.Context, req provider.Request) (provider.Response, error) {
				if req.ResponseFormat != provider.ResponseFormatJSONObject {
					t.Error("remote lost requirement")
				}
				return provider.Response{}, &provider.InferenceError{Status: 429, Code: "rate_limit"}
			}
			local := jsonProvider("local", provider.TierLocal, localState)
			local.completeFunc = func(_ context.Context, req provider.Request) (provider.Response, error) {
				if req.ResponseFormat != provider.ResponseFormatJSONObject {
					t.Error("fallback lost effective requirement")
				}
				return provider.Response{Text: `{}`, FinishReason: "stop"}, nil
			}
			sink := &memoryDecisionSink{}
			_, d, err := New([]provider.Provider{local, remote}).WithDecisionSink(sink).Complete(context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable}, provider.Request{ResponseFormat: provider.ResponseFormatJSONObject})
			if localState == provider.CapabilitySupported {
				if err != nil || d.Lane != LaneLocal || local.completeCall != 1 {
					t.Fatalf("qualified fallback: %+v %v", d, err)
				}
			} else if err == nil || local.availableCall != 0 || local.completeCall != 0 {
				t.Fatalf("fallback bypassed capability requirement: err=%v probes=%d complete=%d", err, local.availableCall, local.completeCall)
			}
			if remote.completeCall != 1 || len(sink.records) != 1 {
				t.Fatalf("replay/duplicate evidence: remote=%d records=%d", remote.completeCall, len(sink.records))
			}
		})
	}
}

func TestGenerationRequirementsRejectMalformedWithoutReplay(t *testing.T) {
	for _, response := range []provider.Response{
		{Text: `{} trailing`, FinishReason: "stop"},
		{Text: `{}`, FinishReason: "length"},
		{Text: `{}`, FinishReason: "stop", ToolCalls: []provider.ToolCall{{Name: "inert"}}},
	} {
		t.Run(response.Text+response.FinishReason, func(t *testing.T) {
			remote := jsonProvider("remote", provider.TierRemote, provider.CapabilitySupported)
			remote.completeFunc = func(context.Context, provider.Request) (provider.Response, error) { return response, nil }
			local := jsonProvider("local", provider.TierLocal, provider.CapabilitySupported)
			sink := &memoryDecisionSink{}
			resp, d, err := New([]provider.Provider{remote, local}).WithDecisionSink(sink).Complete(context.Background(), Request{Task: TaskJudgment, Privacy: PrivacyShareable, ResponseFormat: provider.ResponseFormatJSONObject}, provider.Request{})
			if !errors.Is(err, provider.ErrInvalidJSONObject) || resp.Text != "" || d.FailureCategory != "invalid_json_object" || remote.completeCall != 1 || local.completeCall != 0 || len(sink.records) != 1 {
				t.Fatalf("invalid response accepted/replayed: err=%v resp=%+v d=%+v calls=%d/%d records=%d", err, resp, d, remote.completeCall, local.completeCall, len(sink.records))
			}
		})
	}
}

func TestGenerationRequirementsDeterminismIsContractNotSeed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state provider.CapabilityState
		mode  provider.DeterminismMode
		want  bool
	}{
		{"unknown", provider.CapabilityUnknown, "", false},
		{"unsupported", provider.CapabilityUnsupported, provider.DeterminismBatchInvariant, false},
		{"wrong-contract", provider.CapabilitySupported, "seed", false},
		// A typed inert provider demonstrates matching only, not SNE support.
		{"inert-matching-contract", provider.CapabilitySupported, provider.DeterminismBatchInvariant, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := jsonProvider("inert", provider.TierLocal, provider.CapabilityUnknown)
			p.caps.Determinism, p.caps.DeterminismMode = tc.state, tc.mode
			seed, zero := int64(7), float64(0)
			p.completeFunc = func(_ context.Context, req provider.Request) (provider.Response, error) {
				if req.Determinism != provider.DeterminismBatchInvariant {
					t.Error("contract lost")
				}
				return provider.Response{Text: "inert", FinishReason: "stop"}, nil
			}
			_, _, err := testRouter([]provider.Provider{p}).Complete(context.Background(), Request{Determinism: provider.DeterminismBatchInvariant}, provider.Request{Seed: &seed, Temperature: &zero})
			if (err == nil) != tc.want || (!tc.want && p.availableCall+p.completeCall != 0) {
				t.Fatalf("seed/caps mistaken for contract: err=%v probes=%d calls=%d", err, p.availableCall, p.completeCall)
			}
		})
	}
}

func TestGenerationRequirementsCancellationAndJournalOnce(t *testing.T) {
	for _, cancelDuringCall := range []bool{false, true} {
		t.Run(map[bool]string{false: "journal-failure", true: "canceled"}[cancelDuringCall], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := jsonProvider("local", provider.TierLocal, provider.CapabilitySupported)
			p.completeFunc = func(context.Context, provider.Request) (provider.Response, error) {
				if cancelDuringCall {
					cancel()
				}
				return provider.Response{Text: `{}`, FinishReason: "stop", Model: "fixture-model"}, nil
			}
			sink := &memoryDecisionSink{err: errors.New("inert write failure")}
			_, _, err := New([]provider.Provider{p}).WithDecisionSink(sink).Complete(ctx, Request{ResponseFormat: provider.ResponseFormatJSONObject}, provider.Request{Prompt: "private prompt"})
			var journal *DecisionRecordError
			if !errors.As(err, &journal) || errors.Is(err, context.Canceled) != cancelDuringCall || p.completeCall != 1 || len(sink.records) != 1 {
				t.Fatalf("terminal cause/replay: err=%v calls=%d records=%d", err, p.completeCall, len(sink.records))
			}
			rec := sink.records[0]
			if rec.ResponseFormat != provider.ResponseFormatJSONObject || rec.EffectiveResponseFormat != provider.ResponseFormatJSONObject {
				t.Fatalf("mode evidence absent: %+v", rec)
			}
			b, _ := json.Marshal(rec)
			if strings.Contains(string(b), "private prompt") || strings.Contains(string(b), "inert write failure") {
				t.Fatal("private content leaked")
			}
		})
	}
}
