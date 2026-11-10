//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func routerWatcherTestHome(t *testing.T) (string, func() (string, error)) {
	t.Helper()
	home := t.TempDir()
	return home, func() (string, error) { return home, nil }
}

func TestRouterWatcherPrivatePublicationAndOwnPID(t *testing.T) {
	home, resolve := routerWatcherTestHome(t)
	c, err := newRouterWatcherCustodyWith("thr-private", "fixture", "/fixture/router", resolve, bytes.NewReader(bytes.Repeat([]byte{1}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.record.PID != os.Getpid() || c.retired() {
		t.Fatal("publication must bind this watcher, not a caller PID")
	}
	dir := filepath.Join(home, ".sirsi", "router-watchers")
	for path, want := range map[string]os.FileMode{dir: 0700, filepath.Join(dir, c.leaf): 0600} {
		stat, err := os.Lstat(path)
		if err != nil || stat.Mode().Perm() != want {
			t.Fatalf("private custody permissions %s: stat=%v err=%v", path, stat, err)
		}
	}
	requested, err := requestRouterWatcherStopWith(c.record.ThreadID, resolve)
	if err != nil || !requested || !c.retired() {
		t.Fatalf("own cooperative request failed: requested=%v err=%v", requested, err)
	}
	next, err := newRouterWatcherCustodyWith(c.record.ThreadID, c.record.AgentID, c.record.RouterRoot, resolve, bytes.NewReader(bytes.Repeat([]byte{2}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if !c.retired() || next.retired() {
		t.Fatal("successor must ignore old stop even with the same actual PID")
	}
	publication, err := os.ReadFile(filepath.Join(dir, c.leaf))
	if err != nil {
		t.Fatal(err)
	}
	if foreign, err := newRouterWatcherCustodyWith(c.record.ThreadID, "other-agent", c.record.RouterRoot, resolve, bytes.NewReader(bytes.Repeat([]byte{3}, 32))); err == nil {
		foreign.Close()
		t.Fatal("different agent took over watcher lease")
	}
	if foreign, err := newRouterWatcherCustodyWith(c.record.ThreadID, c.record.AgentID, "/other/router", resolve, bytes.NewReader(bytes.Repeat([]byte{4}, 32))); err == nil {
		foreign.Close()
		t.Fatal("different router took over watcher lease")
	}
	retained, err := os.ReadFile(filepath.Join(dir, c.leaf))
	if err != nil || !bytes.Equal(publication, retained) {
		t.Fatal("foreign scope request changed retained lease")
	}
}

func TestRouterWatcherCustodyRejectsDirectorySubstitution(t *testing.T) {
	for _, kind := range []string{"home-symlink", "sirsi-symlink", "leaf-symlink", "shared-home", "shared-sirsi", "shared-leaf"} {
		t.Run(kind, func(t *testing.T) {
			home, _ := routerWatcherTestHome(t)
			base := filepath.Join(home, ".sirsi")
			leaf := filepath.Join(base, "router-watchers")
			if err := os.MkdirAll(leaf, 0700); err != nil {
				t.Fatal(err)
			}
			path := home
			if kind == "sirsi-symlink" || kind == "shared-sirsi" {
				path = base
			}
			if kind == "leaf-symlink" || kind == "shared-leaf" {
				path = leaf
			}
			if kind == "home-symlink" {
				link := filepath.Join(t.TempDir(), "home-link")
				if err := os.Symlink(home, link); err != nil {
					t.Fatal(err)
				}
				home = link
			} else if kind == "sirsi-symlink" || kind == "leaf-symlink" {
				target := path + "-actual"
				if err := os.Rename(path, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Chmod(path, 0777); err != nil {
				t.Fatal(err)
			}
			files, err := openRouterWatcherFiles(func() (string, error) { return home, nil }, true)
			if err == nil {
				files.Close()
				t.Fatal("substituted/shared custody directory accepted")
			}
		})
	}
}

func TestRouterWatcherPreservesUnsafeOrLegacyPrivateRecord(t *testing.T) {
	for _, kind := range []string{"numeric", "symlink", "hardlink", "fifo", "permissions", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			home, resolve := routerWatcherTestHome(t)
			files, err := openRouterWatcherFiles(resolve, true)
			if err != nil {
				t.Fatal(err)
			}
			defer files.Close()
			leaf, err := routerWatcherLeaf("thr-unsafe")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(home, ".sirsi", "router-watchers", leaf)
			target := filepath.Join(home, "foreign-marker")
			fixture, _ := memoryRouterWatcher(t)
			record := fixture.record
			record.ThreadID, record.PID = "thr-unsafe", 424242
			original, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, original, 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "symlink":
				err = os.Symlink(target, path)
			case "hardlink":
				err = os.Link(target, path)
			case "fifo":
				err = unix.Mkfifo(path, 0600)
			default:
				data, mode := original, os.FileMode(0600)
				if kind == "numeric" {
					data = []byte("424242\n")
				}
				if kind == "permissions" {
					mode = 0644
				}
				if kind == "oversize" {
					data = bytes.Repeat([]byte("x"), 4097)
				}
				err = os.WriteFile(path, data, mode)
				if err == nil && kind == "permissions" {
					err = os.Chmod(path, 0644) // decisive even under a private umask
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if requested, err := requestRouterWatcherStopWith("thr-unsafe", resolve); err == nil || requested {
				t.Fatalf("unsafe record authorized retirement: requested=%v err=%v", requested, err)
			}
			if c, err := newRouterWatcherCustodyWith("thr-unsafe", "fixture", "/fixture/router", resolve, bytes.NewReader(make([]byte, 32))); err == nil {
				c.Close()
				t.Fatal("unsafe/legacy record taken over")
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("unsafe record removed or replaced")
			}
			data, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(data, original) {
				t.Fatal("foreign marker modified")
			}
		})
	}
}

func TestRouterWatcherMissingHomeAndEntropyAreNotRetirement(t *testing.T) {
	t.Run("publication-cleanup-outcomes", func(t *testing.T) {
		original := errors.New("original publication failure")
		for _, prior := range []error{nil, original} {
			for _, cleanup := range []error{nil, unix.ENOENT, unix.EACCES, unix.EIO} {
				got := routerWatcherPublicationCleanupError(prior, cleanup)
				if cleanup == nil || errors.Is(cleanup, unix.ENOENT) {
					if got != prior {
						t.Fatal("expected absent temporary name changed publication outcome")
					}
					continue
				}
				if !errors.Is(got, cleanup) || (prior != nil && !errors.Is(got, prior)) {
					t.Fatal("real cleanup uncertainty or original publication failure was hidden")
				}
			}
		}
	})
	_, resolve := routerWatcherTestHome(t)
	if c, err := newRouterWatcherCustodyWith("thr-entropy", "fixture", "/fixture/router", resolve, bytes.NewReader(nil)); err == nil {
		c.Close()
		t.Fatal("failed entropy authorized publication")
	}
	missingHome := func() (string, error) { return "", os.ErrPermission }
	if requested, err := requestRouterWatcherStopWith("thr-missing", missingHome); requested || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("HOME failure hidden: requested=%v err=%v", requested, err)
	}
	if requested, err := requestRouterWatcherStopWith("thr-missing", resolve); requested || err != nil {
		t.Fatalf("missing private directory is not a stop receipt: requested=%v err=%v", requested, err)
	}
}

// All replacement directories are private, correctly owned and correctly
// formatted: mode/record checks alone would pass. The old open descriptors must
// cease admitting callbacks when any canonical directory link is replaced.
func TestRouterWatcherPostOpenDirectoryReplacementRetiresOldLease(t *testing.T) {
	for _, replaced := range []string{"home", "sirsi", "router-watchers"} {
		t.Run(replaced, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "home")
			if err := os.Mkdir(home, 0700); err != nil {
				t.Fatal(err)
			}
			resolve := func() (string, error) { return home, nil }
			old, err := newRouterWatcherCustodyWith("thr-replaced", "fixture", "/fixture/router", resolve, bytes.NewReader(bytes.Repeat([]byte{1}, 32)))
			if err != nil {
				t.Fatal(err)
			}
			defer old.Close()
			if old.retired() {
				t.Fatal("linked original lease must remain active before replacement")
			}
			canonical := filepath.Join(home, ".sirsi", "router-watchers", old.leaf)
			publication, err := os.ReadFile(canonical)
			if err != nil {
				t.Fatal(err)
			}
			path, suffix := home, filepath.Join(".sirsi", "router-watchers", old.leaf)
			if replaced == "sirsi" {
				path, suffix = filepath.Join(home, ".sirsi"), filepath.Join("router-watchers", old.leaf)
			}
			if replaced == "router-watchers" {
				path, suffix = filepath.Join(home, ".sirsi", "router-watchers"), old.leaf
			}
			detached := path + "-detached"
			if err := os.Rename(path, detached); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			successor, err := newRouterWatcherCustodyWith(old.record.ThreadID, old.record.AgentID, old.record.RouterRoot, resolve, bytes.NewReader(bytes.Repeat([]byte{2}, 32)))
			if err != nil {
				t.Fatal(err)
			}
			defer successor.Close()
			current, err := os.ReadFile(canonical)
			if err != nil || bytes.Equal(current, publication) {
				t.Fatal("replacement must contain a distinct successor publication")
			}
			if _, err := old.files.read(old.leaf); err == nil {
				t.Fatal("detached directory still authorized a publication read")
			}
			if err := old.files.write(old.leaf, publication); err == nil {
				t.Fatal("detached directory still authorized a publication write")
			}
			if !old.retired() || successor.retired() {
				t.Fatal("only the canonically linked successor may remain armed")
			}
			var oldGate, successorGate routerWatcherDispatchGate
			if oldGate.begin(context.Background(), old) {
				oldGate.end()
				t.Fatal("old detached watcher admitted a new callback")
			}
			if !successorGate.begin(context.Background(), successor) {
				t.Fatal("linked successor callback denied")
			}
			successorGate.end()
			retained, err := os.ReadFile(filepath.Join(detached, suffix))
			if err != nil || !bytes.Equal(retained, publication) {
				t.Fatal("old detached publication was modified or removed")
			}
			retained, err = os.ReadFile(canonical)
			if err != nil || !bytes.Equal(retained, current) {
				t.Fatal("old lease operation modified successor publication")
			}
		})
	}
}
