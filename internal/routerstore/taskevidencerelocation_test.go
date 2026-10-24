package routerstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// Uses the existing backend harness: SQLite normally, isolated test PostgreSQL
// only when its existing SIRSI_TEST_PG_DSN admission is supplied. Never Resolve.
func taskEvidenceFixture(t *testing.T) (*SQLiteStore, context.Context, TaskEvidenceRequest) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"SIRSI_ROUTER_URL", "SIRSI_ROUTER_TOKEN", EnvUseMachineIDHost, EnvCloudHost, EnvSPKIPin} {
		t.Setenv(key, "")
	}
	t.Setenv("SIRSI_AGENT_ID", "fixture-lane")
	t.Setenv("SIRSI_THREAD_ID", "fixture-thread")
	s := openBackendStore(t, filepath.Join(t.TempDir(), "evidence.db"))
	t.Cleanup(func() { _ = s.Close() })
	s.notifyDir = t.TempDir()
	a := taskEvidenceActor{Agent: "fixture-lane", Host: "fixture-host", ThreadID: "fixture-thread", Runtime: "fixture-runtime"}
	if err := s.UpsertThreads([]ThreadRecord{{ThreadID: a.ThreadID, Agent: a.Agent, Host: a.Host, Status: "active", LastSeenAt: s.clock().UTC().Format(time.RFC3339Nano), Payload: json.RawMessage(`{"surface":"worker","answer_role":false}`)}}); err != nil {
		t.Fatal(err)
	}
	sess, err := s.MintSessionForThread(a.Host, a.Agent, a.Runtime, a.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	a.SessionID = sess.ID
	req := TaskEvidenceRequest{Action: "inspect", Manifest: TaskEvidenceManifest{Version: 1, ID: "fixture-batch", Agent: a.Agent, AttestationRef: "review://external-process-not-server-approval", PreservationMapSHA256: strings.Repeat("a", 64)}}
	for _, id := range []string{"a", "b"} {
		charter, outline := "preserve charter", "preserve outline"
		task := Task{Agent: a.Agent, TaskID: id, Subject: "preserve subject", Status: "in-progress", Phase: "review", ResponsibleParty: a.Agent, BlockedBy: "preserve blocker", Charter: &charter, Outline: &outline, CommissionedAt: s.clock().UTC().Format(time.RFC3339), CommissionedBy: "fixture-owner", Timeline: []TimelineEntry{{Day: "2026-10-24", Owner: a.Agent, Hours: 1, Label: "original history"}}, Links: []TaskLink{{Kind: "evidence", Label: "receipt", URL: "file:///removed/" + id}, {Kind: "canon", Label: "untouched", URL: "https://example.invalid/spec"}}, TestState: "passed", Stage: "review", TokensConsumed: 42, DurationSeconds: 17, ResultRef: "/removed/result-" + id}
		if err := s.AddTask(task); err != nil {
			t.Fatal(err)
		}
		if id == "b" {
			if _, err := s.exec(`UPDATE tasks SET status='done' WHERE agent=? AND task_id=?`, a.Agent, id); err != nil {
				t.Fatal(err)
			}
		}
		// Positive metadata fixtures are fully unowned. Preserve every empty
		// lease field plus nonsecret accounting/history/scope sentinels.
		if _, err := s.exec(`UPDATE tasks SET lease_token='',lease_expires='',claimed_by='',thread_id='',attempts=?,idempotency_key=?,failure_reason=?,project_id=?,router_namespace=?,result_ref=? WHERE agent=? AND task_id=?`, 3, "fixture-idempotency-"+id, "prior failure", "fixture-project", "fixture-namespace", task.ResultRef, a.Agent, id); err != nil {
			t.Fatal(err)
		}
		req.Manifest.Tasks = append(req.Manifest.Tasks, TaskEvidenceRow{TaskID: id, Changes: []TaskEvidenceChange{{Field: "link", Kind: "evidence", Label: "receipt", OldURI: "file:///removed/" + id, NewURI: "/preserved/" + id, SHA256: strings.Repeat("b", 64)}, {Field: "result_ref", OldURI: "/removed/result-" + id, NewURI: "file:///preserved/result-" + id, SHA256: strings.Repeat("c", 64)}}})
	}
	return s, context.WithValue(context.Background(), taskEvidenceActorKey{}, a), req
}

func taskEvidencePin(t *testing.T, s *SQLiteStore, ctx context.Context, req TaskEvidenceRequest) TaskEvidenceRequest {
	t.Helper()
	out, err := s.RelocateTaskEvidence(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	for i := range req.Manifest.Tasks {
		for _, r := range out.Rows {
			if r.TaskID == req.Manifest.Tasks[i].TaskID {
				req.Manifest.Tasks[i].ExpectedSHA256 = r.Before
			}
		}
	}
	req.Action = "apply"
	return req
}

// SELECT * deliberately includes private lease/scope columns. This is an owned
// fixture assertion, not an API returning them to a caller.
func taskEvidenceRaw(t *testing.T, s *SQLiteStore, agent, id string) map[string]sql.NullString {
	t.Helper()
	rows, err := s.db.Query(`SELECT * FROM tasks WHERE agent=? AND task_id=?`, agent, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("missing fixture row", rows.Err())
	}
	vals := make([]sql.NullString, len(cols))
	args := make([]any, len(cols))
	for i := range vals {
		args[i] = &vals[i]
	}
	if err := rows.Scan(args...); err != nil {
		t.Fatal(err)
	}
	out := map[string]sql.NullString{}
	for i, k := range cols {
		out[k] = vals[i]
	}
	return out
}

func taskEvidenceAuditCount(t *testing.T, s *SQLiteStore) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM state WHERE key LIKE ?`, taskEvidencePrefix+"%").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTaskEvidenceRelocationPreservesAllOtherColumnsAndExactReplay(t *testing.T) {
	s, ctx, req := taskEvidenceFixture(t)
	req = taskEvidencePin(t, s, ctx, req)
	before := map[string]map[string]sql.NullString{}
	for _, r := range req.Manifest.Tasks {
		before[r.TaskID] = taskEvidenceRaw(t, s, req.Manifest.Agent, r.TaskID)
	}
	var wakes int
	if err := s.db.QueryRow(`SELECT count(*) FROM wake_events`).Scan(&wakes); err != nil {
		t.Fatal(err)
	}
	out, err := s.RelocateTaskEvidence(ctx, req)
	if err != nil || !out.Applied || out.Repeated {
		t.Fatalf("apply: %+v %v", out, err)
	}
	for _, r := range req.Manifest.Tasks {
		after := taskEvidenceRaw(t, s, req.Manifest.Agent, r.TaskID)
		for k, v := range before[r.TaskID] {
			if k != "links" && k != "result_ref" && after[k] != v {
				t.Errorf("%s.%s changed: %v -> %v", r.TaskID, k, v, after[k])
			}
		}
		var links []TaskLink
		if err := json.Unmarshal([]byte(after["links"].String), &links); err != nil {
			t.Fatal(err)
		}
		if links[0].URL != "/preserved/"+r.TaskID || links[1].URL != "https://example.invalid/spec" || after["result_ref"].String != "file:///preserved/result-"+r.TaskID {
			t.Fatal("wrong reference projection", after)
		}
	}
	var afterWakes int
	if err := s.db.QueryRow(`SELECT count(*) FROM wake_events`).Scan(&afterWakes); err != nil || afterWakes != wakes {
		t.Fatalf("wake history changed: %d -> %d (%v)", wakes, afterWakes, err)
	}
	repeated, err := s.RelocateTaskEvidence(ctx, req)
	if err != nil || !repeated.Repeated || repeated.Applied || !reflect.DeepEqual(repeated.Rows, out.Rows) || taskEvidenceAuditCount(t, s) != 1 {
		t.Fatalf("replay: %+v %v", repeated, err)
	}
	raw, ok, err := s.GetState(taskEvidencePrefix + req.Manifest.Agent + ":" + req.Manifest.ID)
	if err != nil || !ok {
		t.Fatal(err)
	}
	var audit taskEvidenceAudit
	if err := json.Unmarshal([]byte(raw), &audit); err != nil {
		t.Fatal(err)
	}
	if audit.Actor.SessionID != ctx.Value(taskEvidenceActorKey{}).(taskEvidenceActor).SessionID || audit.Manifest.AttestationRef != req.Manifest.AttestationRef || !strings.Contains(audit.Qualification, "not independently verified") {
		t.Fatal("missing truthful actor attestation")
	}
	// A changed declaration cannot reuse the id, even though target URIs exist.
	req.Manifest.AttestationRef = "another-review"
	if _, err := s.RelocateTaskEvidence(ctx, req); err == nil {
		t.Fatal("manifest collision accepted")
	}
}

func TestTaskEvidenceRelocationBatchCASAndRollback(t *testing.T) {
	for _, mode := range []string{"stale", "missing", "ambiguous", "audit-collision", "expired-during-transaction", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, req := taskEvidenceFixture(t)
			req = taskEvidencePin(t, s, ctx, req)
			switch mode {
			case "stale":
				if _, err := s.exec(`UPDATE tasks SET subject='concurrent edit' WHERE agent=? AND task_id='b'`, req.Manifest.Agent); err != nil {
					t.Fatal(err)
				}
			case "missing":
				req.Manifest.Tasks[1].TaskID = "missing"
			case "ambiguous":
				var links []TaskLink
				raw := taskEvidenceRaw(t, s, req.Manifest.Agent, "b")
				if err := json.Unmarshal([]byte(raw["links"].String), &links); err != nil {
					t.Fatal(err)
				}
				links = append(links, links[0])
				b, _ := json.Marshal(links)
				if _, err := s.exec(`UPDATE tasks SET links=? WHERE agent=? AND task_id='b'`, string(b), req.Manifest.Agent); err != nil {
					t.Fatal(err)
				}
				req.Action = "inspect"
				req = taskEvidencePin(t, s, ctx, req)
			case "audit-collision":
				if _, err := s.exec(`INSERT INTO state(key,value) VALUES(?,?)`, taskEvidencePrefix+req.Manifest.Agent+":"+req.Manifest.ID, `{"unrelated":"record"}`); err != nil {
					t.Fatal(err)
				}
			case "expired-during-transaction":
				now := s.clock()
				calls := 0
				s.now = func() time.Time {
					calls++
					if calls >= 2 {
						return now.Add(time.Hour)
					}
					return now
				}
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			before := taskEvidenceRaw(t, s, req.Manifest.Agent, "a")
			auditBefore := taskEvidenceAuditCount(t, s)
			if _, err := s.RelocateTaskEvidence(ctx, req); err == nil {
				t.Fatal("unsafe batch accepted")
			}
			if !reflect.DeepEqual(before, taskEvidenceRaw(t, s, req.Manifest.Agent, "a")) || taskEvidenceAuditCount(t, s) != auditBefore {
				t.Fatal("failed batch changed first task/audit")
			}
		})
	}
}

func TestTaskEvidenceRelocationReplayRequiresActualAfterState(t *testing.T) {
	s, ctx, req := taskEvidenceFixture(t)
	req = taskEvidencePin(t, s, ctx, req)
	if _, err := s.RelocateTaskEvidence(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := s.exec(`UPDATE tasks SET subject='post-apply edit' WHERE agent=? AND task_id='a'`, req.Manifest.Agent); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RelocateTaskEvidence(ctx, req); err == nil {
		t.Fatal("replay ignored changed after-state")
	}
}

func TestTaskEvidenceRelocationConcurrentExactReplay(t *testing.T) {
	s, ctx, req := taskEvidenceFixture(t)
	peer := taskEvidencePeer(t, s)
	req = taskEvidencePin(t, s, ctx, req)
	var wg sync.WaitGroup
	results := make(chan TaskEvidenceResult, 2)
	errs := make(chan error, 2)
	for _, writer := range []*SQLiteStore{s, peer} {
		wg.Add(1)
		go func(writer *SQLiteStore) {
			defer wg.Done()
			out, err := writer.RelocateTaskEvidence(ctx, req)
			results <- out
			errs <- err
		}(writer)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	applied, repeated := 0, 0
	for r := range results {
		if r.Applied {
			applied++
		}
		if r.Repeated {
			repeated++
		}
	}
	if applied != 1 || repeated != 1 || taskEvidenceAuditCount(t, s) != 1 {
		t.Fatalf("applied=%d repeated=%d", applied, repeated)
	}
}

func TestTaskEvidenceRelocationHTTPAuthenticatesOwnLaneNotAnswerRole(t *testing.T) {
	s, _, req := taskEvidenceFixture(t)
	h, err := Handler(s, ServerOptions{Token: "fixture-bootstrap", RuleOfRa: "off"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	token, _, err := s.MintScopedHostToken("fixture-host", "fixture", []string{req.Manifest.Agent})
	if err != nil {
		t.Fatal(err)
	}
	client := NewRemoteStore(srv.URL, token)
	defer client.Close()
	client.host, client.agent, client.threadID, client.sessionDir = "fixture-host", req.Manifest.Agent, "fixture-thread", ""
	out, err := client.RelocateTaskEvidence(context.Background(), req)
	if err != nil {
		t.Fatalf("worker metadata needs no answering-role grant: %v", err)
	}
	for i := range req.Manifest.Tasks {
		req.Manifest.Tasks[i].ExpectedSHA256 = out.Rows[i].Before
	}
	req.Action = "apply"
	if _, err := client.RelocateTaskEvidence(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	// Even an authenticated bootstrap operator has no foreign-owner override.
	other := NewRemoteStore(srv.URL, "fixture-bootstrap")
	defer other.Close()
	other.host, other.agent, other.threadID, other.sessionDir = "fixture-host", req.Manifest.Agent, "fixture-thread", ""
	foreign := req
	foreign.Manifest.Agent = "foreign-lane"
	if _, err := other.RelocateTaskEvidence(context.Background(), foreign); err == nil {
		t.Fatal("operator rewrote a foreign task")
	}
	for _, c := range []*RemoteStore{client, other} {
		key := taskEvidencePrefix + req.Manifest.Agent + ":" + req.Manifest.ID
		if err := c.SetState(key, "forged"); err == nil {
			t.Fatal("generic SetState overwrote provenance")
		}
		if ok, err := c.CompareAndSwapState(key, "", false, "forged"); err == nil || ok {
			t.Fatal("generic CAS wrote provenance")
		}
	}
	if _, err := s.RelocateTaskEvidence(context.Background(), req); err == nil {
		t.Fatal("missing server-derived principal accepted")
	}
	if _, err := s.exec(`UPDATE threads SET status='closed' WHERE thread_id='fixture-thread'`); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RelocateTaskEvidence(context.Background(), req); err == nil {
		t.Fatal("general off mode bypassed retired principal")
	}
}

func TestTaskEvidenceRelocationRejectsRevokedSession(t *testing.T) {
	s, ctx, req := taskEvidenceFixture(t)
	req = taskEvidencePin(t, s, ctx, req)
	if err := s.RevokeSession(ctx.Value(taskEvidenceActorKey{}).(taskEvidenceActor).SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RelocateTaskEvidence(ctx, req); err == nil {
		t.Fatal("revoked actor accepted")
	}
}

func TestTaskEvidenceManifestClosedAndBounded(t *testing.T) {
	s, _, req := taskEvidenceFixture(t)
	_ = s
	b, _ := json.Marshal(req)
	oversized := req
	oversized.Manifest.AttestationRef = strings.Repeat("x", TaskEvidenceManifestLimit)
	large, err := json.Marshal(oversized)
	if err != nil || len(large) <= TaskEvidenceManifestLimit {
		t.Fatal("invalid oversize fixture", err)
	}
	// Outer json.Unmarshal removes surrounding whitespace before the custom
	// decoder. An oversized string value cannot disappear at that boundary.
	for _, tc := range []struct{ name, bad string }{
		{"duplicate", strings.Replace(string(b), `"action":"inspect"`, `"action":"inspect","action":"apply"`, 1)},
		{"unknown", strings.Replace(string(b), `"version":1`, `"version":1,"status":"done"`, 1)},
		{"trailing", string(b) + `{}`},
		{"oversize-string-value", string(large)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got TaskEvidenceRequest
			if err := json.Unmarshal([]byte(tc.bad), &got); err == nil {
				t.Fatal("unclosed/oversized request accepted")
			}
		})
	}
	var raw TaskEvidenceRequest
	if err := raw.UnmarshalJSON(append([]byte(strings.Repeat(" ", TaskEvidenceManifestLimit)), b...)); err == nil {
		t.Fatal("direct raw decoder lost its byte ceiling")
	}
	for _, mutate := range []func(*TaskEvidenceRequest){func(r *TaskEvidenceRequest) { r.Manifest.Tasks = append(r.Manifest.Tasks, r.Manifest.Tasks[0]) }, func(r *TaskEvidenceRequest) {
		r.Manifest.Tasks[0].Changes = append(r.Manifest.Tasks[0].Changes, r.Manifest.Tasks[0].Changes[0])
	}, func(r *TaskEvidenceRequest) { r.Manifest.Tasks[0].Changes[0].NewURI = "https://example.invalid/remote" }, func(r *TaskEvidenceRequest) { r.Manifest.Tasks[0].Changes[0].Field = "status" }, func(r *TaskEvidenceRequest) { r.Action = "apply" }, func(r *TaskEvidenceRequest) { r.Manifest.PreservationMapSHA256 = "bad" }} {
		var bad TaskEvidenceRequest
		if err := json.Unmarshal(b, &bad); err != nil {
			t.Fatal(err)
		}
		mutate(&bad)
		if err := ValidateTaskEvidenceRequest(bad); err == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
}

func TestTaskEvidenceHTTPRawBodyBoundBeforeJSONWhitespaceRemoval(t *testing.T) {
	s, _, req := taskEvidenceFixture(t)
	h, err := Handler(s, ServerOptions{Token: "fixture-bootstrap", RuleOfRa: "off"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	envelope := `{"args":[` + string(b) + `]}`
	if len(envelope) >= TaskEvidenceManifestLimit {
		t.Fatal("small request fixture exceeds boundary")
	}
	atLimit := strings.Repeat(" ", TaskEvidenceManifestLimit-len(envelope)) + envelope
	large := `{"args":[{"action":"inspect","manifest":{"attestation_ref":"` + strings.Repeat("x", TaskEvidenceManifestLimit) + `"}}]}`
	for _, tc := range []struct {
		name, method, body string
		status             int
		tooLarge           bool
	}{
		{"exact-wire-limit", "RelocateTaskEvidence", atLimit, http.StatusUnauthorized, false},
		{"oversize-wire-padding", "RelocateTaskEvidence", " " + atLimit, http.StatusBadRequest, true},
		{"oversize-wire-value", "RelocateTaskEvidence", large, http.StatusBadRequest, true},
		{"other-method-keeps-existing-limit", "GetTask", " " + atLimit, http.StatusUnauthorized, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/v1/call/"+tc.method, strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer fixture-bootstrap")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			// No session/signature is supplied deliberately. In-bound bodies
			// reach authentication; oversized bytes must fail before that point.
			if w.Code != tc.status || strings.Contains(w.Body.String(), "request body too large") != tc.tooLarge {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
	if taskEvidenceAuditCount(t, s) != 0 {
		t.Fatal("raw body refusal wrote relocation provenance")
	}
}

func TestTaskEvidenceExactJSONKeysRejectAliasesAtEveryLevel(t *testing.T) {
	req := TaskEvidenceRequest{Action: "inspect", Manifest: TaskEvidenceManifest{Version: 1, ID: "case-fixture", Agent: "fixture-lane", AttestationRef: "review://fixture", PreservationMapSHA256: strings.Repeat("a", 64), Tasks: []TaskEvidenceRow{{TaskID: "owned-task", Changes: []TaskEvidenceChange{{Field: "link", Kind: "evidence", Label: "receipt", OldURI: "/old/receipt", NewURI: "/new/receipt", SHA256: strings.Repeat("b", 64)}}}}}}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var canonical TaskEvidenceRequest
	if err := json.Unmarshal(b, &canonical); err != nil || !reflect.DeepEqual(canonical, req) {
		t.Fatal("exact canonical keys refused", err)
	}
	for _, keys := range [][2]string{
		{"action", "Action"}, {"manifest", "Manifest"},
		{"version", "Version"}, {"id", "ID"}, {"agent", "Agent"}, {"attestation_ref", "Attestation_Ref"}, {"preservation_map_sha256", "Preservation_Map_SHA256"}, {"tasks", "Tasks"},
		{"task_id", "Task_ID"}, {"expected_sha256", "Expected_SHA256"}, {"changes", "Changes"},
		{"field", "Field"}, {"kind", "Kind"}, {"label", "Label"}, {"old_uri", "Old_URI"}, {"new_uri", "New_URI"}, {"sha256", "SHA256"},
	} {
		t.Run(keys[1], func(t *testing.T) {
			exact := `"` + keys[0] + `":`
			alias := `"` + keys[1] + `":`
			for _, replacement := range []string{alias, alias + `null,` + exact} {
				bad := strings.Replace(string(b), exact, replacement, 1)
				if bad == string(b) {
					t.Fatal("fixture did not replace a key")
				}
				var got TaskEvidenceRequest
				if err := json.Unmarshal([]byte(bad), &got); err == nil {
					t.Fatal("case alias/semantic duplicate accepted", keys[1])
				}
			}
		})
	}
}

func TestTaskEvidenceStoredLinksRejectExactCaseAliases(t *testing.T) {
	const canonical = `[{"kind":"evidence","label":"receipt","url":"/old/receipt"}]`
	change := []TaskEvidenceChange{{Field: "link", Kind: "evidence", Label: "receipt", OldURI: "/old/receipt", NewURI: "/new/receipt"}}
	var before taskEvidenceSnapshot
	before[14] = sql.NullString{String: canonical, Valid: true}
	if _, err := taskEvidenceChanged(before, change); err != nil {
		t.Fatal("canonical stored link refused", err)
	}
	for _, keys := range [][2]string{{"kind", "Kind"}, {"label", "Label"}, {"url", "URL"}} {
		for _, replacement := range []string{`"` + keys[1] + `":`, `"` + keys[1] + `":null,"` + keys[0] + `":`} {
			before[14].String = strings.Replace(canonical, `"`+keys[0]+`":`, replacement, 1)
			if _, err := taskEvidenceChanged(before, change); err == nil {
				t.Fatal("stored history alias/semantic duplicate normalized", keys[1])
			}
		}
	}
	before[14].String = "null"
	before[19] = sql.NullString{String: "/old/result", Valid: true}
	after, err := taskEvidenceChanged(before, []TaskEvidenceChange{{Field: "result_ref", OldURI: "/old/result", NewURI: "/new/result"}})
	if err != nil || after[14] != before[14] {
		t.Fatal("exact-key guard changed unrelated legacy null links", err)
	}
}

func TestTaskEvidenceStoredLinksRefuseLossyUnrelatedRecords(t *testing.T) {
	const selected = `{"kind":"evidence","label":"receipt","url":"/old/receipt"}`
	for _, unrelated := range []string{
		`{"kind":"canon","label":"unrelated","url":null}`,
		`{"kind":null,"label":"unrelated","url":"/unrelated"}`,
		`{"kind":"canon","label":null,"url":"/unrelated"}`,
		`{"kind":"canon","label":"unrelated"}`,
		`{"label":"unrelated","url":"/unrelated"}`,
		`{"kind":"canon","url":"/unrelated"}`,
		`{"kind":"canon","label":"unrelated","url":17}`,
	} {
		var before taskEvidenceSnapshot
		before[14] = sql.NullString{String: "[" + selected + "," + unrelated + "]", Valid: true}
		after, err := taskEvidenceChanged(before, []TaskEvidenceChange{{Field: "link", Kind: "evidence", Label: "receipt", OldURI: "/old/receipt", NewURI: "/new/receipt"}})
		if err == nil || after != before {
			t.Fatal("unrelated null/missing/nonstring history normalized instead of refusing", unrelated, err)
		}
	}
	var before taskEvidenceSnapshot
	before[14] = sql.NullString{String: "[" + selected + `,{"kind":"canon","label":"unrelated","url":"/unchanged"}]`, Valid: true}
	after, err := taskEvidenceChanged(before, []TaskEvidenceChange{{Field: "link", Kind: "evidence", Label: "receipt", OldURI: "/old/receipt", NewURI: "/new/receipt"}})
	if err != nil {
		t.Fatal("complete string-valued unrelated record refused", err)
	}
	var links []TaskLink
	if err := json.Unmarshal([]byte(after[14].String), &links); err != nil || len(links) != 2 || links[1] != (TaskLink{Kind: "canon", Label: "unrelated", URL: "/unchanged"}) {
		t.Fatal("valid unrelated record changed", err, links)
	}
}

func TestTaskEvidenceFingerprintBindsRawPublicBytesOnly(t *testing.T) {
	s, ctx, req := taskEvidenceFixture(t)
	first, err := s.RelocateTaskEvidence(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.exec(`UPDATE tasks SET lease_token='different-hidden-capability' WHERE agent=? AND task_id='a'`, req.Manifest.Agent); err != nil {
		t.Fatal(err)
	}
	second, err := s.RelocateTaskEvidence(ctx, req)
	if err != nil || second.Rows[0].Before != first.Rows[0].Before {
		t.Fatal("hidden capability entered public digest", err)
	}
	if _, err := s.exec(`UPDATE tasks SET timeline='[ ]' WHERE agent=? AND task_id='a'`, req.Manifest.Agent); err != nil {
		t.Fatal(err)
	}
	third, err := s.RelocateTaskEvidence(ctx, req)
	if err != nil || third.Rows[0].Before == second.Rows[0].Before {
		t.Fatal("raw persisted JSON was normalized in fingerprint", err)
	}
	var a, b taskEvidenceSnapshot
	a[0] = sql.NullString{String: string([]byte{0xff}), Valid: true}
	b[0] = sql.NullString{String: string([]byte{0xfe}), Valid: true}
	if a.digest() == b.digest() {
		t.Fatal("invalid UTF-8 distinct persisted bytes collapsed")
	}
}

func TestTaskEvidenceReferenceIdentityUsesOriginalLinks(t *testing.T) {
	var before taskEvidenceSnapshot
	before[14] = sql.NullString{String: `[{"kind":"evidence","label":"receipt","url":"/old/a"},{"kind":"evidence","label":"receipt","url":"/old/b"}]`, Valid: true}
	after, err := taskEvidenceChanged(before, []TaskEvidenceChange{{Field: "link", Kind: "evidence", Label: "receipt", OldURI: "/old/a", NewURI: "/old/b"}, {Field: "link", Kind: "evidence", Label: "receipt", OldURI: "/old/b", NewURI: "/new/c"}})
	if err != nil {
		t.Fatal("independent original identities became ambiguous after first replacement", err)
	}
	var links []TaskLink
	if err := json.Unmarshal([]byte(after[14].String), &links); err != nil || len(links) != 2 || links[0].URL != "/old/b" || links[1].URL != "/new/c" {
		t.Fatal("wrong original-link rewrite", err, links)
	}
	for _, bad := range []string{`[{"kind":"evidence","label":"receipt","url":"/old/a","url":"/old/b"}]`, `[{"kind":"evidence","label":"receipt","url":"/old/a","unknown":"cannot discard"}]`} {
		before[14].String = bad
		if _, err := taskEvidenceChanged(before, []TaskEvidenceChange{{Field: "link", Kind: "evidence", Label: "receipt", OldURI: "/old/a", NewURI: "/new/a"}}); err == nil {
			t.Fatal("ambiguous/lossy stored links accepted")
		}
	}
}

func TestTaskEvidenceReservedNamespaceIncludesFutureVersions(t *testing.T) {
	s, _, _ := taskEvidenceFixture(t)
	for _, key := range []string{taskEvidencePrefix + "fixture-lane:batch", "task-evidence-relocation:v2:future"} {
		if err := s.SetState(key, "forged"); err == nil {
			t.Fatal("generic write accepted reserved state")
		}
		if ok, err := s.CompareAndSwapState(key, "", false, "forged"); err == nil || ok {
			t.Fatal("generic CAS accepted reserved state")
		}
		if _, exists, err := s.GetState(key); err != nil || exists {
			t.Fatal("refused write left state", err)
		}
	}
}
