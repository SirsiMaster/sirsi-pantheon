package routerstore

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func noticeFixture(t *testing.T) (*SQLiteStore, MailboxManifest, context.Context, context.Context) {
	t.Helper()
	s := openBackendStore(t, filepath.Join(t.TempDir(), "notice.db"))
	t.Cleanup(func() { _ = s.Close() })
	s.notifyDir = t.TempDir()
	it := Item{ID: "notice", From: "sender", To: "horus", Type: "decision", Title: "historical receipt", Instructions: "preserve original bytes", Result: "prior historical result", Status: StatusOpen}
	if err := s.Put(it); err != nil {
		t.Fatal(err)
	}
	m := MailboxManifest{ID: "finite-manifest", Actor: "sirsi-software-admin", Recipient: "horus", ApprovalRef: "owner-request", EvidenceRef: "reviewed-reconciliation-receipt", ExpiresAt: s.clock().Add(time.Hour), Notices: []MailboxNotice{{ID: it.ID, ExpectedSHA256: MailboxNoticeSHA256(it), Result: "Read and acknowledged; actual repair remains on its existing task."}}}
	actor := mailboxActor{Agent: m.Actor, Host: "test-host", SessionID: "real-session", ThreadID: "actual-thread", Runtime: "actual-binary", ServiceRecipient: true}
	m.EvidenceSHA256 = mailboxHash("reviewed fixture evidence")
	if err := s.UpsertThreads([]ThreadRecord{{ThreadID: actor.ThreadID, Agent: actor.Agent, Host: actor.Host, Status: "active", LastSeenAt: s.clock().Format(time.RFC3339Nano), Payload: json.RawMessage(`{"surface":"codex"}`)}}); err != nil {
		t.Fatal(err)
	}
	sess, err := s.MintSessionForThread(actor.Host, actor.Agent, actor.Runtime, actor.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	actor.SessionID = sess.ID
	user := context.WithValue(context.Background(), mailboxActorKey{}, actor)
	actor.Agent, actor.ThreadID, actor.Bootstrap = "operator", "operator-fixture", true
	if err := s.UpsertThreads([]ThreadRecord{{ThreadID: actor.ThreadID, Agent: actor.Agent, Host: actor.Host, Status: "active", LastSeenAt: s.clock().Format(time.RFC3339Nano), Payload: json.RawMessage(`{"surface":"codex"}`)}}); err != nil {
		t.Fatal(err)
	}
	sess, err = s.MintSessionForThread(actor.Host, actor.Agent, actor.Runtime, actor.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	actor.SessionID = sess.ID
	admin := context.WithValue(context.Background(), mailboxActorKey{}, actor)
	return s, m, admin, user
}

func approveNotice(t *testing.T, s *SQLiteStore, m MailboxManifest, admin context.Context) {
	t.Helper()
	if _, err := s.ReconcileMailboxNotice(admin, MailboxReconcileRequest{Action: "approve", Manifest: &m}); err != nil {
		t.Fatal(err)
	}
}

func TestMailboxReconciliationExactIdempotentHistory(t *testing.T) {
	s, m, admin, user := noticeFixture(t)
	if _, err := s.ReconcileMailboxNotice(user, MailboxReconcileRequest{Action: "approve", Manifest: &m}); err == nil {
		t.Fatal("lane self-approved foreign mailbox access")
	}
	approveNotice(t, s, m, admin)
	req := MailboxReconcileRequest{Action: "ack-notice", ManifestID: m.ID, NoticeID: "notice"}
	for i := 0; i < 2; i++ {
		out, err := s.ReconcileMailboxNotice(user, req)
		if err != nil || out.Repeated != (i == 1) {
			t.Fatalf("ack%d: %+v %v", i, out, err)
		}
	}
	got, err := s.Get("notice")
	if err != nil || got.Status != StatusClosed || got.Instructions != "preserve original bytes" || got.To != "horus" || got.From != "sender" {
		t.Fatalf("original identity/history changed: %+v %v", got, err)
	}
	raw, _, err := s.GetState(mailboxManifestPrefix + m.ID)
	if err != nil {
		t.Fatal(err)
	}
	var record mailboxManifestRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		t.Fatal(err)
	}
	if len(record.Audits) != 1 || record.Audits["notice"].OriginalResult != "prior historical result" || record.Audits["notice"].Actor.Agent != m.Actor {
		t.Fatalf("audited prior result or actual actor missing: %+v", record)
	}
	for _, id := range []string{"outside-manifest", "owner-decision"} {
		if _, err := s.ReconcileMailboxNotice(user, MailboxReconcileRequest{Action: "ack-notice", ManifestID: m.ID, NoticeID: id}); err == nil {
			t.Fatal("unapproved notice accepted")
		}
	}
}

func TestMailboxReconciliationRefusesChangesLeasesAndRollback(t *testing.T) {
	for _, mode := range []string{"changed", "leased", "expired", "audit-expiry"} {
		t.Run(mode, func(t *testing.T) {
			s, m, admin, user := noticeFixture(t)
			approveNotice(t, s, m, admin)
			before, _ := s.Get("notice")
			switch mode {
			case "changed":
				before.Title = "changed after approval"
				if err := s.Put(before); err != nil {
					t.Fatal(err)
				}
			case "leased":
				if _, err := s.exec(`UPDATE items SET lease_token='foreign-live-lease' WHERE id='notice'`); err != nil {
					t.Fatal(err)
				}
			case "expired":
				s.now = func() time.Time { return m.ExpiresAt.Add(time.Second) }
			case "audit-expiry":
				calls := 0
				s.now = func() time.Time {
					calls++
					if calls >= 3 {
						return m.ExpiresAt.Add(time.Second)
					}
					return m.ExpiresAt.Add(-time.Hour)
				}
			}
			if _, err := s.ReconcileMailboxNotice(user, MailboxReconcileRequest{Action: "ack-notice", ManifestID: m.ID, NoticeID: "notice"}); err == nil {
				t.Fatal("changed/leased/expired notice accepted")
			}
			got, err := s.Get("notice")
			if err != nil || got.Status != StatusOpen || got.Result != before.Result {
				t.Fatalf("failure committed partial close: %+v %v", got, err)
			}
		})
	}
}

func TestMailboxReconciliationAuthenticatedHTTPNoScopeWidening(t *testing.T) {
	s, m, _, _ := noticeFixture(t)
	h, err := Handler(s, ServerOptions{Token: "bootstrap-fixture", RuleOfRa: "off", RecipientRegistry: deliveryRegistry(t)})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	token, _, err := s.MintScopedHostToken("cloud-host", "fixture", []string{m.Actor})
	if err != nil {
		t.Fatal(err)
	}
	thread := ThreadRecord{ThreadID: "own-registered-thread", Agent: m.Actor, Host: "cloud-host", Status: "active", LastSeenAt: s.clock().Format(time.RFC3339Nano), Payload: json.RawMessage(`{"surface":"codex"}`)}
	if err := s.UpsertThreads([]ThreadRecord{thread}); err != nil {
		t.Fatal(err)
	}
	client := NewRemoteStore(srv.URL, token)
	client.host, client.agent, client.threadID, client.sessionDir = thread.Host, m.Actor, thread.ThreadID, ""
	if _, err := client.ReconcileMailboxNotice(context.Background(), MailboxReconcileRequest{Action: "approve", Manifest: &m}); err == nil {
		t.Fatal("scoped client provisioned its own initial approval")
	}
	opThread := ThreadRecord{ThreadID: "operator-thread", Agent: "operator", Host: "operator-host", Status: "active", LastSeenAt: s.clock().Format(time.RFC3339Nano), Payload: json.RawMessage(`{"surface":"codex"}`)}
	if err := s.UpsertThreads([]ThreadRecord{opThread}); err != nil {
		t.Fatal(err)
	}
	op := NewRemoteStore(srv.URL, "bootstrap-fixture")
	op.host, op.agent, op.threadID, op.sessionDir = opThread.Host, opThread.Agent, opThread.ThreadID, ""
	if _, err := op.ReconcileMailboxNotice(context.Background(), MailboxReconcileRequest{Action: "approve", Manifest: &m}); err != nil {
		t.Fatalf("registered bootstrap operator approval: %v", err)
	}
	if _, err := client.Get("notice"); err == nil {
		t.Fatal("foreign item readable before typed operation")
	}
	if _, err := client.ReconcileMailboxNotice(context.Background(), MailboxReconcileRequest{Action: "ack-notice", ManifestID: m.ID, NoticeID: "notice"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get("notice"); err == nil {
		t.Fatal("typed operation granted generic foreign read")
	}
	if err := client.CloseItem("notice", "not allowed"); err == nil {
		t.Fatal("typed operation granted generic foreign close")
	}
	if _, _, err := client.GetState(mailboxManifestPrefix + m.ID); err == nil {
		t.Fatal("scoped token read private audit state")
	}
	if _, err := client.ReconcileMailboxNotice(context.Background(), MailboxReconcileRequest{Action: "approve", Manifest: &m}); err == nil {
		t.Fatal("scoped client provisioned its own approval")
	}
	// The typed operation still enforces actual registration when the general
	// embedding gate is off. There is no synthetic service/owner identity.
	unregistered := NewRemoteStore(srv.URL, token)
	unregistered.host, unregistered.agent, unregistered.threadID, unregistered.sessionDir = thread.Host, m.Actor, "", ""
	if _, err := unregistered.ReconcileMailboxNotice(context.Background(), MailboxReconcileRequest{Action: "ack-notice", ManifestID: m.ID, NoticeID: "notice"}); err == nil {
		t.Fatal("unregistered actor bypassed mandatory typed gate")
	}
	thread.Payload = json.RawMessage(`{"surface":"worker","answer_role":false}`)
	if err := s.UpsertThreads([]ThreadRecord{thread}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReconcileMailboxNotice(context.Background(), MailboxReconcileRequest{Action: "ack-notice", ManifestID: m.ID, NoticeID: "notice"}); err == nil {
		t.Fatal("headless worker without answering role reconciled a notice")
	}
}

func TestMailboxReconciliationRefusesRevokedOrRetiredPrincipal(t *testing.T) {
	for _, mode := range []string{"revoked", "terminal", "no-answer-role"} {
		t.Run(mode, func(t *testing.T) {
			s, m, admin, user := noticeFixture(t)
			approveNotice(t, s, m, admin)
			a := user.Value(mailboxActorKey{}).(mailboxActor)
			switch mode {
			case "revoked":
				if err := s.RevokeSession(a.SessionID); err != nil {
					t.Fatal(err)
				}
			case "terminal":
				if _, err := s.exec(`UPDATE threads SET status='closed' WHERE thread_id=?`, a.ThreadID); err != nil {
					t.Fatal(err)
				}
			case "no-answer-role":
				if _, err := s.exec(`UPDATE threads SET payload=? WHERE thread_id=?`, `{"surface":"worker","answer_role":false}`, a.ThreadID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.ReconcileMailboxNotice(user, MailboxReconcileRequest{Action: "ack-notice", ManifestID: m.ID, NoticeID: "notice"}); err == nil {
				t.Fatal("changed principal authorized from stale handler context")
			}
			it, err := s.Get("notice")
			if err != nil || it.Status != StatusOpen {
				t.Fatalf("refusal changed notice: %+v %v", it, err)
			}
		})
	}
}

func TestMailboxManifestClosedScopeAndActualActor(t *testing.T) {
	s, m, admin, user := noticeFixture(t)
	for _, recipient := range []string{"owner", "user", "cylton", "ra", "another-service", "*"} {
		bad := m
		bad.Recipient = recipient
		if _, err := s.ReconcileMailboxNotice(admin, MailboxReconcileRequest{Action: "approve", Manifest: &bad}); err == nil {
			t.Fatalf("unsupported recipient %q authorized", recipient)
		}
	}
	if _, err := s.ReconcileMailboxNotice(context.Background(), MailboxReconcileRequest{Action: "approve", Manifest: &m}); err == nil {
		t.Fatal("missing server-derived actor authorized")
	}
	approveNotice(t, s, m, admin)
	other := user.Value(mailboxActorKey{}).(mailboxActor)
	other.Agent = "ra"
	foreign := context.WithValue(context.Background(), mailboxActorKey{}, other)
	if _, err := s.ReconcileMailboxNotice(foreign, MailboxReconcileRequest{Action: "ack-notice", ManifestID: m.ID, NoticeID: "notice"}); err == nil {
		t.Fatal("different actual actor consumed SSA approval")
	}
	bad := m
	bad.Notices = append(append([]MailboxNotice(nil), m.Notices...), m.Notices[0])
	if err := validateMailboxManifest(bad, s.clock()); err == nil {
		t.Fatal("duplicate finite notice accepted")
	}
}

func TestMailboxNoticeDigestPreservesExactLegacyBytes(t *testing.T) {
	a := Item{ID: "same", Instructions: string([]byte{0xff})}
	b := a
	b.Instructions = string([]byte{0xfe})
	if MailboxNoticeSHA256(a) == MailboxNoticeSHA256(b) {
		t.Fatal("distinct invalid UTF-8 legacy bodies collapsed into one digest")
	}
	a.From, a.To = "a", "bc"
	b = a
	b.From, b.To = "ab", "c"
	if MailboxNoticeSHA256(a) == MailboxNoticeSHA256(b) {
		t.Fatal("field boundaries are not bound by the digest")
	}
}
