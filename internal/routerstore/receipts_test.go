package routerstore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func useVerifier(t *testing.T, v ReceiptVerifier) {
	t.Helper()
	SetReceiptVerifier(v)
	t.Cleanup(func() { SetReceiptVerifier(nil) })
}

const (
	sha40   = "0123456789abcdef0123456789abcdef01234567"
	prOK    = "pr:SirsiMaster/sirsi-pantheon#1337"
	prBad   = "pr:SirsiMaster/sirsi-pantheon#9"
	ciOK    = "ci:SirsiMaster/sirsi-pantheon#42@" + sha40
	tagOK   = "tag:SirsiMaster/sirsi-pantheon@v0.24.200"
	maatOK  = "maat:run1@sha256:" + "ab" + "cdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	unknown = "pr:SirsiMaster/sirsi-pantheon#77"
)

// markDoneFixture puts a seeded task straight into `done` for tests about how
// other code treats finished work. Production has no such path: done needs a receipt.
func markDoneFixture(t *testing.T, s *SQLiteStore, agent, id string) {
	t.Helper()
	if _, err := s.exec(`UPDATE tasks SET status='done',result_ref=? WHERE agent=? AND task_id=?;`, maatOK, agent, id); err != nil {
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

func TestCompletionNeedsAVerifiedReceipt(t *testing.T) {
	useVerifier(t, fakeVerifier{
		prOK: nil, ciOK: nil, tagOK: nil,
		prBad: fmt.Errorf("%w: not merged", ErrReceiptInvalid),
	})
	for _, tc := range []struct {
		name, receipt string
		want          error // nil = completes
		tier          string
	}{
		{"merged PR", prOK, nil, "router-verified"},
		{"green CI run", ciOK, nil, "router-verified"},
		{"release tag", tagOK, nil, "router-verified"},
		{"maat run is host-attested", maatOK, nil, "host-attested"},
		{"unmerged PR refused", prBad, ErrReceiptInvalid, ""},
		{"unverifiable refused (fail closed)", unknown, ErrReceiptUnverifiable, ""},
		{"free text refused", "proof://whatever", ErrReceiptInvalid, ""},
		{"ci without the commit refused", "ci:SirsiMaster/sirsi-pantheon#42", ErrReceiptInvalid, ""},
		{"owner is not a lane receipt", "owner:someone", ErrReceiptInvalid, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openTestStore(t)
			tok := claimed(t, s, "lane", "t")
			err := s.CompleteTaskLease("lane", "t", tok, tc.receipt)
			got, _ := s.GetTask("lane", "t")
			if tc.want != nil {
				if !errors.Is(err, tc.want) || got.Status == "done" {
					t.Fatalf("err=%v status=%s, want %v and not done", err, got.Status, tc.want)
				}
				return
			}
			if err != nil || got.Status != "done" || got.ResultRef != tc.receipt {
				t.Fatalf("err=%v status=%s ref=%q", err, got.Status, got.ResultRef)
			}
			if raw, ok, _ := s.GetState(receiptKey("lane", "t")); !ok || !strings.Contains(raw, `"tier":"`+tc.tier+`"`) {
				t.Fatalf("receipt record %q", raw)
			}
		})
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

// The owner and cancel paths are not on the Store interface, so the router
// service can never serve them to a remote caller.
func TestOwnerPathsAreNotServed(t *testing.T) {
	for _, m := range []string{"CompleteTaskByOwner", "CancelTask"} {
		if _, ok := storeType.MethodByName(m); ok {
			t.Fatalf("%s is on the Store interface and would be served remotely", m)
		}
	}
}

func TestRecheckReopensOnlyProvenFalseReceipts(t *testing.T) {
	v := fakeVerifier{prOK: nil, ciOK: nil}
	useVerifier(t, v)
	s := openTestStore(t)
	for id, r := range map[string]string{"merged": prOK, "ci": ciOK, "maat": maatOK} {
		if err := s.CompleteTaskLease("lane", id, claimed(t, s, "lane", id), r); err != nil {
			t.Fatal(id, err)
		}
	}
	// The PR is reverted (now invalid); the CI run's API is down (unverifiable).
	v[prOK] = fmt.Errorf("%w: merge commit no longer on main", ErrReceiptInvalid)
	v[ciOK] = fmt.Errorf("%w: GitHub answered 503", ErrReceiptUnverifiable)
	out, err := s.RecheckTaskReceipts(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 { // maat is host-attested: never re-checked
		t.Fatalf("rechecked %d receipts, want 2: %+v", len(out), out)
	}
	merged, _ := s.GetTask("lane", "merged")
	ci, _ := s.GetTask("lane", "ci")
	if merged.Status != "pending" || ci.Status != "done" {
		t.Fatalf("merged=%s (want pending) ci=%s (want done: unknown is not invalid)", merged.Status, ci.Status)
	}
	last := merged.Timeline[len(merged.Timeline)-1]
	if !strings.HasPrefix(last.Label, "receipt no longer valid:") {
		t.Fatalf("timeline note %q", last.Label)
	}
}

func TestGitHubVerifier(t *testing.T) {
	api := map[string]string{
		"/repos/o/r":                       `{"default_branch":"main"}`,
		"/repos/o/r/pulls/1":               `{"merged":true,"merge_commit_sha":"m1"}`,
		"/repos/o/r/pulls/2":               `{"merged":false}`,
		"/repos/o/r/pulls/3":               `{"merged":true,"merge_commit_sha":"m3"}`,
		"/repos/o/r/compare/main...m1":     `{"status":"behind"}`,
		"/repos/o/r/compare/main...m3":     `{"status":"ahead"}`,
		"/repos/o/r/actions/runs/5":        `{"status":"completed","conclusion":"success","head_sha":"` + sha40 + `"}`,
		"/repos/o/r/actions/runs/6":        `{"status":"completed","conclusion":"failure","head_sha":"` + sha40 + `"}`,
		"/repos/o/r/git/ref/tags/v1":       `{"object":{"type":"tag","sha":"t1"}}`,
		"/repos/o/r/git/tags/t1":           `{"object":{"sha":"c1"}}`,
		"/repos/o/r/commits/c1/check-runs": `{"total_count":1,"check_runs":[{"status":"completed","conclusion":"success"}]}`,
		"/repos/o/r/git/ref/tags/v2":       `{"object":{"type":"commit","sha":"c2"}}`,
		"/repos/o/r/commits/c2/check-runs": `{"total_count":1,"check_runs":[{"status":"completed","conclusion":"failure"}]}`,
		"/repos/o/r/git/ref/tags/v3":       `{"object":{"type":"commit","sha":"c3"}}`,
		"/repos/o/r/commits/c3/check-runs": `{"total_count":0,"check_runs":[]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/repos/o/r/pulls/500" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		body, ok := api[r.URL.Path]
		if !ok {
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
	}{
		{"pr", "o/r#1", nil},
		{"pr", "o/r#2", ErrReceiptInvalid},        // not merged
		{"pr", "o/r#3", ErrReceiptInvalid},        // merge commit not on main
		{"pr", "o/r#4", ErrReceiptInvalid},        // 404
		{"pr", "o/r#500", ErrReceiptUnverifiable}, // GitHub down
		{"ci", "o/r#5@" + sha40, nil},
		{"ci", "o/r#6@" + sha40, ErrReceiptInvalid},                   // failed
		{"ci", "o/r#5@" + strings.Repeat("f", 40), ErrReceiptInvalid}, // different commit
		{"tag", "o/r@v1", nil},                                        // annotated tag, green
		{"tag", "o/r@v2", ErrReceiptInvalid},                          // red check run
		{"tag", "o/r@v3", ErrReceiptInvalid},                          // no CI at all
	} {
		err := g.Verify(context.Background(), tc.kind, tc.value)
		if (tc.want == nil) != (err == nil) || (tc.want != nil && !errors.Is(err, tc.want)) {
			t.Errorf("%s:%s = %v, want %v", tc.kind, tc.value, err, tc.want)
		}
	}
	bad := newGitHubVerifier("wrong")
	bad.base = srv.URL
	if err := bad.Verify(context.Background(), "pr", "o/r#1"); !errors.Is(err, ErrReceiptUnverifiable) {
		t.Errorf("auth failure = %v, want unverifiable", err)
	}
}
