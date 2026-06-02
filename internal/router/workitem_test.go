package router

import (
	"testing"
)

func TestWorkQueue_AddAndPending(t *testing.T) {
	tmp := t.TempDir()
	wq, err := LoadWorkQueue(tmp)
	if err != nil {
		t.Fatalf("LoadWorkQueue() error: %v", err)
	}

	wq.AddItem("doc-1", "claude-pantheon", "codex-pantheon", "test-topic")
	wq.AddItem("doc-2", "codex-pantheon", "claude-pantheon", "test-topic")
	wq.AddItem("doc-3", "claude-pantheon", "codex-pantheon", "other-topic")

	claude := wq.PendingFor("claude-pantheon")
	if len(claude) != 2 {
		t.Errorf("expected 2 pending for claude-pantheon, got %d", len(claude))
	}

	codex := wq.PendingFor("codex-pantheon")
	if len(codex) != 1 {
		t.Errorf("expected 1 pending for codex-pantheon, got %d", len(codex))
	}

	all := wq.AllPending()
	if len(all) != 3 {
		t.Errorf("expected 3 total pending, got %d", len(all))
	}
}

func TestWorkQueue_UpdateStatus(t *testing.T) {
	tmp := t.TempDir()
	wq, _ := LoadWorkQueue(tmp)
	item := wq.AddItem("doc-1", "claude-pantheon", "codex-pantheon", "topic")

	ok := wq.UpdateStatus(item.ID, StatusDispatched, "")
	if !ok {
		t.Error("UpdateStatus should return true for existing item")
	}

	// Item should no longer be pending
	pending := wq.PendingFor("claude-pantheon")
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after dispatch, got %d", len(pending))
	}

	// Unknown item
	ok = wq.UpdateStatus("nonexistent", StatusCompleted, "")
	if ok {
		t.Error("UpdateStatus should return false for unknown item")
	}
}

func TestWorkQueue_RecordAttempt(t *testing.T) {
	tmp := t.TempDir()
	wq, _ := LoadWorkQueue(tmp)
	item := wq.AddItem("doc-1", "claude-pantheon", "codex-pantheon", "topic")

	wq.RecordAttempt(item.ID, 1, "agent crashed", "panic: runtime error")

	if len(wq.Items[0].Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(wq.Items[0].Attempts))
	}
	if wq.Items[0].Attempts[0].ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", wq.Items[0].Attempts[0].ExitCode)
	}
	if wq.Items[0].Attempts[0].Stderr != "panic: runtime error" {
		t.Errorf("stderr not recorded")
	}
}

func TestWorkQueue_SaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	wq, _ := LoadWorkQueue(tmp)
	wq.AddItem("doc-1", "claude-pantheon", "codex-pantheon", "topic")
	wq.Save()

	loaded, err := LoadWorkQueue(tmp)
	if err != nil {
		t.Fatalf("LoadWorkQueue() error: %v", err)
	}
	if len(loaded.Items) != 1 {
		t.Errorf("expected 1 item after save/load, got %d", len(loaded.Items))
	}
}

func TestWorkQueue_StatusTransitions(t *testing.T) {
	tmp := t.TempDir()
	wq, _ := LoadWorkQueue(tmp)
	item := wq.AddItem("doc-1", "claude-pantheon", "codex-pantheon", "topic")

	wq.UpdateStatus(item.ID, StatusDispatched, "")
	if wq.Items[0].DispatchedAt.IsZero() {
		t.Error("DispatchedAt should be set")
	}

	wq.UpdateStatus(item.ID, StatusCompleted, "")
	if wq.Items[0].CompletedAt.IsZero() {
		t.Error("CompletedAt should be set")
	}

	wq.UpdateStatus(item.ID, StatusFailed, "timeout waiting for writeback")
	if wq.Items[0].LastError != "timeout waiting for writeback" {
		t.Errorf("LastError = %q, want timeout message", wq.Items[0].LastError)
	}
}

func TestStateMigratePending(t *testing.T) {
	s := &State{
		PendingForCodex:  []string{"doc-1"},
		PendingForClaude: []string{"doc-2", "doc-3"},
	}
	s.MigratePending()

	if len(s.Pending["codex-pantheon"]) != 1 {
		t.Errorf("expected 1 migrated codex item, got %d", len(s.Pending["codex-pantheon"]))
	}
	if len(s.Pending["claude-pantheon"]) != 2 {
		t.Errorf("expected 2 migrated claude items, got %d", len(s.Pending["claude-pantheon"]))
	}
}

func TestStateDynamicInbox(t *testing.T) {
	s := &State{Pending: make(map[string][]string)}

	s.AddToInbox("claude-finalwishes", "doc-1")
	s.AddToInbox("codex-nexus", "doc-2")

	if len(s.InboxFor("claude-finalwishes")) != 1 {
		t.Error("expected 1 item for claude-finalwishes")
	}
	if len(s.InboxFor("codex-nexus")) != 1 {
		t.Error("expected 1 item for codex-nexus")
	}
	if len(s.InboxFor("unknown-agent")) != 0 {
		t.Error("expected 0 items for unknown agent")
	}
}

func TestStateNormalizePendingKeepsEmptySlices(t *testing.T) {
	s := &State{
		Pending: map[string][]string{
			"claude-pantheon": nil,
			"codex-pantheon":  {"work"},
		},
	}

	s.NormalizePending()

	if s.Pending["claude-pantheon"] == nil {
		t.Fatal("expected empty registered inbox slice, got nil")
	}
	if s.PendingForClaude == nil {
		t.Fatal("expected empty legacy claude inbox slice, got nil")
	}
	if len(s.PendingForCodex) != 1 || s.PendingForCodex[0] != "work" {
		t.Fatalf("legacy codex inbox = %v, want [work]", s.PendingForCodex)
	}
}
