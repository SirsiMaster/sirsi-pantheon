package routerstore

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// Uses the existing shared SQLite/isolated-PG harness, never a live store.
func taskResumeFixture(t *testing.T) (*SQLiteStore, context.Context, TaskResumeRequest) {
	t.Helper()
	s, ctx, _ := taskEvidenceFixture(t)
	if _, err := s.exec(`UPDATE tasks SET status='blocked',blocked_by='',lease_token='',lease_expires='',claimed_by='',thread_id='' WHERE agent='fixture-lane' AND task_id='a'`); err != nil {
		t.Fatal(err)
	}
	r := TaskResumeRequest{Version: 1, Action: "inspect", Agent: "fixture-lane", TaskID: "a"}
	return s, ctx, r
}

func taskResumePin(t *testing.T, s *SQLiteStore, ctx context.Context, r TaskResumeRequest) TaskResumeRequest {
	t.Helper()
	out, err := s.ResumeTaskClaim(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if out.Claimed || out.Lease != nil || out.Attempts != 3 || !taskEvidenceDigest(out.RowVersion) {
		t.Fatalf("inspection changed/disclosed custody: %+v", out)
	}
	r.Action, r.ExpectedVersion, r.EvidenceRef, r.EvidenceSHA256, r.Reason, r.TTLSeconds = "claim", out.RowVersion, "evidence://owned/cause-fixed", strings.Repeat("b", 64), "original cause fixed and proof reviewed externally", 60
	return r
}

func taskResumeAuditCount(t *testing.T, s *SQLiteStore) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM state WHERE key LIKE ?`, taskResumePrefix+"%").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTaskResumePreservesOriginalHistoryAndNormalCompletion(t *testing.T) {
	s, ctx, req := taskResumeFixture(t)
	if _, err := s.ClaimTask(req.Agent, req.TaskID, req.Agent, "fixture-thread", time.Minute); !errors.Is(err, ErrNoClaimableTask) {
		t.Fatal("ordinary retry ceiling changed", err)
	}
	oldExpiry := s.clock().Add(-time.Minute).UTC().Format(time.RFC3339)
	if _, err := s.exec(`UPDATE tasks SET lease_token='old-expired-token',lease_expires=?,claimed_by='old-worker',thread_id='old-thread' WHERE agent=? AND task_id=?`, oldExpiry, req.Agent, req.TaskID); err != nil {
		t.Fatal(err)
	}
	before := taskEvidenceRaw(t, s, req.Agent, req.TaskID)
	if len(before) != len(taskResumeSnapshot{}) {
		t.Fatal("complete persisted schema snapshot is out of date")
	}
	req = taskResumePin(t, s, ctx, req)
	out, err := s.ResumeTaskClaim(ctx, req)
	if err != nil || !out.Claimed || out.Repeated || out.Lease == nil || out.Lease.Attempt != 4 || out.Attempts != 4 || out.Lease.Token == "old-expired-token" {
		t.Fatalf("RESUME_HISTORY_PATH_MISSING: %+v %v", out, err)
	}
	after := taskEvidenceRaw(t, s, req.Agent, req.TaskID)
	changed := map[string]bool{"status": true, "claimed_by": true, "thread_id": true, "lease_token": true, "lease_expires": true, "attempts": true, "updated": true}
	for col, want := range before {
		if !changed[col] && after[col] != want {
			t.Fatalf("non-lifecycle column %s changed", col)
		}
	}
	if after["failure_reason"] != before["failure_reason"] || after["attempts"].String != "4" || taskResumeAuditCount(t, s) != 1 {
		t.Fatal("history reset or audit missing")
	}
	raw, ok, err := s.GetState(out.AuditKey)
	if err != nil || !ok || strings.Contains(raw, out.Lease.Token) || strings.Contains(raw, "old-expired-token") {
		t.Fatal("audit missing or exposed a lease secret", err)
	}
	var audit taskResumeAudit
	if json.Unmarshal([]byte(raw), &audit) != nil || audit.AttemptsBefore != 3 || audit.AttemptsAfter != 4 || audit.Request.EvidenceSHA256 != req.EvidenceSHA256 || audit.OldFailure != before["failure_reason"].String || audit.BeforeVersion != req.ExpectedVersion || audit.OldHolder != "old-worker" || audit.OldThread != "old-thread" || audit.OldExpiry != oldExpiry {
		t.Fatal("audit failed to bind original history/evidence")
	}
	replay, err := s.ResumeTaskClaim(ctx, req)
	if err != nil || !replay.Repeated || replay.Lease == nil || replay.Lease.Token != out.Lease.Token || replay.Attempts != 4 || taskResumeAuditCount(t, s) != 1 {
		t.Fatal("exact retry created another attempt", err)
	}
	if err := s.CompleteTaskLease(req.Agent, req.TaskID, "old-expired-token", "forbidden-old-proof"); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatal("old lease revived", err)
	}
	if err := s.CompleteTaskLease(req.Agent, req.TaskID, out.Lease.Token, "evidence://normal-fenced-completion"); err != nil {
		t.Fatal(err)
	}
	final := taskEvidenceRaw(t, s, req.Agent, req.TaskID)
	if final["status"].String != "done" || final["attempts"].String != "4" || final["failure_reason"] != before["failure_reason"] {
		t.Fatal("normal completion reset original history")
	}
	if _, err := s.ResumeTaskClaim(ctx, req); err == nil {
		t.Fatal("replay accepted changed terminal after-state")
	}
}

func TestTaskResumeRefusesIneligibleAndForeignPrincipals(t *testing.T) {
	for _, tc := range []struct{ name, sql string }{
		{"live-holder", `UPDATE tasks SET lease_token='live',lease_expires='2099-01-01T00:00:00Z',claimed_by='worker',thread_id='thread' WHERE task_id='a'`},
		{"malformed-expiry", `UPDATE tasks SET lease_token='old',lease_expires='bad',claimed_by='worker',thread_id='thread' WHERE task_id='a'`},
		{"partial-custody", `UPDATE tasks SET claimed_by='retained' WHERE task_id='a'`},
		{"done", `UPDATE tasks SET status='done' WHERE task_id='a'`},
		{"real-blocker", `UPDATE tasks SET blocked_by='external-review-pending' WHERE task_id='a'`},
		{"not-exhausted", `UPDATE tasks SET attempts=2 WHERE task_id='a'`},
		{"closed-principal", `UPDATE threads SET status='closed' WHERE thread_id='fixture-thread'`},
		{"stale-principal", `UPDATE threads SET last_seen_at='2000-01-01T00:00:00Z' WHERE thread_id='fixture-thread'`},
		{"revoked-session", `UPDATE sessions SET revoked='2026-01-01T00:00:00Z' WHERE agent='fixture-lane'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, ctx, req := taskResumeFixture(t)
			req = taskResumePin(t, s, ctx, req)
			if _, err := s.exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			// Pin a matching actual row version, so policy negatives do not pass
			// merely because every altered row is rejected by version CAS first.
			if err := s.withTaskEvidenceTx(ctx, func(tx taskEvidenceTx) error {
				r, err := taskResumeRead(ctx, tx, req.Agent, req.TaskID, s.dialect().name == "postgres")
				req.ExpectedVersion = r.version()
				return err
			}); err != nil {
				t.Fatal(err)
			}
			before := taskEvidenceRaw(t, s, req.Agent, req.TaskID)
			if _, err := s.ResumeTaskClaim(ctx, req); err == nil {
				t.Fatal("INELIGIBLE_RESUME_ACCEPTED", tc.name)
			}
			if !reflect.DeepEqual(before, taskEvidenceRaw(t, s, req.Agent, req.TaskID)) || taskResumeAuditCount(t, s) != 0 {
				t.Fatal("refusal changed original/audit")
			}
		})
	}
	s, ctx, req := taskResumeFixture(t)
	req = taskResumePin(t, s, ctx, req)
	for _, foreign := range []context.Context{context.Background(), context.WithValue(ctx, taskEvidenceActorKey{}, taskEvidenceActor{Agent: "foreign"})} {
		if _, err := s.ResumeTaskClaim(foreign, req); err == nil {
			t.Fatal("fabricated/foreign local actor accepted")
		}
	}
}

func TestTaskResumeCASContentionAndAuditRollback(t *testing.T) {
	s, ctx, req := taskResumeFixture(t)
	req = taskResumePin(t, s, ctx, req)
	before := taskEvidenceRaw(t, s, req.Agent, req.TaskID)
	// Both dialects: failing the new immutable audit INSERT must roll back the
	// task lease, ownership binding, wake changes and attempts together.
	var trigger string
	if s.dialect().name == "postgres" {
		trigger = `CREATE FUNCTION fail_resume_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.key LIKE 'task-resume:%' THEN RAISE EXCEPTION 'fixture audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_resume BEFORE INSERT ON state FOR EACH ROW EXECUTE FUNCTION fail_resume_audit()`
	} else {
		trigger = `CREATE TRIGGER fail_resume BEFORE INSERT ON state WHEN NEW.key LIKE 'task-resume:%' BEGIN SELECT RAISE(ABORT,'fixture audit failure'); END`
	}
	if _, err := s.exec(trigger); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResumeTaskClaim(ctx, req); err == nil {
		t.Fatal("audit failure accepted")
	}
	if !reflect.DeepEqual(before, taskEvidenceRaw(t, s, req.Agent, req.TaskID)) || taskResumeAuditCount(t, s) != 0 {
		t.Fatal("audit failure leaked task mutation")
	}
	if owner, err := s.TaskSession(req.Agent, req.TaskID); err != nil || owner != "" {
		t.Fatal("audit failure leaked ownership binding", err)
	}
	drop := `DROP TRIGGER fail_resume`
	if s.dialect().name == "postgres" {
		drop += ` ON state`
	}
	if _, err := s.exec(drop); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan TaskResumeResult, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; out, err := s.ResumeTaskClaim(ctx, req); results <- out; errs <- err }()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var token string
	for out := range results {
		if out.Lease == nil || out.Attempts != 4 || (token != "" && token != out.Lease.Token) {
			t.Fatal("concurrent exact retry created distinct leases")
		}
		token = out.Lease.Token
	}
	if taskResumeAuditCount(t, s) != 1 {
		t.Fatal("concurrent audit count")
	}
}

func TestTaskResumeStaleVersionAndChangedReplayRefuse(t *testing.T) {
	s, ctx, req := taskResumeFixture(t)
	req = taskResumePin(t, s, ctx, req)
	if _, err := s.exec(`UPDATE tasks SET project_id='changed-scope' WHERE agent=? AND task_id=?`, req.Agent, req.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResumeTaskClaim(ctx, req); err == nil || taskResumeAuditCount(t, s) != 0 {
		t.Fatal("row-version CAS omitted persisted scope")
	}
	req = taskResumePin(t, s, ctx, TaskResumeRequest{Version: 1, Action: "inspect", Agent: req.Agent, TaskID: req.TaskID})
	if _, err := s.ResumeTaskClaim(ctx, req); err != nil {
		t.Fatal(err)
	}
	req.EvidenceRef = "evidence://different"
	if _, err := s.ResumeTaskClaim(ctx, req); err == nil {
		t.Fatal("changed evidence reused same recovery")
	}
}

func TestTaskResumeLegacyUpdateRenewAndFailedAttemptRemainCompatible(t *testing.T) {
	s, ctx, req := taskResumeFixture(t)
	// Explicit ordinary claims allow this own-lane row, but automatic dispatch
	// does not. Preserve that distinction rather than inventing a permission.
	if _, err := s.exec(`UPDATE tasks SET responsible_party='owner' WHERE agent=? AND task_id=?`, req.Agent, req.TaskID); err != nil {
		t.Fatal(err)
	}
	req = taskResumePin(t, s, ctx, req)
	out, err := s.ResumeTaskClaim(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	// Old clients send the exact existing TaskUpdate shape, without a new
	// version, role or compatibility flag; ordinary metadata remains usable.
	updated, err := s.UpdateTask(req.Agent, req.TaskID, TaskUpdate{Phase: "legacy metadata", AddTokens: 2})
	if err != nil || updated.Phase != "legacy metadata" || updated.TokensConsumed != 44 || updated.ResponsibleParty != "owner" {
		t.Fatal("legacy metadata request became incompatible", err)
	}
	if err := s.RenewTaskLease(req.Agent, req.TaskID, out.Lease.Token, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseTaskLease(req.Agent, req.TaskID, out.Lease.Token, "new fourth-attempt failure"); err != nil {
		t.Fatal(err)
	}
	e, err := s.TaskEligibility(req.Agent, req.TaskID)
	if err != nil || e.Attempts != 4 || e.Status != "blocked" || !strings.Contains(e.FailureReason, "new fourth-attempt failure") {
		t.Fatal("new failed attempt was refunded or lost", err)
	}
	raw, ok, err := s.GetState(out.AuditKey)
	if err != nil || !ok || !strings.Contains(raw, "prior failure") {
		t.Fatal("prior failure history lost", err)
	}
	if _, err := s.ResumeTaskClaim(ctx, req); err == nil {
		t.Fatal("old resume automatically granted another failed attempt")
	}
	if _, err := s.ClaimTask(req.Agent, req.TaskID, req.Agent, "fixture-thread", time.Minute); !errors.Is(err, ErrNoClaimableTask) {
		t.Fatal("ordinary retry ceiling was widened", err)
	}
}

func TestTaskResumeHTTPActualOwnCallerAndRemintCompletion(t *testing.T) {
	s, _, req := taskResumeFixture(t)
	h, err := Handler(s, ServerOptions{Token: "fixture-bootstrap", RuleOfRa: "off"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	token, _, err := s.MintScopedHostToken("fixture-host", "fixture", []string{req.Agent})
	if err != nil {
		t.Fatal(err)
	}
	client := func(bearer string) *RemoteStore {
		c := NewMemoryOnlyRemoteStore(srv.URL, bearer)
		c.host, c.agent, c.threadID = "fixture-host", req.Agent, "fixture-thread"
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	first := client(token)
	inspected, err := first.ResumeTaskClaim(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	req.Action, req.ExpectedVersion, req.EvidenceRef, req.EvidenceSHA256, req.Reason, req.TTLSeconds = "claim", inspected.RowVersion, "evidence://cause-fixed", strings.Repeat("a", 64), "fixed original cause", 60
	tooLong := req
	tooLong.TTLSeconds = int64((45 * time.Minute) / time.Second)
	_, inputErr := first.ResumeTaskClaim(context.Background(), tooLong)
	if !errors.Is(inputErr, ErrTaskResumeInput) || taskResumeAuditCount(t, s) != 0 {
		t.Fatal("over-cap request lost typed refusal or issued a lease", inputErr)
	}
	w := httptest.NewRecorder()
	(&server{}).writeCallErr(w, "ResumeTaskClaim", inputErr)
	if w.Code != http.StatusBadRequest {
		t.Fatal("typed resume input was reported as a server fault", w.Code)
	}
	req.TTLSeconds = int64((30 * time.Minute) / time.Second)
	lease, err := first.ResumeTaskClaim(context.Background(), req)
	if err != nil || lease.Lease == nil || lease.Attempts != 4 {
		t.Fatal("actual scoped recovery failed", err)
	}
	if err := first.RenewTaskLease(req.Agent, req.TaskID, lease.Lease.Token, 30*time.Minute); err != nil {
		t.Fatal("normal 30m renewal refused", err)
	}
	if err := s.UpsertThreads([]ThreadRecord{{ThreadID: "other-thread", Agent: req.Agent, Host: "fixture-host", Status: "active", LastSeenAt: s.clock().UTC().Format(time.RFC3339Nano), Payload: json.RawMessage(`{"surface":"api"}`)}}); err != nil {
		t.Fatal(err)
	}
	other := client(token)
	other.threadID = "other-thread"
	if _, err := other.ResumeTaskClaim(context.Background(), req); err == nil {
		t.Fatal("another registered thread replayed recovery authority")
	}
	if err := other.CompleteTaskLease(req.Agent, req.TaskID, lease.Lease.Token, "forbidden-foreign-thread"); !errors.Is(err, ErrNotOwner) {
		t.Fatal("copied lease bypassed authenticated owner", err)
	}
	foreign := req
	foreign.Agent = "foreign-lane"
	if _, err := client("fixture-bootstrap").ResumeTaskClaim(context.Background(), foreign); err == nil {
		t.Fatal("operator token supplied foreign authority")
	}
	for _, key := range []string{lease.AuditKey, "task-resume:v2:future"} {
		if err := first.SetState(key, "forged"); err == nil {
			t.Fatal("generic state writer forged resume audit")
		}
		if ok, err := first.CompareAndSwapState(key, "", false, "forged"); err == nil || ok {
			t.Fatal("generic CAS forged resume audit")
		}
	}
	// Fresh session, same registered worker: existing CompleteTaskLease and
	// sameWorkerAcrossRemint enforce ownership; no new completion authority.
	if err := client(token).CompleteTaskLease(req.Agent, req.TaskID, lease.Lease.Token, "evidence://actual-remote-completion"); err != nil {
		t.Fatal(err)
	}
	final, err := s.GetTask(req.Agent, req.TaskID)
	if err != nil || final.Status != "done" || final.ResultRef != "evidence://actual-remote-completion" {
		t.Fatal("normal authenticated completion missing", err)
	}
	if _, err := s.exec(`UPDATE threads SET status='closed' WHERE thread_id='fixture-thread'`); err != nil {
		t.Fatal(err)
	}
	if _, err := first.ResumeTaskClaim(context.Background(), TaskResumeRequest{Version: 1, Action: "inspect", Agent: req.Agent, TaskID: req.TaskID}); err == nil {
		t.Fatal("general off bypassed retired principal")
	}
}

func TestTaskResumeRequestClosedAndBounded(t *testing.T) {
	for _, bad := range []string{
		`{"version":1,"action":"inspect","Action":"claim"}`,
		`{"version":1,"action":"inspect","action":"claim"}`,
		`{"version":1,"worker":"foreign"}`,
		`{"version":1} {}`,
		`{"reason":"` + strings.Repeat("x", TaskResumeRequestLimit) + `"}`,
	} {
		var req TaskResumeRequest
		if json.Unmarshal([]byte(bad), &req) == nil {
			t.Fatal("unclosed/oversize request accepted")
		}
	}
	for _, edit := range []func(*TaskResumeRequest){
		func(r *TaskResumeRequest) { r.ExpectedVersion = "" },
		func(r *TaskResumeRequest) { r.EvidenceSHA256 = "bad" },
		func(r *TaskResumeRequest) { r.TTLSeconds = int64(MaxLeaseTTL/time.Second) + 1 },
		func(r *TaskResumeRequest) { r.Action = "done" },
	} {
		r := TaskResumeRequest{Version: 1, Action: "claim", Agent: "lane", TaskID: "original", ExpectedVersion: strings.Repeat("a", 64), EvidenceSHA256: strings.Repeat("b", 64), EvidenceRef: "evidence://proof", Reason: "corrected cause", TTLSeconds: 60}
		edit(&r)
		if ValidateTaskResumeRequest(r) == nil {
			t.Fatal("invalid claim request accepted")
		}
	}
}

func TestTaskResumeCanceledAndRawWireBound(t *testing.T) {
	s, ctx, req := taskResumeFixture(t)
	req = taskResumePin(t, s, ctx, req)
	before := taskEvidenceRaw(t, s, req.Agent, req.TaskID)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.ResumeTaskClaim(canceled, req); err == nil {
		t.Fatal("canceled resume succeeded")
	}
	if !reflect.DeepEqual(before, taskEvidenceRaw(t, s, req.Agent, req.TaskID)) || taskResumeAuditCount(t, s) != 0 {
		t.Fatal("canceled call changed original")
	}
	h, err := Handler(s, ServerOptions{Token: "fixture-bootstrap"})
	if err != nil {
		t.Fatal(err)
	}
	// Before JSON strips whitespace or invokes nested UnmarshalJSON.
	wire := httptest.NewRequest(http.MethodPost, "/v1/call/ResumeTaskClaim", strings.NewReader(strings.Repeat(" ", TaskResumeRequestLimit+1)+`{"args":[]}`))
	wire.Header.Set("Authorization", "Bearer fixture-bootstrap")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, wire)
	if w.Code != http.StatusBadRequest {
		t.Fatal("raw resume body ceiling missing", w.Code)
	}
}

func TestTaskResumePostgresHeldRowDeadlineBeforeUnlock(t *testing.T) {
	if pgTestDSN() == "" {
		t.Skip("requires existing isolated PG fixture harness; not a SQLite proof")
	}
	s, ctx, req := taskResumeFixture(t)
	req = taskResumePin(t, s, ctx, req)
	blocker, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback() }()
	var id string
	if err := blocker.QueryRow(`SELECT task_id FROM tasks WHERE agent=? AND task_id=? FOR UPDATE`, req.Agent, req.TaskID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	// Direct call must return while independent lock is STILL HELD. No
	// timeout goroutine, releasing blocker to make the deadline assertion pass,
	// or abandoning SQL. Context goes into the actual SELECT FOR UPDATE.
	if _, err := s.ResumeTaskClaim(bounded, req); err == nil || time.Since(started) > 2*time.Second {
		t.Fatal("held-row resume outlived caller deadline", err)
	}
	if err := blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	if taskResumeAuditCount(t, s) != 0 {
		t.Fatal("deadline wrote audit")
	}
	if current, err := s.TaskEligibility(req.Agent, req.TaskID); err != nil || current.Attempts != 3 || current.LeaseHeld {
		t.Fatal("deadline changed original custody", err)
	}
}
