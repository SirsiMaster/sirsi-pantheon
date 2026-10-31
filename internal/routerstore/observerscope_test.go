package routerstore

import (
	"context"
	"testing"
)

// The hosted router board holds a token scoped to its own agent plus ObserverScope: it reads the whole ledger
// and registers only its own thread. It can never send, close, claim, administer, or act as the marker.
func TestObserverReadsWholeLedgerButMutatesNothing(t *testing.T) {
	f := newScopedFixture(t, []string{"router-board", ObserverScope})
	ctx := context.Background()
	toRa, err := f.backend.Send("mac-lane", "ra", "private to ra", "decision", "body")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.backend.UpsertThreads([]ThreadRecord{{ThreadID: "thr-ra", Agent: "ra", Status: "active", Host: "m1", Payload: []byte(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	rs := f.client("router-board")

	if items, err := rs.ListActive(ctx); err != nil || len(items) != 1 {
		t.Fatalf("ListActive = %d items, %v; want the whole ledger", len(items), err)
	}
	if _, err := rs.CountClosed(ctx); err != nil {
		t.Fatalf("CountClosed refused: %v", err)
	}
	if items, err := rs.Inbox("ra"); err != nil || len(items) != 1 {
		t.Fatalf("Inbox(ra) = %d, %v; an observer reads every mailbox", len(items), err)
	}
	threads, err := rs.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads refused: %v", err)
	}
	foreign := false
	for _, r := range threads {
		foreign = foreign || r.Agent == "ra"
	}
	if !foreign {
		t.Fatalf("ListThreads hid ra's thread from the observer: %+v", threads)
	}
	if _, err := rs.ListTasks("ra"); err != nil {
		t.Fatalf("ListTasks(ra) refused: %v", err)
	}
	if _, err := rs.OperationalAgents(); err != nil {
		t.Fatalf("OperationalAgents refused: %v", err)
	}
	if err := rs.UpsertThreads([]ThreadRecord{{ThreadID: "thr-board", Agent: "router-board", Status: "active", Host: f.host, Payload: []byte(`{}`)}}); err != nil {
		t.Fatalf("registering its own thread refused: %v", err)
	}

	// Every mutation is closed, including the lane API's work verbs for its own agent.
	_, err = rs.Send("router-board", "ra", "x", "proposal", "b")
	wantLaneScope(t, "Send as itself", err)
	wantLaneScope(t, "CloseItem", rs.CloseItem(toRa, "x"))
	wantLaneScope(t, "AddTask", rs.AddTask(Task{TaskID: "t", Agent: "router-board", Subject: "t"}))
	wantLaneScope(t, "ForceOwner", rs.ForceOwner(toRa, "close", "r"))
	wantLaneScope(t, "UpsertThreads for ra", rs.UpsertThreads([]ThreadRecord{{ThreadID: "thr-x", Agent: "ra", Status: "active", Host: f.host}}))
	_, err = rs.Counters()
	wantLaneScope(t, "Counters", err)
	_, err = rs.ListAll(ctx)
	wantLaneScope(t, "ListAll (not an observer read)", err)

	// The marker is a capability, never an identity.
	_, err = f.client(ObserverScope).Inbox("ra")
	wantLaneScope(t, "session as @observer", err)
	wantLaneScope(t, "thread for @observer", rs.UpsertThreads([]ThreadRecord{{ThreadID: "thr-o", Agent: ObserverScope, Status: "active", Host: f.host}}))
}

// Without the marker a scoped token keeps today's lane confinement exactly.
func TestScopedTokenWithoutObserverCannotReadTheLedger(t *testing.T) {
	f := newScopedFixture(t, []string{"router-board"})
	_, err := f.client("router-board").ListActive(context.Background())
	wantLaneScope(t, "ListActive without observer", err)
	_, err = f.client("router-board").Inbox("ra")
	wantLaneScope(t, "Inbox(ra) without observer", err)
}
