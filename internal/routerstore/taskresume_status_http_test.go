package routerstore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type taskResumeStatusRecorder struct {
	base  http.RoundTripper
	codes chan int
}

func (r *taskResumeStatusRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.base.RoundTrip(req)
	if err == nil && req.URL.Path == "/v1/call/ResumeTaskClaim" {
		r.codes <- resp.StatusCode
	}
	return resp, err
}

// taskResumeStatusClient serves s over real HTTP and returns an own-lane client
// plus the channel of observed ResumeTaskClaim status codes.
func taskResumeStatusClient(t *testing.T, s *SQLiteStore, agent string) (*RemoteStore, chan int) {
	t.Helper()
	h, err := Handler(s, ServerOptions{Token: "fixture-bootstrap", RuleOfRa: "off"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	token, _, err := s.MintScopedHostToken("fixture-host", "fixture", []string{agent})
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	codes := make(chan int, 8)
	c := NewMemoryOnlyRemoteStore(srv.URL, token)
	c.host, c.agent, c.threadID = "fixture-host", agent, "fixture-thread"
	c.client.Transport = &taskResumeStatusRecorder{base: transport, codes: codes}
	t.Cleanup(func() { _ = c.Close() })
	return c, codes
}

func taskResumeWantStatus(t *testing.T, codes chan int, want int) {
	t.Helper()
	select {
	case got := <-codes:
		if got != want {
			t.Errorf("ResumeTaskClaim HTTP %d, want %d", got, want)
		}
	default:
		t.Fatal("no ResumeTaskClaim HTTP response observed")
	}
}

// Legitimate refusals are the caller's answer, not a server fault: HTTP 409,
// errors.Is(ErrTaskResumeRefused) and the reason text survive the wire. The
// request is never changed, only the status the refusal travels under.
func TestTaskResumeRefusalsAreHTTP409WithReason(t *testing.T) {
	repin := func(t *testing.T, s *SQLiteStore, ctx context.Context, req *TaskResumeRequest) {
		t.Helper()
		if err := s.withTaskEvidenceTx(ctx, func(tx taskEvidenceTx) error {
			r, err := taskResumeRead(ctx, tx, req.Agent, req.TaskID, s.dialect().name == "postgres")
			req.ExpectedVersion = r.version()
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, sql, reason string }{
		{"terminal-state", `UPDATE tasks SET status='done' WHERE task_id='a'`, "terminal/dependency state refuses"},
		{"dependency", `UPDATE tasks SET blocked_by='external-review-pending' WHERE task_id='a'`, "terminal/dependency state refuses"},
		{"live-custody", `UPDATE tasks SET lease_token='live',lease_expires='2099-01-01T00:00:00Z',claimed_by='worker',thread_id='thread' WHERE task_id='a'`, "live or malformed retained custody refuses"},
		{"malformed-custody", `UPDATE tasks SET lease_token='old',lease_expires='bad',claimed_by='worker',thread_id='thread' WHERE task_id='a'`, "live or malformed retained custody refuses"},
		{"below-ceiling", `UPDATE tasks SET attempts=2,status='pending' WHERE task_id='a'`, "below-ceiling original must be blocked"},
		{"negative-attempts", `UPDATE tasks SET attempts=-1 WHERE task_id='a'`, "valid incrementable original attempts required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, ctx, req := taskResumeFixture(t)
			req = taskResumePin(t, s, ctx, req)
			if _, err := s.exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			repin(t, s, ctx, &req) // policy refusal, not version CAS
			c, codes := taskResumeStatusClient(t, s, req.Agent)
			_, err := c.ResumeTaskClaim(context.Background(), req)
			if !errors.Is(err, ErrTaskResumeRefused) || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("want ErrTaskResumeRefused with %q, got %v", tc.reason, err)
			}
			taskResumeWantStatus(t, codes, http.StatusConflict)
		})
	}

	t.Run("stale-row-version", func(t *testing.T) {
		s, ctx, req := taskResumeFixture(t)
		req = taskResumePin(t, s, ctx, req)
		if _, err := s.exec(`UPDATE tasks SET project_id='changed-scope' WHERE agent=? AND task_id=?`, req.Agent, req.TaskID); err != nil {
			t.Fatal(err)
		}
		c, codes := taskResumeStatusClient(t, s, req.Agent)
		_, err := c.ResumeTaskClaim(context.Background(), req)
		if !errors.Is(err, ErrTaskResumeRefused) || !strings.Contains(err.Error(), "stale row version") {
			t.Fatalf("got %v", err)
		}
		taskResumeWantStatus(t, codes, http.StatusConflict)
	})

	t.Run("changed-request-no-replay-and-lease-no-longer-live", func(t *testing.T) {
		s, ctx, req := taskResumeFixture(t)
		req = taskResumePin(t, s, ctx, req)
		c, codes := taskResumeStatusClient(t, s, req.Agent)
		if _, err := c.ResumeTaskClaim(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		taskResumeWantStatus(t, codes, http.StatusOK)
		changed := req
		changed.EvidenceRef = "evidence://different"
		_, err := c.ResumeTaskClaim(context.Background(), changed)
		if !errors.Is(err, ErrTaskResumeRefused) || !strings.Contains(err.Error(), "no replay") {
			t.Fatalf("got %v", err)
		}
		taskResumeWantStatus(t, codes, http.StatusConflict)
		// Exact replay after the recorded lease (60s) lapsed; the principal is
		// still fresh, so this is the lease check, not a principal failure.
		base := s.clock()
		s.now = func() time.Time { return base.Add(90 * time.Second) }
		_, err = c.ResumeTaskClaim(context.Background(), req)
		if !errors.Is(err, ErrTaskResumeRefused) || !strings.Contains(err.Error(), "no longer live") {
			t.Fatalf("got %v", err)
		}
		taskResumeWantStatus(t, codes, http.StatusConflict)
	})
}

// Input errors stay 400, lane scope stays 403, and a genuine store failure
// stays a 500: the new mapping must not swallow real faults.
func TestTaskResumeStoreFailureStaysHTTP500(t *testing.T) {
	s, ctx, req := taskResumeFixture(t)
	req = taskResumePin(t, s, ctx, req)
	trigger := `CREATE TRIGGER fail_resume BEFORE INSERT ON state WHEN NEW.key LIKE 'task-resume:%' BEGIN SELECT RAISE(ABORT,'fixture audit failure'); END`
	if s.dialect().name == "postgres" {
		trigger = `CREATE FUNCTION fail_resume_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.key LIKE 'task-resume:%' THEN RAISE EXCEPTION 'fixture audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_resume BEFORE INSERT ON state FOR EACH ROW EXECUTE FUNCTION fail_resume_audit()`
	}
	if _, err := s.exec(trigger); err != nil {
		t.Fatal(err)
	}
	c, codes := taskResumeStatusClient(t, s, req.Agent)
	_, err := c.ResumeTaskClaim(context.Background(), req)
	if err == nil || errors.Is(err, ErrTaskResumeRefused) || !strings.Contains(err.Error(), "fixture audit failure") {
		t.Fatalf("store failure must surface as a plain error, got %v", err)
	}
	taskResumeWantStatus(t, codes, http.StatusInternalServerError)

	bad := req
	bad.TTLSeconds = 0
	_, err = c.ResumeTaskClaim(context.Background(), bad)
	if !errors.Is(err, ErrTaskResumeInput) {
		t.Fatalf("got %v", err)
	}
	taskResumeWantStatus(t, codes, http.StatusBadRequest)
}
