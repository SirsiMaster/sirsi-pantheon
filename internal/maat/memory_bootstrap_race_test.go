package maat

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestBootstrapLockConcurrentAppearanceAcquiresOnlyExistingIdentity(t *testing.T) {
	for _, cause := range []error{unix.ENOENT, unix.EEXIST} {
		t.Run(cause.Error(), func(t *testing.T) {
			root := defaultStoreTestHome(t)
			parent, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(parent)
			calls := 0
			var created unix.Stat_t
			fd, err := openBootstrapLockWith(parent, true, func(dir int, name string, flags int, mode uint32) (int, error) {
				calls++
				if calls == 1 {
					if flags&unix.O_CREAT == 0 || flags&unix.O_EXCL == 0 {
						t.Fatalf("first acquisition is not exclusive creation: %#x", flags)
					}
					other, err := unix.Openat(dir, name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
					if err != nil {
						t.Fatal(err)
					}
					if err := unix.Fstat(other, &created); err != nil {
						t.Fatal(err)
					}
					if err := unix.Close(other); err != nil {
						t.Fatal(err)
					}
					return -1, cause
				}
				if calls != 2 || flags&unix.O_CREAT != 0 || flags&unix.O_NOFOLLOW == 0 {
					t.Fatalf("existing acquisition flags=%#x calls=%d", flags, calls)
				}
				return unix.Openat(dir, name, flags, mode)
			})
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(fd)
			var actual unix.Stat_t
			if err := unix.Fstat(fd, &actual); err != nil || calls != 2 || actual.Dev != created.Dev || actual.Ino != created.Ino {
				t.Fatalf("wrong acquired object/calls: calls=%d stat=%+v err=%v", calls, actual, err)
			}
			if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
				t.Fatalf("retained object is not lockable: %v", err)
			}
			if err := validateBootstrapLockName(parent, fd); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBootstrapLockPostFlockSubstitutionRefusesStoreAccess(t *testing.T) {
	root := defaultStoreTestHome(t)
	parent, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(parent)
	fd, err := openBootstrapLock(parent, true)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	path := filepath.Join(root, defaultLockName)
	if err := os.Rename(path, filepath.Join(root, "original-lock")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if err := validateBootstrapLockName(parent, fd); err == nil {
		t.Fatal("locking the displaced inode authorized store access")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "replacement" {
		t.Fatalf("replacement changed: %q err=%v", data, err)
	}
}

func TestBootstrapLockConcurrentAppearanceRefusesUnsafeOrSubstitutedObject(t *testing.T) {
	for _, scenario := range []string{"absent", "symlink", "public-mode", "hardlink", "fifo", "substitute-before-open", "unlink-before-open"} {
		t.Run(scenario, func(t *testing.T) {
			root := defaultStoreTestHome(t)
			parent, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(parent)
			path := filepath.Join(root, defaultLockName)
			calls := 0
			fd, err := openBootstrapLockWith(parent, true, func(dir int, name string, flags int, mode uint32) (int, error) {
				calls++
				if calls == 1 {
					switch scenario {
					case "absent":
					case "symlink":
						if err := os.Symlink("missing-target", path); err != nil {
							t.Fatal(err)
						}
					case "fifo":
						if err := unix.Mkfifo(path, 0o600); err != nil {
							t.Fatal(err)
						}
					default:
						if err := os.WriteFile(path, []byte("preserve"), 0o600); err != nil {
							t.Fatal(err)
						}
						if scenario == "public-mode" {
							if err := os.Chmod(path, 0o644); err != nil {
								t.Fatal(err)
							}
						}
						if scenario == "hardlink" {
							if err := os.Link(path, filepath.Join(root, "other-link")); err != nil {
								t.Fatal(err)
							}
						}
					}
					return -1, unix.ENOENT
				}
				if calls != 2 || flags&unix.O_CREAT != 0 {
					t.Fatalf("unexpected creating retry: calls=%d flags=%#x", calls, flags)
				}
				if err := os.Rename(path, filepath.Join(root, "retained-original")); err != nil {
					t.Fatal(err)
				}
				if scenario == "substitute-before-open" {
					if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				return unix.Openat(dir, name, flags, mode)
			})
			if fd >= 0 {
				_ = unix.Close(fd)
			}
			if err == nil {
				t.Fatal("unsafe/substituted object accepted")
			}
			wantCalls := 1
			if scenario == "substitute-before-open" || scenario == "unlink-before-open" {
				wantCalls = 2
				if original, err := os.ReadFile(filepath.Join(root, "retained-original")); err != nil || string(original) != "preserve" {
					t.Fatalf("original changed: %q err=%v", original, err)
				}
			}
			if calls != wantCalls {
				t.Fatalf("calls=%d want%d", calls, wantCalls)
			}
			if scenario == "unlink-before-open" || scenario == "absent" {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed acquisition recreated a leaf: %v", err)
				}
			}
		})
	}
}

func TestBootstrapLockNonCreationAndUnrelatedErrorsNeverRetry(t *testing.T) {
	root := defaultStoreTestHome(t)
	parent, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(parent)
	for _, test := range []struct {
		create bool
		cause  error
	}{{false, unix.ENOENT}, {true, unix.EACCES}, {true, unix.EIO}} {
		calls := 0
		fd, err := openBootstrapLockWith(parent, test.create, func(int, string, int, uint32) (int, error) {
			calls++
			return -1, test.cause
		})
		if fd >= 0 || calls != 1 || !errors.Is(err, test.cause) {
			t.Fatalf("error/calls changed: fd=%d calls=%d err=%v", fd, calls, err)
		}
	}
}
