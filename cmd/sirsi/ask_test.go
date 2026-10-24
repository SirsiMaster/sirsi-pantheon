package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/modelrouter"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
	"github.com/spf13/cobra"
)

func TestAskRejectsJSONFixBeforeDiagnosis(t *testing.T) {
	oldJSON, oldFix := askJSON, askFix
	askJSON, askFix = true, true
	t.Cleanup(func() {
		askJSON, askFix = oldJSON, oldFix
	})

	err := runAsk(nil, []string{"what is wrong?"})
	if err == nil {
		t.Fatal("--json --fix must not silently return diagnostic JSON without performing the requested repair")
	}
	if !strings.Contains(err.Error(), "--json and --fix cannot be combined") {
		t.Fatalf("error = %q, want explicit incompatible-flags diagnosis", err)
	}
}

type askProviderStub struct {
	name           string
	tier           provider.Tier
	available      bool
	streaming      bool
	availableCalls int
	completeCalls  int
	streamCalls    int
	complete       func(context.Context, provider.Request) (provider.Response, error)
	stream         func(context.Context, provider.Request) (<-chan provider.StreamChunk, error)
}

func (p *askProviderStub) Name() string        { return p.name }
func (p *askProviderStub) Tier() provider.Tier { return p.tier }
func (p *askProviderStub) Caps() provider.Caps {
	return provider.Caps{ContextTokens: 8192, Streaming: p.streaming, Offline: p.tier == provider.TierLocal}
}
func (p *askProviderStub) Stream(ctx context.Context, req provider.Request) (<-chan provider.StreamChunk, error) {
	p.streamCalls++
	if p.stream != nil {
		return p.stream(ctx, req)
	}
	chunks := make(chan provider.StreamChunk, 2)
	chunks <- provider.StreamChunk{Text: "streamed answer", Model: "fixture-model"}
	chunks <- provider.StreamChunk{Done: true, FinishReason: "stop"}
	close(chunks)
	return chunks, nil
}
func (p *askProviderStub) Available(context.Context) bool {
	p.availableCalls++
	return p.available
}
func (p *askProviderStub) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	p.completeCalls++
	if p.complete != nil {
		return p.complete(ctx, req)
	}
	return provider.Response{Text: "answer from " + p.name, Provider: p.name, Model: "fixture-model", Tier: p.tier}, nil
}

type askSink struct {
	records  []modelrouter.DecisionRecord
	err      error
	recorded chan modelrouter.DecisionRecord
}

func (s *askSink) Record(r modelrouter.DecisionRecord) error {
	s.records = append(s.records, r)
	if s.recorded != nil {
		s.recorded <- r
	}
	return s.err
}

type heldAskWriter struct {
	entered     chan struct{}
	release     chan struct{}
	once        sync.Once
	releaseOnce sync.Once
	mu          sync.Mutex
	text        bytes.Buffer
}

func newHeldAskWriter() *heldAskWriter {
	return &heldAskWriter{entered: make(chan struct{}), release: make(chan struct{})}
}

func (w *heldAskWriter) unblock() { w.releaseOnce.Do(func() { close(w.release) }) }

func (w *heldAskWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "held-partial-answer") {
		w.once.Do(func() {
			close(w.entered)
			<-w.release
		})
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.text.Write(p)
}

func (w *heldAskWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.text.String()
}

func captureAskOutput(t *testing.T, run func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	runErr := run()
	_ = write.Close()
	os.Stdout = old
	bytes, readErr := io.ReadAll(read)
	_ = read.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(bytes), runErr
}

func withAskFlags(t *testing.T, asJSON, route bool, task, privacy, lane string) {
	t.Helper()
	oldJSON, oldFix, oldRoute, oldTask, oldPrivacy, oldLane, oldStream := askJSON, askFix, askRoute, askTask, askPrivacy, askLane, askStream
	askJSON, askFix, askRoute, askTask, askPrivacy, askLane, askStream = asJSON, false, route, task, privacy, lane, false
	t.Cleanup(func() {
		askJSON, askFix, askRoute, askTask, askPrivacy, askLane, askStream = oldJSON, oldFix, oldRoute, oldTask, oldPrivacy, oldLane, oldStream
	})
}

func TestAskStreamUsesQualifiedRouterPathOnce(t *testing.T) {
	withAskFlags(t, false, true, "generation", "local-only", "")
	askStream = true
	local := &askProviderStub{name: "local-profile", tier: provider.TierLocal, available: true, streaming: true}
	sink := &askSink{}
	runtime := askRuntime{
		doctor: func() (*guard.DoctorReport, error) {
			return &guard.DoctorReport{Status: guard.HealthStatus("green")}, nil
		},
		ladder: func(context.Context, string) []provider.Provider { return []provider.Provider{local} },
		newRouter: func(ladder []provider.Provider) *modelrouter.Router {
			return modelrouter.New(ladder).WithDecisionSink(sink)
		},
		applicableRepairs: func() int { return 0 },
	}
	cmd := &cobra.Command{}
	var selected bytes.Buffer
	cmd.SetOut(&selected)
	if err := runAskWithCommand(cmd, []string{"why?"}, runtime); err != nil {
		t.Fatalf("runAskWithCommand() error = %v", err)
	}
	if !strings.Contains(selected.String(), "streamed answer") || !strings.Contains(selected.String(), "[router] local/local-profile") {
		t.Fatalf("actual ask output omitted streamed text or selected route: %s", selected.String())
	}
	if local.streamCalls != 1 || local.completeCalls != 0 || len(sink.records) != 1 || sink.records[0].Outcome != "completed" {
		t.Fatalf("stream/buffer/evidence calls = %d/%d/%+v", local.streamCalls, local.completeCalls, sink.records)
	}
}

func TestAskStreamDoesNotBypassHeuristicAlarmsOrJSONGuard(t *testing.T) {
	withAskFlags(t, false, false, "generation", "local-only", "")
	askStream = true
	local := &askProviderStub{name: "local-profile", tier: provider.TierLocal, available: true, streaming: true}
	runtime := askRuntime{
		doctor: func() (*guard.DoctorReport, error) {
			return &guard.DoctorReport{Status: guard.HealthStatus("amber"), Findings: []guard.DiagnosticFinding{{Check: "fixture", Severity: guard.SeverityWarn, Message: "alarm remains visible"}}}, nil
		},
		ladder: func(context.Context, string) []provider.Provider { return []provider.Provider{local} },
		newRouter: func(ladder []provider.Provider) *modelrouter.Router {
			return modelrouter.New(ladder).WithDecisionSink(&askSink{})
		},
		applicableRepairs: func() int { return 0 },
	}
	text, err := captureAskOutput(t, func() error { return runAskWith([]string{"why?"}, runtime) })
	if err != nil || !strings.Contains(text, "alarm remains visible") || local.streamCalls != 0 {
		t.Fatalf("stream bypassed heuristic alarm: output=%q stream calls=%d err=%v", text, local.streamCalls, err)
	}
	askJSON = true
	if err := runAskWith([]string{"why?"}, runtime); err == nil || !strings.Contains(err.Error(), "--stream and --json") || local.streamCalls != 0 {
		t.Fatalf("--stream --json was not refused before routing: stream calls=%d err=%v", local.streamCalls, err)
	}
}

func TestAskStreamShowsOneQualifiedLocalFallback(t *testing.T) {
	withAskFlags(t, false, true, "judgment", "shareable", "")
	askStream = true
	remote := &askProviderStub{name: "remote", tier: provider.TierRemote, available: true, streaming: true, stream: func(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
		return nil, &provider.InferenceError{Status: 429, Code: "rate_limit", Message: "private remote body"}
	}}
	local := &askProviderStub{name: "local-profile", tier: provider.TierLocal, available: true, streaming: true}
	sink := &askSink{}
	runtime := askRuntime{
		doctor: func() (*guard.DoctorReport, error) {
			return &guard.DoctorReport{Status: guard.HealthStatus("green")}, nil
		},
		ladder: func(context.Context, string) []provider.Provider { return []provider.Provider{local, remote} },
		newRouter: func(ladder []provider.Provider) *modelrouter.Router {
			return modelrouter.New(ladder).WithDecisionSink(sink)
		},
		applicableRepairs: func() int { return 0 },
	}
	cmd := &cobra.Command{}
	var selected bytes.Buffer
	cmd.SetOut(&selected)
	if err := runAskWithCommand(cmd, []string{"review this"}, runtime); err != nil {
		t.Fatalf("runAskWithCommand() error = %v", err)
	}
	if !strings.Contains(selected.String(), "streamed answer") || !strings.Contains(selected.String(), "local-profile") || strings.Contains(selected.String(), "private remote body") {
		t.Fatalf("fallback output is incomplete or leaked provider error: %s", selected.String())
	}
	if remote.streamCalls != 1 || local.streamCalls != 1 || remote.completeCalls != 0 || local.completeCalls != 0 || len(sink.records) != 1 || sink.records[0].FinalLane != modelrouter.LaneLocal || sink.records[0].Outcome != "fallback_completed" {
		t.Fatalf("route/calls/evidence = remote:%d/%d local:%d/%d records:%+v", remote.streamCalls, remote.completeCalls, local.streamCalls, local.completeCalls, sink.records)
	}
}

func TestAskStreamMakesShareableCapabilityEscalationVisible(t *testing.T) {
	withAskFlags(t, false, true, "generation", "shareable", "")
	askStream = true
	local := &askProviderStub{name: "unprofiled-local", tier: provider.TierLocal, available: true}
	remote := &askProviderStub{name: "remote", tier: provider.TierRemote, available: true, streaming: true}
	runtime := askRuntime{
		doctor: func() (*guard.DoctorReport, error) {
			return &guard.DoctorReport{Status: guard.HealthStatus("green")}, nil
		},
		ladder: func(context.Context, string) []provider.Provider { return []provider.Provider{local, remote} },
		newRouter: func(ladder []provider.Provider) *modelrouter.Router {
			return modelrouter.New(ladder).WithDecisionSink(&askSink{})
		},
		applicableRepairs: func() int { return 0 },
	}
	cmd := &cobra.Command{}
	var selected bytes.Buffer
	cmd.SetOut(&selected)
	if err := runAskWithCommand(cmd, []string{"why?"}, runtime); err != nil {
		t.Fatalf("runAskWithCommand() error = %v", err)
	}
	if !strings.Contains(selected.String(), "[router] remote/remote/fixture-model (fallback: local_capability_insufficient)") || remote.streamCalls != 1 || local.streamCalls != 0 {
		t.Fatalf("shareable remote route was silent or misrouted: output=%s calls=%d/%d", selected.String(), remote.streamCalls, local.streamCalls)
	}
}

func TestAskStreamPartialFailureIsTerminalAndNotReplayed(t *testing.T) {
	withAskFlags(t, false, true, "judgment", "shareable", "")
	askStream = true
	remote := &askProviderStub{name: "remote", tier: provider.TierRemote, available: true, streaming: true, stream: func(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
		chunks := make(chan provider.StreamChunk, 2)
		chunks <- provider.StreamChunk{Text: "partial", Model: "remote-fixture"}
		chunks <- provider.StreamChunk{Err: errors.Join(errors.New("private transport detail"), &provider.InferenceError{Status: 429, Code: "rate_limit"})}
		close(chunks)
		return chunks, nil
	}}
	local := &askProviderStub{name: "local-profile", tier: provider.TierLocal, available: true, streaming: true}
	runtime := askRuntime{
		doctor: func() (*guard.DoctorReport, error) {
			return &guard.DoctorReport{Status: guard.HealthStatus("green")}, nil
		},
		ladder: func(context.Context, string) []provider.Provider { return []provider.Provider{local, remote} },
		newRouter: func(ladder []provider.Provider) *modelrouter.Router {
			return modelrouter.New(ladder).WithDecisionSink(&askSink{})
		},
		applicableRepairs: func() int { return 0 },
	}
	var selected bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&selected)
	err := runAskWithCommand(cmd, []string{"review this"}, runtime)
	if err == nil || !strings.Contains(selected.String(), "partial") || !strings.Contains(selected.String(), "no replay was attempted") || strings.Contains(selected.String(), "private transport detail") {
		t.Fatalf("partial stream was hidden, replayed, or leaked raw error: output=%q err=%v", selected.String(), err)
	}
	if remote.streamCalls != 1 || local.streamCalls != 0 || remote.completeCalls != 0 || local.completeCalls != 0 {
		t.Fatalf("terminal partial stream retried: remote:%d/%d local:%d/%d", remote.streamCalls, remote.completeCalls, local.streamCalls, local.completeCalls)
	}
}

func TestAskStreamUnexpectedEOFAfterPartialOutputFailsWithoutReplay(t *testing.T) {
	withAskFlags(t, false, true, "judgment", "shareable", "")
	askStream = true
	remote := &askProviderStub{name: "remote", tier: provider.TierRemote, available: true, streaming: true, stream: func(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
		chunks := make(chan provider.StreamChunk, 1)
		chunks <- provider.StreamChunk{Text: "partial", Model: "remote-fixture"}
		close(chunks)
		return chunks, nil
	}}
	local := &askProviderStub{name: "local-profile", tier: provider.TierLocal, available: true, streaming: true}
	sink := &askSink{}
	runtime := askRuntime{
		doctor: func() (*guard.DoctorReport, error) {
			return &guard.DoctorReport{Status: guard.HealthStatus("green")}, nil
		},
		ladder: func(context.Context, string) []provider.Provider { return []provider.Provider{local, remote} },
		newRouter: func(ladder []provider.Provider) *modelrouter.Router {
			return modelrouter.New(ladder).WithDecisionSink(sink)
		},
		applicableRepairs: func() int { return 0 },
	}
	var selected bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&selected)
	err := runAskWithCommand(cmd, []string{"review this"}, runtime)
	if err == nil || strings.Count(selected.String(), "partial") != 1 || !strings.Contains(selected.String(), "no replay was attempted") {
		t.Fatalf("ask did not fail once after exposing the partial answer: output=%q err=%v", selected.String(), err)
	}
	if remote.streamCalls != 1 || local.streamCalls != 0 || remote.completeCalls != 0 || local.completeCalls != 0 || len(sink.records) != 1 || sink.records[0].Outcome != "failed" {
		t.Fatalf("unexpected EOF retried or was not recorded as failed: remote:%d/%d local:%d/%d records=%+v", remote.streamCalls, remote.completeCalls, local.streamCalls, local.completeCalls, sink.records)
	}
}

func TestAskStreamRetainsTerminalOutcomeAcrossSlowWriter(t *testing.T) {
	for _, tc := range []struct {
		name          string
		cancelAsk     bool
		sinkErr       error
		wantRecordErr bool
		wantCanceled  bool
	}{
		{name: "unexpected EOF"},
		{name: "caller cancellation", cancelAsk: true, wantCanceled: true},
		{name: "decision journal failure", sinkErr: errors.New("fixture journal failure"), wantRecordErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withAskFlags(t, false, true, "judgment", "shareable", "")
			askStream = true
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			providerCanceled := make(chan struct{})
			remote := &askProviderStub{name: "remote", tier: provider.TierRemote, available: true, streaming: true, stream: func(ctx context.Context, _ provider.Request) (<-chan provider.StreamChunk, error) {
				chunks := make(chan provider.StreamChunk, 1)
				chunks <- provider.StreamChunk{Text: "held-partial-answer", Model: "remote-fixture"}
				if tc.cancelAsk {
					go func() { <-ctx.Done(); close(providerCanceled) }()
				} else {
					close(chunks)
				}
				return chunks, nil
			}}
			local := &askProviderStub{name: "local-profile", tier: provider.TierLocal, available: true, streaming: true}
			sink := &askSink{err: tc.sinkErr, recorded: make(chan modelrouter.DecisionRecord, 1)}
			runtime := askRuntime{
				doctor: func() (*guard.DoctorReport, error) {
					return &guard.DoctorReport{Status: guard.HealthStatus("green")}, nil
				},
				ladder: func(context.Context, string) []provider.Provider { return []provider.Provider{local, remote} },
				newRouter: func(ladder []provider.Provider) *modelrouter.Router {
					return modelrouter.New(ladder).WithDecisionSink(sink)
				},
				applicableRepairs: func() int { return 0 },
			}
			cmd := &cobra.Command{}
			cmd.SetContext(parent)
			writer := newHeldAskWriter()
			t.Cleanup(writer.unblock)
			cmd.SetOut(writer)
			done := make(chan error, 1)
			go func() { done <- runAskWithCommand(cmd, []string{"review this"}, runtime) }()
			select {
			case <-writer.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("actual ask path did not hand partial text to the held writer")
			}
			if tc.cancelAsk {
				cancel()
				select {
				case <-providerCanceled:
				case <-time.After(3 * time.Second):
					t.Fatal("caller cancellation did not reach the provider stream context")
				}
			}
			select {
			case record := <-sink.recorded:
				if record.Outcome != "failed" {
					t.Fatalf("terminal decision outcome = %q, want failed", record.Outcome)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("router did not record terminal failure while writer was held")
			}
			// Exceed the former one-second terminal-chunk delivery timeout before
			// allowing the real ask writer to resume.
			timer := time.NewTimer(1200 * time.Millisecond)
			<-timer.C
			writer.unblock()
			var err error
			select {
			case err = <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("actual ask path did not finish after releasing writer")
			}
			if err == nil || strings.Count(writer.String(), "held-partial-answer") != 1 ||
				(!tc.wantRecordErr && !strings.Contains(writer.String(), "no replay was attempted")) {
				t.Fatalf("terminal outcome was lost, partial output replayed, or failure hidden: output=%q err=%v", writer.String(), err)
			}
			if remote.streamCalls != 1 || remote.completeCalls != 0 || local.streamCalls != 0 || local.completeCalls != 0 || len(sink.records) != 1 {
				t.Fatalf("stream replay/fallback or duplicate decision: remote:%d/%d local:%d/%d records=%+v", remote.streamCalls, remote.completeCalls, local.streamCalls, local.completeCalls, sink.records)
			}
			var recordErr *modelrouter.DecisionRecordError
			if errors.As(err, &recordErr) != tc.wantRecordErr {
				t.Fatalf("decision journal error classification = %v, want record error %t", err, tc.wantRecordErr)
			}
			if errors.Is(err, context.Canceled) != tc.wantCanceled {
				t.Fatalf("caller cancellation cause = %v, want canceled=%t", err, tc.wantCanceled)
			}
		})
	}
}

func TestAskCommandCancellationBeforeRoutingSkipsAvailability(t *testing.T) {
	withAskFlags(t, false, true, "generation", "local-only", "")
	askStream = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	local := &askProviderStub{name: "local-profile", tier: provider.TierLocal, available: true, streaming: true}
	doctorCalls, ladderCalls := 0, 0
	runtime := askRuntime{
		doctor: func() (*guard.DoctorReport, error) {
			doctorCalls++
			return &guard.DoctorReport{Status: guard.HealthStatus("green")}, nil
		},
		ladder:    func(context.Context, string) []provider.Provider { ladderCalls++; return []provider.Provider{local} },
		newRouter: func(ladder []provider.Provider) *modelrouter.Router { return modelrouter.New(ladder) },
	}
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	err := runAskWithCommand(cmd, []string{"why?"}, runtime)
	if !errors.Is(err, context.Canceled) || doctorCalls != 0 || ladderCalls != 0 || local.availableCalls != 0 || local.streamCalls != 0 {
		t.Fatalf("pre-canceled Cobra context reached diagnosis/availability/stream: doctor=%d ladder=%d available=%d stream=%d err=%v", doctorCalls, ladderCalls, local.availableCalls, local.streamCalls, err)
	}
}

func TestAskExplicitRouteKeepsDoctorAlarmsAndUsesSingleFinalDecision(t *testing.T) {
	withAskFlags(t, false, true, "judgment", "shareable", "")
	remote := &askProviderStub{name: "remote", tier: provider.TierRemote, available: true, complete: func(context.Context, provider.Request) (provider.Response, error) {
		return provider.Response{}, &provider.InferenceError{Status: 429, Code: "rate_limit"}
	}}
	local := &askProviderStub{name: "local-profile", tier: provider.TierLocal, available: true, complete: func(_ context.Context, req provider.Request) (provider.Response, error) {
		if !strings.Contains(req.Prompt, "Measured state:") {
			t.Error("actual ask prompt omitted the existing measured-state context")
		}
		return provider.Response{Text: "local answer", Provider: "local-profile", Model: "local-fixture", Tier: provider.TierLocal}, nil
	}}
	sink := &askSink{}
	runtime := askRuntime{
		doctor: func() (*guard.DoctorReport, error) {
			return &guard.DoctorReport{Status: guard.HealthStatus("amber"), Findings: []guard.DiagnosticFinding{{Check: "fixture-alarm", Severity: guard.SeverityWarn, Message: "measured alarm remains visible"}}}, nil
		},
		ladder: func(context.Context, string) []provider.Provider { return []provider.Provider{local, remote} },
		newRouter: func(ladder []provider.Provider) *modelrouter.Router {
			return modelrouter.New(ladder).WithDecisionSink(sink)
		},
		applicableRepairs: func() int { return 0 },
	}
	cmd := &cobra.Command{}
	var selected bytes.Buffer
	cmd.SetOut(&selected)
	err := runAskWithCommand(cmd, []string{"why?"}, runtime)
	if err != nil {
		t.Fatalf("runAskWithCommand() error = %v", err)
	}
	text := selected.String()
	if !strings.Contains(text, "measured alarm remains visible") || !strings.Contains(text, "local answer") || !strings.Contains(text, "fallback: remote_rate_exhausted") {
		t.Fatalf("text omitted alarm, answer, or final fallback: %s", text)
	}
	if remote.completeCalls != 1 || local.completeCalls != 1 || len(sink.records) != 1 {
		t.Fatalf("calls/evidence remote:%d local:%d records:%d", remote.completeCalls, local.completeCalls, len(sink.records))
	}
	if sink.records[0].FinalLane != modelrouter.LaneLocal || sink.records[0].Outcome != "fallback_completed" {
		t.Fatalf("record did not preserve final route: %+v", sink.records[0])
	}
}

func TestAskJSONShowsAlarmAndActualRoutedCompletion(t *testing.T) {
	withAskFlags(t, true, true, "generation", "shareable", "local")
	local := &askProviderStub{name: "local-profile", tier: provider.TierLocal, available: true}
	sink := &askSink{}
	runtime := askRuntime{
		doctor: func() (*guard.DoctorReport, error) {
			return &guard.DoctorReport{Status: guard.HealthStatus("amber"), Findings: []guard.DiagnosticFinding{{Check: "fixture-alarm", Severity: guard.SeverityWarn, Message: "still visible"}}}, nil
		},
		ladder: func(context.Context, string) []provider.Provider { return []provider.Provider{local} },
		newRouter: func(ladder []provider.Provider) *modelrouter.Router {
			return modelrouter.New(ladder).WithDecisionSink(sink)
		},
	}
	text, err := captureAskOutput(t, func() error { return runAskWith([]string{"why?"}, runtime) })
	if err != nil {
		t.Fatalf("runAskWith() error = %v", err)
	}
	var got struct {
		Problems []askProblemJSON `json:"problems"`
		Routing  askRouteJSON     `json:"routing"`
		Answer   string           `json:"answer"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("JSON output malformed: %v: %s", err, text)
	}
	if len(got.Problems) != 1 || got.Problems[0].Message != "still visible" || got.Answer != "answer from local-profile" {
		t.Fatalf("JSON answer/findings = %+v", got)
	}
	if got.Routing.FinalLane != "local" || got.Routing.Provider != "local-profile" || got.Routing.Model != "fixture-model" || !got.Routing.OverrideApplied {
		t.Fatalf("JSON routing omitted actual decision: %+v", got.Routing)
	}
	if local.completeCalls != 1 || len(sink.records) != 1 {
		t.Fatalf("completion/evidence count = %d/%d", local.completeCalls, len(sink.records))
	}
}

func TestAskDefaultHeuristicAlarmDoesNotInvokeModel(t *testing.T) {
	withAskFlags(t, false, false, "generation", "local-only", "")
	local := &askProviderStub{name: "local", tier: provider.TierLocal, available: true}
	runtime := askRuntime{
		doctor: func() (*guard.DoctorReport, error) {
			return &guard.DoctorReport{Findings: []guard.DiagnosticFinding{{Check: "fixture", Severity: guard.SeverityWarn, Message: "alarm"}}}, nil
		},
		ladder: func(context.Context, string) []provider.Provider { return []provider.Provider{local} },
		newRouter: func(ladder []provider.Provider) *modelrouter.Router {
			return modelrouter.New(ladder).WithDecisionSink(&askSink{})
		},
		applicableRepairs: func() int { return 0 },
	}
	text, err := captureAskOutput(t, func() error { return runAskWith([]string{"status"}, runtime) })
	if err != nil || !strings.Contains(text, "alarm") {
		t.Fatalf("heuristic output=%q err=%v", text, err)
	}
	if local.completeCalls != 0 {
		t.Fatalf("default heuristic invoked model %d times", local.completeCalls)
	}
}

func TestAskSurfacesDecisionSinkFailureWithoutRepeatingCompletion(t *testing.T) {
	withAskFlags(t, false, true, "generation", "local-only", "")
	local := &askProviderStub{name: "local", tier: provider.TierLocal, available: true}
	sink := &askSink{err: errors.New("fixture sink failure")}
	runtime := askRuntime{
		doctor: func() (*guard.DoctorReport, error) { return &guard.DoctorReport{}, nil },
		ladder: func(context.Context, string) []provider.Provider { return []provider.Provider{local} },
		newRouter: func(ladder []provider.Provider) *modelrouter.Router {
			return modelrouter.New(ladder).WithDecisionSink(sink)
		},
	}
	text, err := captureAskOutput(t, func() error { return runAskWith([]string{"status"}, runtime) })
	if err == nil || !strings.Contains(err.Error(), "decision evidence write failed") {
		t.Fatalf("sink failure not surfaced: %v", err)
	}
	if !strings.Contains(text, "answer from local") || !strings.Contains(text, "local/local") {
		t.Fatalf("completed answer/final route hidden after evidence failure: %s", text)
	}
	if local.completeCalls != 1 || len(sink.records) != 1 {
		t.Fatalf("inference repeated: calls=%d records=%d", local.completeCalls, len(sink.records))
	}
}
