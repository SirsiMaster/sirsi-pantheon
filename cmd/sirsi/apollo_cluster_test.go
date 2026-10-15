package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
	"github.com/spf13/cobra"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSplitApolloMachines(t *testing.T) {
	if got := splitApolloMachines(""); len(got) != 1 || got[0] != "auto" {
		t.Fatalf("default machines = %v", got)
	}
	if got := splitApolloMachines("m5, m1"); len(got) != 2 || got[0] != "m5" || got[1] != "m1" {
		t.Fatalf("split machines = %v", got)
	}
}

func TestApolloClusterCLIRefusesUnenrolledTransport(t *testing.T) {
	old := apolloClusterEndpoint
	t.Cleanup(func() { apolloClusterEndpoint = old })
	apolloClusterEndpoint = "https://foreign.invalid"
	if client, err := configuredApolloClusterClient(); err == nil || client != nil {
		t.Fatalf("unenrolled transport escaped: %v %v", client, err)
	}
}

func TestApolloChatStreamJSONAndHumanOutcomes(t *testing.T) {
	oldJSON := JsonOutput
	t.Cleanup(func() { JsonOutput = oldJSON })
	job := apollo.Job{Contract: apollo.ClusterContractV1, JobID: "job", ExecutionID: "execution", RequestID: "request", PlanSHA256: strings.Repeat("a", 64), Epoch: 1}
	for _, jsonMode := range []bool{false, true} {
		for _, success := range []bool{false, true} {
			t.Run(fmt.Sprintf("json=%v/success=%v", jsonMode, success), func(t *testing.T) {
				JsonOutput = jsonMode
				var stream strings.Builder
				for i, kind := range []string{"token", map[bool]string{true: "completed", false: "failed"}[success], "released"} {
					event := apollo.Event{Contract: job.Contract, JobID: job.JobID, ExecutionID: job.ExecutionID, RequestID: job.RequestID, PlanSHA256: job.PlanSHA256, Epoch: job.Epoch, Sequence: uint64(i + 1), Type: kind}
					if kind == "token" {
						event.Text = "hello"
					}
					data, err := json.Marshal(event)
					if err != nil {
						t.Fatal(err)
					}
					stream.WriteString("data: " + string(data) + "\n\n")
				}
				client, err := apollo.NewClusterClient("https://fixture.invalid", &http.Client{Transport: apolloTestTransport(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream.String()))}, nil
				})})
				if err != nil {
					t.Fatal(err)
				}
				cmd := &cobra.Command{}
				cmd.SetContext(context.Background())
				var output bytes.Buffer
				cmd.SetOut(&output)
				err = streamApolloChat(cmd, client, job)
				if (err == nil) != success {
					t.Fatalf("success=%v err=%v output=%s", success, err, output.String())
				}
				if !strings.Contains(output.String(), "hello") {
					t.Fatalf("reply missing: %s", output.String())
				}
				if jsonMode && !strings.Contains(output.String(), `"type":"released"`) {
					t.Fatalf("terminal JSON missing: %s", output.String())
				}
			})
		}
	}
}

type apolloTestTransport func(*http.Request) (*http.Response, error)

func (f apolloTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
