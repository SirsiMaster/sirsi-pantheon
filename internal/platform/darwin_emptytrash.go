//go:build darwin

package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

// TrashContents lists the top-level entries in ~/.Trash with their sizes.
// Read-only: it never deletes and is safe to call from any surface.
func (d *Darwin) TrashContents() ([]TrashEntry, error) {
	dir, err := trashDir()
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no Trash yet is not an error
		}
		return nil, fmt.Errorf("read Trash: %w", err)
	}
	var out []TrashEntry
	for _, e := range ents {
		if e.Name() == ".DS_Store" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		out = append(out, TrashEntry{Name: e.Name(), Path: p, Bytes: dirBytes(p)})
	}
	return out, nil
}

// EmptyTrash PERMANENTLY deletes the named entries from ~/.Trash. This is the
// one operation in the codebase with no undo, so containment is enforced here
// rather than trusted from the caller:
//
//   - every path must resolve (via EvalSymlinks, so a symlinked entry cannot
//     point the delete somewhere else) to a DIRECT child of ~/.Trash;
//   - ~/.Trash itself is never removed, only its contents;
//   - a path that fails containment is refused and reported, not skipped
//     silently — a permanent delete that quietly declines is worse than one
//     that errors, because the caller reports success either way.
//
// Returns the entries actually deleted and the bytes freed.
func (d *Darwin) EmptyTrash(paths []string) (deleted []string, freed int64, err error) {
	dir, err := trashDir()
	if err != nil {
		return nil, 0, err
	}
	realTrash, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve Trash: %w", err)
	}
	for _, p := range paths {
		abs, aerr := filepath.Abs(p)
		if aerr != nil {
			return deleted, freed, fmt.Errorf("resolve %q: %w", p, aerr)
		}
		// Resolve symlinks on the PARENT, not the entry: EvalSymlinks on the
		// entry itself would follow a symlinked item to its target and delete
		// that instead — the containment check must describe where the entry
		// LIVES, not where it points.
		parent, perr := filepath.EvalSymlinks(filepath.Dir(abs))
		if perr != nil {
			return deleted, freed, fmt.Errorf("resolve parent of %q: %w", p, perr)
		}
		if parent != realTrash {
			return deleted, freed, fmt.Errorf(
				"refusing to permanently delete %q: it is not a direct child of %s", p, realTrash)
		}
		if filepath.Clean(abs) == realTrash {
			return deleted, freed, fmt.Errorf("refusing to delete the Trash directory itself")
		}
		sz := dirBytes(abs)
		if rerr := os.RemoveAll(abs); rerr != nil {
			return deleted, freed, fmt.Errorf("delete %q: %w", p, rerr)
		}
		deleted = append(deleted, abs)
		freed += sz
	}
	return deleted, freed, nil
}

func trashDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".Trash"), nil
}

// dirBytes sums a file or tree. Unreadable entries contribute 0 rather than
// failing the whole size read — a size is advisory, a refusal is not.
func dirBytes(p string) int64 {
	var total int64
	_ = filepath.Walk(p, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}
