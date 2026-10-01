package routerstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSpoolOutboxHealth(t *testing.T) {
	spool := t.TempDir()

	// agent-a: 2 held items.
	aOutbox := filepath.Join(spool, "agent-a", "outbox")
	if err := os.MkdirAll(aOutbox, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"1.json", "2.json"} {
		if err := os.WriteFile(filepath.Join(aOutbox, name), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// agent-b: outbox dir exists but is empty — must NOT appear in the result
	// (a quiet lane reads as quiet, not as a zero-count row).
	bOutbox := filepath.Join(spool, "agent-b", "outbox")
	if err := os.MkdirAll(bOutbox, 0o755); err != nil {
		t.Fatal(err)
	}

	// agent-c: no outbox dir at all — same as agent-b, absent from the result.
	if err := os.MkdirAll(filepath.Join(spool, "agent-c"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := SpoolOutboxHealth(spool)
	if err != nil {
		t.Fatalf("SpoolOutboxHealth: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 agent with held items, got %d: %+v", len(got), got)
	}
	if got[0].Agent != "agent-a" || got[0].QueuedForRetry != 2 {
		t.Fatalf("want agent-a with 2 queued, got %+v", got[0])
	}
}

// TestSpoolOutboxHealth_NeverMutates asserts the held files are untouched
// after the read — this is a health probe, not a drain.
func TestSpoolOutboxHealth_NeverMutates(t *testing.T) {
	spool := t.TempDir()
	outbox := filepath.Join(spool, "agent-a", "outbox")
	if err := os.MkdirAll(outbox, 0o755); err != nil {
		t.Fatal(err)
	}
	held := filepath.Join(outbox, "1.json")
	if err := os.WriteFile(held, []byte(`{"id":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := SpoolOutboxHealth(spool); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(held); err != nil {
		t.Fatalf("held outbox file must still exist after a health read: %v", err)
	}
}

func TestSpoolOutboxHealth_MissingSpoolRootErrors(t *testing.T) {
	_, err := SpoolOutboxHealth(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("want an error for a missing spool root, got nil (a silent empty reads as all-clear)")
	}
}

// TestSpoolOutboxHealth_UnreadableOutboxIsNotSilentlyEmpty: codex-pantheon
// review of PR #931 (item 20261001-010622) reproduced filepath.Glob silently
// swallowing a permission-denied outbox directory as zero held items — an
// unreadable queue must render as unknown/error, never as quiet. Root runs
// bypass directory permission bits, so this test is meaningless (and would
// false-fail) under root; skip there rather than report a false pass/fail.
func TestSpoolOutboxHealth_UnreadableOutboxIsNotSilentlyEmpty(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission bits")
	}
	spool := t.TempDir()
	outbox := filepath.Join(spool, "agent-a", "outbox")
	if err := os.MkdirAll(outbox, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outbox, "1.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(outbox, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(outbox, 0o755) }) // let t.TempDir() clean up

	got, err := SpoolOutboxHealth(spool)
	if err != nil {
		t.Fatalf("SpoolOutboxHealth: %v", err)
	}
	if len(got) != 1 || got[0].Agent != "agent-a" {
		t.Fatalf("want 1 entry for agent-a, got %+v", got)
	}
	if !got[0].Unreadable || got[0].Error == "" {
		t.Fatalf("want agent-a reported as Unreadable with an error, got %+v (a silent empty reads as all-clear)", got[0])
	}
	if got[0].QueuedForRetry != 0 {
		t.Fatalf("an unreadable outbox has an unknown count, not a confident 0: got %+v", got[0])
	}
}
