package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Only diagnostic read and close exist. Any accidental new store operation
// reaches the nil embedded interface, rather than silently allowing a mutation.
type taskWhyReadOnlyStore struct {
	routerstore.Store
	answer        routerstore.TaskEligibility
	err           error
	reads, closes int
	agent, task   string
}

func (s *taskWhyReadOnlyStore) TaskEligibility(agent, task string) (routerstore.TaskEligibility, error) {
	s.reads++
	s.agent, s.task = agent, task
	return s.answer, s.err
}

func (s *taskWhyReadOnlyStore) Close() error { s.closes++; return nil }

func TestTaskWhyCLIPlainScopesRowAndLegacyAnswers(t *testing.T) {
	for _, tc := range []struct {
		name, wire, warning, verdict string
	}{
		{"row claimable", `{"agent":"lane","task_id":"task","scope":"task-row","caller_evaluation":"not-evaluated","status":"pending","claimable":true,"dispatchable":true,"reasons":["row predicates only"]}`, "NOT EVALUATED", "task-row claimable"},
		{"row refused", `{"agent":"lane","task_id":"task","scope":"task-row","caller_evaluation":"not-evaluated","status":"blocked","claimable":false,"dispatchable":false,"reasons":["status is blocked"]}`, "NOT EVALUATED", "task-row NOT claimable"},
		{"old service", `{"agent":"lane","task_id":"task","status":"pending","claimable":true,"dispatchable":true,"reasons":["no blocking cause: a claim by this lane would succeed"]}`, "scope is unknown or unsupported", "unknown-scope claimable"},
		{"partial metadata", `{"agent":"lane","task_id":"task","scope":"task-row","status":"pending","claimable":true}`, "scope is unknown or unsupported", "unknown-scope claimable"},
		{"unrecognized scope", `{"agent":"lane","task_id":"task","scope":"caller-aware","caller_evaluation":"evaluated","claimable":true}`, "scope is unknown or unsupported", "unknown-scope claimable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var answer routerstore.TaskEligibility
			if err := json.Unmarshal([]byte(tc.wire), &answer); err != nil {
				t.Fatal(err)
			}
			store := &taskWhyReadOnlyStore{answer: answer}
			opens := 0
			cmd := newTaskWhyCommand(func() (*dispatch.Facade, error) {
				opens++
				return dispatch.New("inert-test-root", store), nil
			})
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetArgs([]string{"lane", "task"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			out := stdout.String()
			if !strings.Contains(out, tc.warning) || !strings.Contains(out, tc.verdict) || !strings.Contains(out, "not claim authorization") {
				t.Fatalf("missing scoped diagnostic warning: %q", out)
			}
			if answer.Scope != routerstore.TaskEligibilityScopeRow || answer.CallerEvaluation != routerstore.TaskEligibilityCallerNotEvaluated {
				if !strings.Contains(out, "NOT CONFIRMED") || strings.Contains(out, "task-row claimable") || strings.Contains(out, "service claimable") {
					t.Fatalf("unknown service scope inferred caller/row checks: %q", out)
				}
			}
			if tc.name == "old service" && strings.Index(out, "WARNING:") > strings.Index(out, "would succeed") {
				t.Fatal("legacy service reason preceded its unknown-scope warning")
			}
			if opens != 1 || store.reads != 1 || store.closes != 1 || store.agent != "lane" || store.task != "task" {
				t.Fatalf("not exactly one read/close of requested task: opens=%d store=%+v", opens, store)
			}
		})
	}
}

func TestTaskWhyCLIJSONPreservesFlagsAndDoesNotInventLegacyScope(t *testing.T) {
	for _, metadata := range []bool{true, false} {
		answer := routerstore.TaskEligibility{Agent: "lane", TaskID: "task", Claimable: true, Dispatchable: false}
		if metadata {
			answer.Scope = routerstore.TaskEligibilityScopeRow
			answer.CallerEvaluation = routerstore.TaskEligibilityCallerNotEvaluated
		}
		store := &taskWhyReadOnlyStore{answer: answer}
		cmd := newTaskWhyCommand(func() (*dispatch.Facade, error) { return dispatch.New("inert-test-root", store), nil })
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetArgs([]string{"lane", "task", "--json"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var got routerstore.TaskEligibility
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("JSON contaminated with plain warning: %s: %v", stdout.Bytes(), err)
		}
		if got.Scope != answer.Scope || got.CallerEvaluation != answer.CallerEvaluation || !got.Claimable || got.Dispatchable {
			t.Fatalf("metadata/row flags rewritten: %+v", got)
		}
		if store.reads != 1 || store.closes != 1 {
			t.Fatal("JSON path did more than diagnostic read/close")
		}
	}
}

func TestTaskWhyCLIValidationAndReadErrorsDoNotAttemptClaims(t *testing.T) {
	for _, args := range [][]string{{"lane"}, {"lane", "task", "extra"}, {"lane", "task", "--apply"}} {
		opens := 0
		cmd := newTaskWhyCommand(func() (*dispatch.Facade, error) { opens++; return nil, errors.New("must not resolve") })
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil || opens != 0 {
			t.Fatal("invalid diagnostic flags reached store", args, err)
		}
	}
	want := errors.New("diagnostic read refused")
	store := &taskWhyReadOnlyStore{err: want}
	cmd := newTaskWhyCommand(func() (*dispatch.Facade, error) { return dispatch.New("inert-test-root", store), nil })
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"lane", "task"})
	if err := cmd.Execute(); !errors.Is(err, want) || stdout.Len() != 0 || store.closes != 1 {
		t.Fatalf("read failure became a verdict: output=%q err=%v", stdout.String(), err)
	}
}
