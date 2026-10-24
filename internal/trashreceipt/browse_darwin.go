//go:build darwin

package trashreceipt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

// Browse cannot retain an FD across separate CLI processes. Native read-only
// browsing owns its stream in SirsiEngine; Go Select/Apply remain authoritative.
func Browse(q BrowseRequest) Page {
	return pageFailure(errStatelessBrowseUnsupported)
}

// Internal selected-scope fixture projector. Production Browse never enters
// this former stateless path; tests supply a retained, bounded directory reader.
func browseHomeWith(home string, q BrowseRequest, read func(int, uint64) (directoryChunk, error), stat func(int, string) (Identity, error)) Page {
	if q.Schema != BrowseSchema || q.Directory != "." && !validRelative(q.Directory) || q.Position != 0 && (q.Generation == "" || q.Root == nil) {
		return pageFailure(fmt.Errorf("invalid browse path or continuation"))
	}
	t, err := openTree(home)
	if err != nil {
		return pageFailure(err)
	}
	defer t.close()
	if q.Root != nil && !stable(*q.Root, t.manifest.Root) {
		return pageFailure(fmt.Errorf("Trash root changed; browse again"))
	}
	if q.Directory != "." {
		current := ""
		for _, part := range strings.Split(q.Directory, "/") {
			if current == "" {
				current = part
			} else {
				current += "/" + part
			}
			i, err := statAt(t.dirs[path.Dir(current)], part)
			if err != nil {
				return pageFailure(err)
			}
			if err := t.retainContainer(Entry{Path: current, Identity: i}); err != nil {
				return pageFailure(err)
			}
		}
	}
	fd := t.dirs[q.Directory]
	before, err := statFD(fd)
	if err != nil {
		return pageFailure(err)
	}
	// Generation binds this directory, root identity and retained ancestor
	// identities, not a claimed global inventory or deletion authorization.
	stamp := struct {
		Directory  string
		Root       Identity
		Containers []Entry
		Current    Identity
	}{q.Directory, t.manifest.Root, append([]Entry(nil), t.manifest.Containers...), before}
	stamp.Root.Size = 0
	stamp.Root.MtimeNS = 0
	stamp.Root.CtimeNS = 0
	for i := range stamp.Containers {
		stamp.Containers[i].Identity.Size = 0
		stamp.Containers[i].Identity.MtimeNS = 0
		stamp.Containers[i].Identity.CtimeNS = 0
	}
	raw, _ := json.Marshal(stamp)
	sum := sha256.Sum256(raw)
	generation := hex.EncodeToString(sum[:])
	if q.Generation != "" && q.Generation != generation {
		return pageFailure(fmt.Errorf("Browse page changed; refresh this folder"))
	}
	readFD, err := openDirectory(fd, ".")
	if err != nil {
		return pageFailure(err)
	}
	defer closeDirectory(readFD)
	chunk, err := read(readFD, q.Position)
	if err != nil {
		return pageFailure(err)
	}
	if chunk.Calls != 1 || chunk.Bytes < 0 || chunk.Bytes > DirectoryChunkBytes || len(chunk.Names) > MaxBrowseRows || chunk.EOF && len(chunk.Names) != 0 || !chunk.EOF && chunk.Next == q.Position {
		return pageFailure(fmt.Errorf("invalid or nonprogressing bounded directory read"))
	}
	names := chunk.Names
	sort.Strings(names)
	entries := []Entry{}
	blocked := []BlockedEntry{}
	for _, name := range names {
		if name == "" {
			blocked = append(blocked, BlockedEntry{Path: "(name unavailable)", Reason: "name_unavailable", Type: "unknown"})
			continue
		}
		rel := name
		if q.Directory != "." {
			rel = q.Directory + "/" + name
		}
		if !validRelative(rel) || strings.Contains(name, "/") {
			// Display metadata only, never a round-trippable deletion path.
			display := []rune(strings.ToValidUTF8(rel, "�"))
			if len(display) > 1024 {
				display = display[:1024]
			}
			blocked = append(blocked, BlockedEntry{Path: string(display), Reason: "invalid_path", Type: "unknown"})
			continue
		}
		i, err := stat(fd, name)
		if err != nil {
			blocked = append(blocked, BlockedEntry{Path: rel, Reason: "stat_failed", Type: "unknown"})
			continue
		}
		reason := ""
		switch {
		case !validIdentity(i):
			reason = "unsupported_type"
		case i.UID != t.manifest.UID:
			reason = "unowned"
		case i.Device != t.manifest.Root.Device:
			reason = "cross_device"
		}
		if reason != "" {
			blocked = append(blocked, BlockedEntry{Path: rel, Reason: reason, Type: i.Type})
			continue
		}
		entries = append(entries, Entry{Path: rel, Identity: i})
	}
	after, err := statFD(fd)
	if err != nil || after != before {
		return pageFailure(fmt.Errorf("folder changed during browse; refresh this folder"))
	}
	if err := t.bindings(q.Directory, t.containerIdentities()); err != nil {
		return pageFailure(err)
	}
	p := Page{Schema: BrowseSchema, Status: "page", UID: t.manifest.UID, Root: t.manifest.Root, Directory: q.Directory, Containers: t.manifest.Containers, Entries: entries, Blocked: blocked, Work: BrowseWork{NativeCalls: chunk.Calls, Bytes: chunk.Bytes, Inspected: len(names)}, Generation: generation, More: !chunk.EOF, Errors: []string{}}
	if p.More {
		p.Next = &BrowseRequest{Schema: BrowseSchema, Directory: q.Directory, Position: chunk.Next, Generation: generation, Root: &p.Root}
	}
	return p
}

func (t *tree) containerIdentities() map[string]Identity {
	ids := map[string]Identity{}
	for _, e := range t.manifest.Containers {
		ids[e.Path] = e.Identity
	}
	return ids
}

func (t *tree) retainContainer(e Entry) error {
	if !validRelative(e.Path) || e.Identity.Type != "directory" || !validIdentity(e.Identity) || e.Identity.UID != t.manifest.UID || e.Identity.Device != t.manifest.Root.Device || len(t.dirs)-1 >= MaxDirectories {
		return fmt.Errorf("invalid container or container bound")
	}
	parent, ok := t.dirs[path.Dir(e.Path)]
	if !ok {
		return fmt.Errorf("missing container ancestor")
	}
	current, err := statAt(parent, path.Base(e.Path))
	if err != nil || !stable(current, e.Identity) {
		return fmt.Errorf("container binding changed")
	}
	fd, err := openDirectory(parent, path.Base(e.Path))
	if err != nil {
		return err
	}
	t.dirs[e.Path] = fd
	opened, err := statFD(fd)
	if err != nil || !stable(opened, e.Identity) {
		return fmt.Errorf("opened container changed")
	}
	t.manifest.Containers = append(t.manifest.Containers, Entry{Path: e.Path, Identity: opened})
	return nil
}

// captureSelected discovers descendants only for explicitly selected whole
// directories, before consent/apply. Containers themselves are never selected.
func (t *tree) captureSelected(selected []Entry) error {
	selectedPaths := map[string]bool{}
	for _, e := range selected {
		selectedPaths[e.Path] = true
	}
	for _, e := range selected {
		if selectedPaths[path.Dir(e.Path)] {
			continue
		}
		parent, ok := t.dirs[path.Dir(e.Path)]
		if !ok {
			return fmt.Errorf("selected parent is not retained")
		}
		current, err := statAt(parent, path.Base(e.Path))
		if err != nil || current != e.Identity {
			return fmt.Errorf("selected entry changed; review it again")
		}
		t.manifest.Entries = append(t.manifest.Entries, Entry{Path: e.Path, Identity: current})
		if current.Type == "directory" {
			if len(t.dirs)-1 >= MaxDirectories {
				return fmt.Errorf("whole folder exceeds this batch; open it and select descendants")
			}
			fd, err := openDirectory(parent, path.Base(e.Path))
			if err != nil {
				return err
			}
			t.dirs[e.Path] = fd
			opened, err := statFD(fd)
			if err != nil || opened != current {
				return fmt.Errorf("selected directory changed")
			}
			if err := t.scan(e.Path, strings.Count(e.Path, "/")+1); err != nil {
				return fmt.Errorf("whole folder exceeds or changed during review; open it and select descendants: %w", err)
			}
		}
	}
	sort.Slice(t.manifest.Entries, func(i, j int) bool { return t.manifest.Entries[i].Path < t.manifest.Entries[j].Path })
	if err := Validate(t.manifest); err != nil {
		return fmt.Errorf("selection exceeds this batch; choose fewer entries or open the folder: %w", err)
	}
	for _, e := range t.manifest.Containers {
		if err := t.bindings(e.Path, t.containerIdentities()); err != nil {
			return err
		}
	}
	root, err := statAt(t.home, ".Trash")
	if err != nil || !stable(root, t.manifest.Root) {
		return fmt.Errorf("Trash root changed")
	}
	return nil
}

func Select(q SelectionRequest) Report {
	home, err := os.UserHomeDir()
	if err != nil {
		return Failure(err)
	}
	return selectionHome(home, q)
}

func selectionHome(home string, q SelectionRequest) Report {
	m := Manifest{Schema: ManifestSchema, UID: q.UID, Root: q.Root, Containers: q.Containers, Entries: q.Entries}
	if q.Schema != SelectionSchema || len(q.Entries) == 0 || len(q.Entries) > PageSize {
		return Failure(fmt.Errorf("choose 1..32 entries"))
	}
	if err := Validate(m); err != nil {
		return Failure(err)
	}
	t, err := openTree(home)
	if err != nil {
		return Failure(err)
	}
	defer t.close()
	if !stable(q.Root, t.manifest.Root) || q.UID != t.manifest.UID {
		return Failure(fmt.Errorf("selected root or owner changed"))
	}
	for _, e := range q.Containers {
		if err := t.retainContainer(e); err != nil {
			return Failure(err)
		}
	}
	if err := t.captureSelected(q.Entries); err != nil {
		return Failure(err)
	}
	r := report("ready")
	r.Preview = &t.manifest
	return r
}
