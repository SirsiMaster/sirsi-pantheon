//go:build darwin

package trashreceipt

import (
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const dirFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC

type tree struct {
	home     int
	dirs     map[string]int
	manifest Manifest
}

func identity(s *unix.Stat_t) Identity {
	t := "unsupported"
	switch s.Mode & unix.S_IFMT {
	case unix.S_IFREG:
		t = "file"
	case unix.S_IFDIR:
		t = "directory"
	case unix.S_IFLNK:
		t = "symlink"
	}
	return Identity{Device: uint64(s.Dev), Inode: s.Ino, UID: s.Uid, Mode: uint32(s.Mode), Size: s.Size, MtimeNS: s.Mtim.Nano(), CtimeNS: s.Ctim.Nano(), Type: t}
}

func statAt(fd int, name string) (Identity, error) {
	var s unix.Stat_t
	err := unix.Fstatat(fd, name, &s, unix.AT_SYMLINK_NOFOLLOW)
	return identity(&s), err
}

func statFD(fd int) (Identity, error) {
	var s unix.Stat_t
	err := unix.Fstat(fd, &s)
	return identity(&s), err
}

func (t *tree) close() {
	for _, fd := range t.dirs {
		_ = unix.Close(fd)
	}
	_ = unix.Close(t.home)
}

// No caller-selected root is exposed. The home argument is an internal owned
// fixture seam; public entry points resolve the invoking owner's home only.
func openTree(home string) (*tree, error) {
	h, err := unix.Open(home, dirFlags, 0)
	if err != nil {
		return nil, fmt.Errorf("open owner home: %w", err)
	}
	t := &tree{home: h, dirs: map[string]int{}}
	root, err := unix.Openat(h, ".Trash", dirFlags, 0)
	if err != nil {
		t.close()
		return nil, fmt.Errorf("open Trash without following links: %w", err)
	}
	t.dirs["."] = root
	i, err := statFD(root)
	if err != nil || !validIdentity(i) || i.UID != uint32(os.Geteuid()) || i.Type != "directory" {
		t.close()
		return nil, fmt.Errorf("Trash root is not the invoking owner's directory")
	}
	t.manifest = Manifest{Schema: ManifestSchema, UID: uint32(os.Geteuid()), Root: i, Containers: []Entry{}, Entries: []Entry{}}
	return t, nil
}

func (t *tree) scan(relative string, depth int) error {
	if depth > MaxDepth {
		return fmt.Errorf("Trash nesting exceeds bound")
	}
	fd := t.dirs[relative]
	before, err := statFD(fd)
	if err != nil {
		return err
	}
	// A fresh description avoids sharing a directory offset with retained FDs.
	readFD, err := unix.Openat(fd, ".", dirFlags, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(readFD), "owned Trash directory")
	names, readErr := f.Readdirnames(MaxEntries - len(t.manifest.Entries) + 1)
	_ = f.Close()
	if readErr != nil && readErr != io.EOF {
		return readErr
	}
	if len(names)+len(t.manifest.Entries) > MaxEntries {
		return fmt.Errorf("Trash entry bound exceeded")
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "." || name == ".." || strings.Contains(name, "/") {
			return fmt.Errorf("invalid Trash directory member")
		}
		rel := name
		if relative != "." {
			rel = relative + "/" + name
		}
		i, err := statAt(fd, name)
		if err != nil {
			return err
		}
		if !validIdentity(i) || i.UID != t.manifest.UID || i.Device != t.manifest.Root.Device {
			return fmt.Errorf("unsupported or unowned Trash member")
		}
		t.manifest.Entries = append(t.manifest.Entries, Entry{Path: rel, Identity: i})
		if len(t.manifest.Entries) > MaxEntries {
			return fmt.Errorf("Trash entry bound exceeded")
		}
		if i.Type == "directory" {
			if len(t.dirs)-1 >= MaxDirectories {
				return fmt.Errorf("Trash directory bound exceeded")
			}
			child, err := unix.Openat(fd, name, dirFlags, 0)
			if err != nil {
				return err
			}
			t.dirs[rel] = child
			opened, err := statFD(child)
			if err != nil || opened != i {
				return fmt.Errorf("Trash directory changed while opening")
			}
			if err := t.scan(rel, depth+1); err != nil {
				return err
			}
		}
	}
	after, err := statFD(fd)
	if err != nil || before != after {
		return fmt.Errorf("Trash directory changed during preview")
	}
	return nil
}

func (t *tree) snapshot() error {
	if err := t.scan(".", 0); err != nil {
		return err
	}
	sort.Slice(t.manifest.Entries, func(i, j int) bool { return t.manifest.Entries[i].Path < t.manifest.Entries[j].Path })
	root, err := statAt(t.home, ".Trash")
	if err != nil || root != t.manifest.Root {
		return fmt.Errorf("Trash root changed during preview")
	}
	return Validate(t.manifest)
}

func Preview() Report {
	home, err := os.UserHomeDir()
	if err != nil {
		return Failure(err)
	}
	return previewHome(home)
}

func previewHome(home string) Report {
	t, err := openTree(home)
	if err != nil {
		return Failure(err)
	}
	defer t.close()
	if err := t.snapshot(); err != nil {
		return Failure(err)
	}
	r := report("ready")
	if len(t.manifest.Entries) == 0 {
		r.Status = "empty"
	}
	r.Preview = &t.manifest
	return r
}

func Apply(m Manifest, confirmed bool) Report {
	home, err := os.UserHomeDir()
	if err != nil {
		return Failure(err)
	}
	return applyHome(home, m, confirmed, nil)
}

func openDirectory(parent int, name string) (int, error) {
	return unix.Openat(parent, name, dirFlags, 0)
}

func closeDirectory(fd int) { _ = unix.Close(fd) }

func stable(a, b Identity) bool {
	return a.Device == b.Device && a.Inode == b.Inode && a.UID == b.UID && a.Mode == b.Mode && a.Type == b.Type
}

// Retained descriptors do not make stat/unlink atomic against hostile writers.
// Recheck the visible root and every reviewed parent binding before each unlink.
func (t *tree) bindings(parent string, reviewed map[string]Identity) error {
	root, err := statAt(t.home, ".Trash")
	if err != nil || !stable(root, t.manifest.Root) {
		return fmt.Errorf("Trash root binding changed")
	}
	if parent == "." {
		return nil
	}
	for current := parent; current != "."; current = path.Dir(current) {
		i, err := statAt(t.dirs[path.Dir(current)], path.Base(current))
		if err != nil || !stable(i, reviewed[current]) {
			return fmt.Errorf("reviewed Trash parent binding changed")
		}
	}
	return nil
}

// afterPreflight is per-invocation and fixture-only. Production never supplies
// it; tests can prove that late additions are not discovered or deleted.
func applyHome(home string, m Manifest, confirmed bool, afterPreflight func()) Report {
	r := report("review_required")
	refuse := func(err error) Report { r.Errors = append(r.Errors, err.Error()); return r }
	if !confirmed {
		return refuse(fmt.Errorf("explicit permanent-delete confirmation is required"))
	}
	if err := Validate(m); err != nil {
		return refuse(err)
	}
	if m.UID != uint32(os.Geteuid()) {
		return refuse(fmt.Errorf("review belongs to another effective UID"))
	}
	t, err := openTree(home)
	if err != nil {
		return refuse(err)
	}
	defer t.close()
	if !stable(m.Root, t.manifest.Root) {
		return refuse(fmt.Errorf("Trash root identity changed"))
	}
	for _, e := range m.Containers {
		if err := t.retainContainer(e); err != nil {
			return refuse(err)
		}
	}
	if err := t.captureSelected(m.Entries); err != nil {
		return refuse(err)
	}
	if !reflect.DeepEqual(m.Entries, t.manifest.Entries) {
		return refuse(fmt.Errorf("Selected Trash scope changed since review; review selected items again"))
	}
	if afterPreflight != nil {
		afterPreflight()
	}
	reviewed := map[string]Identity{}
	for _, e := range m.Containers {
		reviewed[e.Path] = e.Identity
	}
	for _, e := range m.Entries {
		reviewed[e.Path] = e.Identity
	}
	for index := len(m.Entries) - 1; index >= 0; index-- {
		e := m.Entries[index]
		parent := path.Dir(e.Path)
		err := t.bindings(parent, reviewed)
		if err == nil {
			current, statErr := statAt(t.dirs[parent], path.Base(e.Path))
			err = statErr
			if err == nil && (e.Identity.Type != "directory" && current != e.Identity || e.Identity.Type == "directory" && !stable(current, e.Identity)) {
				err = fmt.Errorf("reviewed Trash member identity changed")
			}
		}
		if err == nil {
			flags := 0
			if e.Identity.Type == "directory" {
				flags = unix.AT_REMOVEDIR
			}
			r.Attempted++
			err = unix.Unlinkat(t.dirs[parent], path.Base(e.Path), flags)
		}
		if err != nil {
			r.Errors = append(r.Errors, e.Path+": "+err.Error())
			if len(r.Deleted) > 0 {
				r.Status = "partial"
			} else if r.Attempted > 0 {
				r.Status = "failed"
			}
			return r
		}
		r.Deleted = append(r.Deleted, e.Path)
		if e.Identity.Type != "directory" {
			r.LogicalBytes += e.Identity.Size
		}
	}
	r.Status = "completed"
	return r
}
