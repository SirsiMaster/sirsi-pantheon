package routerstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const TaskResumeRequestLimit = 16 << 10
const taskResumePrefix = "task-resume:v1:"

var ErrTaskResumeInput = errors.New("routerstore: invalid task resume input")

// TaskResumeRequest attests a corrected cause, not permission to bypass an
// owner. Actor/thread/host are injected by the authenticated server, not input.
type TaskResumeRequest struct {
	Version         int    `json:"version"`
	Action          string `json:"action"`
	Agent           string `json:"agent"`
	TaskID          string `json:"task_id"`
	ExpectedVersion string `json:"expected_version"`
	EvidenceRef     string `json:"evidence_ref"`
	EvidenceSHA256  string `json:"evidence_sha256"`
	Reason          string `json:"reason"`
	TTLSeconds      int64  `json:"ttl_seconds"`
}

// Literal flat schema: no duplicate keys, case aliases, hidden fields or tails.
func (r *TaskResumeRequest) UnmarshalJSON(b []byte) error {
	if len(b) > TaskResumeRequestLimit || !utf8.Valid(b) {
		return errors.New("task resume: bounded UTF-8 request required")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("task resume: object required")
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return errors.New("task resume: duplicate/invalid key")
		}
		switch key {
		case "version", "action", "agent", "task_id", "expected_version", "evidence_ref", "evidence_sha256", "reason", "ttl_seconds":
		default:
			return fmt.Errorf("task resume: unknown key %q", key)
		}
		seen[key] = true
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return err
		}
	}
	if _, err := d.Token(); err != nil {
		return err
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return errors.New("task resume: trailing JSON")
	}
	type plain TaskResumeRequest
	return json.Unmarshal(b, (*plain)(r))
}

type TaskResumeResult struct {
	RowVersion string     `json:"row_version"`
	Status     string     `json:"status"`
	Attempts   int        `json:"attempts"`
	Claimed    bool       `json:"claimed"`
	Repeated   bool       `json:"repeated"`
	AuditKey   string     `json:"audit_key,omitempty"`
	Lease      *TaskLease `json:"lease,omitempty"`
}

// Fixed complete persisted schema; adding a task column requires updating this
// versioned snapshot too. NULL and empty remain distinct. Nothing derived enters.
const taskResumeSelect = `SELECT agent,task_id,subject,status,phase,responsible_party,blocked_by,created,updated,charter,commissioned_at,commissioned_by,outline,timeline,links,test_state,stage,tokens_consumed,duration_seconds,result_ref,lease_token,lease_expires,claimed_by,thread_id,attempts,failure_reason,idempotency_key,project_id,router_namespace FROM tasks`

type taskResumeSnapshot [29]sql.NullString

func (r taskResumeSnapshot) version() string {
	b, _ := json.Marshal(r)
	h := sha256.Sum256(append([]byte("task-resume-row-v1\x00"), b...))
	return hex.EncodeToString(h[:])
}

func taskResumeRead(ctx context.Context, tx taskEvidenceTx, agent, id string, pg bool) (taskResumeSnapshot, error) {
	var r taskResumeSnapshot
	args := make([]any, len(r))
	for i := range r {
		args[i] = &r[i]
	}
	lock := ""
	if pg {
		lock = " FOR UPDATE"
	}
	err := tx.QueryRowContext(ctx, taskResumeSelect+` WHERE agent=? AND task_id=?`+lock, agent, id).Scan(args...)
	return r, err
}

type taskResumeAudit struct {
	Request                         TaskResumeRequest `json:"request"`
	Actor                           taskEvidenceActor `json:"authenticated_actor"`
	BeforeVersion, AfterVersion     string
	AttemptsBefore, AttemptsAfter   int
	OldHolder, OldThread, OldExpiry string
	OldFailure, RecordedAt          string
	Qualification                   string
}

func ValidateTaskResumeRequest(r TaskResumeRequest) error {
	if r.Version != 1 || (r.Action != "inspect" && r.Action != "claim") || strings.TrimSpace(r.Agent) == "" || len(r.Agent) > 256 || strings.TrimSpace(r.TaskID) == "" || len(r.TaskID) > 256 {
		return fmt.Errorf("%w: exact v1 action/agent/task required", ErrTaskResumeInput)
	}
	if r.Action == "inspect" {
		if r.ExpectedVersion != "" || r.EvidenceRef != "" || r.EvidenceSHA256 != "" || r.Reason != "" || r.TTLSeconds != 0 {
			return fmt.Errorf("%w: inspect does not accept claim inputs", ErrTaskResumeInput)
		}
		return nil
	}
	if !taskEvidenceDigest(r.ExpectedVersion) || !taskEvidenceDigest(r.EvidenceSHA256) || strings.TrimSpace(r.EvidenceRef) == "" || len(r.EvidenceRef) > 4096 || strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 4096 || r.TTLSeconds <= 0 || r.TTLSeconds > int64(MaxLeaseTTL/time.Second) {
		return fmt.Errorf("%w: expected version, evidence ref/SHA256, reason and TTL in 1..%d seconds required", ErrTaskResumeInput, int64(MaxLeaseTTL/time.Second))
	}
	return nil
}

func taskResumeEligible(r taskResumeSnapshot, now time.Time) (int, error) {
	attempts, err := strconv.Atoi(r[24].String)
	if err != nil || attempts < MaxRetriesPerItem || attempts == int(^uint(0)>>1) {
		return 0, errors.New("task resume: only exhausted original attempts qualify")
	}
	if (r[3].String != "pending" && r[3].String != "blocked" && r[3].String != "in-progress") || r[6].String != "" {
		return 0, errors.New("task resume: terminal/dependency state refuses")
	}
	// Like an explicit ordinary ClaimTask, this is not an automatic wake:
	// responsible_party is preserved, not converted into a new permission gate.
	if r[20].String == "" && r[21].String == "" && r[22].String == "" && r[23].String == "" {
		return attempts, nil
	}
	expiry, err := time.Parse(time.RFC3339, r[21].String)
	if err != nil || r[20].String == "" || r[22].String == "" || r[23].String == "" || expiry.After(now) {
		return 0, errors.New("task resume: live or malformed retained custody refuses")
	}
	return attempts, nil
}

// ResumeTaskClaim grants one explicit history-preserving recovery attempt, not
// DONE. Shared transaction/principal primitives use real SQLite write locks or
// PG row locks and Context SQL throughout. Local calls have no fabricated actor.
func (s *SQLiteStore) ResumeTaskClaim(ctx context.Context, req TaskResumeRequest) (TaskResumeResult, error) {
	var out TaskResumeResult
	a, ok := ctx.Value(taskEvidenceActorKey{}).(taskEvidenceActor)
	if !ok || a.Agent == "" || a.Agent != req.Agent || a.ThreadID == "" || a.SessionID == "" || a.Host == "" || a.Runtime == "" {
		return out, errors.New("task resume: authenticated own-lane principal required")
	}
	if err := ValidateTaskResumeRequest(req); err != nil {
		return out, err
	}
	err := s.withTaskEvidenceTx(ctx, func(tx taskEvidenceTx) error {
		out = TaskResumeResult{}
		pg := s.dialect().name == "postgres"
		if err := taskEvidencePrincipal(ctx, tx, a, s.clock(), pg); err != nil {
			return err
		}
		before, err := taskResumeRead(ctx, tx, req.Agent, req.TaskID, pg)
		if err != nil {
			return err
		}
		attempts, err := strconv.Atoi(before[24].String)
		if err != nil {
			return err
		}
		out = TaskResumeResult{RowVersion: before.version(), Status: before[3].String, Attempts: attempts}
		if req.Action == "inspect" {
			return taskEvidencePrincipal(ctx, tx, a, s.clock(), pg)
		}
		identity := sha256.Sum256([]byte(req.Agent + "\x00" + req.TaskID))
		key := taskResumePrefix + hex.EncodeToString(identity[:]) + ":" + req.ExpectedVersion
		var encoded string
		err = tx.QueryRowContext(ctx, `SELECT value FROM state WHERE key=?`, key).Scan(&encoded)
		if err == nil {
			var audit taskResumeAudit
			if json.Unmarshal([]byte(encoded), &audit) != nil || audit.Request != req || audit.Actor.Agent != a.Agent || audit.Actor.Host != a.Host || audit.Actor.ThreadID != a.ThreadID || audit.BeforeVersion != req.ExpectedVersion || audit.AfterVersion != before.version() {
				return errors.New("task resume: changed request/after-state; no replay")
			}
			expires, err := time.Parse(time.RFC3339, before[21].String)
			if err != nil || !expires.After(s.clock()) || before[3].String != "in-progress" || before[20].String == "" || before[22].String != a.Agent || before[23].String != a.ThreadID {
				return errors.New("task resume: recorded lease is no longer live")
			}
			out.Claimed, out.Repeated, out.AuditKey = true, true, key
			out.Lease = &TaskLease{Agent: a.Agent, TaskID: req.TaskID, Worker: a.Agent, ThreadID: a.ThreadID, Token: before[20].String, Expires: expires, Attempt: attempts}
			return taskEvidencePrincipal(ctx, tx, a, s.clock(), pg)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if before.version() != req.ExpectedVersion {
			return errors.New("task resume: stale row version")
		}
		now := s.clock().UTC() // AFTER lock acquisition, not a pre-wait timestamp.
		attempts, err = taskResumeEligible(before, now)
		if err != nil {
			return err
		}
		token, err := newToken()
		if err != nil {
			return err
		}
		expires := now.Add(time.Duration(req.TTLSeconds) * time.Second)
		res, err := tx.ExecContext(ctx, `UPDATE tasks SET status='in-progress',claimed_by=?,thread_id=?,lease_token=?,lease_expires=?,attempts=attempts+1,updated=? WHERE agent=? AND task_id=? AND attempts=? AND status=? AND lease_token=? AND lease_expires=?`, a.Agent, a.ThreadID, token, expires.Format(time.RFC3339), now.Format(time.RFC3339), a.Agent, req.TaskID, attempts, before[3].String, before[20].String, before[21].String)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return errors.New("task resume: lost row CAS")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO lease_sessions(kind,key,session) VALUES('task',?,?) ON CONFLICT(kind,key) DO UPDATE SET session=excluded.session`, a.Agent+"/"+req.TaskID, a.SessionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE wake_events SET status='acked',ack_ref=?,lease_token='',lease_expires='',updated=? WHERE agent=? AND source_kind='ledger_task' AND source_id=? AND status IN ('pending','leased')`, "task-lease:"+a.Agent+":"+req.TaskID+":"+token, now.Format(time.RFC3339), a.Agent, req.TaskID); err != nil {
			return err
		}
		after, err := taskResumeRead(ctx, tx, a.Agent, req.TaskID, pg)
		if err != nil {
			return err
		}
		audit := taskResumeAudit{Request: req, Actor: a, BeforeVersion: before.version(), AfterVersion: after.version(), AttemptsBefore: attempts, AttemptsAfter: attempts + 1, OldHolder: before[22].String, OldThread: before[23].String, OldExpiry: before[21].String, OldFailure: before[25].String, RecordedAt: now.Format(time.RFC3339Nano), Qualification: "authenticated cause-fixed attestation, not service filesystem/hash/review verification"}
		b, err := json.Marshal(audit)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO state(key,value) VALUES(?,?)`, key, string(b)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_lease_log(ts,task_id,agent,op,session_id,thread_id,host,verdict,reason) VALUES(?,?,?,?,?,?,?,?,?)`, now.Format(AudienceTS), req.TaskID, a.Agent, "resume-claim", a.SessionID, a.ThreadID, a.Host, "allowed", key); err != nil {
			return err
		}
		if err := taskEvidencePrincipal(ctx, tx, a, s.clock(), pg); err != nil {
			return err
		}
		out = TaskResumeResult{RowVersion: after.version(), Status: "in-progress", Attempts: attempts + 1, Claimed: true, AuditKey: key, Lease: &TaskLease{Agent: a.Agent, TaskID: req.TaskID, Worker: a.Agent, ThreadID: a.ThreadID, Token: token, Expires: expires, Attempt: attempts + 1}}
		return nil
	})
	if err != nil {
		return TaskResumeResult{}, err
	}
	return out, nil
}
