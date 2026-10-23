package routerstore

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const mailboxManifestPrefix = "mailbox-reconciliation:v1:"

type MailboxNotice struct {
	ID             string `json:"id"`
	ExpectedSHA256 string `json:"expected_sha256"`
	Result         string `json:"result"`
}

// A manifest grants only coordination acknowledgments, never execution,
// completion of product tasks, generic foreign reads, or owner-item dismissal.
type MailboxManifest struct {
	ID             string          `json:"id"`
	Actor          string          `json:"actor"`
	Recipient      string          `json:"recipient"`
	ApprovalRef    string          `json:"approval_ref"`
	EvidenceRef    string          `json:"evidence_ref"`
	EvidenceSHA256 string          `json:"evidence_sha256"`
	ExpiresAt      time.Time       `json:"expires_at"`
	Notices        []MailboxNotice `json:"notices"`
}

type MailboxReconcileRequest struct {
	Action     string           `json:"action"`
	Manifest   *MailboxManifest `json:"manifest,omitempty"`
	ManifestID string           `json:"manifest_id,omitempty"`
	NoticeID   string           `json:"notice_id,omitempty"`
}

type MailboxReconcileResult struct {
	ManifestID string `json:"manifest_id"`
	NoticeID   string `json:"notice_id,omitempty"`
	Repeated   bool   `json:"repeated"`
}

type mailboxActorKey struct{}
type mailboxActor struct {
	Agent, Host, SessionID, ThreadID, Runtime string
	Bootstrap, ServiceRecipient               bool
	FreshFor                                  time.Duration
}

type mailboxNoticeAudit struct {
	Actor          mailboxActor `json:"actor"`
	OriginalSHA256 string       `json:"original_sha256"`
	OriginalResult string       `json:"original_result"`
	ClosedSHA256   string       `json:"closed_sha256"`
	ClosedAt       string       `json:"closed_at"`
}

type mailboxManifestRecord struct {
	Manifest MailboxManifest               `json:"manifest"`
	Approved mailboxActor                  `json:"approved"`
	Audits   map[string]mailboxNoticeAudit `json:"audits"`
}

func mailboxHash(v any) string {
	b, _ := json.Marshal(v) // Only closed structs with JSON-native fields.
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// MailboxNoticeSHA256 binds all retained source fields, including prior result
// and delivery history. A changed row requires a newly reviewed manifest.
func MailboxNoticeSHA256(item Item) string {
	h := sha256.New()
	_, _ = h.Write([]byte("sirsi.mailbox.item.v1\x00"))
	// Length-prefix exact string bytes. JSON alone would normalize distinct
	// invalid UTF-8 legacy strings into the same replacement-character bytes.
	for _, field := range []string{item.ID, item.From, item.To, item.Title, item.Type, item.Status, item.Opened, item.Closed, item.Instructions, item.Result, item.BlockedBy, item.WakeStatus, item.WakeAttemptedAt, item.WakeAdapter, item.WakeError, item.AckedAt} {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		_, _ = h.Write(length[:])
		_, _ = h.Write([]byte(field))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func validateMailboxManifest(m MailboxManifest, now time.Time) error {
	if m.ID == "" || len(m.ID) > 100 || m.Actor == "" || m.Actor == m.Recipient || m.Recipient != "horus" || m.ApprovalRef == "" || m.EvidenceRef == "" || len(m.Notices) < 1 || len(m.Notices) > 64 {
		return errors.New("mailbox reconciliation: finite Horus notice manifest and evidence required")
	}
	evidence, err := hex.DecodeString(m.EvidenceSHA256)
	if err != nil || len(evidence) != 32 || m.EvidenceSHA256 != strings.ToLower(m.EvidenceSHA256) {
		return errors.New("mailbox reconciliation: exact reviewed evidence digest required")
	}
	// Version one is deliberately confined to the owner-requested actor-only
	// service. General service delegation needs its own explicit policy change.
	if !m.ExpiresAt.After(now) || m.ExpiresAt.After(now.Add(24*time.Hour)) {
		return errors.New("mailbox reconciliation: approval must expire within 24 hours")
	}
	seen := map[string]bool{}
	for _, n := range m.Notices {
		h, err := hex.DecodeString(n.ExpectedSHA256)
		if n.ID == "" || seen[n.ID] || err != nil || len(h) != 32 || n.ExpectedSHA256 != strings.ToLower(n.ExpectedSHA256) || strings.TrimSpace(n.Result) == "" || len(n.Result) > 8192 {
			return errors.New("mailbox reconciliation: unique exact notice digest and bounded result required")
		}
		seen[n.ID] = true
	}
	b, err := json.Marshal(m)
	if err != nil || len(b) > 128<<10 {
		return errors.New("mailbox reconciliation: manifest exceeds bound")
	}
	return nil
}

// ReconcileMailboxNotice receives its principal ONLY through server-created
// context. Approvals require the existing bootstrap operator, not a lane token.
// It uses the current schema's private state records; no schema or owner-token
// compatibility change is needed. No ordinary CloseItem permission is widened.
func (s *SQLiteStore) ReconcileMailboxNotice(ctx context.Context, req MailboxReconcileRequest) (MailboxReconcileResult, error) {
	a, ok := ctx.Value(mailboxActorKey{}).(mailboxActor)
	if !ok || !a.ServiceRecipient || a.Agent == "" || a.Host == "" || a.SessionID == "" || a.ThreadID == "" || a.Runtime == "" {
		return MailboxReconcileResult{}, errors.New("mailbox reconciliation: authenticated registered principal required")
	}
	if err := ctx.Err(); err != nil {
		return MailboxReconcileResult{}, err
	}
	var out MailboxReconcileResult
	err := s.withTx(func(tx *txHandle) error {
		now := s.clock()
		if err := mailboxPrincipalLive(tx, a, now, s.dialect().name == "postgres"); err != nil {
			return err
		}
		if req.Action == "approve" {
			if !a.Bootstrap || !a.ServiceRecipient || req.Manifest == nil || req.ManifestID != "" || req.NoticeID != "" {
				return errors.New("mailbox reconciliation: bootstrap approval of actor-only service required")
			}
			m := *req.Manifest
			if err := validateMailboxManifest(m, now); err != nil {
				return err
			}
			key := mailboxManifestPrefix + m.ID
			state := maatTransactionState{tx}
			old, exists, err := state.GetState(key)
			if err != nil {
				return err
			}
			if exists {
				var r mailboxManifestRecord
				if json.Unmarshal([]byte(old), &r) != nil || mailboxHash(r.Manifest) != mailboxHash(m) {
					return errors.New("mailbox reconciliation: immutable approval id collision")
				}
				out = MailboxReconcileResult{ManifestID: m.ID, Repeated: true}
				return nil
			}
			raw, err := json.Marshal(mailboxManifestRecord{Manifest: m, Approved: a, Audits: map[string]mailboxNoticeAudit{}})
			if err != nil {
				return err
			}
			res, err := tx.Exec(`INSERT INTO state(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING`, key, string(raw))
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil || n != 1 {
				return errors.New("mailbox reconciliation: concurrent approval; read back before retry")
			}
			out.ManifestID = m.ID
			return nil
		}
		if req.Action != "ack-notice" || req.Manifest != nil || req.ManifestID == "" || req.NoticeID == "" {
			return errors.New("mailbox reconciliation: exact approved notice required")
		}
		key := mailboxManifestPrefix + req.ManifestID
		query := `SELECT value FROM state WHERE key=?`
		if s.dialect().name == "postgres" {
			query += ` FOR UPDATE`
		}
		var raw string
		if err := tx.QueryRowContext(ctx, query, key).Scan(&raw); err != nil {
			return errors.New("mailbox reconciliation: no applicable approval")
		}
		now = s.clock() // Never authorize using a timestamp taken before lock wait.
		var record mailboxManifestRecord
		if json.Unmarshal([]byte(raw), &record) != nil || record.Audits == nil || record.Manifest.ID != req.ManifestID || record.Manifest.Actor != a.Agent || validateMailboxManifest(record.Manifest, now) != nil {
			return errors.New("mailbox reconciliation: approval is invalid, expired or belongs to another actor")
		}
		var approved *MailboxNotice
		for i := range record.Manifest.Notices {
			if record.Manifest.Notices[i].ID == req.NoticeID {
				approved = &record.Manifest.Notices[i]
				break
			}
		}
		if approved == nil {
			return errors.New("mailbox reconciliation: notice is outside the approved finite set")
		}
		it, err := scanItem(tx.QueryRowContext(ctx, `SELECT `+itemCols+` FROM items WHERE id=?`, req.NoticeID).Scan)
		if err != nil {
			return errors.New("mailbox reconciliation: approved notice is unavailable")
		}
		if audit, repeated := record.Audits[it.ID]; repeated {
			if it.Status != StatusClosed || MailboxNoticeSHA256(it) != audit.ClosedSHA256 {
				return errors.New("mailbox reconciliation: audited notice changed; no replay")
			}
			out = MailboxReconcileResult{ManifestID: req.ManifestID, NoticeID: it.ID, Repeated: true}
			return nil
		}
		if it.To != record.Manifest.Recipient || it.Type != "decision" || it.Status != StatusOpen || MailboxNoticeSHA256(it) != approved.ExpectedSHA256 {
			return errors.New("mailbox reconciliation: notice changed or is not an open coordination decision")
		}
		closed := now.UTC().Format(time.RFC3339Nano)
		result := fmt.Sprintf("Coordination acknowledgment by %s; manifest %s; evidence %s SHA256 %s. Underlying product/service tasks are not completed by this notice.\n\n%s", a.Agent, req.ManifestID, record.Manifest.EvidenceRef, record.Manifest.EvidenceSHA256, approved.Result)
		res, err := tx.ExecContext(ctx, `UPDATE items SET status='closed',closed=?,result=? WHERE id=? AND to_agent=? AND status='open' AND lease_token='' AND instructions=? AND result=? AND acked_at=? AND wake_status=? AND wake_attempted_at=? AND wake_adapter=? AND wake_error=? AND blocked_by=? AND title=? AND type=? AND from_agent=? AND opened=? AND closed=?`, closed, result, it.ID, it.To, it.Instructions, it.Result, it.AckedAt, it.WakeStatus, it.WakeAttemptedAt, it.WakeAdapter, it.WakeError, it.BlockedBy, it.Title, it.Type, it.From, it.Opened, it.Closed)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return errors.New("mailbox reconciliation: notice leased or changed concurrently")
		}
		before := it
		it.Status, it.Closed, it.Result = StatusClosed, closed, result
		record.Audits[it.ID] = mailboxNoticeAudit{Actor: a, OriginalSHA256: MailboxNoticeSHA256(before), OriginalResult: before.Result, ClosedSHA256: MailboxNoticeSHA256(it), ClosedAt: closed}
		next, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if !s.clock().Before(record.Manifest.ExpiresAt) {
			return errors.New("mailbox reconciliation: approval expired before audit publication")
		}
		res, err = tx.ExecContext(ctx, `UPDATE state SET value=? WHERE key=? AND value=?`, string(next), key, raw)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return errors.New("mailbox reconciliation: audit changed concurrently")
		}
		out = MailboxReconcileResult{ManifestID: req.ManifestID, NoticeID: it.ID}
		return nil
	})
	return out, err
}

// Revalidate principal ownership in the transaction which publishes the audit.
func mailboxPrincipalLive(tx *txHandle, a mailboxActor, now time.Time, postgres bool) error {
	lock := ""
	if postgres {
		lock = " FOR SHARE"
	}
	var host, agent, runtime, thread, revoked string
	if err := tx.QueryRow(`SELECT host,agent,runtime_hash,thread_id,revoked FROM sessions WHERE session_id=?`+lock, a.SessionID).Scan(&host, &agent, &runtime, &thread, &revoked); err != nil || revoked != "" || host != a.Host || agent != a.Agent || runtime != a.Runtime || thread != a.ThreadID {
		return errors.New("mailbox reconciliation: authenticated session changed or revoked")
	}
	var status, last, payload string
	if err := tx.QueryRow(`SELECT host,agent,status,last_seen_at,payload FROM threads WHERE thread_id=?`+lock, a.ThreadID).Scan(&host, &agent, &status, &last, &payload); err != nil || host != a.Host || agent != a.Agent || (status != "active" && status != "idle" && status != "blocked") {
		return errors.New("mailbox reconciliation: principal thread is no longer live")
	}
	seen, err := time.Parse(time.RFC3339Nano, last)
	fresh := a.FreshFor
	if fresh <= 0 {
		fresh = ruleOfRaDefaultStale
	}
	if err != nil || seen.After(now.Add(5*time.Second)) || now.Sub(seen) > fresh {
		return errors.New("mailbox reconciliation: principal heartbeat is not fresh")
	}
	var role struct {
		Surface    string `json:"surface"`
		AnswerRole bool   `json:"answer_role"`
	}
	if json.Unmarshal([]byte(payload), &role) != nil || role.Surface == "" || (role.Surface == "worker" && !role.AnswerRole) {
		return ErrWorkerAnswers
	}
	return nil
}
