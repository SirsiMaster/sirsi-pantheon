//go:build darwin

package trashreceipt

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// The original exact-coverage version at immutable b966f2c9 failed on the real
// qualified filesystem (receipt pr42NX). It is retained there, not skipped.
// Regression now demands explicit refusal/no data from this unsupported route;
// restoring b966's apparent cookie success must fail this assertion. Full exact
// coverage is independently required of TrashBrowseSessionTests' retained stream.
func TestDarwinDirectoryPositionAcrossFreshOpens(t *testing.T) {
	_, root := ownedTrash(t)
	const total = 1000
	for i := 0; i < total; i++ {
		put(t, filepath.Join(root, fmt.Sprintf("owned-%04d", i)))
	}
	for _, position := range []uint64{0, 1, 264, ^uint64(0)} {
		fd, err := unix.Open(root, dirFlags, 0)
		if err != nil {
			t.Fatal(err)
		}
		chunk, err := readDirectoryChunk(fd, position)
		closeDirectory(fd)
		if !errors.Is(err, errStatelessBrowseUnsupported) || len(chunk.Names) != 0 || chunk.EOF || chunk.Calls != 0 {
			t.Fatalf("unsupported fresh-open cursor returned apparent data/success at %d: %+v %v", position, chunk, err)
		}
	}
	if p := Browse(firstBrowse()); p.Status != "failed" || len(p.Entries) != 0 || p.Next != nil {
		t.Fatal("public stateless browse advertised success", p)
	}
}

func TestTrashBrowseMixedBlockedRowsDoNotPoisonSiblings(t *testing.T) {
	home, root := ownedTrash(t)
	for _, name := range []string{"good", "stat", "owner", "device"} {
		put(t, filepath.Join(root, name))
	}
	if err := unix.Mkfifo(filepath.Join(root, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	browse := retainedFixtureBrowser(t, home, func(fd int, name string) (Identity, error) {
		calls++
		if name == "stat" {
			return Identity{}, unix.EACCES
		}
		i, err := statAt(fd, name)
		if name == "owner" {
			i.UID++
		}
		if name == "device" {
			i.Device++
		}
		return i, err
	})
	p := browse(firstBrowse())
	if p.Status != "page" || calls != 5 || p.Work.NativeCalls != 1 || len(p.Entries) != 1 || p.Entries[0].Path != "good" || len(p.Blocked) != 4 {
		t.Fatalf("mixed page: %+v calls=%d", p, calls)
	}
	reasons := map[string]string{}
	for _, row := range p.Blocked {
		reasons[row.Path] = row.Reason
	}
	for name, reason := range map[string]string{"stat": "stat_failed", "owner": "unowned", "device": "cross_device", "fifo": "unsupported_type"} {
		if reasons[name] != reason {
			t.Fatalf("%s: got %q want %q", name, reasons[name], reason)
		}
	}
	if p.Work.Inspected != 5 || p.Work.Bytes > DirectoryChunkBytes {
		t.Fatalf("work bound: %+v", p.Work)
	}
	// Only the eligible row is supplied to selected-scope capture.
	m := mustSelection(t, home, selectedFrom(p, p.Entries))
	if len(m.Entries) != 1 || m.Entries[0].Path != "good" {
		t.Fatal("blocked row entered selected manifest")
	}
}

func TestTrashBrowseAllBlockedPageAdvancesToLaterEligible(t *testing.T) {
	home, root := ownedTrash(t)
	for i := 0; i < 600; i++ {
		put(t, filepath.Join(root, fmt.Sprintf("member-%04d", i)))
	}
	blocked := true
	browse := retainedFixtureBrowser(t, home, func(fd int, name string) (Identity, error) {
		if blocked {
			return Identity{}, unix.EACCES
		}
		return statAt(fd, name)
	})
	p := browse(firstBrowse())
	if p.Status != "page" || len(p.Entries) != 0 || len(p.Blocked) == 0 || p.Next == nil || !p.More {
		t.Fatalf("blocked page lost continuation: %+v", p)
	}
	seen := map[string]bool{}
	for _, e := range p.Blocked {
		seen[e.Path] = true
	}
	q := *p.Next
	blocked = false
	eligible := 0
	for requests := 0; requests < 601; requests++ {
		later := browse(q)
		if later.Status != "page" {
			t.Fatalf("retained page: %+v", later)
		}
		if later.Work.NativeCalls != 1 || later.Work.Bytes > DirectoryChunkBytes || later.Work.Inspected > MaxBrowseRows {
			t.Fatalf("unbounded page: %+v", later.Work)
		}
		for _, e := range later.Entries {
			if seen[e.Path] {
				t.Fatal("cursor restarted at blocked name", e.Path)
			}
			seen[e.Path] = true
			eligible++
		}
		if !later.More {
			if eligible == 0 || len(seen) != 600 {
				t.Fatalf("later entries unreachable: eligible=%d total=%d", eligible, len(seen))
			}
			return
		}
		q = *later.Next
	}
	t.Fatal("continuation failed to terminate")
}
