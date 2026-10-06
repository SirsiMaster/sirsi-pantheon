//go:build !windows

package routerstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepairSpoolOutboxRestoresLocalPrivateDirectoryWithoutTouchingMessages(t *testing.T) {
	spool := filepath.Join(repairTempDir(t), "relay")
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
	spool := filepath.Join(repairTempDir(t), "relay")
	realOutbox := filepath.Join(repairTempDir(t), "real-outbox")
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

func TestRepairSpoolOutboxRejectsRootSymlinkToMovedOriginalBeforeMutation(t *testing.T) {
	base := repairTempDir(t)
	spool := filepath.Join(base, "relay")
	moved := filepath.Join(base, "relay-original")
	if err := os.MkdirAll(filepath.Join(spool, "agent-a", "outbox"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(spool, "agent-a", "outbox"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(moved, "agent-a", "outbox"), 0o700) })

	_, err := repairSpoolOutbox(spool, "agent-a", nil, func() {
		if renameErr := os.Rename(spool, moved); renameErr != nil {
			t.Fatalf("move checked root: %v", renameErr)
		}
		if linkErr := os.Symlink(moved, spool); linkErr != nil {
			t.Fatalf("replace root with symlink to original: %v", linkErr)
		}
	})
	if err == nil {
		t.Fatal("expected symlink-backed root replacement to be refused")
	}
	st, statErr := os.Stat(filepath.Join(moved, "agent-a", "outbox"))
	if statErr != nil {
		t.Fatal(statErr)
	}
	if got := st.Mode().Perm(); got != 0o000 {
		t.Fatalf("original outbox was mutated after rejected root substitution: mode=%04o", got)
	}
}

func TestRepairSpoolOutboxRejectsSubstitutedLooseRootBeforeMutation(t *testing.T) {
	base := repairTempDir(t)
	spool := filepath.Join(base, "relay")
	replacement := filepath.Join(base, "relay-replacement")
	if err := os.MkdirAll(filepath.Join(spool, "agent-a", "outbox"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(replacement, "agent-a", "outbox"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(replacement, "agent-a", "outbox"), 0o000); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(replacement, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(replacement, "agent-a", "outbox"), 0o700) })

	_, err := repairSpoolOutbox(spool, "agent-a", nil, func() {
		if renameErr := os.Rename(spool, filepath.Join(base, "relay-original")); renameErr != nil {
			t.Fatalf("move checked root: %v", renameErr)
		}
		if renameErr := os.Rename(replacement, spool); renameErr != nil {
			t.Fatalf("install loose replacement root: %v", renameErr)
		}
	})
	if err == nil {
		t.Fatal("expected loose replacement root to be refused")
	}
	st, statErr := os.Stat(filepath.Join(spool, "agent-a", "outbox"))
	if statErr != nil {
		t.Fatal(statErr)
	}
	if got := st.Mode().Perm(); got != 0o000 {
		t.Fatalf("replacement outbox was mutated after root substitution: mode=%04o", got)
	}
}

func TestRepairSpoolOutboxRejectsParentSymlinkToMovedOriginalBeforeMutation(t *testing.T) {
	base := repairTempDir(t)
	parent := filepath.Join(base, "spool-parent")
	movedParent := filepath.Join(base, "spool-parent-original")
	spool := filepath.Join(parent, "relay")
	if err := os.MkdirAll(filepath.Join(spool, "agent-a", "outbox"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(spool, "agent-a", "outbox"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(movedParent, "relay", "agent-a", "outbox"), 0o700) })

	_, err := repairSpoolOutbox(spool, "agent-a", func() {
		if renameErr := os.Rename(parent, movedParent); renameErr != nil {
			t.Fatalf("move checked parent: %v", renameErr)
		}
		if linkErr := os.Symlink(movedParent, parent); linkErr != nil {
			t.Fatalf("replace parent with symlink to original: %v", linkErr)
		}
	}, nil)
	if err == nil {
		t.Fatal("expected symlink-backed parent replacement to be refused")
	}
	st, statErr := os.Stat(filepath.Join(movedParent, "relay", "agent-a", "outbox"))
	if statErr != nil {
		t.Fatal(statErr)
	}
	if got := st.Mode().Perm(); got != 0o000 {
		t.Fatalf("original outbox was mutated after rejected parent substitution: mode=%04o", got)
	}
}

func TestRepairSpoolOutboxRejectsGroupWritableParentBeforeMutation(t *testing.T) {
	base := repairTempDir(t)
	parent := filepath.Join(base, "spool-parent")
	spool := filepath.Join(parent, "relay")
	if err := os.MkdirAll(filepath.Join(spool, "agent-a", "outbox"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(spool, "agent-a", "outbox"), 0o000); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o720); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(parent, 0o700)
		_ = os.Chmod(filepath.Join(spool, "agent-a", "outbox"), 0o700)
	})

	if _, err := RepairSpoolOutbox(spool, "agent-a"); err == nil {
		t.Fatal("expected group-writable parent to be refused")
	}
	st, err := os.Stat(filepath.Join(spool, "agent-a", "outbox"))
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o000 {
		t.Fatalf("outbox was mutated under group-writable parent: mode=%04o", got)
	}
}

func repairTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("canonicalize temporary directory: %v", err)
	}
	return dir
}

func TestRepairSpoolOutboxRejectsTraversalAgent(t *testing.T) {
	if _, err := RepairSpoolOutbox("/tmp/relay", "../other"); err == nil {
		t.Fatal("expected traversal agent to be refused")
	}
}
