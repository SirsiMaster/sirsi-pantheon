package routerstore

// receipts.go — a ledger task reaches `done` only on a receipt the router
// verifies itself (owner decision 2026-11-02, "Router-checked receipt").
//
//	pr:owner/repo#N             GitHub says merged; merge commit is on the default branch
//	ci:owner/repo#RUN@SHA       the Actions run concluded success on exactly SHA
//	tag:owner/repo@TAG          the tag exists on origin; its commit has a passing CI check
//	maat:RESERVATION-ID         the router's own Ma'at ledger records that a host-token
//	                            holder released, with exit 0, a reservation held under the
//	                            task agent's name. NOT proof the run happened: the exit code
//	                            is what the releaser reported. Scoped cloud tokens cannot
//	                            reserve or release.
//
// Repos are bound to ReceiptRepoOwners and a receipt proves one task only.
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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

var (
	// ErrReceiptInvalid: the receipt was checked and is false (not merged, failed, missing).
	ErrReceiptInvalid = errors.New("routerstore: completion receipt invalid")
	// ErrReceiptUnverifiable: the router could not check it (no token, API down).
	// Fail closed — the task stays not-done — but a re-check never reopens on it.
	ErrReceiptUnverifiable = errors.New("routerstore: completion receipt unverifiable")
	// errReceiptNotFound is a bare GitHub 404/422. At completion it refuses (the
	// thing must exist); on a re-check it is NOT proof of falsity — run
	// retention and token scope both produce it — so it never reopens a task.
	errReceiptNotFound = fmt.Errorf("%w: not found", ErrReceiptInvalid)
)

// ReceiptRepoOwners are the GitHub owners a pr/ci/tag receipt may name
// (compared case-insensitively, as GitHub does).
var ReceiptRepoOwners = []string{"SirsiMaster"}

// Receipt is the verified proof recorded beside a done task (state row
// "task-receipt:<agent>/<task>"), so a re-check can test it again later.
type Receipt struct {
	Kind       string `json:"kind"`
	Value      string `json:"value"`
	Tier       string `json:"tier"` // "router-verified" | "owner"
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
	reMaat   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)
	receipts = "pr:owner/repo#N, ci:owner/repo#RUN@SHA40, tag:owner/repo@TAG or maat:RESERVATION-ID"
)

// cleanSegments refuses empty, "." and ".." path segments, so a receipt can
// never steer an API path outside the repo it names.
func cleanSegments(s string) bool {
	for _, seg := range strings.Split(s, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func allowedRepo(repo string) bool {
	owner, _, _ := strings.Cut(repo, "/")
	for _, o := range ReceiptRepoOwners {
		if strings.EqualFold(o, owner) {
			return true
		}
	}
	return false
}

// ParseReceipt splits and syntax-checks "kind:value".
func ParseReceipt(raw string) (kind, value string, err error) {
	bad := func(why string) (string, string, error) {
		return "", "", fmt.Errorf("%w: %q %s (want %s)", ErrReceiptInvalid, raw, why, receipts)
	}
	kind, value, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || value == "" {
		return bad("is not a receipt")
	}
	re := map[string]*regexp.Regexp{"pr": rePR, "ci": reCI, "tag": reTag, "maat": reMaat}[kind]
	if re == nil {
		return bad("is not a receipt")
	}
	m := re.FindStringSubmatch(value)
	if m == nil {
		return bad("is not a receipt")
	}
	if kind == "maat" {
		return kind, value, nil
	}
	if !cleanSegments(m[1]) || (kind == "tag" && !cleanSegments(m[2])) {
		return bad("has an empty, . or .. path segment")
	}
	if !allowedRepo(m[1]) {
		return bad(fmt.Sprintf("names a repo outside %v", ReceiptRepoOwners))
	}
	return kind, value, nil
}

// verifyReceipt is the one gate in front of status='done' for lane completions.
// agent is the task's ledger agent: a maat receipt must be its own reservation.
func (s *SQLiteStore) verifyReceipt(agent, raw string, now time.Time) (Receipt, error) {
	kind, value, err := ParseReceipt(raw)
	if err != nil {
		return Receipt{}, err
	}
	r := Receipt{Kind: kind, Value: value, Tier: "router-verified", VerifiedAt: now.UTC().Format(time.RFC3339)}
	if kind == "maat" {
		return r, s.verifyMaatReceipt(agent, value)
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

// verifyMaatReceipt reads the router's own reservation ledger: the run every
// sirsi-maat-run books, heartbeats and releases there.
func (s *SQLiteStore) verifyMaatReceipt(agent, id string) error {
	rows, err := s.MaatRead("reservation", id)
	if err != nil {
		return fmt.Errorf("%w: Ma'at ledger read: %v", ErrReceiptUnverifiable, err)
	}
	if len(rows) != 1 {
		return fmt.Errorf("%w: no Ma'at reservation %s", ErrReceiptInvalid, id)
	}
	r := rows[0]
	switch {
	case r.Holder != agent:
		return fmt.Errorf("%w: reservation %s is held by %s, not %s", ErrReceiptInvalid, id, r.Holder, agent)
	case r.Status != schedule.StatusReleased && r.Status != schedule.StatusDone:
		return fmt.Errorf("%w: reservation %s is %s, not released", ErrReceiptInvalid, id, r.Status)
	case r.ExitCode == nil:
		return fmt.Errorf("%w: reservation %s was released without an exit outcome (launcher must release with --exit-code)", ErrReceiptUnverifiable, id)
	case *r.ExitCode != 0:
		return fmt.Errorf("%w: reservation %s run exited %d", ErrReceiptInvalid, id, *r.ExitCode)
	}
	return nil
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

// get decodes one API object. 404/422 → errReceiptNotFound; no token,
// transport error, auth or 5xx → unverifiable.
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
		return fmt.Errorf("%w: %s", errReceiptNotFound, path)
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

// maxCheckRunPages bounds the tag check-run walk (100 per page).
const maxCheckRunPages = 10

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
		return g.verifyTag(ctx, value)
	}
	return fmt.Errorf("%w: kind %q is not network-verifiable", ErrReceiptInvalid, kind)
}

func (g *githubVerifier) verifyTag(ctx context.Context, value string) error {
	m := reTag.FindStringSubmatch(value)
	repo, name := m[1], m[2]
	var ref struct {
		Object struct{ Type, SHA string } `json:"object"`
	}
	if err := g.get(ctx, "/repos/"+repo+"/git/ref/tags/"+name, &ref); err != nil {
		if !errors.Is(err, errReceiptNotFound) {
			return err
		}
		// A missing tag is proof only if the repo itself is readable; otherwise
		// the 404 may just be this token's scope.
		var meta struct{}
		if rerr := g.get(ctx, "/repos/"+repo, &meta); rerr != nil {
			return err
		}
		return fmt.Errorf("%w: tag %s no longer exists in readable %s", ErrReceiptInvalid, name, repo)
	}
	sha := ref.Object.SHA
	if ref.Object.Type == "tag" { // annotated tag → its commit
		var tag struct {
			Object struct{ SHA string } `json:"object"`
		}
		if err := g.get(ctx, "/repos/"+repo+"/git/tags/"+sha, &tag); err != nil {
			return err
		}
		sha = tag.Object.SHA
	}
	total, success := -1, 0
	seen := 0
	for page := 1; page <= maxCheckRunPages && (total < 0 || seen < total); page++ {
		var runs struct {
			Total int `json:"total_count"`
			Runs  []struct {
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			} `json:"check_runs"`
		}
		if err := g.get(ctx, "/repos/"+repo+"/commits/"+sha+"/check-runs?per_page=100&page="+strconv.Itoa(page), &runs); err != nil {
			return err
		}
		total = runs.Total
		if len(runs.Runs) == 0 {
			break
		}
		for _, r := range runs.Runs {
			seen++
			switch {
			case r.Status != "completed":
				return fmt.Errorf("%w: tag %s commit %s has an unfinished check run", ErrReceiptInvalid, name, sha)
			case r.Conclusion == "success":
				success++
			case r.Conclusion == "skipped" || r.Conclusion == "neutral":
			default:
				return fmt.Errorf("%w: tag %s commit %s has a %s check run", ErrReceiptInvalid, name, sha, r.Conclusion)
			}
		}
	}
	if seen < total {
		return fmt.Errorf("%w: read %d of %d check runs for tag %s", ErrReceiptUnverifiable, seen, total, name)
	}
	if success == 0 {
		return fmt.Errorf("%w: tag %s commit %s has no successful CI check run", ErrReceiptInvalid, name, sha)
	}
	return nil
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
		if err = s.putReceiptTx(tx, agent, taskID, Receipt{Kind: "owner", Value: actor, Tier: "owner", VerifiedAt: now.Format(time.RFC3339), Actor: actor, Reason: reason}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// receiptLockKey is the row every receipt uniqueness check locks first. It is
// outside receiptStatePrefix so receipt scans never see it.
const receiptLockKey = "task-receipt-lock"

// putReceiptTx records the receipt beside its task. A pr/ci/tag/maat receipt
// proves one task: the same receipt recorded for any other task is refused.
func (s *SQLiteStore) putReceiptTx(tx *txHandle, agent, taskID string, r Receipt) error {
	key := receiptKey(agent, taskID)
	if r.Kind != "owner" {
		// Serialize scan+insert across service replicas. Under PostgreSQL READ
		// COMMITTED two completions naming the same receipt would otherwise both
		// see "unused" and both commit; SQLite's BEGIN IMMEDIATE already serializes.
		if _, err := tx.Exec(`INSERT INTO state(key,value) VALUES(?,'') ON CONFLICT(key) DO NOTHING;`, receiptLockKey); err != nil {
			return fmt.Errorf("routerstore: receipt lock: %w", err)
		}
		if s.dialect().name == "postgres" {
			var k string
			if err := tx.QueryRow(`SELECT key FROM state WHERE key=? FOR UPDATE;`, receiptLockKey).Scan(&k); err != nil {
				return fmt.Errorf("routerstore: receipt lock: %w", err)
			}
		}
		// ponytail: O(receipts) scan inside the tx; add an indexed receipt column if the ledger grows large.
		rows, err := tx.Query(`SELECT key, value FROM state WHERE key LIKE ? AND key <> ?;`, receiptStatePrefix+"%", key)
		if err != nil {
			return fmt.Errorf("routerstore: receipt uniqueness: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				return err
			}
			var other Receipt
			if json.Unmarshal([]byte(v), &other) == nil && other.Kind == r.Kind && other.Value == r.Value {
				return fmt.Errorf("%w: %s already proves %s", ErrReceiptInvalid, r, strings.TrimPrefix(k, receiptStatePrefix))
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO state (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value;`, key, string(b)); err != nil {
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
// olderThan ago. Only a receipt now positively proven false reopens its task
// (to pending, with a timeline note). Unverifiable answers and bare 404s — run
// retention, token scope — are left alone: unknown is not invalid. Ma'at and
// owner receipts are ledger facts and are never re-checked.
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
		if json.Unmarshal([]byte(r.value), &rc) != nil || rc.Tier != "router-verified" || rc.Kind == "maat" {
			continue
		}
		if at, perr := time.Parse(time.RFC3339, rc.VerifiedAt); perr == nil && now.Sub(at) < olderThan {
			continue
		}
		agent, taskID, _ := strings.Cut(strings.TrimPrefix(r.key, receiptStatePrefix), "/")
		res := ReceiptRecheck{Agent: agent, TaskID: taskID, Receipt: rc.String()}
		fresh, verr := s.verifyReceipt(agent, rc.String(), now)
		switch {
		case verr == nil:
			b, _ := json.Marshal(fresh)
			// Refresh only the row we read; a concurrent reopen or re-completion wins.
			if _, err := s.exec(`UPDATE state SET value=? WHERE key=? AND value=?;`, string(b), r.key, r.value); err != nil {
				res.Error = err.Error()
			}
		case errors.Is(verr, errReceiptNotFound) || !errors.Is(verr, ErrReceiptInvalid):
			res.Error = "unverifiable (left done): " + verr.Error()
		default:
			res.Error = verr.Error()
			reopened, err := s.reopenForReceipt(agent, taskID, r.key, r.value, rc.String(), verr, now)
			if err != nil {
				res.Error += "; reopen: " + err.Error()
			}
			res.Reopened = reopened
		}
		out = append(out, res)
	}
	return out, nil
}

// reopenForReceipt flips done→pending only if the task still carries the exact
// receipt that was checked, and drops only the receipt row that was read.
func (s *SQLiteStore) reopenForReceipt(agent, taskID, key, storedValue, receipt string, why error, now time.Time) (bool, error) {
	t, err := s.GetTask(agent, taskID)
	if err != nil {
		return false, err
	}
	if t.Status != "done" || t.ResultRef != receipt {
		return false, nil
	}
	t.Timeline = append(t.Timeline, TimelineEntry{Day: now.Format("2006-01-02"), Owner: "router", Label: "receipt no longer valid: " + why.Error()})
	timeline, err := json.Marshal(t.Timeline)
	if err != nil {
		return false, err
	}
	tx, err := s.beginImmediate()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`UPDATE tasks SET status='pending',result_ref='',timeline=?,updated=? WHERE agent=? AND task_id=? AND status='done' AND result_ref=?;`,
		string(timeline), now.Format(time.RFC3339), agent, taskID, receipt)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return false, nil
	}
	if _, err = tx.Exec(`DELETE FROM state WHERE key=? AND value=?;`, key, storedValue); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
