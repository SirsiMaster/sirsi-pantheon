package maat

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestDefaultStoreExplicitFirstUseBindsGenesisAndReopens(t *testing.T) {
	home := defaultStoreTestHome(t)
	if status := inspectDefaultStoreAt(home); status.State != DefaultStoreUninitialized {
		t.Fatalf("initial state = %+v, want genuine first use", status)
	}
	storeUUID, err := initializeDefaultStoreAt(home)
	if err != nil {
		t.Fatal(err)
	}
	status := inspectDefaultStoreAt(home)
	if status.State != DefaultStoreReady || status.StoreUUID != storeUUID {
		t.Fatalf("initialized status = %+v", status)
	}
	store, err := openDefaultStoreAt(home)
	if err != nil {
		t.Fatal(err)
	}
	if store.storeUUID != storeUUID {
		t.Fatalf("opened UUID = %q, want %q", store.storeUUID, storeUUID)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	genesis, err := os.ReadFile(filepath.Join(home, ".sirsi", "maat", defaultStoreName, genesisName))
	if err != nil || len(genesis) == 0 {
		t.Fatalf("read genesis: bytes=%d err=%v", len(genesis), err)
	}
	if mode := fileMode(t, filepath.Join(home, ".sirsi", "maat", defaultStoreName)); mode.Perm() != 0o700 {
		t.Fatalf("store root mode = %04o, want 0700", mode.Perm())
	}
}

func TestDefaultStoreUsesExistingOrdinarySirsiAncestorsWithoutChangingThem(t *testing.T) {
	home := defaultStoreTestHome(t)
	sirsiPath := filepath.Join(home, ".sirsi")
	maatPath := filepath.Join(sirsiPath, "maat")
	if err := os.Mkdir(sirsiPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sirsiPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sirsiPath, "unrelated-sentinel"), []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(maatPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(maatPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(maatPath, "unrelated-sentinel"), []byte("also preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	sirsiBefore, err := os.Stat(sirsiPath)
	if err != nil {
		t.Fatal(err)
	}
	maatBefore, err := os.Stat(maatPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := initializeDefaultStoreAt(home); err != nil {
		t.Fatalf("first use should accept owned nonwritable 0755 ancestors: %v", err)
	}
	store, err := openDefaultStoreAt(home)
	if err != nil {
		t.Fatalf("initialized store should reopen below ordinary ancestors: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for _, before := range []struct {
		path string
		info os.FileInfo
	}{{sirsiPath, sirsiBefore}, {maatPath, maatBefore}} {
		after, err := os.Stat(before.path)
		if err != nil || !os.SameFile(before.info, after) || after.Mode().Perm() != 0o755 {
			t.Fatalf("shared ancestor changed: path=%s before=%v after=%v err=%v", before.path, before.info, after, err)
		}
	}
	for path, want := range map[string]string{
		filepath.Join(sirsiPath, "unrelated-sentinel"): "preserve",
		filepath.Join(maatPath, "unrelated-sentinel"):  "also preserve",
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("unrelated ancestor content changed: path=%s got=%q err=%v", path, got, err)
		}
	}
}

func TestSharedDirectoryIdentityRejectsWritableOrForeignAncestors(t *testing.T) {
	actor := uint32(os.Geteuid())
	directory := uint32(unix.S_IFDIR)
	for _, tc := range []struct {
		name  string
		owner uint32
		mode  uint32
		want  bool
	}{
		{"ordinary-owned-0755", actor, directory | 0o755, true},
		{"owned-private", actor, directory | 0o700, true},
		{"group-writable", actor, directory | 0o775, false},
		{"other-writable", actor, directory | 0o757, false},
		{"foreign-owner", actor + 1, directory | 0o755, false},
		{"not-directory", actor, uint32(unix.S_IFREG) | 0o600, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sharedDirectoryIdentityAllowed(actor, tc.owner, tc.mode); got != tc.want {
				t.Fatalf("shared directory identity allowed=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestDefaultStoreRejectsWritableOrSymlinkedSharedAncestors(t *testing.T) {
	for _, target := range []string{"sirsi", "maat"} {
		t.Run(target+"-writable", func(t *testing.T) {
			home := defaultStoreTestHome(t)
			sirsiPath := filepath.Join(home, ".sirsi")
			if err := os.Mkdir(sirsiPath, 0o700); err != nil {
				t.Fatal(err)
			}
			path := sirsiPath
			if target == "maat" {
				path = filepath.Join(sirsiPath, "maat")
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chmod(path, 0o775); err != nil {
				t.Fatal(err)
			}
			if _, err := initializeDefaultStoreAt(home); err == nil {
				t.Fatal("group/other-writable shared ancestor accepted")
			}
		})
		t.Run(target+"-symlink", func(t *testing.T) {
			home := defaultStoreTestHome(t)
			sirsiPath := filepath.Join(home, ".sirsi")
			if target == "sirsi" {
				if err := os.Symlink(t.TempDir(), sirsiPath); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(sirsiPath, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), filepath.Join(sirsiPath, "maat")); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := initializeDefaultStoreAt(home); err == nil {
				t.Fatal("symlinked shared ancestor accepted")
			}
		})
	}
}

func TestDefaultStoreRefusesUnboundAndInterruptedStateWithoutMutation(t *testing.T) {
	t.Run("unbound directory is not adopted", func(t *testing.T) {
		home := defaultStoreTestHome(t)
		root := filepath.Join(home, ".sirsi", "maat", defaultStoreName)
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		before, err := os.Stat(root)
		if err != nil {
			t.Fatal(err)
		}
		if status := inspectDefaultStoreAt(home); status.State != DefaultStoreIncomplete {
			t.Fatalf("unbound state = %+v", status)
		}
		if _, err := initializeDefaultStoreAt(home); err == nil {
			t.Fatal("initializer adopted unbound directory")
		}
		after, err := os.Stat(root)
		if err != nil || !os.SameFile(before, after) {
			t.Fatalf("unbound root changed: before=%v after=%v err=%v", before, after, err)
		}
	})

	t.Run("durable initializing binding is never reset", func(t *testing.T) {
		home := defaultStoreTestHome(t)
		parent, err := openDefaultStoreParent(home, true)
		if err != nil {
			t.Fatal(err)
		}
		binding := storeBinding{Schema: bindingSchema, StoreUUID: "b3c515a2-4773-4653-86d1-8a5734c2ee45", UID: uint32(os.Geteuid()), State: "initializing", Recorded: time.Now().UTC()}
		if err := createBinding(parent, binding); err != nil {
			_ = unix.Close(parent)
			t.Fatal(err)
		}
		_ = unix.Close(parent)
		if got := inspectDefaultStoreAt(home); got.State != DefaultStoreIncomplete {
			t.Fatalf("interrupted state = %+v", got)
		}
		if _, err := initializeDefaultStoreAt(home); err == nil {
			t.Fatal("initializer reset interrupted binding")
		}
	})

	t.Run("missing committed root is unverifiable", func(t *testing.T) {
		home := defaultStoreTestHome(t)
		if _, err := initializeDefaultStoreAt(home); err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(home, ".sirsi", "maat", defaultStoreName)
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
		if status := inspectDefaultStoreAt(home); status.State != DefaultStoreIncomplete {
			t.Fatalf("missing-root state = %+v", status)
		}
		if _, err := initializeDefaultStoreAt(home); err == nil {
			t.Fatal("initializer recreated a missing committed store")
		}
	})
}

func TestDefaultStoreRejectsSymlinkedRootAndBinding(t *testing.T) {
	t.Run("symlinked root", func(t *testing.T) {
		home := defaultStoreTestHome(t)
		if _, err := initializeDefaultStoreAt(home); err != nil {
			t.Fatal(err)
		}
		parent := filepath.Join(home, ".sirsi", "maat")
		root := filepath.Join(parent, defaultStoreName)
		moved := filepath.Join(parent, "saved-root")
		if err := os.Rename(root, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(moved, root); err != nil {
			t.Fatal(err)
		}
		if status := inspectDefaultStoreAt(home); status.State != DefaultStoreIncomplete {
			t.Fatalf("symlinked root state = %+v", status)
		}
	})

	t.Run("symlinked binding", func(t *testing.T) {
		home := defaultStoreTestHome(t)
		parent, err := openDefaultStoreParent(home, true)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(parent)
		if err := os.WriteFile(filepath.Join(home, ".sirsi", "maat", "elsewhere"), []byte("not binding"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("elsewhere", filepath.Join(home, ".sirsi", "maat", defaultBindingName)); err != nil {
			t.Fatal(err)
		}
		status := inspectStoreAt(parent)
		if status.State != DefaultStoreIncomplete {
			t.Fatalf("symlinked binding state = %+v", status)
		}
	})
}

func TestDefaultStoreMissingCommittedNamespaceIsNotRecreated(t *testing.T) {
	home := defaultStoreTestHome(t)
	if _, err := initializeDefaultStoreAt(home); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".sirsi", "maat", defaultStoreName)
	missing := filepath.Join(root, "attempts")
	if err := os.RemoveAll(missing); err != nil {
		t.Fatal(err)
	}
	if status := inspectDefaultStoreAt(home); status.State != DefaultStoreIncomplete {
		t.Fatalf("missing committed namespace state = %+v, want incomplete", status)
	}
	if _, err := openDefaultStoreAt(home); err == nil {
		t.Fatal("open accepted a committed store with a missing namespace")
	}
	if _, err := os.Lstat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open recreated missing namespace: lstat err=%v", err)
	}
}

func TestDefaultStoreConcurrentInitializationCommitsExactlyOnce(t *testing.T) {
	home := defaultStoreTestHome(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := initializeDefaultStoreAt(home)
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	successes, refusals := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, unix.EEXIST) || strings.Contains(err.Error(), "initialize refused") {
			refusals++
		} else {
			t.Fatalf("unexpected concurrent initialization error: %v", err)
		}
	}
	if successes != 1 || refusals != 1 {
		t.Fatalf("successes=%d refusals=%d, want exactly one each", successes, refusals)
	}
}

func TestBootstrapLockErrorCapturesRetainedMetadataAndPreservesCause(t *testing.T) {
	parentFD, err := unix.Open(t.TempDir(), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(parentFD)
	lockFD, err := unix.Openat(parentFD, defaultLockName, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(lockFD)
	var parent, lock unix.Stat_t
	if err := unix.Fstat(parentFD, &parent); err != nil {
		t.Fatal(err)
	}
	if err := unix.Fstat(lockFD, &lock); err != nil {
		t.Fatal(err)
	}
	if parent.Uid != uint32(os.Geteuid()) || lock.Uid != uint32(os.Geteuid()) || lock.Nlink != 1 {
		t.Fatal("fixture must retain owned parent and singly linked lock")
	}
	// The failure is simulated; no actual open/sync race is forced or claimed.
	failure := bootstrapLockError("sync parent", parentFD, lockFD, true, unix.ENOENT)
	if !errors.Is(failure, unix.ENOENT) {
		t.Fatalf("original ENOENT lost: %v", failure)
	}
	for _, want := range []string{
		"sync parent bootstrap lock " + fmt.Sprintf("%q", defaultLockName),
		fmt.Sprintf("parent={dev=%d ino=%d nlink=%d ", parent.Dev, parent.Ino, parent.Nlink),
		fmt.Sprintf("named_lock={dev=%d ino=%d nlink=%d ", lock.Dev, lock.Ino, lock.Nlink),
		fmt.Sprintf("opened_lock={dev=%d ino=%d nlink=%d ", lock.Dev, lock.Ino, lock.Nlink),
	} {
		if !strings.Contains(failure.Error(), want) {
			t.Fatalf("missing diagnostic %q: %v", want, failure)
		}
	}
	if strings.Contains(failure.Error(), "probe_error=") || strings.Contains(failure.Error(), "not opened") {
		t.Fatalf("valid retained metadata reported unavailable: %v", failure)
	}
}

func TestBootstrapLockErrorReportsUnavailableProbesWithoutZeroIdentities(t *testing.T) {
	failure := bootstrapLockError("open", -1, -1, true, unix.ENOENT)
	if !errors.Is(failure, unix.ENOENT) || errors.Is(failure, unix.EBADF) {
		t.Fatalf("metadata probes replaced the original failure: %v", failure)
	}
	for _, want := range []string{
		"open bootstrap lock " + fmt.Sprintf("%q", defaultLockName),
		"parent={probe_error=", "named_lock={probe_error=", "opened_lock={not opened}",
	} {
		if !strings.Contains(failure.Error(), want) {
			t.Fatalf("missing unavailable diagnostic %q: %v", want, failure)
		}
	}
	if strings.Contains(failure.Error(), "dev=") || strings.Contains(failure.Error(), "ino=") || strings.Contains(failure.Error(), "nlink=") {
		t.Fatalf("failed probes fabricated identity metadata: %v", failure)
	}
}

func defaultStoreTestHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("canonicalize temporary fixture home: %v", err)
	}
	return home
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}
