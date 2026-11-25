package routerstore

import "testing"

// A lane whose only tasks are canceled or withdrawn has no nonterminal work and must not stay enumerated.
func TestOperationalAgentsIgnoresCanceledAndWithdrawnTasks(t *testing.T) {
	store, _ := remoteHarness(t)
	backend := store.(*SQLiteStore)
	for _, a := range []string{"closed-lane", "live-lane"} {
		if err := backend.AddTask(Task{Agent: a, TaskID: "t", Subject: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := backend.db.Exec(`UPDATE tasks SET status='withdrawn' WHERE agent='closed-lane'`); err != nil {
		t.Fatal(err)
	}
	got, err := backend.OperationalAgents()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range got {
		if a == "closed-lane" {
			t.Fatalf("withdrawn-only lane still operational: %v", got)
		}
	}
	found := false
	for _, a := range got {
		found = found || a == "live-lane"
	}
	if !found {
		t.Fatalf("pending lane missing: %v", got)
	}
}
