//go:build darwin

package trashreceipt

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func firstBrowse() BrowseRequest { return BrowseRequest{Schema: BrowseSchema, Directory: "."} }
func mustPage(t *testing.T, home string, q BrowseRequest) Page {
	t.Helper()
	p := retainedFixtureBrowser(t, home, statAt)(q)
	if p.Status != "page" {
		t.Fatalf("page: %+v", p)
	}
	return p
}

// Go selected-manifest fixtures need read-only rows, not a fake public cookie.
// This fixture retains one os.File stream throughout a traversal. Its sequence
// is local to this closure; it is never exposed as a filesystem seek offset.
func retainedFixtureBrowser(t *testing.T, home string, stat func(int, string) (Identity, error)) func(BrowseRequest) Page {
	t.Helper()
	var file *os.File
	var sequence uint64
	return func(q BrowseRequest) Page {
		return browseHomeWith(home, q, func(fd int, position uint64) (directoryChunk, error) {
			if position != sequence {
				return directoryChunk{}, fmt.Errorf("fixture sequence mismatch")
			}
			if file == nil {
				opened, err := unix.Openat(fd, ".", dirFlags, 0)
				if err != nil {
					return directoryChunk{}, err
				}
				file = os.NewFile(uintptr(opened), "owned retained fixture directory")
				t.Cleanup(func() { _ = file.Close() })
			}
			names, err := file.Readdirnames(PageSize)
			if err != nil && err != io.EOF {
				return directoryChunk{}, err
			}
			bytes := 0
			for _, name := range names {
				bytes += len(name) + 1
			}
			sequence++
			return directoryChunk{Names: names, Next: sequence, EOF: len(names) == 0, Bytes: bytes, Calls: 1}, nil
		}, stat)
	}
}
func selectedFrom(p Page, entries []Entry) SelectionRequest {
	return SelectionRequest{Schema: SelectionSchema, UID: p.UID, Root: p.Root, Containers: p.Containers, Entries: entries}
}
func mustSelection(t *testing.T, home string, q SelectionRequest) Manifest {
	t.Helper()
	r := selectionHome(home, q)
	if r.Preview == nil || r.Status != "ready" {
		t.Fatalf("selection: %+v", r)
	}
	return *r.Preview
}

func TestTrashBrowseBeyondWholeTreeLimitAndUnselectedSiblingsSurvive(t *testing.T) {
	home, root := ownedTrash(t)
	for i := 0; i < MaxEntries+3; i++ {
		put(t, filepath.Join(root, fmt.Sprintf("item-%04d", i)))
	}
	q := firstBrowse()
	seen := map[string]bool{}
	var first Page
	browse := retainedFixtureBrowser(t, home, statAt)
	for pages := 0; ; pages++ {
		if pages > MaxEntries {
			t.Fatal("continuation did not terminate")
		}
		p := browse(q)
		if p.Status != "page" {
			t.Fatalf("retained fixture page: %+v", p)
		}
		if pages == 0 {
			first = p
		}
		if len(p.Entries) > MaxBrowseRows {
			t.Fatal("page bound exceeded")
		}
		for _, e := range p.Entries {
			if seen[e.Path] {
				t.Fatal("duplicate page entry")
			}
			seen[e.Path] = true
		}
		if !p.More {
			if p.Next != nil {
				t.Fatal("unexpected continuation")
			}
			break
		}
		if p.Next == nil || p.Next.Generation != p.Generation || p.Next.Root == nil {
			t.Fatal("continuation lacks identity/generation")
		}
		q = *p.Next
	}
	if len(seen) != MaxEntries+3 {
		t.Fatalf("saw %d members", len(seen))
	}
	m := mustSelection(t, home, selectedFrom(first, first.Entries[:1]))
	put(t, filepath.Join(root, "unselected-late"))
	r := applyHome(home, m, true, nil)
	if r.Status != "completed" || len(r.Deleted) != 1 {
		t.Fatalf("unrelated sibling blocked selected preflight: %+v", r)
	}
	for _, name := range []string{"unselected-late"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("unselected sibling lost: %s", name)
		}
	}
	for name := range seen {
		if name != m.Entries[0].Path {
			if _, err := os.Stat(filepath.Join(root, name)); err != nil {
				t.Fatal("unselected sibling lost", name, err)
			}
		}
	}
}

func TestTrashOversizedFolderCanBeContainerForDescendantSelection(t *testing.T) {
	home, root := ownedTrash(t)
	folder := filepath.Join(root, "build-cache")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxDirectories+2; i++ {
		dir := filepath.Join(folder, fmt.Sprintf("directory-%03d", i))
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		put(t, filepath.Join(dir, "artifact"))
	}
	p := mustPage(t, home, firstBrowse())
	if r := selectionHome(home, selectedFrom(p, p.Entries)); r.Status != "failed" || r.Attempted != 0 {
		t.Fatalf("oversized whole folder unexpectedly selected: %+v", r)
	}
	inside := mustPage(t, home, BrowseRequest{Schema: BrowseSchema, Directory: "build-cache", Root: &p.Root})
	if !inside.More {
		t.Fatal("large folder must remain paged and navigable")
	}
	deep := mustPage(t, home, BrowseRequest{Schema: BrowseSchema, Directory: inside.Entries[0].Path, Root: &p.Root})
	m := mustSelection(t, home, selectedFrom(deep, deep.Entries[:1]))
	if len(m.Containers) != 2 || len(m.Entries) != 1 {
		t.Fatalf("container/selected scope mixed: %+v", m)
	}
	put(t, filepath.Join(folder, "unselected-sibling"))
	r := applyHome(home, m, true, nil)
	if r.Status != "completed" {
		t.Fatalf("container sibling must not cause global drift refusal: %+v", r)
	}
	for _, p := range []string{folder, filepath.Dir(filepath.Join(root, m.Entries[0].Path)), filepath.Join(folder, "directory-065", "artifact"), filepath.Join(folder, "unselected-sibling")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal("container or unselected entry lost", p, err)
		}
	}
}

func TestTrashSelectedDriftAndContainerReplacementRefuseBeforeAnyDelete(t *testing.T) {
	for _, change := range []string{"selected-file", "container"} {
		t.Run(change, func(t *testing.T) {
			home, root := ownedTrash(t)
			dir := filepath.Join(root, "container")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			put(t, filepath.Join(dir, "selected"))
			p := mustPage(t, home, BrowseRequest{Schema: BrowseSchema, Directory: "container"})
			m := mustSelection(t, home, selectedFrom(p, p.Entries))
			if change == "container" {
				if err := os.Rename(dir, filepath.Join(home, "retained-container")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(filepath.Join(dir, "selected"), []byte("changed selected bytes"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			r := applyHome(home, m, true, nil)
			if r.Status != "review_required" || r.Attempted != 0 || len(r.Deleted) != 0 {
				t.Fatalf("selected drift attempted deletion: %+v", r)
			}
		})
	}
}

func TestTrashBrowseGenerationRefusesStaleCursorAndCannotAuthorizeApply(t *testing.T) {
	home, root := ownedTrash(t)
	for i := 0; i < PageSize+2; i++ {
		put(t, filepath.Join(root, fmt.Sprintf("entry-%03d", i)))
	}
	browse := retainedFixtureBrowser(t, home, statAt)
	p := browse(firstBrowse())
	if p.Next == nil {
		t.Fatal("missing next page")
	}
	raw, err := json.Marshal(p.Next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(strings.NewReader(string(raw))); err == nil {
		t.Fatal("browse cursor accepted as deletion manifest")
	}
	put(t, filepath.Join(root, "new-generation"))
	if next := browse(*p.Next); next.Status != "failed" {
		t.Fatal("stale cursor accepted")
	}
}

func TestTrashSelectionWireHasClosedKeysAnd64KiBBound(t *testing.T) {
	home, root := ownedTrash(t)
	put(t, filepath.Join(root, "selected"))
	p := mustPage(t, home, firstBrowse())
	q := selectedFrom(p, p.Entries)
	raw, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeSelection(strings.NewReader(string(raw))); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeSelection(strings.NewReader(string(raw) + strings.Repeat(" ", MaxManifestBytes))); err == nil {
		t.Fatal("oversized request accepted")
	}
	bad := strings.Replace(string(raw), `"schema":`, `"SCHEMA":"alias","schema":`, 1)
	if _, err := DecodeSelection(strings.NewReader(bad)); err == nil {
		t.Fatal("selection case alias accepted")
	}
	initial, err := json.Marshal(firstBrowse())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeBrowse(strings.NewReader(string(initial))); err != nil {
		t.Fatal(err)
	}
	bad = strings.Replace(string(initial), `"directory":`, `"DIRECTORY":"alias","directory":`, 1)
	if _, err := DecodeBrowse(strings.NewReader(bad)); err == nil {
		t.Fatal("browse case alias accepted")
	}
}
