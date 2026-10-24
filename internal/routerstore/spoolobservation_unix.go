//go:build darwin || linux

package routerstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type relayObservationRoot struct {
	path       string
	parent, fd int
	stat       unix.Stat_t
	parentStat unix.Stat_t
	uid        uint32
	gid        int
	mode       uint32
}

// The shared-owner case is the existing exact root-relay service policy
// (launchdservice_darwin.go), NOT "any uid in the trust group". Same-user
// private roots remain portable. Unknown foreign-owner configurations refuse
// projection rather than treating group access as producer identity.
func expectedRelayObservationUID(path string, root, parent unix.Stat_t, self int, lookup func(string) (*user.User, error)) (uint32, error) {
	if path != "/private/var/sirsipantheon/relay" {
		if int(root.Uid) == self && (parent.Uid == 0 || int(parent.Uid) == self) {
			return root.Uid, nil
		}
		return 0, relayObservationError("producer_policy")
	}
	service, err := lookup("_sirsipantheon")
	if err != nil {
		return 0, relayObservationError("producer_policy")
	}
	uid, err := strconv.ParseUint(service.Uid, 10, 32)
	if err != nil || uid == 0 || uint32(uid) != root.Uid || parent.Uid != root.Uid {
		return 0, relayObservationError("producer_policy")
	}
	return uint32(uid), nil
}

// Read-only: never call bootstrap CheckSpoolDir here (it can chmod/create).
func openRelayObservationRoot(path, group string) (*relayObservationRoot, error) {
	canon, err := filepath.EvalSymlinks(path)
	if err != nil || !filepath.IsAbs(canon) {
		return nil, relayObservationError("root_unavailable")
	}
	parent, err := openAbsoluteDirectoryNoFollow(filepath.Dir(canon))
	if err != nil {
		return nil, relayObservationError("root_unavailable")
	}
	r := &relayObservationRoot{path: canon, parent: parent, fd: -1, gid: -1}
	ok := false
	defer func() {
		if !ok {
			r.close()
		}
	}()
	var pst unix.Stat_t
	if unix.Fstat(parent, &pst) != nil || pst.Mode&0o022 != 0 {
		return nil, relayObservationError("parent_policy")
	}
	r.parentStat = pst
	r.fd, err = unix.Openat(parent, filepath.Base(canon), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil || unix.Fstat(r.fd, &r.stat) != nil {
		return nil, relayObservationError("root_unavailable")
	}
	r.uid, err = expectedRelayObservationUID(canon, r.stat, pst, os.Geteuid(), user.Lookup)
	if err != nil {
		return nil, err
	}
	if group != "" {
		g, err := user.LookupGroup(group)
		if err != nil {
			return nil, relayObservationError("group_policy")
		}
		r.gid, err = strconv.Atoi(g.Gid)
		if err != nil || int(r.stat.Gid) != r.gid || r.stat.Mode&0o777 != 0o770 || r.stat.Mode&0o5000 != 0 {
			return nil, relayObservationError("group_policy")
		}
		r.mode = 0o640
	} else {
		if int(r.uid) != os.Geteuid() || r.stat.Mode&0o7777 != 0o700 {
			return nil, relayObservationError("private_policy")
		}
		r.gid, r.mode = int(r.stat.Gid), 0o600
	}
	if err := r.continuity(); err != nil {
		return nil, err
	}
	ok = true
	return r, nil
}

func (r *relayObservationRoot) close() {
	if r.fd >= 0 {
		_ = unix.Close(r.fd)
	}
	if r.parent >= 0 {
		_ = unix.Close(r.parent)
	}
}

func sameRelayObject(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid
}

func (r *relayObservationRoot) continuity() error {
	var named, opened unix.Stat_t
	if unix.Fstat(r.parent, &named) != nil || !sameRelayObject(r.parentStat, named) {
		return relayObservationError("changed")
	}
	if unix.Fstat(r.fd, &opened) != nil || unix.Fstatat(r.parent, filepath.Base(r.path), &named, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameRelayObject(r.stat, opened) || !sameRelayObject(opened, named) {
		return relayObservationError("changed")
	}
	// The named parent itself may have moved above the retained descriptor.
	fd, err := openAbsoluteDirectoryNoFollow(r.path)
	if err != nil {
		return relayObservationError("changed")
	}
	defer unix.Close(fd)
	if unix.Fstat(fd, &named) != nil || !sameRelayObject(opened, named) {
		return relayObservationError("changed")
	}
	return nil
}

func (r *relayObservationRoot) validFile(fd int) (unix.Stat_t, error) {
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Uid != r.uid || int(st.Gid) != r.gid || uint32(st.Mode&0o7777) != r.mode || st.Size > relayObservationBytes || !relayObservationNoACL(fd) {
		return st, relayObservationError("artifact_policy")
	}
	return st, nil
}

func (r *relayObservationRoot) read(clock func() time.Time, afterRead func()) (RelayOutboxObservation, error) {
	var s RelayOutboxObservation
	fd, err := unix.Openat(r.fd, relayObservationName, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return s, relayObservationError("publisher_missing")
	}
	if err != nil {
		return s, relayObservationError("artifact_unavailable")
	}
	f := os.NewFile(uintptr(fd), relayObservationName)
	defer f.Close()
	before, err := r.validFile(fd)
	if err != nil {
		return s, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, relayObservationBytes+1))
	if err != nil {
		return s, relayObservationError("artifact_unavailable")
	}
	if afterRead != nil {
		afterRead()
	}
	after, err := r.validFile(fd)
	var named unix.Stat_t
	if err != nil || !sameRelayObject(before, after) || before.Size != after.Size || before.Mtim != after.Mtim ||
		unix.Fstatat(r.fd, relayObservationName, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameRelayObject(after, named) || named.Nlink != 1 || after.Size != named.Size || after.Mtim != named.Mtim {
		return s, relayObservationError("changed")
	}
	if err := r.continuity(); err != nil {
		return s, err
	}
	s, err = decodeRelayObservation(raw)
	if err != nil {
		return s, err
	}
	return s, s.validate(clock(), uint64(r.stat.Dev), uint64(r.stat.Ino), r.uid)
}

func (r *relayObservationRoot) publish(s RelayOutboxObservation) error {
	if int(r.uid) != os.Geteuid() {
		return relayObservationError("producer_policy")
	}
	if err := s.validate(time.Now(), uint64(r.stat.Dev), uint64(r.stat.Ino), r.uid); err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil || len(raw) > relayObservationBytes {
		return relayObservationError("size")
	}
	if err := r.continuity(); err != nil {
		return err
	}
	// Existing summary is reserved producer metadata, never a queue file.
	if old, err := unix.Openat(r.fd, relayObservationName, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0); err == nil {
		_, check := r.validFile(old)
		_ = unix.Close(old)
		if check != nil {
			return check
		}
	} else if !errors.Is(err, unix.ENOENT) {
		return relayObservationError("artifact_policy")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return relayObservationError("publication")
	}
	name := ".relay-health-new-" + hex.EncodeToString(nonce[:])
	fd, err := unix.Openat(r.fd, name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return relayObservationError("publication")
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	// Only the exclusively created new object receives ownership/mode changes.
	// On failure leave bounded cleanup debt, never unlink a substituted name.
	if unix.Fchown(fd, -1, r.gid) != nil || unix.Fchmod(fd, r.mode) != nil {
		return relayObservationError("metadata_permissions")
	}
	st, err := r.validFile(fd)
	if err != nil {
		return err
	}
	if n, err := f.Write(raw); err != nil || n != len(raw) {
		return relayObservationError("publication")
	}
	if f.Sync() != nil {
		return relayObservationError("publication")
	}
	var named unix.Stat_t
	if unix.Fstatat(r.fd, name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameRelayObject(st, named) {
		return relayObservationError("changed")
	}
	if err := r.continuity(); err != nil {
		return err
	}
	if unix.Renameat(r.fd, name, r.fd, relayObservationName) != nil || unix.Fsync(r.fd) != nil {
		return relayObservationError("publication")
	}
	_, err = r.read(time.Now, nil)
	return err
}

type relayScanBudget struct {
	deadline       time.Time
	entries, bytes int
}

func (b *relayScanBudget) take(name string) bool {
	b.entries++
	b.bytes += len(name)
	return b.entries <= 16384 && b.bytes <= 1<<20 && time.Now().Before(b.deadline)
}

func (b *relayScanBudget) current() bool { return time.Now().Before(b.deadline) }

func relayOpenDirectory(parent int, name string) (*os.File, unix.Stat_t, error) {
	var st unix.Stat_t
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, st, err
	}
	f := os.NewFile(uintptr(fd), name)
	if err := unix.Fstat(fd, &st); err != nil {
		_ = f.Close()
		return nil, st, err
	}
	return f, st, nil
}

func relayDirectoryUnchanged(parent int, name string, file *os.File, before unix.Stat_t, stable bool) bool {
	var named, after unix.Stat_t
	return unix.Fstat(int(file.Fd()), &after) == nil && unix.Fstatat(parent, name, &named, unix.AT_SYMLINK_NOFOLLOW) == nil &&
		sameRelayObject(before, after) && sameRelayObject(after, named) && (!stable || before.Mtim == after.Mtim)
}

// Incremental name-only scan, bounded across ALL lanes. No request/body file
// is opened, even for an unreadable/malformed entry. A changing directory is
// unavailable for this interval; eligible siblings remain visible.
func (r *relayObservationRoot) scan(now time.Time) RelayOutboxObservation {
	s := RelayOutboxObservation{Version: 1, RootDevice: uint64(r.stat.Dev), RootInode: uint64(r.stat.Ino), ProducerUID: r.uid, StartedAt: now, Enumerated: true, Lanes: []RelayLaneObservation{}}
	b := relayScanBudget{deadline: time.Now().Add(100 * time.Millisecond)}
	root, before, err := relayOpenDirectory(r.parent, filepath.Base(r.path))
	if err != nil || !sameRelayObject(r.stat, before) {
		if root != nil {
			_ = root.Close()
		}
		s.Enumerated = false
		s.ErrorCode = "unavailable"
	} else {
		defer root.Close()
		for {
			entries, err := root.ReadDir(1)
			if !b.current() {
				s.Enumerated = false
				s.ErrorCode = "limit"
				break
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				s.Enumerated = false
				s.ErrorCode = "unavailable"
				break
			}
			entry := entries[0]
			if !b.take(entry.Name()) {
				s.Enumerated = false
				s.ErrorCode = "limit"
				break
			}
			if !entry.IsDir() {
				if entry.Type()&os.ModeSymlink != 0 {
					s.Enumerated = false
					s.ErrorCode = "invalid_layout"
				}
				continue
			}
			if len(s.Lanes) == 256 {
				s.Enumerated = false
				s.ErrorCode = "limit"
				break
			}
			if !validSpoolAgentName(entry.Name()) {
				s.Enumerated = false
				s.ErrorCode = "invalid_layout"
				continue
			}
			s.Lanes = append(s.Lanes, relayScanLane(int(root.Fd()), entry.Name(), &b))
		}
		if !relayDirectoryUnchanged(r.parent, filepath.Base(r.path), root, before, true) {
			s.Enumerated = false
			s.ErrorCode = "changed"
		}
	}
	s.Complete = s.Enumerated
	total := 0
	for _, lane := range s.Lanes {
		if lane.State == "unavailable" {
			s.Complete = false
		}
		if lane.Queued != nil {
			total += *lane.Queued
		}
	}
	if s.Complete {
		s.Queued = &total
	}
	s.ObservedAt = now.Add(time.Since(b.deadline.Add(-100 * time.Millisecond)))
	sort.Slice(s.Lanes, func(i, j int) bool { return s.Lanes[i].Agent < s.Lanes[j].Agent })
	return s
}

func relayScanLane(root int, name string, b *relayScanBudget) RelayLaneObservation {
	lane := RelayLaneObservation{Agent: name, State: "unavailable", ErrorCode: "unavailable"}
	dir, st, err := relayOpenDirectory(root, name)
	if err != nil {
		return lane
	}
	defer dir.Close()
	out, ost, err := relayOpenDirectory(int(dir.Fd()), "outbox")
	if errors.Is(err, unix.ENOENT) {
		lane.State = "absent"
		lane.ErrorCode = ""
		var named unix.Stat_t
		if !errors.Is(unix.Fstatat(int(dir.Fd()), "outbox", &named, unix.AT_SYMLINK_NOFOLLOW), unix.ENOENT) {
			lane.State = "unavailable"
			lane.ErrorCode = "changed"
		}
	} else if err == nil {
		defer out.Close()
		count := 0
		for {
			entries, err := out.ReadDir(1)
			if !b.current() {
				lane.ErrorCode = "limit"
				break
			}
			if err == io.EOF {
				lane.State = "observed"
				lane.ErrorCode = ""
				lane.Queued = &count
				break
			}
			if err != nil {
				break
			}
			e := entries[0]
			if !b.take(e.Name()) {
				lane.ErrorCode = "limit"
				break
			}
			var file unix.Stat_t
			if unix.Fstatat(int(out.Fd()), e.Name(), &file, unix.AT_SYMLINK_NOFOLLOW) != nil || file.Mode&unix.S_IFMT != unix.S_IFREG {
				lane.ErrorCode = "invalid_layout"
				break
			}
			if strings.HasSuffix(e.Name(), ".json") {
				count++
			}
		}
		if !relayDirectoryUnchanged(int(dir.Fd()), "outbox", out, ost, true) {
			lane.State = "unavailable"
			lane.ErrorCode = "changed"
			lane.Queued = nil
		}
	}
	if !relayDirectoryUnchanged(root, name, dir, st, false) {
		lane.State = "unavailable"
		lane.ErrorCode = "changed"
		lane.Queued = nil
	}
	return lane
}
