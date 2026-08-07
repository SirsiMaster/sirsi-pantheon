package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Both directions (A35 question 4): a clean ledger must produce zero findings,
// and each rot shape must be caught AND land in the right bucket. The two
// buckets have different remedies, so a contradiction reported as a stall is a
// wrong answer, not a near miss.
func TestFindLedgerRot(t *testing.T) {
	tasks := []routerstore.Task{
		// clean: active, ordinary subject
		{Agent: "a", TaskID: "clean", Subject: "wire the board", Status: "in-progress", Liveness: "active"},
		// done rows are never findings, even when stale-looking
		{Agent: "a", TaskID: "finished", Subject: "PR #1 MERGED", Status: "done", Liveness: "stalled"},
		// contradiction: body shouts a verdict, status disagrees
		{Agent: "a", TaskID: "rotted", Subject: "PR #506 MERGED this pass", Status: "in-progress", Liveness: "active"},
		// stalled: no verdict in the body, just untouched
		{Agent: "b", TaskID: "stale", Subject: "audit the runners", Status: "pending", Liveness: "stalled"},
		// blocked rows derive liveness=blocked, so they are not stalls
		{Agent: "b", TaskID: "blocked", Subject: "waiting on codex", Status: "pending", Liveness: "blocked"},
		// lowercase prose must NOT trip the verdict matcher
		{Agent: "b", TaskID: "prose", Subject: "blocked on the merged branch, work is done soon", Status: "in-progress", Liveness: "active"},
		// a row that DISCUSSES verdict words past the window is not asserting
		// one — this is the live false positive the window was added to kill.
		{Agent: "b", TaskID: "describes", Subject: "no doctor check on stalled rows; bodies read DONE/MERGED while status stayed pending", Status: "pending", Liveness: "active"},
	}

	stalled, contradicted := findLedgerRot(tasks)

	if len(contradicted) != 1 || contradicted[0].TaskID != "rotted" {
		t.Fatalf("contradicted = %+v, want exactly [rotted]", contradicted)
	}
	if len(stalled) != 1 || stalled[0].TaskID != "stale" {
		t.Fatalf("stalled = %+v, want exactly [stale]", stalled)
	}
}

func TestFindLedgerRotCleanLedgerIsSilent(t *testing.T) {
	stalled, contradicted := findLedgerRot([]routerstore.Task{
		{Agent: "a", TaskID: "one", Subject: "build it", Status: "in-progress", Liveness: "active"},
		{Agent: "a", TaskID: "two", Subject: "shipped it, PR MERGED", Status: "done", Liveness: "unknown"},
	})
	if len(stalled) != 0 || len(contradicted) != 0 {
		t.Fatalf("clean ledger produced findings: stalled=%+v contradicted=%+v", stalled, contradicted)
	}
}
