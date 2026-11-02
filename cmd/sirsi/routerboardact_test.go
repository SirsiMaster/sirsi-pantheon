package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestBoardActEditsAreAttributedToTheSignedInAccount(t *testing.T) {
	st, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	t.Setenv("SIRSI_THREAD_ID", "thr-board")
	const actor = "board:owner@sirsi.ai"
	run := func(verb, body string) any {
		t.Helper()
		out, err := runBoardAct(st, actor, verb, []byte(body))
		if err != nil {
			t.Fatalf("%s %s: %v", verb, body, err)
		}
		return out
	}
	closeMe, _ := st.Send("mac-lane", "ra", "close me", "decision", "b")
	convertMe, _ := st.Send("mac-lane", "ra", "make me a task", "proposal", "b")
	moveMe, _ := st.Send("mac-lane", "ra", "move me", "proposal", "b")

	run("close", `{"id":"`+closeMe+`","note":"handled"}`)
	if it, _ := st.Get(closeMe); it.Status != "closed" || !strings.HasPrefix(it.Result, actor+": handled") {
		t.Fatalf("close = %+v", it)
	}
	run("convert", `{"id":"`+convertMe+`","task_id":"from-item"}`)
	task, err := st.GetTask("ra", "from-item")
	if err != nil || task.Subject != "make me a task" || task.CommissionedBy != actor {
		t.Fatalf("converted task = %+v, %v", task, err)
	}
	if it, _ := st.Get(convertMe); it.Status != "closed" || !strings.Contains(it.Result, "ra/from-item") {
		t.Fatalf("converted item = %+v", it)
	}
	run("reassign-item", `{"id":"`+moveMe+`","to":"codex-pantheon","note":"theirs"}`)
	if it, _ := st.Get(moveMe); it.To != "codex-pantheon" {
		t.Fatalf("reassign-item left it with %q", it.To)
	}
	run("add-task", `{"agent":"codex-pantheon","task_id":"t2","subject":"board task"}`)
	run("reassign-task", `{"agent":"codex-pantheon","task_id":"t2","responsible":"owner"}`)
	t2, _ := st.GetTask("codex-pantheon", "t2")
	if t2.ResponsibleParty != "owner" || len(t2.Timeline) != 1 || t2.Timeline[0].Owner != actor {
		t.Fatalf("reassign-task = %+v", t2)
	}
	run("update-task", `{"agent":"codex-pantheon","task_id":"t2","status":"blocked","blocked_by":"waiting on owner"}`)
	if t2, _ = st.GetTask("codex-pantheon", "t2"); t2.Status != "blocked" || t2.BlockedBy != "waiting on owner" || len(t2.Timeline) != 2 {
		t.Fatalf("update-task = %+v", t2)
	}
	// Executing work stays with the lane's own fenced lease.
	if _, err := runBoardAct(st, actor, "update-task", []byte(`{"agent":"codex-pantheon","task_id":"t2","status":"done"}`)); err == nil {
		t.Error("board marked a task done without a lease")
	}

	// Refusals: no verb for sending, deleting or holding a lease; required fields enforced.
	for _, verb := range []string{"send", "delete", "token-mint", "claim", "complete"} {
		if _, err := runBoardAct(st, actor, verb, []byte(`{}`)); !errors.Is(err, dashboard.ErrBoardVerb) {
			t.Errorf("%s = %v, want ErrBoardVerb", verb, err)
		}
	}
	if _, err := runBoardAct(st, actor, "close", []byte(`{}`)); err == nil {
		t.Error("close without id accepted")
	}
}
