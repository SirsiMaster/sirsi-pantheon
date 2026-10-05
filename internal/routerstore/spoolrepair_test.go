//go:build !windows

package routerstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepairSpoolOutboxRestoresLocalPrivateDirectoryWithoutTouchingMessages(t *testing.T) {
	spool := filepath.Join(t.TempDir(), "relay")
	outbox := filepath.Join(spool, "agent-a", "outbox")
	if err := os.MkdirAll(outbox, 0o700); err != nil {
		t.Fatal(err)
	}
	message := filepath.Join(outbox, "held.json")
	if err := os.WriteFile(message, []byte(`{"id":"held"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(outbox, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(outbox, 0o700) })

	repair, err := RepairSpoolOutbox(spool, "agent-a")
	if err != nil {
		t.Fatalf("RepairSpoolOutbox: %v", err)
	}
	if repair.Agent != "agent-a" || repair.RepairedMode != "0700" {
		t.Fatalf("unexpected repair: %+v", repair)
	}
	st, err := os.Stat(outbox)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o700 {
		t.Fatalf("outbox mode = %04o, want 0700", got)
	}
	if got, err := os.ReadFile(message); err != nil || string(got) != `{"id":"held"}` {
		t.Fatalf("held message changed after repair: bytes=%q err=%v", got, err)
	}
}

func TestRepairSpoolOutboxRejectsSymlinkLeaf(t *testing.T) {
	spool := filepath.Join(t.TempDir(), "relay")
	realOutbox := filepath.Join(t.TempDir(), "real-outbox")
	if err := os.MkdirAll(filepath.Join(spool, "agent-a"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(realOutbox, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realOutbox, filepath.Join(spool, "agent-a", "outbox")); err != nil {
		t.Fatal(err)
	}
	if _, err := RepairSpoolOutbox(spool, "agent-a"); err == nil {
		t.Fatal("expected symlink outbox to be refused")
	}
}

func TestRepairSpoolOutboxRejectsTraversalAgent(t *testing.T) {
	if _, err := RepairSpoolOutbox("/tmp/relay", "../other"); err == nil {
		t.Fatal("expected traversal agent to be refused")
	}
}
