//go:build darwin

package setup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func relaySpoolFixture(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func relaySpoolQueue(t *testing.T, root, queue string) string {
	t.Helper()
	path := filepath.Join(root, "codex-pantheon", queue)
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRootRelaySpoolOrdinaryEmptyLanes(t *testing.T) {
	root := relaySpoolFixture(t)
	if err := rootRelaySpoolQuiescentAt(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	// Ordinary letters x/r/n/t must not be treated as escaped control bytes.
	for _, queue := range []string{"req", "res", "inflight", "outbox", "failed", "slots"} {
		relaySpoolQueue(t, root, queue)
	}
	// Responses, parked failures and client slots are not forwarding sources.
	for _, queue := range []string{"res", "failed", "slots"} {
		if err := os.WriteFile(filepath.Join(root, "codex-pantheon", queue, "fixture"), []byte("retained"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := rootRelaySpoolQuiescentAt(context.Background(), root, nil); err != nil {
		t.Fatalf("ordinary empty queues refused: %v", err)
	}
	// Existing lanes with truly absent queue directories are also valid.
	if err := os.Mkdir(filepath.Join(root, "ordinary-runtime-x0"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := rootRelaySpoolQuiescentAt(context.Background(), root, nil); err != nil {
		t.Fatalf("absent queues refused: %v", err)
	}
}

func TestRootRelaySpoolForwardMarkersRefusedWithoutOpeningBodies(t *testing.T) {
	for _, queue := range []string{"req", "outbox", "inflight"} {
		t.Run(queue, func(t *testing.T) {
			root := relaySpoolFixture(t)
			dir := relaySpoolQueue(t, root, queue)
			// A FIFO has no readable request body and would hang a content read;
			// the probe must refuse from directory names alone, without opening it.
			if err := unix.Mkfifo(filepath.Join(dir, "ambiguous.json"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := rootRelaySpoolQuiescentAt(context.Background(), root, nil); err == nil {
				t.Fatal("forwarding marker accepted")
			}
		})
	}
}

func TestRootRelaySpoolUnknownAndSymlinkDirectoriesRefused(t *testing.T) {
	for _, name := range []string{"lane-symlink", "queue-symlink", "root-symlink", "ancestor-symlink", "unknown-entry", "root-file", "queue-file", "unreadable", "malformed-lane"} {
		t.Run(name, func(t *testing.T) {
			root := relaySpoolFixture(t)
			dir := relaySpoolQueue(t, root, "req")
			var err error
			switch name {
			case "lane-symlink":
				err = os.Symlink(t.TempDir(), filepath.Join(root, "other-lane"))
			case "queue-symlink":
				err = os.Symlink(t.TempDir(), filepath.Join(root, "codex-pantheon", "outbox"))
			case "root-symlink":
				link := filepath.Join(relaySpoolFixture(t), "spool-link")
				err = os.Symlink(root, link)
				root = link
			case "ancestor-symlink":
				base := relaySpoolFixture(t)
				link := filepath.Join(relaySpoolFixture(t), "parent-link")
				if err = os.Mkdir(filepath.Join(base, "spool"), 0700); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(base, link)
				root = filepath.Join(link, "spool")
			case "unknown-entry":
				err = os.Mkdir(filepath.Join(root, "codex-pantheon", "unknown"), 0700)
			case "root-file":
				err = os.WriteFile(filepath.Join(root, "unknown"), nil, 0600)
			case "queue-file":
				err = os.WriteFile(filepath.Join(root, "codex-pantheon", "inflight"), nil, 0600)
			case "unreadable":
				err = os.Chmod(dir, 0000)
				t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
			case "malformed-lane":
				err = os.Mkdir(filepath.Join(root, "bad\nlane"), 0700)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := rootRelaySpoolQuiescentAt(context.Background(), root, nil); err == nil {
				t.Fatal("unknown or unsafe spool accepted")
			}
		})
	}
}

func TestRootRelaySpoolRetainedDirectoryContinuity(t *testing.T) {
	for _, name := range []string{"queue-replaced", "lane-replaced", "queue-arrival", "missing-queue-arrival"} {
		t.Run(name, func(t *testing.T) {
			root := relaySpoolFixture(t)
			dir := relaySpoolQueue(t, root, "req")
			called := false
			err := rootRelaySpoolQuiescentAt(context.Background(), root, func(lane string) {
				called = true
				if lane != "codex-pantheon" {
					t.Fatal("unexpected lane")
				}
				switch name {
				case "queue-replaced":
					if err := os.Rename(dir, dir+".retained"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(dir, 0700); err != nil {
						t.Fatal(err)
					}
				case "lane-replaced":
					lanePath := filepath.Dir(dir)
					if err := os.Rename(lanePath, lanePath+".retained"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(lanePath, 0700); err != nil {
						t.Fatal(err)
					}
				case "queue-arrival":
					if err := os.WriteFile(filepath.Join(dir, "new.json"), nil, 0600); err != nil {
						t.Fatal(err)
					}
				case "missing-queue-arrival":
					if err := os.Mkdir(filepath.Join(root, lane, "inflight"), 0700); err != nil {
						t.Fatal(err)
					}
				}
			})
			if err == nil || !called {
				t.Fatal("retained-directory change escaped probe")
			}
		})
	}
}

func TestRootRelaySpoolActualProbeBlocksBootoutWithNoChildren(t *testing.T) {
	for _, name := range []string{"inflight", "second-pass-arrival", "probe-error"} {
		t.Run(name, func(t *testing.T) {
			f, b := newRelayNativeFixture(t)
			root := relaySpoolFixture(t)
			dir := relaySpoolQueue(t, root, "inflight")
			if name == "inflight" {
				if err := os.WriteFile(filepath.Join(dir, "unknown.json"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if name == "probe-error" {
				root = filepath.Join(root, "absent")
			}
			b.relay.spoolClear = func(ctx context.Context) error {
				f.spoolReads++
				if name == "second-pass-arrival" && f.spoolReads == 2 {
					if err := os.WriteFile(filepath.Join(dir, "arrival.json"), nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
				return rootRelaySpoolQuiescentAt(ctx, root, nil)
			}
			if err := b.Refresh(context.Background(), relayInstanceFixture()); err == nil {
				t.Fatal("inflight/unknown spool accepted with no OS children")
			}
			if f.bootouts != 0 || f.bootstraps != 0 {
				t.Fatal("spool probe did not fence bootout")
			}
			if name == "second-pass-arrival" && (f.childReads != 2 || f.spoolReads != 2) {
				t.Fatal("second pass not exercised")
			}
		})
	}
}

func TestRootRelaySpoolEnumerationBoundAndCancellation(t *testing.T) {
	root := relaySpoolFixture(t)
	for i := 0; i < 17; i++ {
		// The lane bound is deliberately smaller than an untrusted directory.
		if err := os.MkdirAll(filepath.Join(root, "codex-pantheon", string(rune('a'+i))), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := rootRelaySpoolQuiescentAt(context.Background(), root, nil); err == nil {
		t.Fatal("unbounded lane enumeration accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rootRelaySpoolQuiescentAt(ctx, relaySpoolFixture(t), nil); err == nil {
		t.Fatal("canceled probe accepted")
	}
}
