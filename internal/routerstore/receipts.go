package routerstore

// receipts.go — a ledger task reaches `done` only on a receipt the router
// verifies itself (owner decision 2026-11-02, "Router-checked receipt").
//
//	pr:owner/repo#N             GitHub says merged; merge commit is on the default branch
//	ci:owner/repo#RUN@SHA       the Actions run concluded success on exactly SHA
//	tag:owner/repo@TAG          the tag exists on origin; its commit's check runs all passed
//	maat:RUN@sha256:HEX         host-attested: the submitting CLI hashed a clean Ma'at run;
//	                            the service cannot read Mac files, so it records, not verifies
//
// Owner sign-off is not a receipt string; it is CompleteTaskByOwner, which is
// not served remotely. Every path that can write status='done' routes through
// verifyReceipt or that method — nothing else sets done.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	// ErrReceiptInvalid: the receipt was checked and is false (not merged, failed, missing).
	ErrReceiptInvalid = errors.New("routerstore: completion receipt invalid")
	// ErrReceiptUnverifiable: the router could not check it (no token, API down).
	// Fail closed — the task stays not-done — but a re-check never reopens on it.
	ErrReceiptUnverifiable = errors.New("routerstore: completion receipt unverifiable")
)

// Receipt is the verified proof recorded beside a done task (state row
// "task-receipt:<agent>/<task>"), so a re-check can test it again later.
type Receipt struct {
	Kind       string `json:"kind"`
	Value      string `json:"value"`
	Tier       string `json:"tier"` // "router-verified" | "host-attested" | "owner"
	VerifiedAt string `json:"verified_at"`
	Actor      string `json:"actor,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

func (r Receipt) String() string { return r.Kind + ":" + r.Value }

const receiptStatePrefix = "task-receipt:"

func receiptKey(agent, taskID string) string { return receiptStatePrefix + agent + "/" + taskID }

// ReceiptVerifier checks the network-verifiable kinds (pr, ci, tag). It returns
// nil, ErrReceiptInvalid (wrapped) or ErrReceiptUnverifiable (wrapped).
type ReceiptVerifier interface {
	Verify(ctx context.Context, kind, value string) error
}

var (
	receiptVerifierMu sync.RWMutex
	receiptVerifier   ReceiptVerifier = newGitHubVerifier(os.Getenv("SIRSI_ROUTER_GITHUB_TOKEN"))
)

// SetReceiptVerifier installs a verifier (tests, service wiring). nil restores
// the GitHub default read from SIRSI_ROUTER_GITHUB_TOKEN.
func SetReceiptVerifier(v ReceiptVerifier) {
	receiptVerifierMu.Lock()
	defer receiptVerifierMu.Unlock()
	if v == nil {
		v = newGitHubVerifier(os.Getenv("SIRSI_ROUTER_GITHUB_TOKEN"))
	}
	receiptVerifier = v
}

func getReceiptVerifier() ReceiptVerifier {
	receiptVerifierMu.RLock()
	defer receiptVerifierMu.RUnlock()
	return receiptVerifier
}

var (
	reRepo   = `[A-Za-z0-9._-]+/[A-Za-z0-9._-]+`
	rePR     = regexp.MustCompile(`^(` + reRepo + `)#([1-9][0-9]*)$`)
	reCI     = regexp.MustCompile(`^(` + reRepo + `)#([1-9][0-9]*)@([0-9a-f]{40})$`)
	reTag    = regexp.MustCompile(`^(` + reRepo + `)@([A-Za-z0-9._/+-]+)$`)
	reMaat   = regexp.MustCompile(`^([A-Za-z0-9._-]+)@sha256:([0-9a-f]{64})$`)
	receipts = "pr:owner/repo#N, ci:owner/repo#RUN@SHA40, tag:owner/repo@TAG or maat:RUN@sha256:HEX"
)

// ParseReceipt splits and syntax-checks "kind:value".
func ParseReceipt(raw string) (kind, value string, err error) {
	kind, value, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || value == "" {
		return "", "", fmt.Errorf("%w: %q is not a receipt (want %s)", ErrReceiptInvalid, raw, receipts)
	}
	re := map[string]*regexp.Regexp{"pr": rePR, "ci": reCI, "tag": reTag, "maat": reMaat}[kind]
	if re == nil || !re.MatchString(value) {
		return "", "", fmt.Errorf("%w: %q is not a receipt (want %s)", ErrReceiptInvalid, raw, receipts)
	}
	return kind, value, nil
}

// verifyReceipt is the one gate in front of status='done' for lane completions.
func verifyReceipt(raw string, now time.Time) (Receipt, error) {
	kind, value, err := ParseReceipt(raw)
	if err != nil {
		return Receipt{}, err
	}
	r := Receipt{Kind: kind, Value: value, Tier: "router-verified", VerifiedAt: now.UTC().Format(time.RFC3339)}
	if kind == "maat" {
		r.Tier = "host-attested"
		return r, nil
	}
	v := getReceiptVerifier()
	if v == nil {
		return Receipt{}, fmt.Errorf("%w: no verifier configured", ErrReceiptUnverifiable)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := v.Verify(ctx, kind, value); err != nil {
		return Receipt{}, err
	}
	return r, nil
}

// githubVerifier checks receipts against the GitHub REST API with a read-only token.
type githubVerifier struct {
	token string
	base  string
	http  *http.Client
}

func newGitHubVerifier(token string) *githubVerifier {
	return &githubVerifier{token: strings.TrimSpace(token), base: "https://api.github.com", http: &http.Client{Timeout: 15 * time.Second}}
}

// get decodes one API object. 404/422 → invalid (the thing does not exist);
// no token, transport error, auth or 5xx → unverifiable.
func (g *githubVerifier) get(ctx context.Context, path string, out any) error {
	if g.token == "" {
		return fmt.Errorf("%w: SIRSI_ROUTER_GITHUB_TOKEN is not set on the router service", ErrReceiptUnverifiable)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.base+path, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReceiptUnverifiable, err)
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := g.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReceiptUnverifiable, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: %s not found", ErrReceiptInvalid, path)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: GitHub %s answered %d", ErrReceiptUnverifiable, path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decode %s: %v", ErrReceiptUnverifiable, path, err)
	}
	return nil
}

// onDefaultBranch: sha is the default branch head or an ancestor of it.
func (g *githubVerifier) onDefaultBranch(ctx context.Context, repo, sha string) error {
	var meta struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := g.get(ctx, "/repos/"+repo, &meta); err != nil {
		return err
	}
	var cmp struct {
		Status string `json:"status"`
	}
	if err := g.get(ctx, "/repos/"+repo+"/compare/"+url.PathEscape(meta.DefaultBranch)+"..."+sha, &cmp); err != nil {
		return err
	}
	// compare base...head: "behind"/"identical" means sha is already contained in base.
	if cmp.Status != "behind" && cmp.Status != "identical" {
		return fmt.Errorf("%w: %s is not on %s/%s (compare %s)", ErrReceiptInvalid, sha, repo, meta.DefaultBranch, cmp.Status)
	}
	return nil
}

func (g *githubVerifier) Verify(ctx context.Context, kind, value string) error {
	switch kind {
	case "pr":
		m := rePR.FindStringSubmatch(value)
		var pr struct {
			Merged         bool   `json:"merged"`
			MergeCommitSHA string `json:"merge_commit_sha"`
		}
		if err := g.get(ctx, "/repos/"+m[1]+"/pulls/"+m[2], &pr); err != nil {
			return err
		}
		if !pr.Merged || pr.MergeCommitSHA == "" {
			return fmt.Errorf("%w: %s is not merged", ErrReceiptInvalid, value)
		}
		return g.onDefaultBranch(ctx, m[1], pr.MergeCommitSHA)
	case "ci":
		m := reCI.FindStringSubmatch(value)
		var run struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HeadSHA    string `json:"head_sha"`
		}
		if err := g.get(ctx, "/repos/"+m[1]+"/actions/runs/"+m[2], &run); err != nil {
			return err
		}
		if run.HeadSHA != m[3] {
			return fmt.Errorf("%w: run %s ran on %s, not %s", ErrReceiptInvalid, m[2], run.HeadSHA, m[3])
		}
		if run.Status != "completed" || run.Conclusion != "success" {
			return fmt.Errorf("%w: run %s is %s/%s", ErrReceiptInvalid, m[2], run.Status, run.Conclusion)
		}
		return nil
	case "tag":
		m := reTag.FindStringSubmatch(value)
		var ref struct {
			Object struct{ Type, SHA string } `json:"object"`
		}
		if err := g.get(ctx, "/repos/"+m[1]+"/git/ref/tags/"+m[2], &ref); err != nil {
			return err
		}
		sha := ref.Object.SHA
		if ref.Object.Type == "tag" { // annotated tag → its commit
			var tag struct {
				Object struct{ SHA string } `json:"object"`
			}
			if err := g.get(ctx, "/repos/"+m[1]+"/git/tags/"+sha, &tag); err != nil {
				return err
			}
			sha = tag.Object.SHA
		}
		var runs struct {
			Total int `json:"total_count"`
			Runs  []struct {
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			} `json:"check_runs"`
		}
		if err := g.get(ctx, "/repos/"+m[1]+"/commits/"+sha+"/check-runs?per_page=100", &runs); err != nil {
			return err
		}
		if runs.Total == 0 {
			return fmt.Errorf("%w: tag %s commit %s has no CI check runs", ErrReceiptInvalid, m[2], sha)
		}
		for _, r := range runs.Runs {
			if r.Status != "completed" || (r.Conclusion != "success" && r.Conclusion != "skipped" && r.Conclusion != "neutral") {
				return fmt.Errorf("%w: tag %s commit %s has a %s/%s check run", ErrReceiptInvalid, m[2], sha, r.Status, r.Conclusion)
			}
		}
		return nil
	}
	return fmt.Errorf("%w: kind %q is not network-verifiable", ErrReceiptInvalid, kind)
}

// CompleteTaskByOwner is owner sign-off for work with nothing to build. It is
// deliberately NOT on the Store interface, so the router service never serves
// it to a remote caller; the board reaches it only through its own owner-auth
// path. Any live lease is released.
func (s *SQLiteStore) CompleteTaskByOwner(agent, taskID, actor, reason string) error {
	return s.ownerTerminal(agent, taskID, actor, reason, "done")
}

// CancelTask ends a task that will not be done. Canceled never counts as done.
// Like CompleteTaskByOwner it is owner-only and not served remotely.
func (s *SQLiteStore) CancelTask(agent, taskID, actor, reason string) error {
	return s.ownerTerminal(agent, taskID, actor, reason, "canceled")
}

func (s *SQLiteStore) ownerTerminal(agent, taskID, actor, reason, status string) error {
	actor, reason = strings.TrimSpace(actor), strings.TrimSpace(reason)
	if actor == "" || reason == "" {
		return fmt.Errorf("%w: owner %s needs an actor and a reason", ErrReasonRequired, status)
	}
	now := s.clock().UTC()
	tx, err := s.beginImmediate()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	ref := "owner:" + actor
	res, err := tx.Exec(`UPDATE tasks SET status=?,result_ref=?,failure_reason=?,lease_token='',lease_expires='',claimed_by='',thread_id='',updated=?
		WHERE agent=? AND task_id=? AND status NOT IN ('done','canceled');`, status, ref, reason, now.Format(time.RFC3339), agent, taskID)
	if err != nil {
		return fmt.Errorf("routerstore: owner %s %s/%s: %w", status, agent, taskID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("%w: %s/%s is missing or already terminal", ErrNotFound, agent, taskID)
	}
	if status == "done" {
		if err = putReceiptTx(tx, agent, taskID, Receipt{Kind: "owner", Value: actor, Tier: "owner", VerifiedAt: now.Format(time.RFC3339), Actor: actor, Reason: reason}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func putReceiptTx(tx *txHandle, agent, taskID string, r Receipt) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO state (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value;`,
		receiptKey(agent, taskID), string(b)); err != nil {
		return fmt.Errorf("routerstore: record receipt: %w", err)
	}
	return nil
}

// ReceiptRecheck is one re-check outcome.
type ReceiptRecheck struct {
	Agent    string `json:"agent"`
	TaskID   string `json:"task_id"`
	Receipt  string `json:"receipt"`
	Reopened bool   `json:"reopened"`
	Error    string `json:"error,omitempty"`
}

// RecheckTaskReceipts re-verifies pr/ci/tag receipts last verified more than
// olderThan ago. A receipt now proven false reopens its task to pending with a
// timeline note; an unverifiable one (network, token) is left alone — unknown
// is not invalid.
func (s *SQLiteStore) RecheckTaskReceipts(olderThan time.Duration) ([]ReceiptRecheck, error) {
	now := s.clock().UTC()
	rows, err := s.db.Query(`SELECT key, value FROM state WHERE key LIKE ?;`, receiptStatePrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("routerstore: list receipts: %w", err)
	}
	type row struct{ key, value string }
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.key, &r.value); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, r)
	}
	rows.Close()
	var out []ReceiptRecheck
	for _, r := range all {
		var rc Receipt
		if json.Unmarshal([]byte(r.value), &rc) != nil || rc.Tier != "router-verified" {
			continue
		}
		if at, perr := time.Parse(time.RFC3339, rc.VerifiedAt); perr == nil && now.Sub(at) < olderThan {
			continue
		}
		agent, taskID, _ := strings.Cut(strings.TrimPrefix(r.key, receiptStatePrefix), "/")
		res := ReceiptRecheck{Agent: agent, TaskID: taskID, Receipt: rc.String()}
		fresh, verr := verifyReceipt(rc.String(), now)
		switch {
		case verr == nil:
			b, _ := json.Marshal(fresh)
			if _, err := s.exec(`UPDATE state SET value=? WHERE key=?;`, string(b), r.key); err != nil {
				res.Error = err.Error()
			}
		case errors.Is(verr, ErrReceiptInvalid):
			res.Error = verr.Error()
			if err := s.reopenForReceipt(agent, taskID, r.key, verr, now); err != nil {
				res.Error += "; reopen: " + err.Error()
			} else {
				res.Reopened = true
			}
		default:
			res.Error = verr.Error()
		}
		out = append(out, res)
	}
	return out, nil
}

func (s *SQLiteStore) reopenForReceipt(agent, taskID, key string, why error, now time.Time) error {
	t, err := s.GetTask(agent, taskID)
	if err != nil {
		return err
	}
	if t.Status != "done" {
		return nil
	}
	t.Timeline = append(t.Timeline, TimelineEntry{Day: now.Format("2006-01-02"), Owner: "router", Label: "receipt no longer valid: " + why.Error()})
	timeline, err := json.Marshal(t.Timeline)
	if err != nil {
		return err
	}
	tx, err := s.beginImmediate()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`UPDATE tasks SET status='pending',result_ref='',timeline=?,updated=? WHERE agent=? AND task_id=? AND status='done';`,
		string(timeline), now.Format(time.RFC3339), agent, taskID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM state WHERE key=?;`, key); err != nil {
		return err
	}
	return tx.Commit()
}
