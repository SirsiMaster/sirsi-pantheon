package routerstore

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// fakePR is one PR on the fake GitHub: merged state for REST, metadata for GraphQL.
type fakePR struct {
	merged       bool
	mergedAt     string
	lastEditedAt string // "" = never edited
	body         string
	gqlStatus    int // 0 = 200
	gqlErrors    bool
	gqlUnmerged  bool
}

func fakeGitHub(t *testing.T, prs map[int]*fakePR) *githubVerifier {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const pulls = "/repos/SirsiMaster/sirsi-pantheon/pulls/"
		switch {
		case strings.HasPrefix(r.URL.Path, pulls):
			n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, pulls))
			pr := prs[n]
			if pr == nil {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"merged": pr.merged, "merge_commit_sha": sha40})
		case r.URL.Path == "/repos/SirsiMaster/sirsi-pantheon":
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		case strings.HasPrefix(r.URL.Path, "/repos/SirsiMaster/sirsi-pantheon/compare/"):
			_, _ = w.Write([]byte(`{"status":"behind"}`))
		case r.URL.Path == "/graphql" && r.Method == http.MethodPost:
			var req struct {
				Variables struct {
					N int `json:"n"`
				} `json:"variables"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			pr := prs[req.Variables.N]
			if pr == nil || pr.gqlStatus != 0 {
				code := http.StatusNotFound
				if pr != nil {
					code = pr.gqlStatus
				}
				http.Error(w, "no", code)
				return
			}
			if pr.gqlErrors {
				_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}],"data":null}`))
				return
			}
			node := map[string]any{"merged": pr.merged && !pr.gqlUnmerged, "body": pr.body, "mergedAt": nil, "lastEditedAt": nil}
			if pr.mergedAt != "" {
				node["mergedAt"] = pr.mergedAt
			}
			if pr.lastEditedAt != "" {
				node["lastEditedAt"] = pr.lastEditedAt
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": node}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return &githubVerifier{token: "t", base: srv.URL, http: srv.Client()}
}

func prRef(n int) string { return "pr:SirsiMaster/sirsi-pantheon#" + strconv.Itoa(n) }

const (
	t0 = "2026-11-20T10:00:00Z" // merge
	tB = "2026-11-20T09:00:00Z" // edited before merge
	tA = "2026-11-21T09:00:00Z" // edited after merge
)

// PR #1421's own wording must work.
const body1421 = "Fixes.\n\nTask `router-findcallerreporoot-unbounded-git`.\nTask `scoped-mint-cleanup-reporting`.\nTask `ci-routerstore-suite-near-timeout`.\n"

func TestSharedPRReceipt(t *testing.T) {
	const first = "router-findcallerreporoot-unbounded-git"
	for _, tc := range []struct {
		name   string
		pr     fakePR
		second string
		want   string // "" = completes; else substring of the refusal
	}{
		{"named", fakePR{merged: true, mergedAt: t0, lastEditedAt: tB, body: body1421}, "scoped-mint-cleanup-reporting", ""},
		{"named, never edited", fakePR{merged: true, mergedAt: t0, body: body1421}, "ci-routerstore-suite-near-timeout", ""},
		{"closes form", fakePR{merged: true, mergedAt: t0, body: "Closes task other-task, done."}, "other-task", ""},
		{"sentence period", fakePR{merged: true, mergedAt: t0, body: "Task other-task."}, "other-task", ""},
		{"unnamed", fakePR{merged: true, mergedAt: t0, body: body1421}, "unrelated", "already used by task lane/" + first},
		{"id only in prose, no Task form", fakePR{merged: true, mergedAt: t0, body: "see other-task for context"}, "other-task", "already used by task"},
		{"id is a prefix of the named one", fakePR{merged: true, mergedAt: t0, body: "Task `foo-bar`"}, "foo", "already used by task"},
		{"id is a suffix of the named one", fakePR{merged: true, mergedAt: t0, body: "Task `xfoo` and Task bar-foo"}, "foo", "already used by task"},
		{"edited after merge", fakePR{merged: true, mergedAt: t0, lastEditedAt: tA, body: body1421}, "scoped-mint-cleanup-reporting", "edited at or after merge"},
		{"edited at merge instant", fakePR{merged: true, mergedAt: t0, lastEditedAt: t0, body: body1421}, "scoped-mint-cleanup-reporting", "edited at or after merge"},
		{"metadata http error", fakePR{merged: true, mergedAt: t0, body: body1421, gqlStatus: 502}, "scoped-mint-cleanup-reporting", "metadata unavailable"},
		{"metadata graphql errors", fakePR{merged: true, mergedAt: t0, body: body1421, gqlErrors: true}, "scoped-mint-cleanup-reporting", "metadata unavailable"},
		{"no merge time", fakePR{merged: true, body: body1421}, "scoped-mint-cleanup-reporting", "not merged"},
		{"GraphQL says unmerged", fakePR{merged: true, gqlUnmerged: true, mergedAt: t0, body: body1421}, "scoped-mint-cleanup-reporting", "not merged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pr := tc.pr
			useVerifier(t, fakeGitHub(t, map[int]*fakePR{1421: {merged: true, mergedAt: t0, body: "x"}, 7: &pr}))
			s := openTestStore(t)
			// First use is today's behavior: no naming check, even for a body that names nothing.
			ref := prRef(7)
			if err := s.CompleteTaskLease("lane", first, claimed(t, s, "lane", first), ref); err != nil {
				t.Fatal(err)
			}
			err := s.CompleteTaskLease("lane", tc.second, claimed(t, s, "lane", tc.second), ref)
			got, _ := s.GetTask("lane", tc.second)
			if tc.want == "" {
				if err != nil || got.Status != "done" || got.ResultRef != ref {
					t.Fatalf("err=%v status=%s ref=%q", err, got.Status, got.ResultRef)
				}
				if raw, _, _ := s.GetState(receiptKey("lane", tc.second)); !strings.Contains(raw, "names task "+tc.second) {
					t.Fatalf("shared receipt not annotated: %s", raw)
				}
				return
			}
			if !errors.Is(err, ErrReceiptInvalid) || !strings.Contains(err.Error(), tc.want) || got.Status == "done" {
				t.Fatalf("err=%v status=%s, want invalid containing %q", err, got.Status, tc.want)
			}
			if !strings.Contains(err.Error(), "already used by task") { // every refusal keeps the familiar message
				t.Fatalf("refusal lost the current message: %v", err)
			}
		})
	}
}

// The first use of an unmerged PR is refused by Verify, so reuse never gets that far.
func TestSharedPRReceiptUnmergedRefused(t *testing.T) {
	useVerifier(t, fakeGitHub(t, map[int]*fakePR{9: {merged: false, body: "Task `a`"}}))
	s := openTestStore(t)
	err := s.CompleteTaskLease("lane", "a", claimed(t, s, "lane", "a"), prRef(9))
	if !errors.Is(err, ErrReceiptInvalid) || !strings.Contains(err.Error(), "not merged") {
		t.Fatalf("unmerged: %v", err)
	}
}

// ci:, tag: and maat: receipts stay single-use, and a verifier that cannot read
// PR descriptions never permits reuse of a pr receipt.
func TestOnlyPRReceiptsAreShareable(t *testing.T) {
	useVerifier(t, fakeVerifier{ciOK: nil, tagOK: nil, prOK: nil})
	s := openTestStore(t)
	run := maatRun(t, s, "res-share", "lane", 0)
	for i, r := range []string{ciOK, tagOK, run, prOK} {
		a, b := "a"+strconv.Itoa(i), "b"+strconv.Itoa(i)
		if err := s.CompleteTaskLease("lane", a, claimed(t, s, "lane", a), r); err != nil {
			t.Fatal(r, err)
		}
		err := s.CompleteTaskLease("lane", b, claimed(t, s, "lane", b), r)
		if !errors.Is(err, ErrReceiptInvalid) || !strings.Contains(err.Error(), "already used by task lane/"+a) {
			t.Fatalf("%s reused: %v", r, err)
		}
		if got, _ := s.GetTask("lane", b); got.Status == "done" {
			t.Fatalf("%s completed a second task", r)
		}
	}
}

// A ci receipt is refused even when the PR-shaped naming path would say yes.
func TestSharedCIReceiptRefusedWithNamingVerifier(t *testing.T) {
	v := fakeGitHub(t, map[int]*fakePR{1: {merged: true, mergedAt: t0, body: "Task `b`"}})
	useVerifier(t, namingFake{v, fakeVerifier{ciOK: nil}})
	// namingFake can name tasks, so only the receipt kind keeps ci: single-use.
	s := openTestStore(t)
	if err := s.CompleteTaskLease("lane", "a", claimed(t, s, "lane", "a"), ciOK); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteTaskLease("lane", "b", claimed(t, s, "lane", "b"), ciOK); !errors.Is(err, ErrReceiptInvalid) {
		t.Fatalf("ci reuse: %v", err)
	}
}

// namingFake verifies through fakeVerifier but names tasks through the fake GitHub.
type namingFake struct {
	*githubVerifier
	fakeVerifier
}

func (n namingFake) Verify(ctx context.Context, kind, value string) error {
	return n.fakeVerifier.Verify(ctx, kind, value)
}

func TestRecheckPassesForEveryTaskSharingAPR(t *testing.T) {
	v := fakeGitHub(t, map[int]*fakePR{1421: {merged: true, mergedAt: t0, lastEditedAt: tB, body: body1421}})
	useVerifier(t, v)
	s := openTestStore(t)
	ids := []string{"router-findcallerreporoot-unbounded-git", "scoped-mint-cleanup-reporting", "ci-routerstore-suite-near-timeout"}
	for _, id := range ids {
		if err := s.CompleteTaskLease("lane", id, claimed(t, s, "lane", id), prRef(1421)); err != nil {
			t.Fatal(id, err)
		}
	}
	out, err := s.RecheckTaskReceipts(0)
	if err != nil || len(out) != 3 {
		t.Fatalf("recheck: %v %+v", err, out)
	}
	for _, o := range out {
		if o.Reopened || o.Error != "" {
			t.Fatalf("recheck disturbed %s: %+v", o.TaskID, o)
		}
	}
	for _, id := range ids {
		if got, _ := s.GetTask("lane", id); got.Status != "done" {
			t.Fatalf("%s = %s", id, got.Status)
		}
	}
}

func TestBodyNamesTask(t *testing.T) {
	for body, want := range map[string]bool{
		"Task `foo`.":                 true,
		"task `foo`":                  true,
		"Closes task foo":             true,
		"Closes task foo.":            true,
		"Task: foo\nnext":             true,
		"Task `foo-bar`":              false,
		"Task `xfoo`":                 false,
		"Task xfoo":                   false,
		"Task foo-bar":                false,
		"Task foo.bar":                false,
		"Task foo_bar":                false,
		"foo":                         false,
		"see foo here":                false,
		"Subtask foo":                 false,
		"Task `foo":                   false,
		"Task bar, and also Task foo": true,
	} {
		if got := bodyNamesTask(body, "foo"); got != want {
			t.Errorf("bodyNamesTask(%q, foo) = %v, want %v", body, got, want)
		}
	}
}
