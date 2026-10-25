//go:build darwin && cgo

package setup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func maintenanceOwnedDirectory(t *testing.T) (string, *os.File) {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return path, f
}

func TestMaintenanceCustodyCrossPathBoundary(t *testing.T) {
	path, _ := maintenanceOwnedDirectory(t)
	id := strings.Repeat("a", 32)
	budget := new(servicedrain.CustodyBudget)
	controllerCount := 0
	for laneIndex := 0; laneIndex < 2; laneIndex++ {
		name := fmt.Sprintf("lane-%d", laneIndex)
		lanePath := filepath.Join(path, name)
		custody := filepath.Join(lanePath, "failed", ".maintenance-"+id)
		if err := os.MkdirAll(custody, 0700); err != nil {
			t.Fatal(err)
		}
		for _, dir := range []string{lanePath, filepath.Dir(custody), custody} {
			if err := os.Chown(dir, os.Getuid(), os.Getgid()); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chmod(custody, 0750); err != nil {
			t.Fatal(err)
		}
		// Inert names-only custody records. No queued body, inference, sender,
		// network or durability claim is part of this dimension-bound fixture.
		for index := 0; index < 16385; index++ {
			for _, prefix := range []string{"audit-", "envelope-"} {
				if err := os.WriteFile(filepath.Join(custody, fmt.Sprintf("%s%064x", prefix, index)), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
		lane, err := os.Open(lanePath)
		if err != nil {
			t.Fatal(err)
		}
		count, err := maintenancePreserveLane(context.Background(), lane, []byte(name), id, os.Getuid(), os.Getgid(), budget)
		closeErr := lane.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
		controllerCount += count
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	successorCount, err := routerstore.CountMaintenanceCustody(context.Background(), root, id)
	if err != nil || controllerCount != 32770 || successorCount != controllerCount || budget.Entries != 65540 {
		t.Fatalf("controller=%d successor=%d entries=%d err=%v", controllerCount, successorCount, budget.Entries, err)
	}
}

func TestLegacyMaintenanceRetainsUnknownEnvelopesWithoutReplay(t *testing.T) {
	path, lane := maintenanceOwnedDirectory(t)
	for _, queue := range []string{"req", "inflight", "outbox", "res"} {
		if err := os.Mkdir(filepath.Join(path, queue), 0700); err != nil {
			t.Fatal(err)
		}
	}
	var originals []os.FileInfo
	for _, queue := range []string{"req", "inflight", "outbox"} {
		name := filepath.Join(path, queue, "opaque-malformed.tmp")
		if err := os.WriteFile(name, []byte("owned opaque body, not JSON; never interpret or forward"), 0600); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		originals = append(originals, info)
	}
	response := []byte("already published response must survive")
	if err := os.WriteFile(filepath.Join(path, "res", "response.json"), response, 0600); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("1", 32)
	n, err := maintenancePreserveLane(context.Background(), lane, []byte("codex-pantheon"), id, os.Getuid(), os.Getgid(), new(servicedrain.CustodyBudget))
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("preserved %d want3", n)
	}
	for _, queue := range []string{"req", "inflight", "outbox"} {
		entries, err := os.ReadDir(filepath.Join(path, queue))
		if err != nil || len(entries) != 0 {
			t.Fatalf("%s not preserved: %v %v", queue, entries, err)
		}
	}
	custody := filepath.Join(path, "failed", ".maintenance-"+id)
	entries, err := os.ReadDir(custody)
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range originals {
		found := false
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), "envelope-") {
				continue
			}
			now, err := os.Stat(filepath.Join(custody, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if os.SameFile(old, now) && old.Size() == now.Size() {
				found = true
			}
		}
		if !found {
			t.Fatal("original envelope identity lost")
		}
	}
	if got, err := os.ReadFile(filepath.Join(path, "res", "response.json")); err != nil || !bytes.Equal(got, response) {
		t.Fatal("existing response changed")
	}
	// A restarted controller re-counts retained custody; no duplicate moves or
	// inference about an old remote commit follows from local envelope custody.
	if got, err := maintenancePreserveLane(context.Background(), lane, []byte("codex-pantheon"), id, os.Getuid(), os.Getgid(), new(servicedrain.CustodyBudget)); err != nil || got != 3 {
		t.Fatalf("recovery %d %v", got, err)
	}
}

func TestLegacyMaintenanceUnknownQueueObjectStaysInPlace(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			path, lane := maintenanceOwnedDirectory(t)
			if err := os.Mkdir(filepath.Join(path, "inflight"), 0700); err != nil {
				t.Fatal(err)
			}
			leaf := filepath.Join(path, "inflight", "unknown")
			switch kind {
			case "symlink":
				if err := os.Symlink("missing", leaf); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(leaf, 0600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				source := filepath.Join(path, "original")
				if err := os.WriteFile(source, []byte("owned"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(source, leaf); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := maintenancePreserveLane(context.Background(), lane, []byte("owned"), strings.Repeat("2", 32), os.Getuid(), os.Getgid(), new(servicedrain.CustodyBudget)); err == nil {
				t.Fatal("unknown queue object accepted")
			}
			if _, err := os.Lstat(leaf); err != nil {
				t.Fatal("unknown object removed", err)
			}
		})
	}
}

func TestLegacyMaintenanceJournalCASAndRetainedDamage(t *testing.T) {
	path, _ := maintenanceOwnedDirectory(t)
	j := maintenanceJournal{path, uint32(os.Getuid())}
	s := MaintenanceStatus{Schema: 1, ID: strings.Repeat("1", 32), Revision: 1, Plan: fixtureMaintenancePlan(), Phase: "prepared", OldOutcomes: "unknown_not_certified_drained"}
	if err := j.persist(0, s); err != nil {
		t.Fatal(err)
	}
	if got, err := j.load(); err != nil || got != s {
		t.Fatalf("readback %+v %v", got, err)
	}
	if err := j.persist(0, s); err == nil {
		t.Fatal("stale append accepted")
	}
	bad := s
	bad.Revision++
	bad.Phase = "activated"
	if err := j.persist(1, bad); err == nil {
		t.Fatal("skipped retirement/custody accepted")
	}
	leaf := filepath.Join(path, maintenanceJournalLeaf)
	f, err := os.OpenFile(leaf, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"incomplete":`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := j.load(); err == nil {
		t.Fatal("torn phase accepted")
	}
	if _, err := os.Stat(leaf); errors.Is(err, os.ErrNotExist) {
		t.Fatal("damaged journal discarded")
	}
}
