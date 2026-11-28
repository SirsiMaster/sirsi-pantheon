package routerstore

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type outcomeFake struct {
	p                                  TaskYieldProtocol
	capErr, yieldErr                   error
	caps, yields, failures             int
	kind, evidence, dependency, reason string
}

func (f *outcomeFake) TaskYieldCapabilities(string) (TaskYieldProtocol, error) {
	f.caps++
	return f.p, f.capErr
}
func (f *outcomeFake) YieldTaskLease(a, b, c, k, e, d string) error {
	f.yields++
	f.kind = k
	f.evidence = e
	f.dependency = d
	return f.yieldErr
}
func (f *outcomeFake) ReleaseTaskLease(a, b, c, r string) error {
	f.failures++
	f.reason = r
	return nil
}
func TestTaskOutcomeExplicitDispatchAndNoHeuristics(t *testing.T) {
	f := &outcomeFake{p: TaskYieldProtocol{1, true, true, true, true}}
	if e := SettleTaskAttempt(f, "ra", "t", "fence", TaskAttemptOutcome{Disposition: "failure", FailureReason: "clean external wait"}); e != nil {
		t.Fatal(e)
	}
	if f.failures != 1 || f.caps != 0 || f.yields != 0 {
		t.Fatal("reason heuristics")
	}
	for _, o := range []TaskAttemptOutcome{{Disposition: "clean", EvidenceRef: "receipt:handoff"}, {Disposition: "external-wait", EvidenceRef: "receipt:partial", Dependency: "external:owner"}} {
		if e := SettleTaskAttempt(f, "ra", "t", "fence", o); e != nil {
			t.Fatal(e)
		}
		if f.kind != o.Disposition || f.evidence != o.EvidenceRef || f.dependency != o.Dependency {
			t.Fatal("structured outcome lost")
		}
	}
	if f.caps != 2 || f.yields != 2 || f.failures != 1 {
		t.Fatal(f)
	}
}
func TestTaskOutcomeUnsupportedProtocolNeverFailureFallback(t *testing.T) {
	for _, f := range []*outcomeFake{{capErr: errors.New("404 old server")}, {p: TaskYieldProtocol{}}, {p: TaskYieldProtocol{Version: 1, Clean: true, CurrentAttemptOnly: true}}, {p: TaskYieldProtocol{1, true, true, true, true}, yieldErr: errors.New("404 mixed old route")}} {
		if e := SettleTaskAttempt(f, "ra", "t", "fence", TaskAttemptOutcome{Disposition: "clean", EvidenceRef: "receipt"}); e == nil {
			t.Fatal("unsupported accepted")
		}
		if f.failures != 0 {
			t.Fatal("fell back to failed release")
		}
	}
	for _, o := range []TaskAttemptOutcome{{Disposition: "expired"}, {Disposition: "clean"}, {Disposition: "external-wait", EvidenceRef: "r"}, {Disposition: "clean", EvidenceRef: "r", FailureReason: "success"}} {
		f := &outcomeFake{}
		if e := SettleTaskAttempt(f, "ra", "t", "fence", o); e == nil || f.caps+f.yields+f.failures != 0 {
			t.Fatal("invalid intent dispatched", o, e)
		}
	}
}
func TestTaskOutcomeRealCallerResumesSameOriginal(t *testing.T) {
	s := newTestStore(t)
	if e := s.AddTask(Task{Agent: "ra", TaskID: "original", Subject: "phase"}); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < MaxRefundedYields; i++ {
		l, e := s.ClaimTask("ra", "original", "w", "thread", time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		if e = SettleTaskAttempt(s, "ra", "original", l.Token, TaskAttemptOutcome{Disposition: "clean", EvidenceRef: "receipt:phase"}); e != nil {
			t.Fatal(e)
		}
	}
	e, _ := s.TaskEligibility("ra", "original")
	if e.Attempts != 0 || !e.Claimable {
		t.Fatal(e)
	}
}

// Actual HTTP path, not the fake dispatch: a revoked/unknown protocol cannot
// change executable ownership. Normal failure callers remain compatible.
func TestTaskOutcomeWireProtocolAndRefund(t *testing.T) {
	// main requires a registered thread for a remote claim (Rule of Ra).
	var logs strings.Builder
	backend, client := ruleHarness(t, "enforce", &logs)
	host, _ := os.Hostname()
	register(t, backend, "own", "ra", host, "active", time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC))
	rs := client("ra", "own")
	if e := backend.AddTask(Task{Agent: "ra", TaskID: "wire", Subject: "work"}); e != nil {
		t.Fatal(e)
	}
	l, e := rs.ClaimTask("ra", "wire", "w", "own", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	if e = SettleTaskAttempt(rs, "ra", "wire", l.Token, TaskAttemptOutcome{Disposition: "clean", EvidenceRef: "receipt:wire"}); e != nil {
		t.Fatal(e)
	}
	got, e := backend.TaskEligibility("ra", "wire")
	if e != nil || got.Attempts != 0 || got.LeaseHeld {
		t.Fatal(got, e)
	}
}

func TestTaskOutcomeOldHTTPServerRetainsLease(t *testing.T) {
	s := newTestStore(t)
	s.now = nil
	if e := s.AddTask(Task{Agent: "ra", TaskID: "old", Subject: "work"}); e != nil {
		t.Fatal(e)
	}
	host, _ := os.Hostname()
	fixed := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	register(t, s, "own", "ra", host, "active", fixed)
	h, e := Handler(s, ServerOptions{Token: "test-token", RuleOfRa: "enforce", RuleOfRaStale: 10 * time.Minute, now: func() time.Time { return fixed }})
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("SIRSI_AGENT_ID", "ra")
	t.Setenv("SIRSI_THREAD_ID", "own")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/call/TaskYieldCapabilities" {
			writeErr(w, http.StatusNotFound, "", "no such store method")
			return
		}
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()
	rs := NewRemoteStore(srv.URL, "test-token")
	rs.sessionDir = ""
	rs.now = func() time.Time { return fixed }
	l, e := rs.ClaimTask("ra", "old", "w", "own", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	if e = SettleTaskAttempt(rs, "ra", "old", l.Token, TaskAttemptOutcome{Disposition: "external-wait", EvidenceRef: "receipt:wait", Dependency: "external:owner"}); e == nil {
		t.Fatal("old server activated yield")
	}
	got, e := s.TaskEligibility("ra", "old")
	if e != nil || !got.LeaseHeld || got.Attempts != 1 || got.BlockedBy != "" {
		t.Fatal("unsupported protocol changed task", got, e)
	}
}
