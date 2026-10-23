//go:build darwin

package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// launchd argv uses /var, but native inspection walks only its known physical
// namespace, never a symlink alias supplied by an environment or caller.
const rootRelayPhysicalSpool = "/private/var/sirsipantheon/relay"

type relaySpoolDirectory struct {
	file   *os.File
	info   os.FileInfo
	parent int
	name   string
}

func openRelaySpoolDirectory(parent int, name string) (relaySpoolDirectory, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return relaySpoolDirectory{}, err
	}
	f := os.NewFile(uintptr(fd), name)
	info, err := f.Stat()
	if err != nil || !info.IsDir() || info.Mode().Perm()&0444 == 0 || info.Mode().Perm()&0111 == 0 {
		_ = f.Close()
		return relaySpoolDirectory{}, errors.New("service lifecycle: relay spool directory unreadable or invalid")
	}
	return relaySpoolDirectory{file: f, info: info, parent: parent, name: name}, nil
}

func (d relaySpoolDirectory) verify(stable bool) error {
	var opened, named unix.Stat_t
	if err := unix.Fstat(int(d.file.Fd()), &opened); err != nil {
		return err
	}
	if err := unix.Fstatat(d.parent, d.name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if named.Mode&unix.S_IFMT != unix.S_IFDIR || opened.Dev != named.Dev || opened.Ino != named.Ino {
		return errors.New("service lifecycle: relay spool directory name changed")
	}
	after, err := d.file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(d.info, after) || d.info.Mode() != after.Mode() ||
		(stable && (!d.info.ModTime().Equal(after.ModTime()) || d.info.Size() != after.Size())) {
		return errors.New("service lifecycle: relay spool directory changed during quiescence sample")
	}
	return nil
}

func relaySpoolEntries(dir *os.File, limit int) ([]os.DirEntry, error) {
	entries, err := dir.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > limit {
		return nil, errors.New("service lifecycle: relay spool enumeration exceeds bound")
	}
	return entries, nil
}

// Observe marker names only: never open request bodies, move/delete markers or
// replay ambiguous inflight work. beforeVerify is a per-call fixture seam, nil
// in production. This is a sample, not a writer lock: --drained must additionally
// mean sources are quiesced. Legacy Serve cancellation does not join forwards;
// an actual cutover still needs bounded owner quiescence, preserved spool and
// explicit recovery of outcome-unknown semantics, not automatic replay here.
func rootRelaySpoolQuiescentAt(ctx context.Context, physical string, beforeVerify func(string)) error {
	if !filepath.IsAbs(physical) || filepath.Clean(physical) != physical || physical == "/" {
		return errors.New("service lifecycle: canonical physical relay spool path required")
	}
	anchor, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(anchor)
	var chain []relaySpoolDirectory
	defer func() {
		for i := len(chain) - 1; i >= 0; i-- {
			_ = chain[i].file.Close()
		}
	}()
	parent := anchor
	for _, name := range strings.Split(strings.TrimPrefix(physical, "/"), "/") {
		if err := ctx.Err(); err != nil {
			return err
		}
		dir, err := openRelaySpoolDirectory(parent, name)
		if err != nil {
			return fmt.Errorf("service lifecycle: relay spool path unavailable: %w", err)
		}
		chain = append(chain, dir)
		parent = int(dir.file.Fd())
	}
	root := chain[len(chain)-1]
	lanes, err := relaySpoolEntries(root.file, 1024)
	if err != nil {
		return err
	}
	for _, entry := range lanes {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if name == "." || name == ".." || len(name) > 255 || strings.ContainsAny(name, "/\x00\r\n\t") {
			return errors.New("service lifecycle: malformed relay spool lane")
		}
		if err := rootRelayLaneQuiescent(ctx, root, name, beforeVerify); err != nil {
			return err
		}
	}
	for i, dir := range chain {
		if err := dir.verify(i == len(chain)-1); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func rootRelayLaneQuiescent(ctx context.Context, root relaySpoolDirectory, name string, beforeVerify func(string)) error {
	lane, err := openRelaySpoolDirectory(int(root.file.Fd()), name)
	if err != nil {
		return fmt.Errorf("service lifecycle: relay lane unavailable: %w", err)
	}
	defer lane.file.Close()
	entries, err := relaySpoolEntries(lane.file, 16)
	if err != nil {
		return err
	}
	var dirs []relaySpoolDirectory
	defer func() {
		for _, dir := range dirs {
			_ = dir.file.Close()
		}
	}()
	found := map[string]bool{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		queue := entry.Name()
		switch queue {
		case "req", "outbox", "inflight", "res", "failed", "slots":
		default:
			return errors.New("service lifecycle: unknown relay lane entry")
		}
		dir, err := openRelaySpoolDirectory(int(lane.file.Fd()), queue)
		if err != nil {
			return fmt.Errorf("service lifecycle: relay queue directory unavailable: %w", err)
		}
		dirs = append(dirs, dir)
		found[queue] = true
		if queue == "req" || queue == "outbox" || queue == "inflight" {
			markers, err := relaySpoolEntries(dir.file, 0)
			if err != nil || len(markers) != 0 {
				return errors.New("service lifecycle: relay forwarding sources/inflight are nonempty or unreadable; owner must quiesce without replay")
			}
		}
	}
	if beforeVerify != nil {
		beforeVerify(name)
	}
	for _, queue := range []string{"req", "outbox", "inflight"} {
		if !found[queue] {
			var st unix.Stat_t
			if err := unix.Fstatat(int(lane.file.Fd()), queue, &st, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
				return errors.New("service lifecycle: absent relay queue appeared or is unknown")
			}
		}
	}
	for _, dir := range dirs {
		if err := dir.verify(true); err != nil {
			return err
		}
	}
	if err := lane.verify(true); err != nil {
		return err
	}
	return root.verify(true)
}
