package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// waitForInbox must return promptly once a matching item is written by another
// goroutine — proving router_wait unblocks on real inbox work (anti-idle leg 2).
func TestWaitForInbox_ReturnsWhenItemArrives(t *testing.T) {
	root := t.TempDir()
	if err := work.EnsureRoot(root); err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}

	// Writer drops an item addressed to claude-pantheon shortly after the wait
	// begins, so the watcher (or fallback poll) picks it up.
	go func() {
		time.Sleep(100 * time.Millisecond)
		if _, err := work.Send(root, "codex-pantheon", "claude-pantheon", "review canon-sync", "please review"); err != nil {
			t.Errorf("Send: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	items, err := waitForInbox(ctx, root, "claude-pantheon")
	if err != nil {
		t.Fatalf("waitForInbox: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].To != "claude-pantheon" || items[0].From != "codex-pantheon" {
		t.Errorf("wrong item routing: from=%s to=%s", items[0].From, items[0].To)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("waitForInbox took too long: %s (should unblock near the 100ms write)", elapsed)
	}
}

// A pre-existing item must be returned immediately, before the watcher arms —
// covers the missed-event race (item written before the watch is added).
func TestWaitForInbox_ReturnsPreExistingItem(t *testing.T) {
	root := t.TempDir()
	if _, err := work.Send(root, "codex-pantheon", "claude-pantheon", "already here", "body"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	items, err := waitForInbox(ctx, root, "claude-pantheon")
	if err != nil {
		t.Fatalf("waitForInbox: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 pre-existing item, got %d", len(items))
	}
}

// waitForInbox must return empty (not an error) when the timeout elapses with no
// matching item — the long-poll exits cleanly so the session can re-arm.
func TestWaitForInbox_ReturnsEmptyOnTimeout(t *testing.T) {
	root := t.TempDir()
	if err := work.EnsureRoot(root); err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}

	// Short timeout, no writer — must time out empty.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	items, err := waitForInbox(ctx, root, "claude-pantheon")
	if err != nil {
		t.Fatalf("waitForInbox returned error on timeout: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty result on timeout, got %d items", len(items))
	}
}

// An item addressed to a DIFFERENT agent must not satisfy the wait — it times
// out empty, proving the filter is by recipient.
func TestWaitForInbox_IgnoresOtherAgentsItems(t *testing.T) {
	root := t.TempDir()
	if _, err := work.Send(root, "codex-pantheon", "someone-else", "not yours", "body"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	items, err := waitForInbox(ctx, root, "claude-pantheon")
	if err != nil {
		t.Fatalf("waitForInbox: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items for claude-pantheon, got %d", len(items))
	}
}

// handleRouterSend / handleRouterWait surface missing required args as IsError,
// matching the existing router handler convention.
func TestRouterWaitHandler_MissingAgent(t *testing.T) {
	result, err := handleRouterWait(map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when agent is missing")
	}
}

func TestRouterSendHandler_MissingArgs(t *testing.T) {
	result, err := handleRouterSend(map[string]interface{}{"from": "a"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when to/title are missing")
	}
}
