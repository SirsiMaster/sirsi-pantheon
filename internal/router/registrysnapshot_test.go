package router

import (
	"errors"
	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const pinnedJSON = `{"agents":{"lane-a":{"type":"claude","workstream":"w","wake":{"mechanism":"launchagent"}}}}`
const treeJSON = `{"agents":{"lane-tree":{"type":"claude","workstream":"w","wake":{"mechanism":"none"}}}}`

func repoWithTree(t *testing.T) (root string) {
	t.Helper()
	repo := t.TempDir()
	root = filepath.Join(repo, ".agents", "idea-router")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(treeJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	setSnapshotDirForTest(filepath.Join(t.TempDir(), "registry"))
	t.Cleanup(func() { setSnapshotDirForTest("") })
	return root
}

func fakeGit(show string) GitRunner {
	return func(_ string, args ...string) (string, error) {
		switch args[0] {
		case "fetch":
			return "", nil
		case "rev-parse":
			return "abc123\n", nil
		case "show":
			if show == "" {
				return "", errors.New("no such path")
			}
			return show, nil
		}
		return "", errors.New("unexpected git " + args[0])
	}
}

// Before a sync the working tree is the source (unchanged behavior); after one the
// origin snapshot wins over a working-tree edit and writes are refused; unpinning
// restores the working tree (both directions).
func TestRegistryReadsOriginSnapshotOnceSynced(t *testing.T) {
	root := repoWithTree(t)
	if reg, err := LoadRegistry(root); err != nil || reg.Agents["lane-tree"].ID == "" {
		t.Fatalf("unsynced host must read the working tree: %+v err=%v", reg, err)
	}
	if _, err := SyncRegistrySnapshot(root, fakeGit(pinnedJSON)); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadRegistry(root)
	if err != nil || reg.Agents["lane-a"].ID == "" || len(reg.Agents) != 1 {
		t.Fatalf("synced host must read origin, not the working tree: %+v err=%v", reg, err)
	}
	if err := SaveRegistry(root, reg); !errors.Is(err, ErrRegistryPinned) {
		t.Fatalf("a pinned host must refuse registry writes, got %v", err)
	}
	if err := UnpinRegistry(); err != nil {
		t.Fatal(err)
	}
	if reg, _ := LoadRegistry(root); reg.Agents["lane-tree"].ID == "" {
		t.Fatal("unpinned host must read the working tree again")
	}
}

// A bad origin file must never replace a good source, and a stale snapshot is not
// trusted (both directions).
func TestRegistrySyncRefusesBadOriginAndStaleSnapshotIsIgnored(t *testing.T) {
	root := repoWithTree(t)
	if _, err := SyncRegistrySnapshot(root, fakeGit(pinnedJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncRegistrySnapshot(root, fakeGit("{not json")); err == nil || !strings.Contains(err.Error(), "not a usable registry") {
		t.Fatalf("garbage origin accepted: %v", err)
	}
	if _, err := SyncRegistrySnapshot(root, fakeGit(`{"agents":{}}`)); err == nil {
		t.Fatal("an empty registry must not replace the snapshot")
	}
	if reg, _ := LoadRegistry(root); reg.Agents["lane-a"].ID == "" {
		t.Fatal("a failed sync must leave the previous snapshot in place")
	}
	_, mp := snapshotPaths()
	old := `{"router_root":"` + filepath.Clean(root) + `","sha256":"x","source_commit":"y","fetched_at":"` + time.Now().Add(-100*time.Hour).UTC().Format(time.RFC3339) + `"}`
	if err := os.WriteFile(mp, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if reg, _ := LoadRegistry(root); reg.Agents["lane-tree"].ID == "" {
		t.Fatal("a snapshot older than the max age must not be trusted")
	}
}

// A snapshot belongs to one checkout: another router root reads its own tree even
// while this host is pinned.
func TestRegistrySnapshotAppliesOnlyToItsOwnRouterRoot(t *testing.T) {
	root := repoWithTree(t)
	if _, err := SyncRegistrySnapshot(root, fakeGit(pinnedJSON)); err != nil {
		t.Fatal(err)
	}
	other := repoWithTree2(t)
	if reg, _ := LoadRegistry(other); reg.Agents["lane-tree"].ID == "" {
		t.Fatal("another router root must read its own agents.json, not this host's snapshot")
	}
}

func repoWithTree2(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".agents", "idea-router")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(treeJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// A send is validated against the same registry the fabric reads: a lane declared only
// on origin must be addressable once synced, and one declared only in the working tree
// must not be (both directions; before this, dispatch always read the working tree).
func TestDispatchValidatesAgainstThePinnedRegistry(t *testing.T) {
	root := repoWithTree(t)
	decl := func(id string) string {
		return `{"agents":{"` + id + `":{"id":"` + id + `","type":"claude","repo":"/r","workstream":"w","wake":{"mechanism":"none"}}}}`
	}
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(decl("lane-tree")), 0o644); err != nil {
		t.Fatal(err)
	}
	f := dispatch.New(root, nil)
	if err := f.ValidateAgent("to", "lane-tree"); err != nil {
		t.Fatalf("unsynced host must accept the working-tree lane: %v", err)
	}
	if _, err := SyncRegistrySnapshot(root, fakeGit(decl("lane-a"))); err != nil {
		t.Fatal(err)
	}
	if err := f.ValidateAgent("to", "lane-a"); err != nil {
		t.Fatalf("lane declared on origin must be addressable once synced: %v", err)
	}
	if err := f.ValidateAgent("to", "lane-tree"); err == nil {
		t.Fatal("lane declared only in the working tree must not be addressable on a pinned host")
	}
}
