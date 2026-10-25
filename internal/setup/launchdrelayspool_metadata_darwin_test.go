//go:build darwin

package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"golang.org/x/sys/unix"
)

const rootRelaySummaryFixtureName = ".relay-outbox-health-v1.json"

func relayMetadataRoot(t *testing.T, mode os.FileMode) string {
	t.Helper()
	root := relaySpoolFixture(t)
	if err := os.Chmod(root, mode); err != nil {
		t.Fatal(err)
	}
	for _, queue := range []string{"req", "outbox", "inflight", "res", "failed", "slots"} {
		relaySpoolQueue(t, root, queue)
	}
	return root
}

// Match the real producer's schema and create-only/rename publication. Actual
// attainable fixture ownership replaces Studio's reported UID502/GID501; the
// expected producer/root identity is never taken from the document itself.
func publishRelayMetadataFixture(t *testing.T, root string, mode os.FileMode, change func(*routerstore.RelayOutboxObservation)) []byte {
	t.Helper()
	dir, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(dir)
	var st unix.Stat_t
	if err := unix.Fstat(dir, &st); err != nil {
		t.Fatal(err)
	}
	zero, now := 0, time.Now().UTC()
	s := routerstore.RelayOutboxObservation{Version: 1, RootDevice: uint64(st.Dev), RootInode: uint64(st.Ino), ProducerUID: st.Uid,
		StartedAt: now, ObservedAt: now, Enumerated: true, Complete: true, Queued: &zero,
		Lanes: []routerstore.RelayLaneObservation{{Agent: "codex-pantheon", State: "observed", Queued: &zero}}}
	if change != nil {
		change(&s)
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	stage := ".relay-health-new-" + strings.Repeat("b", 32)
	fd, err := unix.Openat(dir, stage, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f := os.NewFile(uintptr(fd), stage)
	defer f.Close()
	if err := f.Chmod(mode); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := unix.Renameat(dir, stage, dir, rootRelaySummaryFixtureName); err != nil {
		t.Fatal(err)
	}
	if err := unix.Fsync(dir); err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertRelayMetadataUntouched(t *testing.T, root string, before os.FileInfo, raw []byte) {
	t.Helper()
	path := filepath.Join(root, rootRelaySummaryFixtureName)
	after, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !bytes.Equal(raw, got) {
		t.Fatal("quiescence reader changed the fixture metadata")
	}
}

func TestRootRelaySpoolDeployedReservedMetadataIsNotALane(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0640} {
		t.Run(strconv.FormatUint(uint64(mode), 8), func(t *testing.T) {
			rootMode := os.FileMode(0700)
			if mode == 0640 {
				rootMode = 0770
			}
			root := relayMetadataRoot(t, rootMode)
			raw := publishRelayMetadataFixture(t, root, mode, nil)
			before, err := os.Lstat(filepath.Join(root, rootRelaySummaryFixtureName))
			if err != nil {
				t.Fatal(err)
			}
			if err := rootRelaySpoolQuiescentAt(context.Background(), root, nil); err != nil {
				t.Fatalf("canonical regular%04o root summary was treated as a lane: %v", mode, err)
			}
			assertRelayMetadataUntouched(t, root, before, raw)
		})
	}
}

type relayMetadataNoNetwork struct{ calls atomic.Int32 }

func (r *relayMetadataNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls.Add(1)
	return nil, errors.New("fixture forbids network")
}

func TestRootRelaySpoolActualCanonicalPublisherThenMockRefresh(t *testing.T) {
	root := relayMetadataRoot(t, 0700)
	transport := &relayMetadataNoNetwork{}
	rl := &routerstore.Relay{Spool: root, Base: "https://fixture.invalid", Token: "inert-fixture-only",
		Client: &http.Client{Transport: transport}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	done := make(chan error, 1)
	joined := false
	// Cleanup is registered before launch and joins the only started producer.
	t.Cleanup(func() {
		cancel()
		if !joined {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("fixture producer did not stop")
			}
		}
	})
	go func() { done <- rl.Serve(ctx) }()
	clock := time.NewTicker(5 * time.Millisecond)
	defer clock.Stop()
	path := filepath.Join(root, rootRelaySummaryFixtureName)
	for {
		if _, err := os.Lstat(path); err == nil {
			break
		}
		select {
		case err := <-done:
			joined = true
			t.Fatalf("canonical producer ended before publishing metadata: %v", err)
		case <-ctx.Done():
			t.Fatal("canonical producer did not publish within fixture bound")
		case <-clock.C:
		}
	}
	cancel()
	select {
	case err := <-done:
		joined = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canonical producer did not join")
	}
	if transport.calls.Load() != 0 {
		t.Fatal("empty-queue producer attempted network")
	}
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, b := newRelayNativeFixture(t) // real/effective root and every privileged operation are inert seams.
	b.relay.spoolClear = func(ctx context.Context) error {
		f.spoolReads++
		return rootRelaySpoolQuiescentAt(ctx, root, nil)
	}
	if err := b.Refresh(context.Background(), relayInstanceFixture()); err != nil {
		t.Fatalf("actual publisher summary blocked mock refresh before bootout: %v (bootouts=%d)", err, f.bootouts)
	}
	if f.bootouts != 1 || f.bootstraps != 1 || f.spoolReads != 2 || f.childReads != 2 {
		t.Fatal("mock refresh lost its exact double-probe/once-only path")
	}
	assertRelayMetadataUntouched(t, root, before, raw)
}

func TestRootRelayReservedMetadataUnsafeObjectsRefused(t *testing.T) {
	for _, name := range []string{"unknown-file", "unknown-hidden-json", "staging-debt", "symlink", "directory", "fifo", "hardlink", "public-mode", "writable-mode", "wrong-root", "wrong-json-uid", "malformed", "duplicate", "case-alias", "trailing", "oversized"} {
		t.Run(name, func(t *testing.T) {
			root := relayMetadataRoot(t, 0700)
			raw := publishRelayMetadataFixture(t, root, 0600, nil)
			path := filepath.Join(root, rootRelaySummaryFixtureName)
			var err error
			switch name {
			case "unknown-file":
				err = os.WriteFile(filepath.Join(root, "unknown"), nil, 0600)
			case "unknown-hidden-json":
				err = os.WriteFile(filepath.Join(root, ".not-reserved.json"), raw, 0600)
			case "staging-debt":
				err = os.WriteFile(filepath.Join(root, ".relay-health-new-"+strings.Repeat("a", 32)), nil, 0600)
			case "symlink", "directory", "fifo":
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				switch name {
				case "symlink":
					err = os.Symlink(filepath.Join(t.TempDir(), "missing"), path)
				case "directory":
					err = os.Mkdir(path, 0700)
				case "fifo":
					err = unix.Mkfifo(path, 0600)
				}
			case "hardlink":
				err = os.Link(path, filepath.Join(t.TempDir(), "linked"))
			case "public-mode":
				err = os.Chmod(path, 0644)
			case "writable-mode":
				err = os.Chmod(path, 0660)
			case "wrong-root":
				publishRelayMetadataFixture(t, root, 0600, func(s *routerstore.RelayOutboxObservation) { s.RootInode++ })
			case "wrong-json-uid":
				publishRelayMetadataFixture(t, root, 0600, func(s *routerstore.RelayOutboxObservation) { s.ProducerUID++ })
			case "malformed":
				err = os.WriteFile(path, []byte("{}"), 0600)
			case "duplicate":
				err = os.WriteFile(path, bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1), 0600)
			case "case-alias":
				err = os.WriteFile(path, bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"Version":1`), 1), 0600)
			case "trailing":
				err = os.WriteFile(path, append(raw, []byte(" {}")...), 0600)
			case "oversized":
				err = os.WriteFile(path, bytes.Repeat([]byte(" "), 131073), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			f, b := newRelayNativeFixture(t)
			b.relay.spoolClear = func(ctx context.Context) error { f.spoolReads++; return rootRelaySpoolQuiescentAt(ctx, root, nil) }
			if err := b.Refresh(context.Background(), relayInstanceFixture()); err == nil {
				t.Fatal("unsafe root entry reached refresh")
			}
			if f.bootouts+f.bootstraps != 0 {
				t.Fatal("unsafe metadata escaped the pre-bootout guard")
			}
		})
	}
}

func TestRootRelayMetadataIsNotQueueProof(t *testing.T) {
	for _, queue := range []string{"req", "outbox", "inflight"} {
		t.Run(queue, func(t *testing.T) {
			root := relayMetadataRoot(t, 0700)
			publishRelayMetadataFixture(t, root, 0600, nil)
			marker := filepath.Join(root, "codex-pantheon", queue, "ambiguous.json")
			if err := unix.Mkfifo(marker, 0600); err != nil {
				t.Fatal(err)
			}
			f, b := newRelayNativeFixture(t)
			b.relay.spoolClear = func(ctx context.Context) error { f.spoolReads++; return rootRelaySpoolQuiescentAt(ctx, root, nil) }
			if err := b.Refresh(context.Background(), relayInstanceFixture()); err == nil {
				t.Fatal("summary zero bypassed real forwarding marker")
			}
			if f.bootouts+f.bootstraps != 0 {
				t.Fatal("real queue marker escaped retirement guard")
			}
			if st, err := os.Lstat(marker); err != nil || st.Mode()&os.ModeNamedPipe == 0 {
				t.Fatal("queue body was changed")
			}
		})
	}
}
