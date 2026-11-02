package routerstore

import (
	"errors"
	"strings"
	"testing"
)

func sendItem(t *testing.T, s *SQLiteStore, from, to, title string) string {
	t.Helper()
	id, err := s.Send(from, to, title, "proposal", "do the thing")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestConvertItemToTaskClosesAndAddsTask(t *testing.T) {
	s := newTestStore(t)
	id := sendItem(t, s, "mercury", "ra", "Schedule an M5 window")
	task, err := s.ConvertItemToTask(id, "ra", "", "ra@m1")
	if err != nil {
		t.Fatal(err)
	}
	if task.TaskID != id || task.Subject != "Schedule an M5 window" || task.Status != "pending" || task.CommissionedBy != "mercury" {
		t.Fatalf("task = %+v", task)
	}
	if task.Outline == nil || !strings.Contains(*task.Outline, id) || !strings.Contains(*task.Outline, "do the thing") {
		t.Fatalf("outline must reference the item and carry its instructions: %v", task.Outline)
	}
	it, _ := s.Get(id)
	if it.Status != "closed" || it.Result != "converted to task ra/"+id+" by ra@m1" {
		t.Fatalf("item = %s %q", it.Status, it.Result)
	}
	// Idempotent: a re-run returns the same task and adds nothing.
	for _, tid := range []string{"", id} {
		again, err := s.ConvertItemToTask(id, "ra", tid, "ra@m1")
		if err != nil || again.TaskID != id {
			t.Fatalf("re-run(%q) = %+v, %v", tid, again, err)
		}
	}
	if tasks, _ := s.ListTasks("ra"); len(tasks) != 1 {
		t.Fatalf("re-run duplicated the task: %d tasks", len(tasks))
	}
	// A re-run naming a different task or another ledger is refused, not answered with this task.
	if _, err := s.ConvertItemToTask(id, "ra", "other", "ra@m1"); !errors.Is(err, ErrAlreadyClosed) {
		t.Fatalf("different task id: err = %v, want ErrAlreadyClosed", err)
	}
	if _, err := s.ConvertItemToTask(id, "codex-pantheon", "", "x"); !errors.Is(err, ErrAlreadyClosed) {
		t.Fatalf("different ledger: err = %v, want ErrAlreadyClosed", err)
	}
}

func TestConvertItemToTaskRejectsUnsafeTaskIDs(t *testing.T) {
	s := newTestStore(t)
	id := sendItem(t, s, "mercury", "ra", "Bad id")
	for _, tid := range []string{"a/b", "a b", "a\tb"} {
		if _, err := s.ConvertItemToTask(id, "ra", tid, "ra@m1"); err == nil {
			t.Fatalf("task id %q accepted", tid)
		}
	}
	if it, _ := s.Get(id); it.Status != "open" {
		t.Fatal("a rejected task id closed the item")
	}
}

// Regression (independent review of #1338): the idempotent path once trusted the item's free-text close
// result. A scoped lane could close its OWN item with "converted to task ra/<task> by x" and then call
// ConvertItemToTask to read another lane's task. The link is now a state row only the convert writes.
func TestRemoteConvertCannotBeForgedThroughCloseResult(t *testing.T) {
	f := newSSAFixture(t, []string{"cloud-a"})
	if err := f.backend.AddTask(Task{Agent: "ra", TaskID: "victim", Subject: "RA PRIVATE TASK"}); err != nil {
		t.Fatal(err)
	}
	mine := sendItem(t, f.backend, "ra", "cloud-a", "Mine")
	rs := f.client()
	if err := rs.CloseItem(mine, "converted to task ra/victim by x"); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"cloud-a", "ra"} {
		got, err := rs.ConvertItemToTask(mine, agent, "", "cloud-a")
		if err == nil || got.Subject == "RA PRIVATE TASK" {
			t.Fatalf("CONFIDENTIALITY BREACH: forged close result returned %+v (agent %s, err %v)", got, agent, err)
		}
	}
}

func TestConvertItemToTaskIsAtomic(t *testing.T) {
	s := newTestStore(t)
	id := sendItem(t, s, "mercury", "ra", "Half-done convert")
	convertAfterTaskHook = func() error { return errors.New("injected crash between task insert and item close") }
	_, err := s.ConvertItemToTask(id, "ra", "t1", "ra@m1")
	convertAfterTaskHook = nil
	if err == nil {
		t.Fatal("injected failure did not surface")
	}
	if _, err := s.GetTask("ra", "t1"); err == nil {
		t.Fatal("task survived a failed convert")
	}
	if it, _ := s.Get(id); it.Status != "open" {
		t.Fatalf("item = %s after a failed convert, want open", it.Status)
	}
	// A clash on the task id also leaves the item open.
	if err := s.AddTask(Task{Agent: "ra", TaskID: "taken", Subject: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConvertItemToTask(id, "ra", "taken", "ra@m1"); !errors.Is(err, ErrTaskExists) {
		t.Fatalf("clash = %v, want ErrTaskExists", err)
	}
	if it, _ := s.Get(id); it.Status != "open" {
		t.Fatal("task-id clash closed the item")
	}
}

func TestConvertItemToTaskRefusesAClosedItem(t *testing.T) {
	s := newTestStore(t)
	id := sendItem(t, s, "mercury", "ra", "Already answered")
	if err := s.CloseItem(id, "answered"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConvertItemToTask(id, "ra", "", "ra@m1"); !errors.Is(err, ErrAlreadyClosed) {
		t.Fatalf("err = %v, want ErrAlreadyClosed", err)
	}
	if _, err := s.ConvertItemToTask("no-such-item", "ra", "", "ra@m1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing item err = %v, want ErrNotFound", err)
	}
}

// Over the wire, a scoped lane converts only items addressed to it, onto its own ledger, and the
// close result names the authenticated session, not whatever actor the client sent.
func TestRemoteConvertItemToTaskIsLaneScoped(t *testing.T) {
	f := newSSAFixture(t, []string{"cloud-a"})
	mine := sendItem(t, f.backend, "ra", "cloud-a", "Mine")
	theirs := sendItem(t, f.backend, "cloud-a", "ra", "Theirs")
	rs := f.client()

	// The wire carries the refusal as text (HTTP 403 ErrLaneScope), not the sentinel value.
	if _, err := rs.ConvertItemToTask(theirs, "cloud-a", "", "cloud-a"); err == nil || !strings.Contains(err.Error(), ErrLaneScope.Error()) {
		t.Fatalf("foreign item: err = %v, want ErrLaneScope", err)
	}
	if _, err := rs.ConvertItemToTask(mine, "ra", "", "cloud-a"); err == nil || !strings.Contains(err.Error(), ErrLaneScope.Error()) {
		t.Fatalf("foreign ledger: err = %v, want ErrLaneScope", err)
	}
	for _, id := range []string{mine, theirs} {
		if it, _ := f.backend.Get(id); it.Status != "open" {
			t.Fatalf("refused convert changed %s to %s", id, it.Status)
		}
	}
	task, err := rs.ConvertItemToTask(mine, "cloud-a", "", "forged-actor")
	if err != nil {
		t.Fatal(err)
	}
	it, _ := f.backend.Get(mine)
	if task.Agent != "cloud-a" || strings.Contains(it.Result, "forged-actor") || !strings.Contains(it.Result, "by cloud-a@") {
		t.Fatalf("task %+v, result %q: actor must come from the session", task, it.Result)
	}
}
