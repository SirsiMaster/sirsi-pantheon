package routerstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const taskEvidencePrefix = "task-evidence-relocation:v1:"
const TaskEvidenceManifestLimit = 128 << 10

type TaskEvidenceChange struct {
	Field  string `json:"field"` // link or result_ref, never an arbitrary column
	Kind   string `json:"kind,omitempty"`
	Label  string `json:"label,omitempty"`
	OldURI string `json:"old_uri"`
	NewURI string `json:"new_uri"`
	SHA256 string `json:"sha256"`
}

type TaskEvidenceRow struct {
	TaskID         string               `json:"task_id"`
	ExpectedSHA256 string               `json:"expected_sha256"`
	Changes        []TaskEvidenceChange `json:"changes"`
}

// AttestationRef identifies external review/process evidence, not an approval
// capability. The server cannot verify the client's filesystem or artifact bytes.
type TaskEvidenceManifest struct {
	Version               int               `json:"version"`
	ID                    string            `json:"id"`
	Agent                 string            `json:"agent"`
	AttestationRef        string            `json:"attestation_ref"`
	PreservationMapSHA256 string            `json:"preservation_map_sha256"`
	Tasks                 []TaskEvidenceRow `json:"tasks"`
}

type TaskEvidenceRequest struct {
	Action   string               `json:"action"` // inspect or apply
	Manifest TaskEvidenceManifest `json:"manifest"`
}

type TaskEvidenceFingerprint struct {
	TaskID string `json:"task_id"`
	Before string `json:"before_sha256"`
	After  string `json:"after_sha256"`
}

type TaskEvidenceResult struct {
	ManifestSHA256 string                    `json:"manifest_sha256"`
	Repeated       bool                      `json:"repeated"`
	Applied        bool                      `json:"applied"`
	Rows           []TaskEvidenceFingerprint `json:"rows"`
}

// Strict at the RPC boundary too, not merely when the friendly CLI decodes it.
func (r *TaskEvidenceRequest) UnmarshalJSON(b []byte) error {
	if len(b) > TaskEvidenceManifestLimit {
		return errors.New("task evidence: request exceeds 128 KiB")
	}
	if !utf8.Valid(b) {
		return errors.New("task evidence: invalid request encoding")
	}
	if err := taskEvidenceExactJSON(json.NewDecoder(bytes.NewReader(b)), taskEvidenceRequestJSON, 0); err != nil {
		return err
	}
	type plain TaskEvidenceRequest
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode((*plain)(r)); err != nil {
		return err
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return errors.New("task evidence: trailing JSON")
	}
	return nil
}

type taskEvidenceJSONSchema uint8

const (
	taskEvidenceScalarJSON taskEvidenceJSONSchema = iota
	taskEvidenceStringJSON
	taskEvidenceRequestJSON
	taskEvidenceManifestJSON
	taskEvidenceTasksJSON
	taskEvidenceTaskJSON
	taskEvidenceChangesJSON
	taskEvidenceChangeJSON
	taskEvidenceLinksJSON
	taskEvidenceLinkJSON
)

// Literal keys, not casing heuristics: encoding/json's case-insensitive struct
// matching must never accept aliases or collapse ambiguous stored history.
func taskEvidenceChildJSON(schema taskEvidenceJSONSchema, key string) (taskEvidenceJSONSchema, bool) {
	switch schema {
	case taskEvidenceRequestJSON:
		switch key {
		case "action":
			return taskEvidenceScalarJSON, true
		case "manifest":
			return taskEvidenceManifestJSON, true
		}
	case taskEvidenceManifestJSON:
		switch key {
		case "version", "id", "agent", "attestation_ref", "preservation_map_sha256":
			return taskEvidenceScalarJSON, true
		case "tasks":
			return taskEvidenceTasksJSON, true
		}
	case taskEvidenceTaskJSON:
		switch key {
		case "task_id", "expected_sha256":
			return taskEvidenceScalarJSON, true
		case "changes":
			return taskEvidenceChangesJSON, true
		}
	case taskEvidenceChangeJSON:
		switch key {
		case "field", "kind", "label", "old_uri", "new_uri", "sha256":
			return taskEvidenceScalarJSON, true
		}
	case taskEvidenceLinkJSON:
		switch key {
		case "kind", "label", "url":
			return taskEvidenceStringJSON, true
		}
	}
	return 0, false
}

func taskEvidenceExactJSON(d *json.Decoder, schema taskEvidenceJSONSchema, depth int) error {
	if depth > 16 {
		return errors.New("task evidence: JSON nesting exceeds bound")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	if schema == taskEvidenceStringJSON {
		if _, ok := t.(string); !ok {
			return errors.New("task evidence: stored link fields must be strings")
		}
		return nil
	}
	delim, composite := t.(json.Delim)
	if schema == taskEvidenceScalarJSON && !composite {
		return nil
	}
	// Legacy null links contain no keys; result-only edits retain their bytes.
	if schema == taskEvidenceLinksJSON && t == nil {
		return nil
	}
	want, element := json.Delim('{'), taskEvidenceScalarJSON
	switch schema {
	case taskEvidenceTasksJSON:
		want, element = '[', taskEvidenceTaskJSON
	case taskEvidenceChangesJSON:
		want, element = '[', taskEvidenceChangeJSON
	case taskEvidenceLinksJSON:
		want, element = '[', taskEvidenceLinkJSON
	case taskEvidenceScalarJSON:
		return errors.New("task evidence: scalar JSON value required")
	}
	if !composite || delim != want {
		return errors.New("task evidence: invalid JSON schema shape")
	}
	seen := map[string]bool{}
	for d.More() {
		child := element
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("task evidence: duplicate/invalid JSON key")
			}
			seen[name] = true
			child, ok = taskEvidenceChildJSON(schema, name)
			if !ok {
				return errors.New("task evidence: unknown/exact-case JSON key required")
			}
		}
		if err := taskEvidenceExactJSON(d, child, depth+1); err != nil {
			return err
		}
	}
	end, err := d.Token()
	if err != nil {
		return err
	}
	close := json.Delim('}')
	if delim == '[' {
		close = ']'
	}
	if end != close {
		return errors.New("task evidence: invalid JSON closing delimiter")
	}
	if schema == taskEvidenceLinkJSON && len(seen) != 3 {
		return errors.New("task evidence: complete stored link fields required")
	}
	return nil
}

var taskEvidenceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

func taskEvidenceDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && s == strings.ToLower(s)
}

// TaskEvidenceLocalPath decodes only absolute local paths/file URIs. It does not
// touch the filesystem and never follows, fetches, or normalizes a URI in a row.
func TaskEvidenceLocalPath(s string) (string, error) {
	p := s
	if strings.HasPrefix(s, "file:") {
		u, err := url.Parse(s)
		if err != nil || u.Scheme != "file" || u.Host != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
			return "", errors.New("task evidence: plain absolute local file URI required")
		}
		p = u.Path
	}
	if !strings.HasPrefix(p, "/") || path.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n") || len(p) > 4096 {
		return "", errors.New("task evidence: canonical absolute local path required")
	}
	return p, nil
}

func ValidateTaskEvidenceRequest(r TaskEvidenceRequest) error {
	m := r.Manifest
	if (r.Action != "inspect" && r.Action != "apply") || m.Version != 1 || !taskEvidenceID.MatchString(m.ID) || !taskEvidenceID.MatchString(m.Agent) || strings.TrimSpace(m.AttestationRef) == "" || len(m.AttestationRef) > 4096 || !taskEvidenceDigest(m.PreservationMapSHA256) || len(m.Tasks) < 1 || len(m.Tasks) > 64 {
		return errors.New("task evidence: bounded v1 own-row manifest required")
	}
	b, err := json.Marshal(r)
	if err != nil || len(b) > TaskEvidenceManifestLimit {
		return errors.New("task evidence: oversized manifest")
	}
	seen, count := map[string]bool{}, 0
	for _, row := range m.Tasks {
		if row.TaskID == "" || len(row.TaskID) > 256 || seen[row.TaskID] || (row.ExpectedSHA256 != "" && !taskEvidenceDigest(row.ExpectedSHA256)) || (r.Action == "apply" && row.ExpectedSHA256 == "") || len(row.Changes) == 0 {
			return errors.New("task evidence: unique tasks and exact expected digests required")
		}
		seen[row.TaskID] = true
		fields := map[string]bool{}
		for _, c := range row.Changes {
			count++
			identity := c.Field + "\x00" + c.Kind + "\x00" + c.Label + "\x00" + c.OldURI
			if count > 128 || fields[identity] || c.OldURI == c.NewURI || !taskEvidenceDigest(c.SHA256) || (c.Field != "link" && c.Field != "result_ref") || (c.Field == "link" && (!validLinkKind(c.Kind) || strings.TrimSpace(c.Label) == "")) || (c.Field == "result_ref" && (c.Kind != "" || c.Label != "" || fields["result_ref"])) {
				return errors.New("task evidence: duplicate/unsupported reference change")
			}
			fields[identity] = true
			if c.Field == "result_ref" {
				fields["result_ref"] = true
			}
			if _, err := TaskEvidenceLocalPath(c.OldURI); err != nil {
				return err
			}
			if _, err := TaskEvidenceLocalPath(c.NewURI); err != nil {
				return err
			}
		}
	}
	return nil
}

type taskEvidenceActorKey struct{}
type taskEvidenceActor struct {
	Agent, Host, SessionID, ThreadID, Runtime string
	FreshFor                                  time.Duration
}

type taskEvidenceAudit struct {
	Manifest      TaskEvidenceManifest `json:"manifest"`
	Actor         taskEvidenceActor    `json:"authenticated_actor"`
	At            string               `json:"recorded_at"`
	Result        TaskEvidenceResult   `json:"result"`
	Qualification string               `json:"qualification"`
}

type taskEvidenceTx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// A real SQLite write lock precedes the snapshot; do not use the misleadingly
// named beginImmediate() (a deferred database/sql transaction). PG uses row locks.
func (s *SQLiteStore) withTaskEvidenceTx(ctx context.Context, fn func(taskEvidenceTx) error) error {
	return s.retryWrite(func() error {
		if s.dialect().name == "postgres" {
			tx, err := s.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer func() { _ = tx.Rollback() }()
			if err := fn(tx); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			return tx.Commit()
		}
		conn, err := s.db.Conn(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()
		if err := beginImmediateWithRetry(ctx, conn); err != nil {
			return err
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, _ = conn.ExecContext(cleanup, "ROLLBACK")
		}()
		if err := fn(conn); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, "COMMIT")
		return err
	})
}

func taskEvidencePrincipal(ctx context.Context, tx taskEvidenceTx, a taskEvidenceActor, now time.Time, pg bool) error {
	lock := ""
	if pg {
		lock = " FOR SHARE"
	}
	var host, agent, runtime, thread, revoked string
	if err := tx.QueryRowContext(ctx, `SELECT host,agent,runtime_hash,thread_id,revoked FROM sessions WHERE session_id=?`+lock, a.SessionID).Scan(&host, &agent, &runtime, &thread, &revoked); err != nil || revoked != "" || host != a.Host || agent != a.Agent || runtime != a.Runtime || thread != a.ThreadID {
		return errors.New("task evidence: authenticated session changed/revoked")
	}
	var status, last string
	if err := tx.QueryRowContext(ctx, `SELECT host,agent,status,last_seen_at FROM threads WHERE thread_id=?`+lock, a.ThreadID).Scan(&host, &agent, &status, &last); err != nil || host != a.Host || agent != a.Agent || (status != "active" && status != "idle" && status != "blocked") {
		return errors.New("task evidence: own principal is not live")
	}
	seen, err := time.Parse(time.RFC3339Nano, last)
	fresh := a.FreshFor
	if fresh <= 0 {
		fresh = ruleOfRaDefaultStale
	}
	if err != nil || seen.After(now.Add(5*time.Second)) || now.Sub(seen) > fresh {
		return errors.New("task evidence: stale principal")
	}
	return nil // Metadata maintenance is not a worker answering-role grant.
}

// Same exact public persisted columns as taskSelect; nullable/raw JSON storage is
// retained. No liveness or hidden lease/capability column enters the digest/API.
type taskEvidenceSnapshot [20]sql.NullString

func (r taskEvidenceSnapshot) digest() string {
	h := sha256.New()
	_, _ = h.Write([]byte("sirsi.task.public-persisted.v1\x00"))
	for _, f := range r {
		present := byte(0)
		if f.Valid {
			present = 1
		}
		_, _ = h.Write([]byte{present})
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(f.String)))
		_, _ = h.Write(n[:])
		_, _ = h.Write([]byte(f.String))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func taskEvidenceRead(ctx context.Context, tx taskEvidenceTx, agent, id string, pg bool) (taskEvidenceSnapshot, error) {
	var r taskEvidenceSnapshot
	args := make([]any, len(r))
	for i := range r {
		args[i] = &r[i]
	}
	lock := ""
	if pg {
		lock = " FOR UPDATE"
	}
	err := tx.QueryRowContext(ctx, taskSelect+` WHERE agent=? AND task_id=?`+lock, agent, id).Scan(args...)
	return r, err
}

// Called only after this transaction holds the task row's write lock. Private
// ownership is never exposed, repaired or cleared by metadata relocation.
func taskEvidenceUnheld(ctx context.Context, tx taskEvidenceTx, agent, id string) error {
	var token, expiry, worker, thread string
	if err := tx.QueryRowContext(ctx, `SELECT lease_token,lease_expires,claimed_by,thread_id FROM tasks WHERE agent=? AND task_id=?`, agent, id).Scan(&token, &expiry, &worker, &thread); err != nil {
		return err
	}
	if token == "" && expiry == "" && worker == "" && thread == "" {
		return nil
	}
	// Expiry is not proof of quiescence: a lease writer can have captured its
	// live fence timestamp before waiting on SQL. Never race that retained
	// custody, and never clear/reclaim it in this metadata-only operation.
	return errors.New("task evidence: retained task ownership; normal custody reconciliation required before relocation")
}

func taskEvidenceChanged(before taskEvidenceSnapshot, changes []TaskEvidenceChange) (taskEvidenceSnapshot, error) {
	after := before
	var links []TaskLink
	if !utf8.ValidString(before[14].String) {
		return after, errors.New("task evidence: invalid stored link encoding")
	}
	if err := taskEvidenceExactJSON(json.NewDecoder(strings.NewReader(before[14].String)), taskEvidenceLinksJSON, 0); err != nil {
		return after, err
	}
	d := json.NewDecoder(strings.NewReader(before[14].String))
	d.DisallowUnknownFields()
	if err := d.Decode(&links); err != nil {
		return after, err
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return after, errors.New("task evidence: invalid stored links")
	}
	linkChange := false
	original := append([]TaskLink(nil), links...)
	changed := map[int]bool{}
	for _, c := range changes {
		if c.Field == "result_ref" {
			if before[19].String != c.OldURI {
				return after, errors.New("task evidence: result reference changed")
			}
			after[19].String = c.NewURI
			continue
		}
		found := -1
		for i, l := range original {
			if l.Kind == c.Kind && l.Label == c.Label && l.URL == c.OldURI {
				if found >= 0 {
					return after, errors.New("task evidence: ambiguous link identity")
				}
				found = i
			}
		}
		if found < 0 || changed[found] {
			return after, errors.New("task evidence: link changed/unmatched")
		}
		changed[found] = true
		links[found].URL = c.NewURI
		linkChange = true
	}
	if linkChange {
		b, err := json.Marshal(links)
		if err != nil {
			return after, err
		}
		after[14].String = string(b)
	}
	return after, nil
}

// RelocateTaskEvidence cannot be invoked as an arbitrary local repair: only the
// server injects its principal. Its audit is an actor attestation, not filesystem
// verification or a server-certified independent review.
func (s *SQLiteStore) RelocateTaskEvidence(ctx context.Context, req TaskEvidenceRequest) (TaskEvidenceResult, error) {
	var out TaskEvidenceResult
	a, ok := ctx.Value(taskEvidenceActorKey{}).(taskEvidenceActor)
	if !ok || a.Agent == "" || a.Host == "" || a.SessionID == "" || a.ThreadID == "" || a.Runtime == "" || req.Manifest.Agent != a.Agent {
		return out, errors.New("task evidence: authenticated own-lane principal required")
	}
	if err := ValidateTaskEvidenceRequest(req); err != nil {
		return out, err
	}
	b, _ := json.Marshal(req.Manifest)
	sum := sha256.Sum256(b)
	out.ManifestSHA256 = hex.EncodeToString(sum[:])
	rows := append([]TaskEvidenceRow(nil), req.Manifest.Tasks...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].TaskID < rows[j].TaskID })
	err := s.withTaskEvidenceTx(ctx, func(tx taskEvidenceTx) error {
		out.Rows = nil
		out.Applied = false
		out.Repeated = false
		pg := s.dialect().name == "postgres"
		if err := taskEvidencePrincipal(ctx, tx, a, s.clock(), pg); err != nil {
			return err
		}
		before := make([]taskEvidenceSnapshot, len(rows))
		for i, row := range rows {
			r, err := taskEvidenceRead(ctx, tx, a.Agent, row.TaskID, pg)
			if err != nil {
				return err
			}
			before[i] = r
			if req.Action == "apply" {
				if err := taskEvidenceUnheld(ctx, tx, a.Agent, row.TaskID); err != nil {
					return err
				}
			}
			out.Rows = append(out.Rows, TaskEvidenceFingerprint{TaskID: row.TaskID, Before: r.digest(), After: r.digest()})
		}
		if req.Action == "inspect" {
			return taskEvidencePrincipal(ctx, tx, a, s.clock(), pg)
		}
		key := taskEvidencePrefix + a.Agent + ":" + req.Manifest.ID
		query := `SELECT value FROM state WHERE key=?`
		if pg {
			query += ` FOR UPDATE`
		}
		var raw string
		err := tx.QueryRowContext(ctx, query, key).Scan(&raw)
		if err == nil {
			var audit taskEvidenceAudit
			if json.Unmarshal([]byte(raw), &audit) != nil {
				return errors.New("task evidence: corrupt provenance")
			}
			recorded, _ := json.Marshal(audit.Manifest)
			if !bytes.Equal(recorded, b) || !audit.Result.Applied || audit.Result.Repeated || audit.Result.ManifestSHA256 != out.ManifestSHA256 || audit.Actor.Agent != a.Agent || len(audit.Result.Rows) != len(rows) {
				return errors.New("task evidence: manifest id collision/corrupt provenance")
			}
			for i, r := range audit.Result.Rows {
				if r.TaskID != rows[i].TaskID || r.After != before[i].digest() {
					return errors.New("task evidence: recorded after-state changed; no replay")
				}
			}
			if err := taskEvidencePrincipal(ctx, tx, a, s.clock(), pg); err != nil {
				return err
			}
			out = audit.Result
			out.Repeated = true
			out.Applied = false
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		after := make([]taskEvidenceSnapshot, len(rows))
		for i, row := range rows {
			if before[i].digest() != row.ExpectedSHA256 {
				return errors.New("task evidence: stale public row fingerprint")
			}
			r, err := taskEvidenceChanged(before[i], row.Changes)
			if err != nil {
				return err
			}
			after[i] = r
			out.Rows[i].After = r.digest()
		}
		for i, row := range rows {
			res, err := tx.ExecContext(ctx, `UPDATE tasks SET links=?,result_ref=? WHERE agent=? AND task_id=? AND links=? AND result_ref=?`, after[i][14].String, after[i][19].String, a.Agent, row.TaskID, before[i][14].String, before[i][19].String)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil || n != 1 {
				return errors.New("task evidence: lost reference CAS")
			}
			actual, err := taskEvidenceRead(ctx, tx, a.Agent, row.TaskID, pg)
			if err != nil || actual.digest() != after[i].digest() {
				return errors.New("task evidence: unexpected readback; rollback")
			}
		}
		if err := taskEvidencePrincipal(ctx, tx, a, s.clock(), pg); err != nil {
			return err
		}
		out.Applied = true
		audit := taskEvidenceAudit{Manifest: req.Manifest, Actor: a, At: s.clock().UTC().Format(time.RFC3339Nano), Result: out, Qualification: "authenticated actor attestation; external review reference and client filesystem hashes are not independently verified by this service"}
		encoded, err := json.Marshal(audit)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO state(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING`, key, string(encoded))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil || n != 1 {
			return errors.New("task evidence: concurrent provenance collision; rollback")
		}
		return nil
	})
	if err != nil {
		return TaskEvidenceResult{}, err
	}
	return out, nil
}
