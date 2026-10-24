package routerstore

import (
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Same admitted database, independent real connection/pool; never reset the
// existing PostgreSQL schema when opening this peer.
func taskEvidencePeer(t *testing.T, s *SQLiteStore) *SQLiteStore {
	t.Helper()
	var peer *SQLiteStore
	var err error
	if dsn := pgTestDSN(); dsn != "" {
		peer, err = OpenPostgres(dsn)
	} else {
		peer, err = OpenPath(s.path)
	}
	if err != nil {
		t.Fatal(err)
	}
	peer.notifyDir = t.TempDir()
	t.Cleanup(func() { _ = peer.Close() })
	return peer
}

func taskEvidenceAssertMerged(t *testing.T, s *SQLiteStore, agent string, tokens, seconds int64) {
	t.Helper()
	got, err := s.GetTask(agent, "a")
	if err != nil {
		t.Fatal(err)
	}
	urls := map[string]bool{}
	for _, l := range got.Links {
		urls[l.URL] = true
	}
	if urls["file:///removed/a"] || !urls["/preserved/a"] || !urls["/ordinary/addition"] || got.Phase != "ordinary legacy metadata" || got.TokensConsumed != 42+tokens || got.DurationSeconds != 17+seconds || got.ResultRef != "file:///preserved/result-a" {
		t.Fatalf("stale links or lost/doubled update: %+v", got)
	}
}

func TestTaskEvidenceStaleOrdinaryUpdateRemergesCurrentLinks(t *testing.T) {
	s, ctx, req := taskEvidenceFixture(t)
	peer := taskEvidencePeer(t, s)
	req = taskEvidencePin(t, s, ctx, req)
	calls := 0
	afterTaskReadHook = func() {
		afterTaskReadHook = nil
		calls++
		if _, err := peer.RelocateTaskEvidence(ctx, req); err != nil {
			t.Fatal("interleaved relocation", err)
		}
	}
	t.Cleanup(func() { afterTaskReadHook = nil })
	// Same pre-existing request shape; no capability, flag, role or new caller
	// field. Only the successful internal attempt may add accounting.
	_, err := s.UpdateTask(req.Manifest.Agent, "a", TaskUpdate{Phase: "ordinary legacy metadata", Links: []TaskLink{{Kind: "evidence", Label: "ordinary addition", URL: "/ordinary/addition"}}, AddTokens: 7, AddSeconds: 9})
	if err != nil || calls != 1 {
		t.Fatal("legacy update failed instead of bounded remerge", err, calls)
	}
	taskEvidenceAssertMerged(t, s, req.Manifest.Agent, 7, 9)
	if taskEvidenceAuditCount(t, s) != 1 {
		t.Fatal("ordinary remerge rewrote relocation provenance")
	}
}

func TestTaskEvidenceLegacyHTTPUpdateAfterAndAcrossRelocation(t *testing.T) {
	s, ctx, req := taskEvidenceFixture(t)
	peer := taskEvidencePeer(t, s)
	req = taskEvidencePin(t, s, ctx, req)
	h, err := Handler(s, ServerOptions{Token: "fixture-bootstrap", RuleOfRa: "enforce"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	client := NewRemoteStore(srv.URL, "fixture-bootstrap")
	defer client.Close()
	client.host, client.agent, client.threadID, client.sessionDir = "fixture-host", req.Manifest.Agent, "fixture-thread", ""
	hookResult := make(chan error, 1)
	afterTaskReadHook = func() { afterTaskReadHook = nil; _, err := peer.RelocateTaskEvidence(ctx, req); hookResult <- err }
	t.Cleanup(func() { afterTaskReadHook = nil })
	_, err = client.UpdateTask(req.Manifest.Agent, "a", TaskUpdate{Phase: "ordinary legacy metadata", Links: []TaskLink{{Kind: "evidence", Label: "ordinary addition", URL: "/ordinary/addition"}}, AddTokens: 2, AddSeconds: 3})
	if err != nil {
		t.Fatal("unchanged old-client RPC rejected", err)
	}
	select {
	case hookErr := <-hookResult:
		if hookErr != nil {
			t.Fatal(hookErr)
		}
	default:
		t.Fatal("old-client interleaving did not run")
	}
	taskEvidenceAssertMerged(t, s, req.Manifest.Agent, 2, 3)
	// Ordinary updates remain accepted after metadata publication as well.
	if _, err := client.UpdateTask(req.Manifest.Agent, "a", TaskUpdate{Phase: "ordinary legacy metadata", AddTokens: 1}); err != nil {
		t.Fatal("post-relocation legacy request rejected", err)
	}
	taskEvidenceAssertMerged(t, s, req.Manifest.Agent, 3, 3)
}

func TestTaskEvidenceOrdinaryReferenceRemergeIsBounded(t *testing.T) {
	s, _, req := taskEvidenceFixture(t)
	peer := taskEvidencePeer(t, s)
	calls := 0
	afterTaskReadHook = func() {
		calls++
		// A deterministic competing link writer in an owned fixture. The test
		// does not change status or lease identity, so only reference CAS loses.
		value := `[{"kind":"evidence","label":"contended","url":"/contended/` + strings.Repeat("x", calls) + `"}]`
		if _, err := peer.exec(`UPDATE tasks SET links=? WHERE agent=? AND task_id='a'`, value, req.Manifest.Agent); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { afterTaskReadHook = nil })
	_, err := s.UpdateTask(req.Manifest.Agent, "a", TaskUpdate{AddTokens: 8, AddSeconds: 9})
	if !errors.Is(err, ErrConcurrentTaskUpdate) || calls != 4 {
		t.Fatal("reference retry not bounded", err, calls)
	}
	got, err := s.GetTask(req.Manifest.Agent, "a")
	if err != nil || got.TokensConsumed != 42 || got.DurationSeconds != 17 {
		t.Fatal("failed attempts incremented accounting", err, got)
	}
}

func TestTaskEvidenceOrdinaryUpdateKeepsStatusAndLeaseRefusals(t *testing.T) {
	for _, mode := range []string{"claim", "renew-expired-snapshot", "terminal", "relocate-then-claim"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, req := taskEvidenceFixture(t)
			peer := taskEvidencePeer(t, s)
			if _, err := s.exec(`UPDATE tasks SET blocked_by='' WHERE agent=? AND task_id='a'`, req.Manifest.Agent); err != nil {
				t.Fatal(err)
			}
			u := TaskUpdate{Phase: "must not land", AddTokens: 11}
			renewStartedAt := s.clock().Add(-2 * time.Second).UTC()
			switch mode {
			case "claim":
				if _, err := s.exec(`UPDATE tasks SET status='pending',lease_token='',lease_expires='',claimed_by='',thread_id='',attempts=0 WHERE agent=? AND task_id='a'`, req.Manifest.Agent); err != nil {
					t.Fatal(err)
				}
			case "renew-expired-snapshot":
				u.Status = "blocked"
				if _, err := s.exec(`UPDATE tasks SET lease_token='secret-expired-lease',lease_expires=?,claimed_by='fixture-worker',thread_id='fixture-thread' WHERE agent=? AND task_id='a'`, renewStartedAt.Add(time.Second).Format(time.RFC3339Nano), req.Manifest.Agent); err != nil {
					t.Fatal(err)
				}
			case "terminal":
				if _, err := s.exec(`UPDATE tasks SET lease_token='secret-expired-lease',lease_expires=?,claimed_by='fixture-worker',thread_id='fixture-thread' WHERE agent=? AND task_id='a'`, s.clock().Add(time.Hour).UTC().Format(time.RFC3339), req.Manifest.Agent); err != nil {
					t.Fatal(err)
				}
			case "relocate-then-claim":
				if _, err := s.exec(`UPDATE tasks SET attempts=0 WHERE agent=? AND task_id='a'`, req.Manifest.Agent); err != nil {
					t.Fatal(err)
				}
				req = taskEvidencePin(t, s, ctx, req)
			}
			var committed map[string]any
			afterTaskReadHook = func() {
				afterTaskReadHook = nil
				switch mode {
				case "claim", "relocate-then-claim":
					if mode == "relocate-then-claim" {
						if _, err := peer.RelocateTaskEvidence(ctx, req); err != nil {
							t.Fatal(err)
						}
					}
					if _, err := peer.ClaimTask(req.Manifest.Agent, "a", "new-worker", "fixture-thread", time.Minute); err != nil {
						t.Fatal(err)
					}
				case "renew-expired-snapshot":
					// Model a renewal whose fence timestamp was captured BEFORE the
					// old expiry, but whose SQL lands after the ordinary expired read.
					// This is the exact production RenewTaskLease UPDATE/fence, with
					// earlier-bound arguments; no clock rewind or new production hook.
					res, err := peer.db.Exec(`UPDATE tasks SET lease_expires=?,updated=? WHERE agent=? AND task_id=?`+taskLeaseFence, s.clock().Add(time.Hour).UTC().Format(time.RFC3339), s.clock().UTC().Format(time.RFC3339), req.Manifest.Agent, "a", "secret-expired-lease", renewStartedAt.Format(time.RFC3339Nano))
					if err != nil {
						t.Fatal(err)
					}
					n, err := res.RowsAffected()
					if err != nil || n != 1 {
						t.Fatal("renewal did not land", err, n)
					}
				case "terminal":
					if err := peer.CompleteTaskLease(req.Manifest.Agent, "a", "secret-expired-lease", "/terminal/proof"); err != nil {
						t.Fatal(err)
					}
				}
				committed = map[string]any{}
				for k, v := range taskEvidenceRaw(t, peer, req.Manifest.Agent, "a") {
					committed[k] = v
				}
			}
			t.Cleanup(func() { afterTaskReadHook = nil })
			if _, err := s.UpdateTask(req.Manifest.Agent, "a", u); !errors.Is(err, ErrConcurrentTaskUpdate) {
				t.Fatal("status/lease conflict was retried or cleared", err)
			}
			after := map[string]any{}
			for k, v := range taskEvidenceRaw(t, s, req.Manifest.Agent, "a") {
				after[k] = v
			}
			if committed == nil || !reflect.DeepEqual(committed, after) {
				t.Fatal("refused ordinary write mutated committed owner/terminal state")
			}
		})
	}
}

func TestTaskEvidenceRelocationRefusesHeldOrMalformedOwnership(t *testing.T) {
	for _, mode := range []string{"live", "expired", "missing-expiry", "bad-expiry", "missing-token", "missing-worker", "missing-thread", "expiry-without-token", "terminal-owner", "replay-live", "replay-expired"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, req := taskEvidenceFixture(t)
			req = taskEvidencePin(t, s, ctx, req)
			if mode == "replay-live" || mode == "replay-expired" {
				if _, err := s.RelocateTaskEvidence(ctx, req); err != nil {
					t.Fatal(err)
				}
			}
			token, expiry, worker, thread := "fixture-token", "2000-01-01T00:00:00Z", "fixture-worker", "fixture-thread"
			id := "a"
			switch mode {
			case "live", "replay-live":
				expiry = s.clock().Add(time.Hour).UTC().Format(time.RFC3339Nano)
			case "missing-expiry":
				expiry = ""
			case "bad-expiry":
				expiry = "not-a-time"
			case "missing-token":
				token = ""
			case "missing-worker":
				worker = ""
			case "missing-thread":
				thread = ""
			case "expiry-without-token":
				token, worker, thread = "", "", ""
			case "terminal-owner":
				id = "b"
			}
			if _, err := s.exec(`UPDATE tasks SET lease_token=?,lease_expires=?,claimed_by=?,thread_id=? WHERE agent=? AND task_id=?`, token, expiry, worker, thread, req.Manifest.Agent, id); err != nil {
				t.Fatal(err)
			}
			beforeA, beforeB := taskEvidenceRaw(t, s, req.Manifest.Agent, "a"), taskEvidenceRaw(t, s, req.Manifest.Agent, "b")
			count := taskEvidenceAuditCount(t, s)
			if _, err := s.RelocateTaskEvidence(ctx, req); err == nil {
				t.Fatal("held/malformed ownership accepted")
			}
			if !reflect.DeepEqual(beforeA, taskEvidenceRaw(t, s, req.Manifest.Agent, "a")) || !reflect.DeepEqual(beforeB, taskEvidenceRaw(t, s, req.Manifest.Agent, "b")) || taskEvidenceAuditCount(t, s) != count {
				t.Fatal("ownership refusal changed row/lease/provenance")
			}
		})
	}
}

func TestTaskEvidenceLeaseTransitionsBeforeRelocation(t *testing.T) {
	for _, mode := range []string{"claim", "renew", "complete"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, req := taskEvidenceFixture(t)
			peer := taskEvidencePeer(t, s)
			if mode == "claim" {
				if _, err := s.exec(`UPDATE tasks SET status='pending',blocked_by='',lease_token='',lease_expires='',claimed_by='',thread_id='',attempts=0 WHERE agent=? AND task_id='a'`, req.Manifest.Agent); err != nil {
					t.Fatal(err)
				}
			} else if _, err := s.exec(`UPDATE tasks SET lease_token='secret-expired-lease',lease_expires=?,claimed_by='fixture-worker',thread_id='fixture-thread' WHERE agent=? AND task_id='a'`, s.clock().Add(time.Hour).UTC().Format(time.RFC3339), req.Manifest.Agent); err != nil {
				t.Fatal(err)
			}
			req = taskEvidencePin(t, s, ctx, req)
			switch mode {
			case "claim":
				if _, err := peer.ClaimTask(req.Manifest.Agent, "a", "fixture-worker", "fixture-thread", time.Minute); err != nil {
					t.Fatal(err)
				}
			case "renew":
				if err := peer.RenewTaskLease(req.Manifest.Agent, "a", "secret-expired-lease", 20*time.Minute); err != nil {
					t.Fatal(err)
				}
			case "complete":
				if err := peer.CompleteTaskLease(req.Manifest.Agent, "a", "secret-expired-lease", "/terminal/proof"); err != nil {
					t.Fatal(err)
				}
			}
			before := taskEvidenceRaw(t, s, req.Manifest.Agent, "a")
			if _, err := s.RelocateTaskEvidence(ctx, req); err == nil {
				t.Fatal("claim/renewal/terminal change ignored")
			}
			if !reflect.DeepEqual(before, taskEvidenceRaw(t, s, req.Manifest.Agent, "a")) || taskEvidenceAuditCount(t, s) != 0 {
				t.Fatal("failed relocation changed new owner or terminal result")
			}
		})
	}
}

func TestTaskEvidenceDelayedPriorLiveCompletionCannotOverwriteRelocation(t *testing.T) {
	t.Run("retained-expired-custody", func(t *testing.T) {
		s, ctx, req := taskEvidenceFixture(t)
		peer := taskEvidencePeer(t, s)
		now := s.clock().UTC()
		capturedLiveTime := now.Add(-2 * time.Second)
		if _, err := s.exec(`UPDATE tasks SET lease_token='delayed-complete-fixture',lease_expires=?,claimed_by='fixture-worker',thread_id='fixture-thread' WHERE agent=? AND task_id='a'`, now.Add(-time.Second).Format(time.RFC3339Nano), req.Manifest.Agent); err != nil {
			t.Fatal(err)
		}
		req = taskEvidencePin(t, s, ctx, req)
		beforeA, beforeB := taskEvidenceRaw(t, s, req.Manifest.Agent, "a"), taskEvidenceRaw(t, s, req.Manifest.Agent, "b")
		if _, err := s.RelocateTaskEvidence(ctx, req); err == nil {
			t.Fatal("expired retained custody accepted before delayed completion")
		}
		if !reflect.DeepEqual(beforeA, taskEvidenceRaw(t, s, req.Manifest.Agent, "a")) || !reflect.DeepEqual(beforeB, taskEvidenceRaw(t, s, req.Manifest.Agent, "b")) || taskEvidenceAuditCount(t, s) != 0 {
			t.Fatal("refused maintenance changed row/custody/audit")
		}
		// Exact CompleteTaskLease/taskLeaseUpdate SQL with a live timestamp
		// captured before its possible SQL wait. It CAN still land after current
		// expiry. No accepted relocation exists for it to overwrite.
		res, err := peer.db.Exec(`UPDATE tasks SET status='done',result_ref=?,lease_token='',lease_expires='',claimed_by='',thread_id='',updated=? WHERE agent=? AND task_id=?`+taskLeaseFence, "/delayed/original-proof", capturedLiveTime.Format(time.RFC3339), req.Manifest.Agent, "a", "delayed-complete-fixture", capturedLiveTime.Format(time.RFC3339))
		if err != nil {
			t.Fatal(err)
		}
		n, err := res.RowsAffected()
		if err != nil || n != 1 {
			t.Fatal("prior-live completion control did not land", err, n)
		}
		after := taskEvidenceRaw(t, s, req.Manifest.Agent, "a")
		if after["status"].String != "done" || after["result_ref"].String != "/delayed/original-proof" || after["links"] != beforeA["links"] || taskEvidenceAuditCount(t, s) != 0 {
			t.Fatal("delayed completion overwrote accepted maintenance or rewrote links")
		}
	})
	t.Run("normal-unowned-positive", func(t *testing.T) {
		s, ctx, req := taskEvidenceFixture(t)
		req = taskEvidencePin(t, s, ctx, req)
		out, err := s.RelocateTaskEvidence(ctx, req)
		if err != nil || !out.Applied || taskEvidenceAuditCount(t, s) != 1 {
			t.Fatal("unowned relocation refused", err, out)
		}
		for _, id := range []string{"a", "b"} {
			row := taskEvidenceRaw(t, s, req.Manifest.Agent, id)
			for _, key := range []string{"lease_token", "lease_expires", "claimed_by", "thread_id"} {
				if row[key].String != "" {
					t.Fatal("unowned positive manufactured custody", key)
				}
			}
		}
	})
}
