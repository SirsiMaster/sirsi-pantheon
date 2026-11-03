package routerstore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeVerifier answers per receipt value; anything unlisted is unverifiable.
type fakeVerifier map[string]error

func (f fakeVerifier) Verify(_ context.Context, kind, value string) error {
	if err, ok := f[kind+":"+value]; ok {
		return err
	}
	return fmt.Errorf("%w: fake has no answer for %s:%s", ErrReceiptUnverifiable, kind, value)
}

// fixtureVerifier accepts only pr:SirsiMaster/fixture#N. The package's other
// tests complete tasks with those receipts; they test leases, not receipts.
type fixtureVerifier struct{}

func (fixtureVerifier) Verify(_ context.Context, kind, value string) error {
	if kind == "pr" && strings.HasPrefix(value, "SirsiMaster/fixture#") {
		return nil
	}
	return fmt.Errorf("%w: fixture verifier only accepts pr:SirsiMaster/fixture#N", ErrReceiptUnverifiable)
}

func init() { SetReceiptVerifier(fixtureVerifier{}) }

func useVerifier(t *testing.T, v ReceiptVerifier) {
	t.Helper()
	prev := getReceiptVerifier()
	SetReceiptVerifier(v)
	t.Cleanup(func() { SetReceiptVerifier(prev) })
}

const (
	sha40   = "0123456789abcdef0123456789abcdef01234567"
	prOK    = "pr:SirsiMaster/sirsi-pantheon#1337"
	prBad   = "pr:SirsiMaster/sirsi-pantheon#9"
	ciOK    = "ci:SirsiMaster/sirsi-pantheon#42@" + sha40
	tagOK   = "tag:SirsiMaster/sirsi-pantheon@v0.24.200"
	unknown = "pr:SirsiMaster/sirsi-pantheon#77"
)

// markDoneFixture puts a seeded task straight into `done` for tests about how
// other code treats finished work. Production has no such path: done needs a receipt.
func markDoneFixture(t *testing.T, s *SQLiteStore, agent, id string) {
	t.Helper()
	if _, err := s.exec(`UPDATE tasks SET status='done',result_ref=? WHERE agent=? AND task_id=?;`, "pr:SirsiMaster/fixture#0", agent, id); err != nil {
		t.Fatal(err)
	}
}

func claimed(t *testing.T, s *SQLiteStore, agent, id string) string {
	t.Helper()
	if err := s.AddTask(Task{Agent: agent, TaskID: id, Subject: "work " + id}); err != nil {
		t.Fatal(err)
	}
	l, err := s.ClaimTask(agent, id, "w", "thr-"+id, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return l.Token
}

// maatRun books a reservation in the router's own Ma'at ledger and releases it
// the way sirsi-maat-run does; exit < 0 releases without an outcome.
func maatRun(t *testing.T, s *SQLiteStore, id, holder string, exit int) string {
	t.Helper()
	if _, err := s.MaatReserve(maatRequest(s, id, "m1", holder), false); err != nil {
		t.Fatal(err)
	}
	value := ""
	if exit >= 0 {
		value = "exit=" + strconv.Itoa(exit)
	}
	if _, err := s.MaatMutate("release", id, value); err != nil {
		t.Fatal(err)
	}
	return "maat:" + id
}

func TestCompletionNeedsAVerifiedReceipt(t *testing.T) {
	useVerifier(t, fakeVerifier{
		prOK: nil, ciOK: nil, tagOK: nil,
		prBad: fmt.Errorf("%w: not merged", ErrReceiptInvalid),
	})
	for _, tc := range []struct {
		name    string
		receipt func(*SQLiteStore) string
		want    error // nil = completes
	}{
		{"merged PR", func(*SQLiteStore) string { return prOK }, nil},
		{"green CI run", func(*SQLiteStore) string { return ciOK }, nil},
		{"release tag", func(*SQLiteStore) string { return tagOK }, nil},
		{"own clean Ma'at run", func(s *SQLiteStore) string { return maatRun(t, s, "res-ok", "lane", 0) }, nil},
		{"Ma'at run that failed", func(s *SQLiteStore) string { return maatRun(t, s, "res-fail", "lane", 2) }, ErrReceiptInvalid},
		{"another agent's Ma'at run", func(s *SQLiteStore) string { return maatRun(t, s, "res-other", "someone-else", 0) }, ErrReceiptInvalid},
		{"Ma'at release without outcome", func(s *SQLiteStore) string { return maatRun(t, s, "res-noexit", "lane", -1) }, ErrReceiptUnverifiable},
		{"Ma'at run still active", func(s *SQLiteStore) string {
			if _, err := s.MaatReserve(maatRequest(s, "res-live", "m1", "lane"), false); err != nil {
				t.Fatal(err)
			}
			return "maat:res-live"
		}, ErrReceiptInvalid},
		{"unknown Ma'at reservation", func(*SQLiteStore) string { return "maat:nope" }, ErrReceiptInvalid},
		{"old self-attested Ma'at hash refused", func(*SQLiteStore) string { return "maat:run1@sha256:" + strings.Repeat("0", 64) }, ErrReceiptInvalid},
		{"unmerged PR refused", func(*SQLiteStore) string { return prBad }, ErrReceiptInvalid},
		{"unverifiable refused (fail closed)", func(*SQLiteStore) string { return unknown }, ErrReceiptUnverifiable},
		{"free text refused", func(*SQLiteStore) string { return "proof://whatever" }, ErrReceiptInvalid},
		{"ci without the commit refused", func(*SQLiteStore) string { return "ci:SirsiMaster/sirsi-pantheon#42" }, ErrReceiptInvalid},
		{"owner is not a lane receipt", func(*SQLiteStore) string { return "owner:someone" }, ErrReceiptInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openTestStore(t)
			receipt := tc.receipt(s)
			tok := claimed(t, s, "lane", "t")
			err := s.CompleteTaskLease("lane", "t", tok, receipt)
			got, _ := s.GetTask("lane", "t")
			if tc.want != nil {
				if !errors.Is(err, tc.want) || got.Status == "done" {
					t.Fatalf("err=%v status=%s, want %v and not done", err, got.Status, tc.want)
				}
				return
			}
			if err != nil || got.Status != "done" || got.ResultRef != receipt {
				t.Fatalf("err=%v status=%s ref=%q", err, got.Status, got.ResultRef)
			}
			if raw, ok, _ := s.GetState(receiptKey("lane", "t")); !ok || !strings.Contains(raw, `"tier":"router-verified"`) {
				t.Fatalf("receipt record %q", raw)
			}
		})
	}
}

func TestReceiptBinding(t *testing.T) {
	for _, raw := range []string{
		"pr:someone-else/repo#1",              // outside the org allowlist
		"tag:SirsiMaster/sirsi-pantheon@../x", // path traversal in the tag
		"tag:SirsiMaster/sirsi-pantheon@a//b",
		"pr:SirsiMaster/..#1",
		"ci:../sirsi-pantheon#1@" + sha40,
	} {
		if _, _, err := ParseReceipt(raw); !errors.Is(err, ErrReceiptInvalid) {
			t.Errorf("ParseReceipt(%q) = %v, want invalid", raw, err)
		}
	}
	if _, _, err := ParseReceipt("pr:sirsimaster/sirsi-pantheon#1"); err != nil {
		t.Errorf("owner match must be case-insensitive: %v", err)
	}
	// One receipt proves one task.
	useVerifier(t, fakeVerifier{prOK: nil})
	s := openTestStore(t)
	if err := s.CompleteTaskLease("lane", "a", claimed(t, s, "lane", "a"), prOK); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteTaskLease("lane", "b", claimed(t, s, "lane", "b"), prOK); !errors.Is(err, ErrReceiptInvalid) {
		t.Fatalf("reused receipt: %v, want invalid", err)
	}
	if b, _ := s.GetTask("lane", "b"); b.Status == "done" {
		t.Fatal("a reused receipt completed a second task")
	}
	run := maatRun(t, s, "res-once", "lane", 0)
	if err := s.CompleteTaskLease("lane", "c", claimed(t, s, "lane", "c"), run); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteTaskLease("lane", "d", claimed(t, s, "lane", "d"), run); !errors.Is(err, ErrReceiptInvalid) {
		t.Fatalf("reused Ma'at run: %v, want invalid", err)
	}
	// The uniqueness scan ran behind the shared lock row (PostgreSQL also takes
	// FOR UPDATE on it; the cross-replica race itself is covered by review).
	if _, ok, _ := s.GetState(receiptLockKey); !ok {
		t.Fatal("receipt lock row was not taken")
	}
	if err := s.SetState(receiptLockKey, "x"); err == nil {
		t.Fatal("generic SetState wrote the receipt lock row")
	}
}

func TestNoVerifierFailsClosed(t *testing.T) {
	useVerifier(t, newGitHubVerifier("")) // what the service has with no token
	s := openTestStore(t)
	tok := claimed(t, s, "lane", "t")
	if err := s.CompleteTaskLease("lane", "t", tok, prOK); !errors.Is(err, ErrReceiptUnverifiable) {
		t.Fatalf("no token: %v, want unverifiable", err)
	}
}

// Every other door to `done` stays shut: AddTask, UpdateTask, generic state.
func TestEveryDonePathIsGated(t *testing.T) {
	s := openTestStore(t)
	if err := s.AddTask(Task{Agent: "lane", TaskID: "d", Subject: "x", Status: "done"}); err == nil {
		t.Fatal("AddTask created a done task")
	}
	if err := s.AddTask(Task{Agent: "lane", TaskID: "c", Subject: "x", Status: "canceled"}); err == nil {
		t.Fatal("AddTask created a canceled task")
	}
	if err := s.AddTask(Task{Agent: "lane", TaskID: "p", Subject: "x"}); err != nil {
		t.Fatal(err)
	}
	for _, st := range []string{"done", "canceled"} {
		if _, err := s.UpdateTask("lane", "p", TaskUpdate{Status: st}); err == nil {
			t.Fatalf("UpdateTask set %s", st)
		}
	}
	if err := s.SetState(receiptKey("lane", "p"), `{"kind":"pr"}`); err == nil {
		t.Fatal("generic SetState forged a receipt")
	}
	if ok, err := s.CompareAndSwapState(receiptKey("lane", "p"), "", false, `{}`); err == nil || ok {
		t.Fatal("generic CAS forged a receipt")
	}
}

func TestOwnerSignOffAndCancel(t *testing.T) {
	s := openTestStore(t)
	claimed(t, s, "lane", "decide")
	if err := s.AddTask(Task{Agent: "lane", TaskID: "drop", Subject: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteTaskByOwner("lane", "decide", "board:owner@sirsi.ai", ""); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("owner done without reason: %v", err)
	}
	if err := s.CompleteTaskByOwner("lane", "decide", "board:owner@sirsi.ai", "decision recorded in chat"); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelTask("lane", "drop", "board:owner@sirsi.ai", "superseded"); err != nil {
		t.Fatal(err)
	}
	d, _ := s.GetTask("lane", "decide")
	c, _ := s.GetTask("lane", "drop")
	if d.Status != "done" || c.Status != "canceled" {
		t.Fatalf("decide=%s drop=%s", d.Status, c.Status)
	}
	if raw, _, _ := s.GetState(receiptKey("lane", "decide")); !strings.Contains(raw, `"tier":"owner"`) {
		t.Fatalf("owner receipt %q", raw)
	}
	if _, ok, _ := s.GetState(receiptKey("lane", "drop")); ok {
		t.Fatal("a cancel recorded a completion receipt")
	}
	var done int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE agent='lane' AND status='done'`).Scan(&done); err != nil || done != 1 {
		t.Fatalf("done count %d (canceled must not count): %v", done, err)
	}
}

// Owner sign-off and cancel are served to the IAP board's @ledger-editor token for a board:<email> actor,
// and refused to every other caller: a lane's own scoped token, an observer, and the unscoped bootstrap token.
func TestOwnerPathsServedOnlyToTheBoardToken(t *testing.T) {
	f := newScopedFixture(t, []string{"router-board", ObserverScope, LedgerEditorScope})
	for _, id := range []string{"sign", "drop", "boot-try", "no-actor"} {
		if err := f.backend.AddTask(Task{Agent: "ra", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	board := f.client("router-board")
	if err := board.CompleteTaskByOwner("ra", "sign", "board:owner@sirsi.ai", "verified in the browser"); err != nil {
		t.Fatalf("board sign-off refused: %v", err)
	}
	if err := board.CancelTask("ra", "drop", "board:owner@sirsi.ai", "superseded"); err != nil {
		t.Fatalf("board cancel refused: %v", err)
	}
	wantLaneScope(t, "board actor without board: prefix", board.CompleteTaskByOwner("ra", "no-actor", "ra", "self sign-off"))

	boot := NewRemoteStore(f.url, "t0k")
	boot.sessionDir, boot.host, boot.agent = "", f.host, "ra"
	wantLaneScope(t, "unscoped bootstrap token", boot.CompleteTaskByOwner("ra", "boot-try", "board:owner@sirsi.ai", "x"))

	// Assert before the next fixture: under the PostgreSQL CI leg every fixture shares, and resets, one database.
	for id, want := range map[string]string{"sign": "done", "drop": "canceled", "boot-try": "pending", "no-actor": "pending"} {
		if got, _ := f.backend.GetTask("ra", id); got.Status != want {
			t.Errorf("%s = %s, want %s", id, got.Status, want)
		}
	}
	if raw, _, _ := f.backend.GetState(receiptKey("ra", "sign")); !strings.Contains(raw, `"actor":"board:owner@sirsi.ai"`) {
		t.Fatalf("owner receipt %q", raw)
	}

	lane := newScopedFixture(t, []string{"ra"})
	if err := lane.backend.AddTask(Task{Agent: "ra", TaskID: "lane-try", Subject: "x"}); err != nil {
		t.Fatal(err)
	}
	wantLaneScope(t, "lane signs off its own task", lane.client("ra").CompleteTaskByOwner("ra", "lane-try", "board:owner@sirsi.ai", "x"))
	if got, _ := lane.backend.GetTask("ra", "lane-try"); got.Status != "pending" {
		t.Errorf("lane-try = %s, want pending", got.Status)
	}
	observer := newScopedFixture(t, []string{"router-board", ObserverScope})
	wantLaneScope(t, "observer-only cancel", observer.client("router-board").CancelTask("ra", "x", "board:owner@sirsi.ai", "x"))
}

func TestRecheckReopensOnlyProvenFalseReceipts(t *testing.T) {
	prGone := "pr:SirsiMaster/sirsi-pantheon#55"
	ciGone := "ci:SirsiMaster/sirsi-pantheon#43@" + sha40
	v := fakeVerifier{prOK: nil, ciOK: nil, prGone: nil, ciGone: nil}
	useVerifier(t, v)
	s := openTestStore(t)
	run := maatRun(t, s, "res-r", "lane", 0)
	for id, r := range map[string]string{"merged": prOK, "ci": ciOK, "maat": run, "pr404": prGone, "ci404": ciGone} {
		if err := s.CompleteTaskLease("lane", id, claimed(t, s, "lane", id), r); err != nil {
			t.Fatal(id, err)
		}
	}
	v[prOK] = fmt.Errorf("%w: merge commit no longer on main", ErrReceiptInvalid) // history rewritten: proven false
	v[ciOK] = fmt.Errorf("%w: GitHub answered 503", ErrReceiptUnverifiable)       // down: unknown
	v[prGone] = fmt.Errorf("%w: /repos/x/pulls/55", errReceiptNotFound)           // token scope: unknown
	v[ciGone] = fmt.Errorf("%w: /repos/x/actions/runs/43", errReceiptNotFound)    // run retention: unknown
	out, err := s.RecheckTaskReceipts(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 4 { // the Ma'at receipt is a ledger fact: never re-checked
		t.Fatalf("rechecked %d receipts, want 4: %+v", len(out), out)
	}
	want := map[string]string{"merged": "pending", "ci": "done", "maat": "done", "pr404": "done", "ci404": "done"}
	for id, st := range want {
		if got, _ := s.GetTask("lane", id); got.Status != st {
			t.Errorf("%s = %s, want %s", id, got.Status, st)
		}
	}
	merged, _ := s.GetTask("lane", "merged")
	if last := merged.Timeline[len(merged.Timeline)-1]; !strings.HasPrefix(last.Label, "receipt no longer valid:") {
		t.Fatalf("timeline note %q", last.Label)
	}
}

// A reopen decided on a stale read must not clobber a task that has since
// moved on (reopened and re-completed with a different receipt).
func TestRecheckReopenIsGuardedOnTheCheckedReceipt(t *testing.T) {
	useVerifier(t, fakeVerifier{prOK: nil})
	s := openTestStore(t)
	if err := s.CompleteTaskLease("lane", "t", claimed(t, s, "lane", "t"), prOK); err != nil {
		t.Fatal(err)
	}
	key := receiptKey("lane", "t")
	stored, _, _ := s.GetState(key)
	// Meanwhile the task was reopened and finished again on a different receipt.
	if _, err := s.exec(`UPDATE tasks SET result_ref='pr:SirsiMaster/sirsi-pantheon#2' WHERE agent='lane' AND task_id='t';`); err != nil {
		t.Fatal(err)
	}
	reopened, err := s.reopenForReceipt("lane", "t", key, stored, prOK, ErrReceiptInvalid, time.Now())
	if err != nil || reopened {
		t.Fatalf("stale reopen applied: reopened=%v err=%v", reopened, err)
	}
	if got, _ := s.GetTask("lane", "t"); got.Status != "done" {
		t.Fatalf("status %s, want done", got.Status)
	}
	if _, ok, _ := s.GetState(key); !ok {
		t.Fatal("stale reopen deleted the receipt row")
	}
}

func TestGitHubVerifier(t *testing.T) {
	runs := func(n int, conclusion string) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = `{"status":"completed","conclusion":"` + conclusion + `"}`
		}
		return strings.Join(parts, ",")
	}
	api := map[string]string{
		"/repos/SirsiMaster/r":                         `{"default_branch":"main"}`,
		"/repos/SirsiMaster/r/pulls/1":                 `{"merged":true,"merge_commit_sha":"m1"}`,
		"/repos/SirsiMaster/r/pulls/2":                 `{"merged":false}`,
		"/repos/SirsiMaster/r/pulls/3":                 `{"merged":true,"merge_commit_sha":"m3"}`,
		"/repos/SirsiMaster/r/compare/main...m1":       `{"status":"behind"}`,
		"/repos/SirsiMaster/r/compare/main...m3":       `{"status":"ahead"}`,
		"/repos/SirsiMaster/r/actions/runs/5":          `{"status":"completed","conclusion":"success","head_sha":"` + sha40 + `"}`,
		"/repos/SirsiMaster/r/actions/runs/6":          `{"status":"completed","conclusion":"failure","head_sha":"` + sha40 + `"}`,
		"/repos/SirsiMaster/r/git/ref/tags/v1":         `{"object":{"type":"tag","sha":"t1"}}`,
		"/repos/SirsiMaster/r/git/tags/t1":             `{"object":{"sha":"c1"}}`,
		"/repos/SirsiMaster/r/commits/c1/check-runs?1": `{"total_count":1,"check_runs":[` + runs(1, "success") + `]}`,
		"/repos/SirsiMaster/r/git/ref/tags/v2":         `{"object":{"type":"commit","sha":"c2"}}`,
		"/repos/SirsiMaster/r/commits/c2/check-runs?1": `{"total_count":2,"check_runs":[` + runs(1, "success") + `,` + runs(1, "failure") + `]}`,
		"/repos/SirsiMaster/r/git/ref/tags/v3":         `{"object":{"type":"commit","sha":"c3"}}`,
		"/repos/SirsiMaster/r/commits/c3/check-runs?1": `{"total_count":0,"check_runs":[]}`,
		"/repos/SirsiMaster/r/git/ref/tags/v4":         `{"object":{"type":"commit","sha":"c4"}}`, // only skipped/neutral
		"/repos/SirsiMaster/r/commits/c4/check-runs?1": `{"total_count":2,"check_runs":[` + runs(1, "skipped") + `,` + runs(1, "neutral") + `]}`,
		"/repos/SirsiMaster/r/git/ref/tags/v5":         `{"object":{"type":"commit","sha":"c5"}}`, // failure on page 2
		"/repos/SirsiMaster/r/commits/c5/check-runs?1": `{"total_count":101,"check_runs":[` + runs(100, "success") + `]}`,
		"/repos/SirsiMaster/r/commits/c5/check-runs?2": `{"total_count":101,"check_runs":[` + runs(1, "failure") + `]}`,
		"/repos/SirsiMaster/r/git/ref/tags/v6":         `{"object":{"type":"commit","sha":"c6"}}`, // total lies beyond pages
		"/repos/SirsiMaster/r/commits/c6/check-runs?1": `{"total_count":5,"check_runs":[` + runs(1, "success") + `]}`,
		"/repos/SirsiMaster/r/commits/c6/check-runs?2": `{"total_count":5,"check_runs":[]}`,
		"/repos/SirsiMaster/hidden/git/ref/tags/v1":    "", // 404, and the repo is not readable either
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/repos/SirsiMaster/r/pulls/500" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		key := r.URL.Path
		if p := r.URL.Query().Get("page"); p != "" {
			key += "?" + p
		}
		body, ok := api[key]
		if !ok || body == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	g := newGitHubVerifier("tok")
	g.base = srv.URL
	for _, tc := range []struct {
		kind, value string
		want        error
		notFound    bool // a bare 404 that a re-check must not treat as proof
	}{
		{"pr", "SirsiMaster/r#1", nil, false},
		{"pr", "SirsiMaster/r#2", ErrReceiptInvalid, false}, // not merged
		{"pr", "SirsiMaster/r#3", ErrReceiptInvalid, false}, // merge commit not on main
		{"pr", "SirsiMaster/r#4", ErrReceiptInvalid, true},  // 404
		{"pr", "SirsiMaster/r#500", ErrReceiptUnverifiable, false},
		{"ci", "SirsiMaster/r#5@" + sha40, nil, false},
		{"ci", "SirsiMaster/r#6@" + sha40, ErrReceiptInvalid, false},
		{"ci", "SirsiMaster/r#5@" + strings.Repeat("f", 40), ErrReceiptInvalid, false},
		{"ci", "SirsiMaster/r#7@" + sha40, ErrReceiptInvalid, true}, // expired run: bare 404
		{"tag", "SirsiMaster/r@v1", nil, false},                     // annotated tag, green
		{"tag", "SirsiMaster/r@v2", ErrReceiptInvalid, false},       // a red check run
		{"tag", "SirsiMaster/r@v3", ErrReceiptInvalid, false},       // no CI at all
		{"tag", "SirsiMaster/r@v4", ErrReceiptInvalid, false},       // skipped/neutral only: not sufficient
		{"tag", "SirsiMaster/r@v5", ErrReceiptInvalid, false},       // failure on page 2 is found
		{"tag", "SirsiMaster/r@v6", ErrReceiptUnverifiable, false},  // could not read every run
		{"tag", "SirsiMaster/r@gone", ErrReceiptInvalid, false},     // deleted tag in a readable repo: proof
		{"tag", "SirsiMaster/hidden@v1", ErrReceiptInvalid, true},   // 404 with unreadable repo: not proof
	} {
		err := g.Verify(context.Background(), tc.kind, tc.value)
		if (tc.want == nil) != (err == nil) || (tc.want != nil && !errors.Is(err, tc.want)) {
			t.Errorf("%s:%s = %v, want %v", tc.kind, tc.value, err, tc.want)
		}
		if errors.Is(err, errReceiptNotFound) != tc.notFound {
			t.Errorf("%s:%s notFound=%v, want %v (%v)", tc.kind, tc.value, !tc.notFound, tc.notFound, err)
		}
	}
	bad := newGitHubVerifier("wrong")
	bad.base = srv.URL
	if err := bad.Verify(context.Background(), "pr", "SirsiMaster/r#1"); !errors.Is(err, ErrReceiptUnverifiable) {
		t.Errorf("auth failure = %v, want unverifiable", err)
	}
}
