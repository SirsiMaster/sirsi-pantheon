package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/modelrouter"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
	"github.com/spf13/cobra"
)

type askGenerationProvider struct {
	*askProviderStub
	jsonState   provider.CapabilityState
	ladderCalls int
}

func (p *askGenerationProvider) Caps() provider.Caps {
	caps := p.askProviderStub.Caps()
	caps.JSONObject = p.jsonState
	return caps
}

// Execute Cobra with the actual production flag set and command adapter; only
// Doctor/providers/journal are inert. No process, model or host scan is invoked.
func executeAskGenerationFixture(t *testing.T, args []string, state provider.CapabilityState, ctx context.Context, sinkErr error) (string, *askGenerationProvider, *askSink, int, error) {
	t.Helper()
	withAskFlags(t, false, false, "generation", "local-only", "")
	oldFormat, oldMode := askResponseFormat, askDeterminism
	askResponseFormat, askDeterminism = "", ""
	t.Cleanup(func() { askResponseFormat, askDeterminism = oldFormat, oldMode })
	p := &askGenerationProvider{askProviderStub: &askProviderStub{name: "owned-inert", tier: provider.TierLocal, available: true}, jsonState: state}
	p.complete = func(_ context.Context, req provider.Request) (provider.Response, error) {
		if askResponseFormat == "json-object" && (req.ResponseFormat != provider.ResponseFormatJSONObject || !strings.Contains(req.System, "JSON object")) {
			t.Error("actual ask adapter lost required format or JSON instruction")
		}
		text := "ordinary text"
		if req.ResponseFormat == provider.ResponseFormatJSONObject {
			text = `{"inert":true}`
		}
		return provider.Response{Text: text, FinishReason: "stop", Model: "inert-model", Provider: p.name, Tier: p.tier}, nil
	}
	sink := &askSink{err: sinkErr}
	doctorCalls := 0
	runtime := askRuntime{
		doctor: func() (*guard.DoctorReport, error) {
			doctorCalls++
			return &guard.DoctorReport{Status: guard.HealthStatus("amber"), Findings: []guard.DiagnosticFinding{{Severity: guard.SeverityWarn, Message: "owned inert warning"}}}, nil
		},
		ladder: func(context.Context, string) []provider.Provider { p.ladderCalls++; return []provider.Provider{p} },
		newRouter: func(ladder []provider.Provider) *modelrouter.Router {
			return modelrouter.New(ladder).WithDecisionSink(sink)
		},
		applicableRepairs: func() int { return 0 },
	}
	cmd := &cobra.Command{Use: "ask", SilenceUsage: true, SilenceErrors: true, RunE: func(cmd *cobra.Command, args []string) error { return runAskWithCommand(cmd, args, runtime) }}
	cmd.Flags().AddFlagSet(askCmd.Flags())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	cmd.SetContext(ctx)
	err := cmd.Execute()
	return out.String(), p, sink, doctorCalls, err
}

func TestAskGenerationCobraJSONModeDistinctFromEnvelope(t *testing.T) {
	out, p, sink, _, err := executeAskGenerationFixture(t, []string{"--route", "--response-format=json-object", "--json", "inert request"}, provider.CapabilitySupported, context.Background(), nil)
	var envelope struct {
		Answer   string `json:"answer"`
		Problems []any  `json:"problems"`
	}
	if err != nil || json.Unmarshal([]byte(out), &envelope) != nil || envelope.Answer != `{"inert":true}` || len(envelope.Problems) != 1 || p.completeCalls != 1 || len(sink.records) != 1 {
		t.Fatalf("actual JSON mode/envelope flow: output=%q err=%v calls=%d records=%d", out, err, p.completeCalls, len(sink.records))
	}
	if sink.records[0].ResponseFormat != provider.ResponseFormatJSONObject || sink.records[0].EffectiveResponseFormat != provider.ResponseFormatJSONObject {
		t.Fatalf("actual mode evidence=%+v", sink.records[0])
	}
}

func TestAskGenerationCobraEnvelopeAlonePreservesHeuristic(t *testing.T) {
	out, p, sink, _, err := executeAskGenerationFixture(t, []string{"--json"}, provider.CapabilityUnknown, context.Background(), nil)
	if err != nil || !json.Valid([]byte(out)) || p.availableCalls+p.completeCalls != 0 || len(sink.records) != 0 {
		t.Fatalf("envelope flag became generation authority: output=%q err=%v calls=%d", out, err, p.completeCalls)
	}
}

func TestAskGenerationCobraRoutedEnvelopeStillAllowsText(t *testing.T) {
	out, p, sink, _, err := executeAskGenerationFixture(t, []string{"--json", "--route"}, provider.CapabilityUnknown, context.Background(), nil)
	var envelope struct {
		Answer string `json:"answer"`
	}
	if err != nil || json.Unmarshal([]byte(out), &envelope) != nil || envelope.Answer != "ordinary text" || p.completeCalls != 1 || len(sink.records) != 1 {
		t.Fatalf("ordinary routed envelope regressed: output=%q err=%v calls=%d", out, err, p.completeCalls)
	}
}

func TestAskGenerationCobraUnqualifiedAndInvalidRefusals(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		args                    []string
		state                   provider.CapabilityState
		wantDoctor, wantRecords int
	}{
		{"unknown-json", []string{"--route", "--response-format=json-object"}, provider.CapabilityUnknown, 1, 1},
		{"unsupported-json", []string{"--route", "--response-format=json-object"}, provider.CapabilityUnsupported, 1, 1},
		{"determinism", []string{"--route", "--require-determinism=batch_invariant"}, provider.CapabilitySupported, 1, 1},
		{"invalid-format", []string{"--response-format=json_object"}, provider.CapabilitySupported, 0, 0},
		{"invalid-mode", []string{"--require-determinism=seed"}, provider.CapabilitySupported, 0, 0},
		{"stream-json", []string{"--response-format=json-object", "--stream"}, provider.CapabilitySupported, 0, 0},
		{"fix-json", []string{"--response-format=json-object", "--fix"}, provider.CapabilitySupported, 0, 0},
		{"privacy-override", []string{"--route", "--response-format=json-object", "--lane=remote"}, provider.CapabilitySupported, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, p, sink, doctorCalls, err := executeAskGenerationFixture(t, tc.args, tc.state, context.Background(), nil)
			if err == nil || p.availableCalls+p.completeCalls+p.streamCalls != 0 || doctorCalls != tc.wantDoctor || len(sink.records) != tc.wantRecords {
				t.Fatalf("actual refusal bypassed: err=%v output=%q probes/calls=%d/%d doctor=%d records=%d", err, out, p.availableCalls, p.completeCalls, doctorCalls, len(sink.records))
			}
		})
	}
}

func TestAskGenerationCobraCancellationAndJournalNoReplay(t *testing.T) {
	t.Run("pre-canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, p, sink, doctorCalls, err := executeAskGenerationFixture(t, []string{"--response-format=json-object"}, provider.CapabilitySupported, ctx, nil)
		if !errors.Is(err, context.Canceled) || doctorCalls != 0 || p.completeCalls != 0 || len(sink.records) != 0 {
			t.Fatalf("canceled entry: %v", err)
		}
	})
	t.Run("journal-failure", func(t *testing.T) {
		out, p, sink, _, err := executeAskGenerationFixture(t, []string{"--route", "--response-format=json-object", "--json"}, provider.CapabilitySupported, context.Background(), errors.New("inert journal refusal"))
		var journal *modelrouter.DecisionRecordError
		if !errors.As(err, &journal) || p.completeCalls != 1 || len(sink.records) != 1 || !strings.Contains(out, "inert") {
			t.Fatalf("journal failure replayed/lost: output=%q err=%v calls=%d records=%d", out, err, p.completeCalls, len(sink.records))
		}
	})
}

func TestAskGenerationCobraFormatDoesNotConsentToInference(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"text", []string{"--response-format=text"}},
		{"json-object", []string{"--response-format=json-object"}},
		{"json-envelope", []string{"--response-format=json-object", "--json"}},
		{"determinism", []string{"--require-determinism=batch_invariant"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, p, sink, doctorCalls, err := executeAskGenerationFixture(t, tc.args, provider.CapabilitySupported, context.Background(), nil)
			if err == nil || !strings.Contains(err.Error(), "add --route") || !strings.Contains(out, "owned inert warning") || strings.Contains(out, "explicit --route requested") || doctorCalls != 1 || p.ladderCalls+p.availableCalls+p.completeCalls+p.streamCalls != 0 || len(sink.records) != 0 {
				t.Fatalf("format became inference consent: out=%q err=%v ladder=%d probes=%d calls=%d records=%d", out, err, p.ladderCalls, p.availableCalls, p.completeCalls, len(sink.records))
			}
		})
	}
}

func TestAskGenerationCobraSelectedProfileToHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, declaration string
		want              provider.CapabilityState
		malformed         bool
	}{
		{"supported", `,"json_object":true`, provider.CapabilitySupported, false},
		{"unsupported", `,"json_object":false`, provider.CapabilityUnsupported, false},
		{"absent", "", provider.CapabilityUnknown, false},
		{"malformed", `,"json_object":"true"`, provider.CapabilityUnknown, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withAskFlags(t, false, false, "generation", "local-only", "")
			oldFormat, oldMode := askResponseFormat, askDeterminism
			askResponseFormat, askDeterminism = "", ""
			t.Cleanup(func() { askResponseFormat, askDeterminism = oldFormat, oldMode })
			home := t.TempDir()
			t.Setenv("HOME", home)
			for _, key := range []string{"SIRSI_REMOTE_API_KEY", "OPENAI_API_KEY", "SIRSI_REMOTE_ENDPOINT"} {
				t.Setenv(key, "")
			}
			var modelReads, probes, completions atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/models":
					modelReads.Add(1)
					fmt.Fprint(w, `{"data":[{"id":"profile-owned-model"}]}`)
				case "/v1/chat/completions":
					var req struct {
						Model          string          `json:"model"`
						MaxTokens      int             `json:"max_tokens"`
						ResponseFormat json.RawMessage `json:"response_format"`
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Error(err)
						return
					}
					if req.Model != "profile-owned-model" {
						t.Errorf("selected model lost: %s", req.Model)
					}
					if req.MaxTokens == 1 {
						probes.Add(1)
						if len(req.ResponseFormat) != 0 {
							t.Error("readiness probe changed wire mode")
						}
						fmt.Fprint(w, `{"model":"profile-owned-model","choices":[{"finish_reason":"stop","message":{"content":"ready"}}]}`)
						return
					}
					completions.Add(1)
					if string(req.ResponseFormat) != `{"type":"json_object"}` {
						t.Errorf("profile->Cobra->HTTP lost exact format: %s", req.ResponseFormat)
					}
					fmt.Fprint(w, `{"model":"profile-owned-model","choices":[{"finish_reason":"stop","message":{"content":"{\"profile\":true}"}}]}`)
				default:
					t.Errorf("unexpected fixture route: %s", r.URL.Path)
					http.Error(w, "unexpected fixture route", http.StatusNotFound)
				}
			}))
			defer server.Close()
			if err := os.Mkdir(filepath.Join(home, ".sirsi"), 0700); err != nil {
				t.Fatal(err)
			}
			profile := filepath.Join(home, ".sirsi", "owned-profile.json")
			profileBytes := fmt.Sprintf(`{"endpoint":%q,"model":"profile-owned-model","context_tokens":1024,"output_tokens":256,"streaming":false%s}`, server.URL, tc.declaration)
			if err := os.WriteFile(profile, []byte(profileBytes), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, ".sirsi", "orchestrator.conf"), []byte("local_profile="+profile+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SIRSI_LOCAL_PROFILE", "")
			resolved := provider.Local(home, provider.LoadConf(home))
			if resolved == nil || resolved.Caps().JSONObject != tc.want || (resolved.ConfigurationError != nil) != tc.malformed {
				t.Fatalf("profile declaration not resolved: %+v", resolved)
			}
			if !tc.malformed && (resolved.Model != "profile-owned-model" || resolved.Endpoint != server.URL+"/v1") {
				t.Fatal("declaration detached from exact selected endpoint/model")
			}
			sink := &askSink{}
			ladderCalls := 0
			runtime := askRuntime{
				doctor: func() (*guard.DoctorReport, error) {
					return &guard.DoctorReport{Status: guard.HealthStatus("amber"), Findings: []guard.DiagnosticFinding{{Severity: guard.SeverityWarn, Message: "owned profile warning"}}}, nil
				},
				ladder: func(ctx context.Context, actualHome string) []provider.Provider {
					ladderCalls++
					if actualHome != home {
						t.Error("Cobra did not consume private HOME")
					}
					return provider.Ladder(ctx, actualHome)
				},
				newRouter: func(ladder []provider.Provider) *modelrouter.Router {
					return modelrouter.New(ladder).WithDecisionSink(sink)
				},
				applicableRepairs: func() int { return 0 },
			}
			cmd := &cobra.Command{Use: "ask", SilenceErrors: true, SilenceUsage: true, RunE: func(cmd *cobra.Command, args []string) error { return runAskWithCommand(cmd, args, runtime) }}
			cmd.Flags().AddFlagSet(askCmd.Flags())
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"--route", "--response-format=json-object", "--json", "owned inert request"})
			err := cmd.Execute()
			if ladderCalls != 1 || len(sink.records) != 1 {
				t.Fatalf("actual resolver/evidence calls=%d/%d", ladderCalls, len(sink.records))
			}
			if tc.want == provider.CapabilitySupported {
				var envelope struct{ Answer, Model string }
				if err != nil || json.Unmarshal(out.Bytes(), &envelope) != nil || envelope.Answer != `{"profile":true}` || envelope.Model != "profile-owned-model" || modelReads.Load() != 1 || probes.Load() != 1 || completions.Load() != 1 {
					t.Fatalf("profile entry failure: err=%v out=%q HTTP=%d/%d/%d", err, out.String(), modelReads.Load(), probes.Load(), completions.Load())
				}
			} else if err == nil || modelReads.Load()+probes.Load()+completions.Load() != 0 {
				t.Fatalf("unqualified profile reached HTTP: err=%v HTTP=%d/%d/%d", err, modelReads.Load(), probes.Load(), completions.Load())
			}
		})
	}
}
