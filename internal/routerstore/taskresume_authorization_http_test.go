package routerstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type taskResumeAuthorizationStatusTransport struct {
	base  http.RoundTripper
	codes chan int
}

func (t *taskResumeAuthorizationStatusTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if err == nil && r.URL.Path == "/v1/call/ResumeTaskClaim" {
		t.codes <- response.StatusCode
	}
	return response, err
}

// Exercise the real authenticated HTTP handler, not just err != nil or a
// recorder fed an independently constructed error. Both bearer classes must
// refuse nested foreign ownership before row access/lifecycle mutation.
func TestTaskResumeHTTPForeignPrincipalIs403WithoutTaskMutation(t *testing.T) {
	for _, tokenClass := range []string{"scoped", "operator"} {
		for _, action := range []string{"inspect", "claim"} {
			t.Run(tokenClass+"/"+action, func(t *testing.T) {
				s, _, req := taskResumeFixture(t)
				if err := s.AddTask(Task{Agent: "foreign-lane", TaskID: "foreign-existing", Subject: "foreign private fixture", Status: "pending"}); err != nil {
					t.Fatal(err)
				}
				ownBefore := taskEvidenceRaw(t, s, req.Agent, req.TaskID)
				foreignBefore := taskEvidenceRaw(t, s, "foreign-lane", "foreign-existing")
				auditsBefore := taskResumeAuditCount(t, s)
				handler, err := Handler(s, ServerOptions{Token: "fixture-bootstrap", RuleOfRa: "off"})
				if err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(handler)
				t.Cleanup(server.Close)
				token := "fixture-bootstrap"
				if tokenClass == "scoped" {
					token, _, err = s.MintScopedHostToken("fixture-host", "fixture", []string{req.Agent})
					if err != nil {
						t.Fatal(err)
					}
				}
				transport := http.DefaultTransport.(*http.Transport).Clone()
				t.Cleanup(transport.CloseIdleConnections)
				codes := make(chan int, 2)
				client := NewMemoryOnlyRemoteStore(server.URL, token)
				client.host, client.agent, client.threadID = "fixture-host", req.Agent, "fixture-thread"
				client.client.Transport = &taskResumeAuthorizationStatusTransport{base: transport, codes: codes}
				t.Cleanup(func() { _ = client.Close() })
				foreign := req
				foreign.Agent, foreign.TaskID, foreign.Action = "foreign-lane", "foreign-existing", action
				if action == "claim" {
					foreign.ExpectedVersion, foreign.EvidenceRef, foreign.EvidenceSHA256 = strings.Repeat("a", 64), "evidence://foreign-not-authorized", strings.Repeat("b", 64)
					foreign.Reason, foreign.TTLSeconds = "foreign request must never reach the task", 60
				}
				_, err = client.ResumeTaskClaim(context.Background(), foreign)
				if err == nil || !strings.Contains(err.Error(), "authenticated own-lane principal required") {
					t.Fatalf("foreign authorization refusal missing: %v", err)
				}
				select {
				case status := <-codes:
					if status != http.StatusForbidden {
						t.Errorf("foreign authorization returned HTTP%d, want403 (not a server fault)", status)
					}
				default:
					t.Fatal("actual ResumeTaskClaim HTTP response was not observed")
				}
				if !reflect.DeepEqual(ownBefore, taskEvidenceRaw(t, s, req.Agent, req.TaskID)) || !reflect.DeepEqual(foreignBefore, taskEvidenceRaw(t, s, "foreign-lane", "foreign-existing")) || taskResumeAuditCount(t, s) != auditsBefore {
					t.Fatal("refused foreign request changed task rows or resume audit")
				}
			})
		}
	}
}
