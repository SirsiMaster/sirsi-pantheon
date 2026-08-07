package main

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// A shared task id is NOT itself a finding. The fabric mirrors rows onto a
// second lane on purpose, and 14 of the 24 shared ids measured on 2026-08-07
// agreed on status. Flagging agreement would bury the 10 that matter.
func TestSharedTaskIDWithMatchingStatusIsNotReported(t *testing.T) {
	got := findCrossAgentDivergence([]routerstore.Task{
		{Agent: "claude-home", TaskID: "shared", Status: "done"},
		{Agent: "claude-nexus", TaskID: "shared", Status: "done"},
	})
	if len(got) != 0 {
		t.Fatalf("agreement reported as divergence: %+v", got)
	}
}

// THE case: same id, different status, on two ledgers that are each internally
// consistent. Acting on one of them re-opened eight already-closed rows on
// 2026-08-07 and was caught only by a hand cross-check.
func TestDivergentStatusAcrossLanesIsReportedWithEveryHolder(t *testing.T) {
	got := findCrossAgentDivergence([]routerstore.Task{
		{Agent: "codex-inference", TaskID: "sne-05", Status: "pending"},
		{Agent: "claude-nexus", TaskID: "sne-05", Status: "done"},
	})
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	// BOTH sides must appear. Naming only one lane tells an operator which
	// ledger to open and nothing about what it disagrees with.
	for _, want := range []string{"claude-nexus=done", "codex-inference=pending"} {
		if !strings.Contains(got[0].Reason, want) {
			t.Errorf("reason omits %q: %s", want, got[0].Reason)
		}
	}
}

// A row on exactly one ledger is the normal case and must stay silent.
func TestSingleHolderIsNotReported(t *testing.T) {
	got := findCrossAgentDivergence([]routerstore.Task{
		{Agent: "claude-home", TaskID: "solo", Status: "pending"},
	})
	if len(got) != 0 {
		t.Fatalf("single-holder row reported: %+v", got)
	}
}

// Three-way splits happen and must not be truncated to a pair.
func TestThreeWayDivergenceNamesAllThree(t *testing.T) {
	got := findCrossAgentDivergence([]routerstore.Task{
		{Agent: "a", TaskID: "x", Status: "done"},
		{Agent: "b", TaskID: "x", Status: "pending"},
		{Agent: "c", TaskID: "x", Status: "blocked"},
	})
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	for _, want := range []string{"a=done", "b=pending", "c=blocked", "3 lanes"} {
		if !strings.Contains(got[0].Reason, want) {
			t.Errorf("reason omits %q: %s", want, got[0].Reason)
		}
	}
}

// The 8-of-10 case. When a lane closes its copy with an explicit ledger-scoped
// marker, the divergence is EXPLAINED and must not be reported. Without this,
// the SNE set handed from claude-nexus to codex-inference on 2026-08-05 would
// bury the genuine splits under 8 correct handoffs.
func TestDeclaredLedgerHandoffIsNotReported(t *testing.T) {
	got := findCrossAgentDivergence([]routerstore.Task{
		{Agent: "claude-nexus", TaskID: "sne-05", Status: "done",
			Subject: "CLOSED ON THIS LEDGER — not abandoned. Owner moved SNE engine ownership to codex-inference 2026-08-05"},
		{Agent: "codex-inference", TaskID: "sne-05", Status: "pending", Subject: "SNE-05 admission control"},
	})
	if len(got) != 0 {
		t.Fatalf("declared handoff reported as a defect: %+v", got)
	}
}

// The marker only counts when the lane LEADS with it. A subject that merely
// mentions the phrase deep in prose is not making a scope declaration, and
// treating it as one would let any row suppress its own finding.
func TestHandoffMarkerBuriedInProseDoesNotSuppress(t *testing.T) {
	buried := "Blocked on the migration; see the note about how we handled it when ownership transferred last week"
	if declaresLedgerHandoff(buried) {
		t.Fatalf("a buried mention suppressed the finding: %q", buried)
	}
	got := findCrossAgentDivergence([]routerstore.Task{
		{Agent: "a", TaskID: "x", Status: "done", Subject: buried},
		{Agent: "b", TaskID: "x", Status: "pending", Subject: "still working it"},
	})
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1 — a buried mention must not suppress", len(got))
	}
}
