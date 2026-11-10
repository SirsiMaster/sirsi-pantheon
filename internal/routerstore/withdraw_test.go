package routerstore

import (
	"errors"
	"strings"
	"testing"
)

func TestWithdrawTaskNeedsALiveSuccessorAndNeverCountsAsDone(t *testing.T) {
	s := openTestStore(t)
	for _, id := range []string{"old", "new", "gone", "dropped"} {
		if err := s.AddTask(Task{Agent: "lane", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CancelTask("lane", "dropped", "board:owner@sirsi.ai", "not needed"); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"no reason":          s.WithdrawTask("lane", "old", "lane/new", "lane@m1", ""),
		"no successor":       s.WithdrawTask("lane", "old", "", "lane@m1", "dup"),
		"self":               s.WithdrawTask("lane", "old", "lane/old", "lane@m1", "dup"),
		"missing successor":  s.WithdrawTask("lane", "old", "lane/nope", "lane@m1", "dup"),
		"canceled successor": s.WithdrawTask("lane", "old", "lane/dropped", "lane@m1", "dup"),
	} {
		if err == nil {
			t.Errorf("%s: withdraw allowed", name)
		}
	}
	if err := s.WithdrawTask("lane", "old", "lane/new", "lane@m1", "duplicate of new"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTask("lane", "old")
	if got.Status != "withdrawn" || got.ResultRef != "superseded:lane/new" {
		t.Fatalf("withdrawn task = %s %q", got.Status, got.ResultRef)
	}
	if n := len(got.Timeline); n == 0 || !strings.Contains(got.Timeline[n-1].Label, "lane/new") || got.Timeline[n-1].Owner != "lane@m1" {
		t.Fatalf("no audit entry: %+v", got.Timeline)
	}
	if _, ok, _ := s.GetState(receiptKey("lane", "old")); ok {
		t.Fatal("a withdraw recorded a completion receipt")
	}
	// Terminal: no second withdraw, no owner sign-off, no update back to open, and
	// nothing may name it as its successor.
	if err := s.WithdrawTask("lane", "old", "lane/new", "lane@m1", "again"); err == nil {
		t.Fatal("withdrew a withdrawn task")
	}
	if err := s.CompleteTaskByOwner("lane", "old", "board:owner@sirsi.ai", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("owner sign-off on a withdrawn task: %v", err)
	}
	if err := s.WithdrawTask("lane", "gone", "lane/old", "lane@m1", "chain"); err == nil {
		t.Fatal("superseded by a withdrawn task")
	}
}

// Over the wire a lane withdraws only its own task, and the audit names its session, not a client-supplied actor.
func TestWithdrawTaskOverTheWireIsOwnLaneOnly(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a", "cloud-b"})
	for _, tk := range []Task{{Agent: "cloud-a", TaskID: "dup", Subject: "dup"}, {Agent: "cloud-a", TaskID: "keep", Subject: "keep"}, {Agent: "cloud-b", TaskID: "theirs", Subject: "theirs"}} {
		if err := f.backend.AddTask(tk); err != nil {
			t.Fatal(err)
		}
	}
	a := f.client("cloud-a")
	wantLaneScope(t, "another lane's task", a.WithdrawTask("cloud-b", "theirs", "cloud-a/keep", "cloud-a", "mine now"))
	if err := a.WithdrawTask("cloud-a", "dup", "cloud-a/keep", "board:owner@sirsi.ai", "duplicate"); err != nil {
		t.Fatalf("own withdraw refused: %v", err)
	}
	got, _ := f.backend.GetTask("cloud-a", "dup")
	if got.Status != "withdrawn" {
		t.Fatalf("dup = %s", got.Status)
	}
	if owner := got.Timeline[len(got.Timeline)-1].Owner; !strings.HasPrefix(owner, "cloud-a@") {
		t.Fatalf("audit actor %q is not the authenticated session", owner)
	}
	if theirs, _ := f.backend.GetTask("cloud-b", "theirs"); theirs.Status != "pending" {
		t.Fatalf("theirs = %s", theirs.Status)
	}
}
