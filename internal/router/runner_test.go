package router

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerDryRunDoesNotAck(t *testing.T) {
	r, _ := setupTestRouter(t)
	id, err := r.SubmitAddressed(DocReview, "claude", "Needs Codex", "# Review: Needs Codex\n\nreviewer: claude", "codex")
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	rr := NewRunner(r, RunnerOptions{Agent: "all", DryRun: true, Once: true, Out: &buf})
	if err := rr.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), id) {
		t.Fatalf("dry-run output missing id: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "dry-run") {
		t.Error("dry-run output should contain [dry-run] prefix")
	}

	// Inbox should NOT be cleared
	pending, _ := r.PollInbox("codex")
	if len(pending) != 1 || pending[0] != id {
		t.Fatalf("runner auto-acked inbox: %v", pending)
	}
}

func TestRunnerNotifyCalledOncePerSession(t *testing.T) {
	r, _ := setupTestRouter(t)
	id, err := r.SubmitAddressed(DocReview, "claude", "Needs Codex", "# Review: Needs Codex\n\nreviewer: claude", "codex")
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	notify := func(target, docType, docID, repoRoot string) error {
		calls++
		if target != "codex" || docID != id {
			t.Fatalf("bad notify: %s %s", target, docID)
		}
		return nil
	}

	rr := NewRunner(r, RunnerOptions{Agent: "all", Out: io.Discard, Notify: notify})
	if err := rr.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Second tick should NOT re-dispatch (repeat suppression)
	if err := rr.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("notify calls = %d, want 1 (repeat suppression)", calls)
	}
}

func TestRunnerNoPendingItems(t *testing.T) {
	r, _ := setupTestRouter(t)

	var buf bytes.Buffer
	rr := NewRunner(r, RunnerOptions{Agent: "all", DryRun: true, Once: true, Out: &buf})
	if err := rr.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "No pending dispatches") {
		t.Errorf("expected no-pending message, got: %s", buf.String())
	}
}

func TestRunnerFilterByAgent(t *testing.T) {
	r, _ := setupTestRouter(t)
	r.SubmitAddressed(DocReview, "claude", "For Codex", "content", "codex")
	r.SubmitAddressed(DocReview, "codex", "For Claude", "content", "claude")

	// Filter to codex only
	var buf bytes.Buffer
	rr := NewRunner(r, RunnerOptions{Agent: "codex", DryRun: true, Once: true, Out: &buf})
	if err := rr.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "codex") {
		t.Error("expected codex dispatch in output")
	}
	if strings.Contains(output, "claude") && !strings.Contains(output, "codex") {
		t.Error("should not dispatch to claude when agent=codex")
	}
}

func TestRunnerMissingDocSkipped(t *testing.T) {
	r, _ := setupTestRouter(t)

	// Manually inject a pending ID that doesn't exist as a file
	state, _ := r.ReadState()
	state.AddToInbox("codex", "nonexistent-doc-id")
	r.WriteState(state)

	var buf bytes.Buffer
	rr := NewRunner(r, RunnerOptions{Agent: "all", DryRun: true, Once: true, Out: &buf})
	if err := rr.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "Skipping") {
		t.Errorf("expected skip message for missing doc, got: %s", buf.String())
	}
}

func TestRunnerNotifyFailureDoesNotMarkDispatched(t *testing.T) {
	r, _ := setupTestRouter(t)
	r.SubmitAddressed(DocReview, "claude", "Fails", "content", "codex")

	calls := 0
	failNotify := func(target, docType, docID, repoRoot string) error {
		calls++
		return io.ErrUnexpectedEOF // simulate failure
	}

	var buf bytes.Buffer
	rr := NewRunner(r, RunnerOptions{Agent: "all", Out: &buf, Notify: failNotify})

	// First tick: notify fails, should not mark dispatched
	rr.Tick(context.Background())
	// Second tick: should retry
	rr.Tick(context.Background())

	if calls != 2 {
		t.Fatalf("notify calls = %d, want 2 (retry after failure)", calls)
	}
}

func TestRunnerLedgerSuppressesDuplicateAfterRestart(t *testing.T) {
	r, tmp := setupTestRouter(t)
	id, err := r.SubmitAddressed(DocReview, "claude", "Needs Codex", "# Review: Needs Codex\n\nreviewer: claude", "codex")
	if err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(tmp, ".agents", "idea-router", "dispatch-ledger.json")

	calls := 0
	notify := func(target, docType, docID, repoRoot string) error {
		calls++
		if target != "codex" || docID != id {
			t.Fatalf("bad notify: %s %s", target, docID)
		}
		return nil
	}

	rr := NewRunner(r, RunnerOptions{RepoRoot: tmp, Agent: "all", Out: io.Discard, Notify: notify, LedgerPath: ledgerPath})
	if err := rr.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	restarted := NewRunner(r, RunnerOptions{RepoRoot: tmp, Agent: "all", Out: io.Discard, Notify: notify, LedgerPath: ledgerPath})
	if err := restarted.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("notify calls = %d, want 1 across restart", calls)
	}
	if _, err := os.Stat(ledgerPath); err != nil {
		t.Fatalf("ledger not written: %v", err)
	}
}

func TestRunnerLedgerRedispatchesEditedDocument(t *testing.T) {
	r, tmp := setupTestRouter(t)
	id, err := r.SubmitAddressed(DocReview, "claude", "Needs Codex", "# Review: Needs Codex\n\nreviewer: claude", "codex")
	if err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(tmp, ".agents", "idea-router", "dispatch-ledger.json")

	calls := 0
	notify := func(target, docType, docID, repoRoot string) error {
		calls++
		return nil
	}

	rr := NewRunner(r, RunnerOptions{RepoRoot: tmp, Agent: "all", Out: io.Discard, Notify: notify, LedgerPath: ledgerPath})
	if tickErr := rr.Tick(context.Background()); tickErr != nil {
		t.Fatal(tickErr)
	}

	doc, err := r.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if err := os.WriteFile(doc.Path, []byte("# Review: Needs Codex\n\nreviewer: claude\n\nedited"), 0o644); err != nil {
		t.Fatal(err)
	}

	restarted := NewRunner(r, RunnerOptions{RepoRoot: tmp, Agent: "all", Out: io.Discard, Notify: notify, LedgerPath: ledgerPath})
	if err := restarted.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("notify calls = %d, want 2 after document edit", calls)
	}
}

func TestRunnerClearedInboxStopsDispatch(t *testing.T) {
	r, tmp := setupTestRouter(t)
	id, err := r.SubmitAddressed(DocReview, "claude", "Needs Codex", "content", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.AckInbox("codex", []string{id}); err != nil {
		t.Fatal(err)
	}

	calls := 0
	notify := func(target, docType, docID, repoRoot string) error {
		calls++
		return nil
	}
	rr := NewRunner(r, RunnerOptions{RepoRoot: tmp, Agent: "all", Out: io.Discard, Notify: notify})
	if err := rr.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("notify calls = %d, want 0 after inbox clear", calls)
	}
}

func TestRunnerExecutorPersistsWorkQueueStatus(t *testing.T) {
	r, tmp := setupTestRouter(t)

	fakeAgent := filepath.Join(tmp, "fake-codex-agent.sh")
	script := `#!/bin/sh
cat > .agents/idea-router/state.json <<'JSON'
{
  "version": 1,
  "active_topics": ["safety-reset"],
  "completed_topics": [],
  "last_codex_read": "2026-05-18T14:00:00Z",
  "last_claude_read": null,
  "rules": {"no_feature_expansion": true},
  "pending": {"codex-pantheon": ["keep"]}
}
JSON
`
	if err := os.WriteFile(fakeAgent, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	routerRoot := filepath.Join(tmp, ".agents", "idea-router")
	err := SaveRegistry(routerRoot, &Registry{Agents: map[string]AgentConfig{
		"codex-pantheon": {
			Type:    "codex",
			Command: []string{fakeAgent},
			Cwd:     tmp,
		},
	}})
	if err != nil {
		t.Fatalf("SaveRegistry() error: %v", err)
	}

	id, err := r.SubmitAddressed(DocReview, "claude", "Needs Registered Codex", "content", "codex-pantheon")
	if err != nil {
		t.Fatal(err)
	}

	reg, err := LoadRegistry(routerRoot)
	if err != nil {
		t.Fatal(err)
	}
	wq, err := LoadWorkQueue(routerRoot)
	if err != nil {
		t.Fatal(err)
	}
	exec := NewExecutor(reg, r, wq, io.Discard)
	rr := NewRunner(r, RunnerOptions{RepoRoot: tmp, Agent: "codex-pantheon", Out: io.Discard, Executor: exec})
	if tickErr := rr.Tick(context.Background()); tickErr != nil {
		t.Fatal(tickErr)
	}

	loaded, err := LoadWorkQueue(routerRoot)
	if err != nil {
		t.Fatal(err)
	}
	item := loaded.Find("codex-pantheon:" + id)
	if item == nil {
		t.Fatalf("work item not persisted")
	}
	if item.Status != StatusCompleted {
		t.Fatalf("work item status = %s, want %s; last error: %s", item.Status, StatusCompleted, item.LastError)
	}
}

func TestRunnerExecutorSkipsInFlightWorkItemAfterRestart(t *testing.T) {
	r, tmp := setupTestRouter(t)
	routerRoot := filepath.Join(tmp, ".agents", "idea-router")
	err := SaveRegistry(routerRoot, &Registry{Agents: map[string]AgentConfig{
		"codex-pantheon": {
			Type:    "codex",
			Command: []string{"/bin/echo"},
			Cwd:     tmp,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	id, err := r.SubmitAddressed(DocReview, "claude", "Needs Registered Codex", "content", "codex-pantheon")
	if err != nil {
		t.Fatal(err)
	}
	reg, _ := LoadRegistry(routerRoot)
	wq, _ := LoadWorkQueue(routerRoot)
	item := wq.AddItem(id, "codex-pantheon", "claude-pantheon", "test")
	wq.UpdateStatus(item.ID, StatusDispatched, "")
	if err := wq.Save(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	exec := NewExecutor(reg, r, wq, &buf)
	rr := NewRunner(r, RunnerOptions{RepoRoot: tmp, Agent: "all", Out: &buf, Executor: exec})
	if err := rr.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "Dispatching to codex-pantheon") {
		t.Fatalf("in-flight work item was redispatched after restart:\n%s", buf.String())
	}
}
