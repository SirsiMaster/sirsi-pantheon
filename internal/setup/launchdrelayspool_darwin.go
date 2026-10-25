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

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"golang.org/x/sys/unix"
)

// launchd argv uses /var, but native inspection walks only its known physical
// namespace, never a symlink alias supplied by an environment or caller.
const rootRelayPhysicalSpool = "/private/var/sirsipantheon/relay"

var errRelayMetadataPublicationBusy = errors.New("service lifecycle: relay metadata publication in progress or retained debt")

type relaySpoolDirectory struct {
	file   *os.File
	info   os.FileInfo
	parent int
	name   string
	stat   unix.Stat_t
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
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		_ = f.Close()
		return relaySpoolDirectory{}, err
	}
	return relaySpoolDirectory{file: f, info: info, parent: parent, name: name, stat: st}, nil
}

func (d relaySpoolDirectory) verify(stable bool) error {
	var opened, named unix.Stat_t
	if err := unix.Fstat(int(d.file.Fd()), &opened); err != nil {
		return err
	}
	if err := unix.Fstatat(d.parent, d.name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if named.Mode&unix.S_IFMT != unix.S_IFDIR || !sameRelaySpoolIdentity(d.stat, opened) || !sameRelaySpoolIdentity(opened, named) {
		return errors.New("service lifecycle: relay spool directory name changed")
	}
	after, err := d.file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(d.info, after) || d.info.Mode() != after.Mode() ||
		(stable && (!d.info.ModTime().Equal(after.ModTime()) || d.info.Size() != after.Size() || !sameRelaySpoolStable(d.stat, opened))) {
		return errors.New("service lifecycle: relay spool directory changed during quiescence sample")
	}
	return nil
}

func sameRelaySpoolIdentity(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid
}

func sameRelaySpoolStable(a, b unix.Stat_t) bool {
	return sameRelaySpoolIdentity(a, b) && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim && a.Nlink == b.Nlink
}

type relayLaneNamespace struct {
	lane   unix.Stat_t
	queues map[string]unix.Stat_t
}

type relayRootNamespace struct {
	lanes    map[string]relayLaneNamespace
	metadata *unix.Stat_t
}

func relayHealthStagingName(name string) bool {
	const prefix = ".relay-health-new-"
	if !strings.HasPrefix(name, prefix) || len(name) != len(prefix)+32 {
		return false
	}
	for _, ch := range name[len(prefix):] {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}

// Each complete observation enumerates from a fresh descriptor relative to
// the retained root. Only the exact finished summary is metadata. Staging
// publication/debt is busy, and every other entry must still be a real lane.
func readRelayRootNamespace(ctx context.Context, physical string, root relaySpoolDirectory) (relayRootNamespace, error) {
	result := relayRootNamespace{lanes: map[string]relayLaneNamespace{}}
	if err := root.verify(false); err != nil {
		return result, err
	}
	fd, err := unix.Openat(int(root.file.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return result, err
	}
	enumerator := os.NewFile(uintptr(fd), "relay root enumeration")
	defer enumerator.Close()
	entries, err := relaySpoolEntries(enumerator, 1024)
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		name := entry.Name()
		if name == "." || name == ".." || len(name) > 255 || strings.ContainsAny(name, "/\x00\r\n\t") {
			return result, errors.New("service lifecycle: malformed relay spool lane")
		}
		if relayHealthStagingName(name) {
			return result, errRelayMetadataPublicationBusy
		}
		if name == routerstore.RelayObservationMetadataName {
			var parent unix.Stat_t
			if err := unix.Fstat(root.parent, &parent); err != nil {
				return result, err
			}
			policy, err := routerstore.RelayObservationPolicyForRoot(physical, root.stat, parent)
			if err != nil {
				return result, fmt.Errorf("service lifecycle: relay metadata policy: %w", err)
			}
			st, err := routerstore.ReadRelayObservationMetadataAt(ctx, int(root.file.Fd()), policy)
			if err != nil {
				return result, fmt.Errorf("service lifecycle: relay metadata invalid or changed: %w", err)
			}
			result.metadata = &st
			continue
		}
		lane, err := openRelaySpoolDirectory(int(root.file.Fd()), name)
		if err != nil {
			return result, fmt.Errorf("service lifecycle: relay lane unavailable: %w", err)
		}
		snapshot, err := relayLaneNamespaceAt(ctx, lane)
		_ = lane.file.Close()
		if err != nil {
			return result, err
		}
		result.lanes[name] = snapshot
	}
	return result, root.verify(false)
}

func relayLaneNamespaceAt(ctx context.Context, lane relaySpoolDirectory) (relayLaneNamespace, error) {
	result := relayLaneNamespace{lane: lane.stat, queues: map[string]unix.Stat_t{}}
	entries, err := relaySpoolEntries(lane.file, 16)
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		name := entry.Name()
		switch name {
		case "req", "outbox", "inflight", "res", "failed", "slots":
		default:
			return result, errors.New("service lifecycle: unknown relay lane entry")
		}
		queue, err := openRelaySpoolDirectory(int(lane.file.Fd()), name)
		if err != nil {
			return result, fmt.Errorf("service lifecycle: relay queue directory unavailable: %w", err)
		}
		result.queues[name] = queue.stat
		err = queue.verify(true)
		_ = queue.file.Close()
		if err != nil {
			return result, err
		}
	}
	return result, lane.verify(true)
}

// Metadata may be newly published or atomically replaced between complete
// observations. No lane/queue addition, removal, identity or metadata change
// receives that exception; in-place summary writes and disappearance refuse.
func compareRelayRootNamespace(before, after relayRootNamespace) (bool, error) {
	if len(before.lanes) != len(after.lanes) {
		return false, errors.New("service lifecycle: relay lane namespace changed")
	}
	for name, lane := range before.lanes {
		next, ok := after.lanes[name]
		if !ok || !sameRelaySpoolStable(lane.lane, next.lane) || len(lane.queues) != len(next.queues) {
			return false, errors.New("service lifecycle: relay lane namespace changed")
		}
		for name, queue := range lane.queues {
			other, ok := next.queues[name]
			if !ok || !sameRelaySpoolStable(queue, other) {
				return false, errors.New("service lifecycle: relay queue namespace changed")
			}
		}
	}
	if before.metadata == nil {
		return after.metadata != nil, nil
	}
	if after.metadata == nil {
		return false, errors.New("service lifecycle: relay metadata disappeared")
	}
	if before.metadata.Dev == after.metadata.Dev && before.metadata.Ino == after.metadata.Ino {
		if !sameRelaySpoolStable(*before.metadata, *after.metadata) {
			return false, errors.New("service lifecycle: relay metadata changed in place")
		}
		return false, nil
	}
	return true, nil
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

// Observe marker names and the exact validated summary only: never open work bodies, move/delete markers or
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
	initial, err := readRelayRootNamespace(ctx, physical, root)
	if err != nil {
		return err
	}
	for name := range initial.lanes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := rootRelayLaneQuiescent(ctx, root, name, beforeVerify); err != nil {
			return err
		}
	}
	final, err := readRelayRootNamespace(ctx, physical, root)
	if err != nil {
		return err
	}
	metadataChanged, err := compareRelayRootNamespace(initial, final)
	if err != nil {
		return err
	}
	// Re-run the real empty-queue/absent-queue probes, then close the sample
	// with one more bounded namespace observation. No lifecycle action retries.
	for name := range final.lanes {
		if err := rootRelayLaneQuiescent(ctx, root, name, nil); err != nil {
			return err
		}
	}
	last, err := readRelayRootNamespace(ctx, physical, root)
	if err != nil {
		return err
	}
	replaced, err := compareRelayRootNamespace(final, last)
	if err != nil {
		return err
	}
	for i, dir := range chain {
		if err := dir.verify(i == len(chain)-1 && !metadataChanged && !replaced); err != nil {
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
	// The caller owns the bounded full-root rescans and grants the sole
	// metadata replacement exception. Identity/owner/mode never relax here.
	return root.verify(false)
}
