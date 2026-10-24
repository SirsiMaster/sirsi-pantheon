package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/modelrouter"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
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
	name          string
	tier          provider.Tier
	available     bool
	completeCalls int
	complete      func(context.Context, provider.Request) (provider.Response, error)
}

func (p *askProviderStub) Name() string        { return p.name }
func (p *askProviderStub) Tier() provider.Tier { return p.tier }
func (p *askProviderStub) Caps() provider.Caps {
	return provider.Caps{ContextTokens: 8192, Offline: p.tier == provider.TierLocal}
}
func (p *askProviderStub) Available(context.Context) bool { return p.available }
func (p *askProviderStub) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	p.completeCalls++
	if p.complete != nil {
		return p.complete(ctx, req)
	}
	return provider.Response{Text: "answer from " + p.name, Provider: p.name, Model: "fixture-model", Tier: p.tier}, nil
}

type askSink struct {
	records []modelrouter.DecisionRecord
	err     error
}

func (s *askSink) Record(r modelrouter.DecisionRecord) error {
	s.records = append(s.records, r)
	return s.err
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
	oldJSON, oldFix, oldRoute, oldTask, oldPrivacy, oldLane := askJSON, askFix, askRoute, askTask, askPrivacy, askLane
	askJSON, askFix, askRoute, askTask, askPrivacy, askLane = asJSON, false, route, task, privacy, lane
	t.Cleanup(func() {
		askJSON, askFix, askRoute, askTask, askPrivacy, askLane = oldJSON, oldFix, oldRoute, oldTask, oldPrivacy, oldLane
	})
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
	text, err := captureAskOutput(t, func() error { return runAskWith([]string{"why?"}, runtime) })
	if err != nil {
		t.Fatalf("runAskWith() error = %v", err)
	}
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
