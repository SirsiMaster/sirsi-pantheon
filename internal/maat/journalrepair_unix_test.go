//go:build !windows

package maat

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func journalMutationLockTestParent(t *testing.T) (int, string) {
	t.Helper()
	root := t.TempDir()
	parentFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(parentFD) })
	return parentFD, root
}

func TestOpenJournalMutationLockAtUsesExclusiveCreateFlags(t *testing.T) {
	parentFD, _ := journalMutationLockTestParent(t)
	wantFlags := unix.O_RDWR | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_CREAT | unix.O_EXCL
	calls := 0
	fd, err := openJournalMutationLockAt(parentFD, ".decisions.jsonl.lock", func(dirfd int, name string, flags int, mode uint32) (int, error) {
		calls++
		if dirfd != parentFD || name != ".decisions.jsonl.lock" || flags != wantFlags || mode != 0o600 {
			t.Fatalf("exclusive create args = dirfd %d name %q flags %#x mode %#o", dirfd, name, flags, mode)
		}
		return unix.Openat(dirfd, name, flags, mode)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if calls != 1 {
		t.Fatalf("exclusive-create calls = %d, want 1", calls)
	}
}

func TestOpenJournalMutationLockAtExistingLeafUsesNoCreateFallback(t *testing.T) {
	parentFD, root := journalMutationLockTestParent(t)
	name := ".decisions.jsonl.lock"
	if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	createFlags := unix.O_RDWR | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_CREAT | unix.O_EXCL
	openFlags := unix.O_RDWR | unix.O_NOFOLLOW | unix.O_CLOEXEC
	calls := 0
	fd, err := openJournalMutationLockAt(parentFD, name, func(dirfd int, gotName string, flags int, mode uint32) (int, error) {
		calls++
		if dirfd != parentFD || gotName != name {
			t.Fatalf("open args = dirfd %d name %q", dirfd, gotName)
		}
		switch calls {
		case 1:
			if flags != createFlags || mode != 0o600 {
				t.Fatalf("create args = flags %#x mode %#o", flags, mode)
			}
		case 2:
			if flags != openFlags || mode != 0 {
				t.Fatalf("fallback args = flags %#x mode %#o", flags, mode)
			}
		default:
			t.Fatalf("unexpected openat call %d", calls)
		}
		return unix.Openat(dirfd, gotName, flags, mode)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if calls != 2 {
		t.Fatalf("create/open calls = %d, want 2", calls)
	}
}

func TestOpenJournalMutationLockAtConcurrentFirstCreateConverges(t *testing.T) {
	parentFD, _ := journalMutationLockTestParent(t)
	name := ".decisions.jsonl.lock"
	createFlags := unix.O_RDWR | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_CREAT | unix.O_EXCL
	openFlags := unix.O_RDWR | unix.O_NOFOLLOW | unix.O_CLOEXEC
	var mu sync.Mutex
	createCalls, openCalls := 0, 0
	openat := func(dirfd int, leaf string, flags int, mode uint32) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		switch flags {
		case createFlags:
			createCalls++
		case openFlags:
			openCalls++
		default:
			t.Errorf("unexpected concurrent open flags %#x", flags)
		}
		return unix.Openat(dirfd, leaf, flags, mode)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			fd, err := openJournalMutationLockAt(parentFD, name, openat)
			if err == nil {
				err = unix.Close(fd)
			}
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent lock acquisition: %v", err)
		}
	}
	if createCalls != 2 || openCalls != 1 {
		t.Fatalf("concurrent calls create=%d existing-open=%d, want 2 and 1", createCalls, openCalls)
	}
}

func TestOpenJournalMutationLockAtPropagatesCreateFailure(t *testing.T) {
	parentFD, _ := journalMutationLockTestParent(t)
	calls := 0
	fd, err := openJournalMutationLockAt(parentFD, ".decisions.jsonl.lock", func(int, string, int, uint32) (int, error) {
		calls++
		return -1, unix.ENOENT
	})
	if fd != -1 || !errors.Is(err, unix.ENOENT) || calls != 1 {
		t.Fatalf("create refusal = fd %d err %v calls %d, want one preserved ENOENT", fd, err, calls)
	}
}

func TestOpenJournalMutationLockAtPropagatesExistingOpenErrors(t *testing.T) {
	for _, openErr := range []error{unix.EACCES, unix.ENOENT} {
		t.Run(openErr.Error(), func(t *testing.T) {
			parentFD, _ := journalMutationLockTestParent(t)
			calls := 0
			fd, err := openJournalMutationLockAt(parentFD, ".decisions.jsonl.lock", func(_ int, _ string, flags int, _ uint32) (int, error) {
				calls++
				if calls == 1 {
					if flags&unix.O_EXCL == 0 || flags&unix.O_CREAT == 0 {
						t.Fatalf("first open flags %#x lack exclusive create", flags)
					}
					return -1, unix.EEXIST
				}
				if flags&(unix.O_EXCL|unix.O_CREAT) != 0 {
					t.Fatalf("fallback flags %#x still create", flags)
				}
				return -1, openErr
			})
			if fd != -1 || !errors.Is(err, openErr) || calls != 2 {
				t.Fatalf("existing-open refusal = fd %d err %v calls %d, want preserved %v after two calls", fd, err, calls, openErr)
			}
		})
	}
}

func TestOpenJournalMutationLockAtRefusesSymlink(t *testing.T) {
	parentFD, root := journalMutationLockTestParent(t)
	name := ".decisions.jsonl.lock"
	if err := os.Symlink("missing-target", filepath.Join(root, name)); err != nil {
		t.Fatal(err)
	}
	fd, err := openJournalMutationLockAt(parentFD, name, unix.Openat)
	if err == nil {
		_ = unix.Close(fd)
		t.Fatal("symlink lock leaf was opened")
	}
	if !errors.Is(err, unix.ELOOP) {
		t.Fatalf("symlink refusal = %v, want ELOOP", err)
	}
}

func TestJournalMutationLockRefusesHardlinkedLeafBeforeCallback(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "decisions.jsonl")
	lockPath := filepath.Join(root, ".decisions.jsonl.lock")
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(lockPath, lockPath+".alias"); err != nil {
		t.Fatal(err)
	}
	called := false
	err := withJournalMutationLock(path, func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "owned private singly-linked regular file") {
		t.Fatalf("hard-linked mutation lock error = %v, want identity refusal", err)
	}
	if called {
		t.Fatal("callback ran with hard-linked mutation lock")
	}
}

func TestJournalMutationLockRefusesPublicModeLeafBeforeCallback(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "decisions.jsonl")
	lockPath := filepath.Join(root, ".decisions.jsonl.lock")
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lockPath, 0o644); err != nil {
		t.Fatal(err)
	}
	called := false
	err := withJournalMutationLock(path, func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "owned private singly-linked regular file") {
		t.Fatalf("public-mode mutation lock error = %v, want identity refusal", err)
	}
	if called {
		t.Fatal("callback ran with public-mode mutation lock")
	}
}

func TestFileDecisionJournalRepairRefusesNameSubstitutionBeforeInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	valid := `{"time":"2026-09-26T10:01:00Z","host":"m5","kind":"reservation","requester":"alpha","assessed":"free","determination":"grant","why":"no overlap"}`
	legacy := `{"time":"2026-09-26T10:02:00Z","host":"m5","kind":"assessment","assessed":"legacy","determination":"warn","why":"missing requester"}`
	original := valid + "\n" + legacy + "\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	oldHook := journalRepairBeforeInstall
	t.Cleanup(func() { journalRepairBeforeInstall = oldHook })
	journalRepairBeforeInstall = func() {
		moved := path + ".moved-original"
		if err := os.Rename(path, moved); err != nil {
			t.Fatalf("move original inside substitution hook: %v", err)
		}
		if err := os.WriteFile(path, []byte("attacker replacement\n"), 0o600); err != nil {
			t.Fatalf("create attacker replacement inside substitution hook: %v", err)
		}
	}

	_, err := (&FileDecisionJournal{Path: path}).RepairInvalidRecords()
	if err == nil || !strings.Contains(err.Error(), "governed journal name changed") {
		t.Fatalf("repair substitution error = %v, want governed-name refusal", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "attacker replacement\n" {
		t.Fatalf("substituted journal was overwritten: %q", got)
	}
	if _, err := os.Stat(path + ".moved-original"); err != nil {
		t.Fatalf("original source should remain preserved after refused repair: %v", err)
	}
}

func TestFileDecisionJournalRepairRefusesSameInodeSameSizeMutationBeforeInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	valid := `{"time":"2026-09-26T10:01:00Z","host":"m5","kind":"reservation","requester":"alpha","assessed":"free","determination":"grant","why":"no overlap"}`
	legacy := `{"time":"2026-09-26T10:02:00Z","host":"m5","kind":"assessment","assessed":"legacy","determination":"warn","why":"missing requester"}`
	original := valid + "\n" + legacy + "\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(original, `"alpha"`, `"bravo"`, 1)
	if len(mutated) != len(original) {
		t.Fatal("fixture must preserve exact journal size")
	}

	oldHook := journalRepairBeforeSourceRevalidation
	t.Cleanup(func() { journalRepairBeforeSourceRevalidation = oldHook })
	journalRepairBeforeSourceRevalidation = func() {
		if err := os.WriteFile(path, []byte(mutated), 0o600); err != nil {
			t.Fatalf("mutate retained source inode: %v", err)
		}
	}

	_, err := (&FileDecisionJournal{Path: path}).RepairInvalidRecords()
	if err == nil || !strings.Contains(err.Error(), "source changed during repair") {
		t.Fatalf("repair same-inode mutation error = %v, want source-change refusal", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != mutated {
		t.Fatalf("mutated active journal was overwritten: %q", got)
	}
}

func TestFileDecisionJournalRepairTightensOwnedLegacyPermissionsBeforeRebuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	valid := `{"time":"2026-09-26T10:01:00Z","host":"m5","kind":"reservation","requester":"alpha","assessed":"free","determination":"grant","why":"no overlap"}`
	legacy := `{"time":"2026-09-26T10:02:00Z","host":"m5","kind":"assessment","assessed":"legacy","determination":"warn","why":"missing requester"}`
	if err := os.WriteFile(path, []byte(valid+"\n"+legacy+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	receipt, err := (&FileDecisionJournal{Path: path}).RepairInvalidRecords()
	if err != nil {
		t.Fatalf("repair owned legacy journal: %v", err)
	}
	if receipt.RemovedCount != 1 || receipt.RetainedCount != 1 {
		t.Fatalf("repair receipt = %+v, want one removed and one retained", receipt)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("repaired mode = %o, want 600", got)
	}
	if _, err := os.Stat(receipt.BackupPath); err != nil {
		t.Fatalf("preserved backup missing: %v", err)
	}
}
