package routerstore

import (
	"context"
	"testing"
	"time"
)

// The hosted board with @ledger-editor edits any lane's items and tasks for the signed-in owner, but it still
// cannot send, administer tokens, or drive another lane's threads (owner decision 2026-11-02).
func TestLedgerEditorEditsAnyLaneButCannotSendOrAdminister(t *testing.T) {
	f := newScopedFixture(t, []string{"router-board", ObserverScope, LedgerEditorScope})
	toRa, err := f.backend.Send("mac-lane", "ra", "for ra", "decision", "body")
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.backend.Send("mac-lane", "codex-pantheon", "for codex", "proposal", "body")
	if err != nil {
		t.Fatal(err)
	}
	rs := f.client("router-board")

	if _, err := rs.Get(toRa); err != nil {
		t.Fatalf("Get(ra item) refused: %v", err)
	}
	if err := rs.CloseItem(toRa, "board:owner@sirsi.ai: done"); err != nil {
		t.Fatalf("CloseItem on ra's item refused: %v", err)
	}
	if err := rs.ReassignItem(other, "codex-pantheon", "ra", "board:owner@sirsi.ai: moved"); err != nil {
		t.Fatalf("ReassignItem refused: %v", err)
	}
	if err := rs.AddTask(Task{Agent: "ra", TaskID: "board-t1", Subject: "from the board", CommissionedBy: "board:owner@sirsi.ai"}); err != nil {
		t.Fatalf("AddTask on ra refused: %v", err)
	}
	if _, err := rs.UpdateTask("ra", "board-t1", TaskUpdate{ResponsibleParty: "owner"}); err != nil {
		t.Fatalf("UpdateTask refused: %v", err)
	}
	if _, err := rs.UpdateTask("ra", "board-t1", TaskUpdate{Status: "blocked"}); err != nil {
		t.Fatalf("UpdateTask relabel to blocked refused: %v", err)
	}
	if got, err := f.backend.GetTask("ra", "board-t1"); err != nil || got.CommissionedBy != "board:owner@sirsi.ai" || got.Status != "blocked" {
		t.Fatalf("task = %+v, %v", got, err)
	}

	// A task lease stays bound to the lane's own thread: the editor cannot claim on another lane's behalf.
	_, err = rs.ClaimTask("ra", "board-t1", "board:owner@sirsi.ai", "thr-board", 10*time.Minute)
	wantLaneScope(t, "ClaimTask on ra's task", err)

	// Still closed: messaging, token administration, other lanes' threads, deletion paths.
	_, err = rs.Send("router-board", "ra", "x", "proposal", "b")
	wantLaneScope(t, "Send", err)
	wantLaneScope(t, "ForceOwner", rs.ForceOwner(other, "close", "r"))
	wantLaneScope(t, "UpsertThreads for ra", rs.UpsertThreads([]ThreadRecord{{ThreadID: "thr-x", Agent: "ra", Status: "active", Host: f.host}}))
	_, err = rs.ListAll(context.Background())
	wantLaneScope(t, "ListAll", err)
	_, err = rs.Counters()
	wantLaneScope(t, "Counters", err)

	// The marker is a capability, never an identity.
	_, err = f.client(LedgerEditorScope).Get(other)
	wantLaneScope(t, "session as @ledger-editor", err)
}

// Negative control: the same calls with only @observer stay refused, so the edits come from the new marker.
func TestObserverWithoutLedgerEditorStillCannotEdit(t *testing.T) {
	f := newScopedFixture(t, []string{"router-board", ObserverScope})
	toRa, err := f.backend.Send("mac-lane", "ra", "for ra", "decision", "body")
	if err != nil {
		t.Fatal(err)
	}
	rs := f.client("router-board")
	wantLaneScope(t, "CloseItem", rs.CloseItem(toRa, "x"))
	wantLaneScope(t, "ReassignItem", rs.ReassignItem(toRa, "ra", "codex-pantheon", "x"))
	wantLaneScope(t, "AddTask on ra", rs.AddTask(Task{Agent: "ra", TaskID: "t", Subject: "t"}))
}
