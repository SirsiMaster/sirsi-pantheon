package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
	"github.com/spf13/cobra"
)

const apolloTestDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestSplitApolloMachines(t *testing.T) {
	if got := splitApolloMachines(""); len(got) != 1 || got[0] != "auto" {
		t.Fatalf("default machines = %v", got)
	}
	if got := splitApolloMachines("m5, m1"); len(got) != 2 || got[0] != "m5" || got[1] != "m1" {
		t.Fatalf("split machines = %v", got)
	}
}

func TestApolloClusterCLIRefusesUnenrolledTransport(t *testing.T) {
	oldEndpoint := apolloClusterEndpoint
	apolloClusterEndpoint = "https://apollo.example.test"
	t.Cleanup(func() { apolloClusterEndpoint = oldEndpoint })
	client, err := configuredApolloClusterClient()
	if err == nil || client != nil || !strings.Contains(err.Error(), "enrolled authentication transport") {
		t.Fatalf("unenrolled cluster client = %v, %v", client, err)
	}
}

func TestApolloChatJSONRecordsCompletedBeforeRendering(t *testing.T) {
	client := apolloChatFixture(t, []apollo.Event{
		apolloFixtureEvent("admitted", 1),
		apolloFixtureEvent("token", 2),
		apolloFixtureEvent("completed", 3),
		apolloFixtureEvent("released", 4),
	})
	output := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(output)
	configureApolloChatTest(t)

	if err := runApolloChat(cmd, client); err != nil {
		t.Fatalf("runApolloChat JSON success = %v", err)
	}
	for _, typ := range []string{"admitted", "completed", "released"} {
		if !strings.Contains(output.String(), `"type":"`+typ+`"`) {
			t.Fatalf("JSON output missing %q: %s", typ, output.String())
		}
	}
}

func TestApolloChatJSONRecordsFailedBeforeRendering(t *testing.T) {
	failed := apolloFixtureEvent("failed", 2)
	failed.ErrorCode = "EXECUTION_FAILED"
	client := apolloChatFixture(t, []apollo.Event{
		apolloFixtureEvent("admitted", 1), failed, apolloFixtureEvent("released", 3),
	})
	output := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(output)
	configureApolloChatTest(t)

	err := runApolloChat(cmd, client)
	if err == nil || !strings.Contains(err.Error(), "failed: EXECUTION_FAILED") {
		t.Fatalf("runApolloChat JSON failed job error = %v", err)
	}
	if !strings.Contains(output.String(), `"type":"failed"`) {
		t.Fatalf("JSON output missing failed event: %s", output.String())
	}
}

func configureApolloChatTest(t *testing.T) {
	t.Helper()
	oldJSON, oldRequest, oldSession, oldIdempotency := JsonOutput, apolloRequestID, apolloSessionID, apolloIdempotencyKey
	oldModel, oldMode, oldPrecision, oldMaxOutput := apolloModelID, apolloMode, apolloPrecision, apolloMaxOutput
	oldPrompt, oldReadStdin, oldMachines := apolloPrompt, apolloReadStdin, apolloMachines
	JsonOutput = true
	apolloRequestID, apolloSessionID, apolloIdempotencyKey = "request-1", "session-1", "idempotency-1"
	apolloModelID, apolloMode, apolloPrecision, apolloMaxOutput = "gemma", "plain", "Q8", 32
	apolloPrompt, apolloReadStdin, apolloMachines = "hello", false, "auto"
	t.Cleanup(func() {
		JsonOutput, apolloRequestID, apolloSessionID, apolloIdempotencyKey = oldJSON, oldRequest, oldSession, oldIdempotency
		apolloModelID, apolloMode, apolloPrecision, apolloMaxOutput = oldModel, oldMode, oldPrecision, oldMaxOutput
		apolloPrompt, apolloReadStdin, apolloMachines = oldPrompt, oldReadStdin, oldMachines
	})
}

func apolloChatFixture(t *testing.T, events []apollo.Event) *apollo.ClusterClient {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apollo/capabilities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apollo.Capability{Contract: apollo.ClusterContractV1, LaunchAvailable: true, StreamAvailable: true, CancelAvailable: true, Modes: []string{"plain"}})
	})
	mux.HandleFunc("/v1/apollo/prepare", func(w http.ResponseWriter, r *http.Request) {
		var request apollo.PrepareRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.RequestID != "request-1" || request.SessionID != "session-1" || request.ModelID != "gemma" || len(request.Messages) != 1 || request.Messages[0].Content != "hello" {
			t.Fatalf("prepare request = %#v", request)
		}
		_ = json.NewEncoder(w).Encode(apollo.Preparation{
			Contract: apollo.ClusterContractV1, PreparationID: "preparation-1", RequestID: request.RequestID, SessionID: request.SessionID,
			ExpiresAt: time.Now().UTC().Add(2 * time.Minute).Format(time.RFC3339), ModelID: request.ModelID,
			PlanSHA256: apolloTestDigest, CheckpointSHA256: apolloTestDigest, TokenizerSHA256: apolloTestDigest, TemplateSHA256: apolloTestDigest,
			Mode: "plain", Precision: "Q8", InputTokens: []uint32{2, 105}, StopTokens: []uint32{1}, MaxOutputTokens: 32, RankCount: 2,
		})
	})
	mux.HandleFunc("/v1/apollo/jobs", func(w http.ResponseWriter, r *http.Request) {
		var request apollo.JobRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.PreparationID != "preparation-1" || request.IdempotencyKey != "idempotency-1" || request.PlanSHA256 != apolloTestDigest || len(request.InputTokens) != 2 {
			t.Fatalf("job request = %#v", request)
		}
		_ = json.NewEncoder(w).Encode(apollo.Job{
			Contract: apollo.ClusterContractV1, JobID: "job-1", ExecutionID: "execution-1", RequestID: "request-1", PlanSHA256: apolloTestDigest, Epoch: 9, State: "admitting",
			RankFences: []apollo.RankFence{
				{Rank: 0, HostID: "m5", Fence: "fence-1", Generation: "generation-1", ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339), MemoryAdmissionBytes: 1 << 30},
				{Rank: 1, HostID: "m1", Fence: "fence-2", Generation: "generation-2", ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339), MemoryAdmissionBytes: 1 << 30},
			},
		})
	})
	mux.HandleFunc("/v1/apollo/jobs/job-1/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range events {
			encoded, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
		}
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client, err := apollo.NewClusterClient(srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func apolloFixtureEvent(typ string, sequence uint64) apollo.Event {
	return apollo.Event{Contract: apollo.ClusterContractV1, JobID: "job-1", ExecutionID: "execution-1", RequestID: "request-1", PlanSHA256: apolloTestDigest, Epoch: 9, Sequence: sequence, Type: typ}
}
