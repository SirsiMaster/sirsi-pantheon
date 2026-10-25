//go:build darwin

package setup

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
	"golang.org/x/sys/unix"
)

func maintenanceSpool(device, inode uint64) (*os.Root, unix.Stat_t, error) {
	var st unix.Stat_t
	fd, err := maintenanceOpenDirectory(rootRelayPhysicalSpool)
	if err != nil {
		return nil, st, err
	}
	defer unix.Close(fd)
	uid, gid, err := maintenanceRelayUser()
	if err != nil {
		return nil, st, err
	}
	if unix.Fstat(fd, &st) != nil || (st.Uid != uint32(uid) && st.Uid != 0) || st.Mode&0002 != 0 || (st.Mode&0020 != 0 && st.Gid != uint32(gid)) || (inode != 0 && (uint64(st.Dev) != device || uint64(st.Ino) != inode)) {
		return nil, st, errors.New("legacy maintenance: physical spool identity/owner/group changed")
	}
	root, err := os.OpenRoot(rootRelayPhysicalSpool)
	if err != nil {
		return nil, st, err
	}
	held, err := root.Open(".")
	if err != nil {
		_ = root.Close()
		return nil, st, err
	}
	var check unix.Stat_t
	err = unix.Fstat(int(held.Fd()), &check)
	_ = held.Close()
	if err != nil || check.Dev != st.Dev || check.Ino != st.Ino {
		_ = root.Close()
		return nil, st, errors.New("legacy maintenance: physical spool substituted during open")
	}
	return root, st, nil
}

func maintenanceMarkerValue(s MaintenanceStatus) servicedrain.MaintenanceMarker {
	return servicedrain.MaintenanceMarker{Schema: 1, ID: s.ID, PlanSHA256: s.Plan.SHA256, RootDevice: s.Plan.RootDevice, RootInode: s.Plan.RootInode}
}

func maintenanceMarker(s MaintenanceStatus, remove bool) error {
	root, _, err := maintenanceSpool(s.Plan.RootDevice, s.Plan.RootInode)
	if err != nil {
		return err
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	want, err := jsonMaintenance(maintenanceMarkerValue(s))
	if err != nil {
		return err
	}
	_, gid, err := maintenanceRelayUser()
	if err != nil {
		return err
	}
	f, err := root.OpenFile(servicedrain.MaintenanceMarkerName, os.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		if remove {
			return nil // Idempotent finish after a previously verified activation.
		}
		if err := maintenancePublishIntentAt(root, servicedrain.MaintenanceMarkerName, s.ID, want, 0, gid, nil); err != nil {
			return err
		}
		f, err = root.OpenFile(servicedrain.MaintenanceMarkerName, os.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW, 0)
	}
	if err != nil {
		return err
	}
	defer f.Close()
	var before unix.Stat_t
	if unix.Fstat(int(f.Fd()), &before) != nil || before.Uid != 0 || before.Gid != uint32(gid) || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Mode&07777 != 0640 || before.Nlink != 1 || before.Size != int64(len(want)) {
		return errors.New("legacy maintenance: startup marker metadata changed; hold retained")
	}
	actual, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || !bytes.Equal(actual, want) {
		return errors.New("legacy maintenance: startup marker belongs to another/unknown transaction")
	}
	var after, named unix.Stat_t
	if unix.Fstat(int(f.Fd()), &after) != nil || unix.Fstatat(int(dir.Fd()), servicedrain.MaintenanceMarkerName, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || before.Dev != after.Dev || before.Ino != after.Ino || after.Dev != named.Dev || after.Ino != named.Ino || after.Size != named.Size || after.Nlink != 1 {
		return errors.New("legacy maintenance: startup marker continuity changed")
	}
	if remove {
		if s.Phase != "activated" && s.Phase != "completed" && s.Phase != "aborted" {
			return errors.New("legacy maintenance: pending transaction cannot release startup hold")
		}
		if err := unix.Unlinkat(int(dir.Fd()), servicedrain.MaintenanceMarkerName, 0); err != nil {
			return err
		}
		return dir.Sync()
	}
	return nil
}

type maintenanceEnvelope struct {
	Schema  int    `json:"schema"`
	ID      string `json:"transaction"`
	Lane    []byte `json:"lane_bytes"`
	Queue   string `json:"queue"`
	Leaf    []byte `json:"leaf_bytes"`
	Device  uint64 `json:"device"`
	Inode   uint64 `json:"inode"`
	Size    int64  `json:"size"`
	Outcome string `json:"outcome"`
}

func maintenanceChild(parent int, name string) (*os.File, unix.Stat_t, error) {
	var st unix.Stat_t
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
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

func maintenancePreserve(ctx context.Context, s MaintenanceStatus) (int, error) {
	root, _, err := maintenanceSpool(s.Plan.RootDevice, s.Plan.RootInode)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return 0, err
	}
	defer dir.Close()
	uid, gid, err := maintenanceRelayUser()
	if err != nil {
		return 0, err
	}
	count := 0
	budget := new(servicedrain.CustodyBudget)
	for lanes := 0; ; lanes++ {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		entries, err := dir.ReadDir(1)
		if err == io.EOF {
			return count, nil
		}
		if err != nil || lanes >= servicedrain.MaxCustodyLanes {
			return count, errors.New("legacy maintenance: bounded lane inventory unavailable")
		}
		entry := entries[0]
		if entry.Name() == routerstore.RelayObservationMetadataName {
			if err := routerstore.ReadMaintenanceRelayMetadata(ctx, int(dir.Fd()), rootRelayPhysicalSpool); err != nil {
				return count, err
			}
			continue
		}
		if entry.Name() == servicedrain.MaintenanceMarkerName || entry.Name() == servicedrain.MaintenancePrepareName {
			if err := maintenanceControlMetadata(int(dir.Fd()), entry.Name(), uid); err != nil {
				return count, err
			}
			continue
		}
		// The existing health-summary policy is owned by the separately
		// qualified shared namespace correction. Until composition, retain an
		// unknown non-lane instead of inventing a second metadata allowlist.
		if !entry.IsDir() {
			return count, errors.New("legacy maintenance: unknown lane retained; no activation")
		}
		lane, _, err := maintenanceChild(int(dir.Fd()), entry.Name())
		if err != nil {
			return count, err
		}
		n, preserveErr := maintenancePreserveLane(ctx, lane, []byte(entry.Name()), s.ID, uid, gid, budget)
		if preserveErr == nil {
			preserveErr = maintenanceVerifyDirectory(int(dir.Fd()), entry.Name(), lane)
		}
		closeErr := lane.Close()
		count += n
		if preserveErr != nil || closeErr != nil {
			return count, errors.Join(errors.New("legacy maintenance: custody incomplete; no replay or startup release"), preserveErr, closeErr)
		}
	}
}

func maintenanceControlMetadata(parent int, name string, serviceUID int) error {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var held, named unix.Stat_t
	if unix.Fstat(fd, &held) != nil || held.Mode&unix.S_IFMT != unix.S_IFREG || held.Nlink != 1 || (held.Uid != 0 && held.Uid != uint32(serviceUID)) || held.Mode&0022 != 0 || held.Size < 0 || held.Size > 131072 || unix.Fstatat(parent, name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || held.Dev != named.Dev || held.Ino != named.Ino {
		return errors.New("legacy maintenance: known control leaf policy/identity changed; no payload read")
	}
	return nil
}

func maintenancePreserveLane(ctx context.Context, lane *os.File, laneName []byte, id string, serviceUID, gid int, budget *servicedrain.CustodyBudget) (int, error) {
	failed, err := maintenancePublishDirectory(lane, "failed", serviceUID, gid, 0700)
	if err != nil {
		return 0, err
	}
	defer failed.Close()
	name := ".maintenance-" + id
	custody, err := maintenancePublishDirectory(failed, name, os.Geteuid(), gid, 0750)
	if err != nil {
		return 0, err
	}
	defer custody.Close()
	var owned unix.Stat_t
	if unix.Fstat(int(custody.Fd()), &owned) != nil || owned.Uid != uint32(os.Geteuid()) || owned.Gid != uint32(gid) || owned.Mode&07777 != 0750 {
		return 0, errors.New("legacy maintenance: transaction custody directory is not exact root/service-group policy")
	}
	// Preserve req as well: no implicit replay is needed to activate a healthy
	// transport. Res and existing failed/audit leaves are not moved or removed.
	for _, queue := range []string{"req", "inflight", "outbox"} {
		source, _, err := maintenanceChild(int(lane.Fd()), queue)
		if errors.Is(err, unix.ENOENT) {
			continue
		}
		if err != nil {
			return 0, err
		}
		err = maintenancePreserveQueue(ctx, source, custody, laneName, queue, id)
		if err == nil {
			err = maintenanceVerifyDirectory(int(lane.Fd()), queue, source)
		}
		closeErr := source.Close()
		if err != nil || closeErr != nil {
			return 0, errors.Join(err, closeErr)
		}
	}
	entries, err := custody.ReadDir(servicedrain.MaxCustodyEntries + 1)
	if err != nil && err != io.EOF {
		return 0, err
	}
	if len(entries) > servicedrain.MaxCustodyEntries {
		return 0, errors.New("legacy maintenance: custody entry bound exceeded")
	}
	count := 0
	for _, e := range entries {
		if err := budget.Observe(e.Name()); err != nil {
			return 0, err
		}
		if len(e.Name()) == len("envelope-")+64 && e.Name()[:len("envelope-")] == "envelope-" {
			var st unix.Stat_t
			if unix.Fstatat(int(custody.Fd()), e.Name(), &st, unix.AT_SYMLINK_NOFOLLOW) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 {
				return 0, errors.New("legacy maintenance: custody envelope metadata changed")
			}
			count++
		}
	}
	if err := errors.Join(maintenanceVerifyDirectory(int(lane.Fd()), "failed", failed), maintenanceVerifyDirectory(int(failed.Fd()), name, custody)); err != nil {
		return 0, err
	}
	return count, nil
}

func maintenanceVerifyDirectory(parent int, name string, f *os.File) error {
	var held, named unix.Stat_t
	if unix.Fstat(int(f.Fd()), &held) != nil || unix.Fstatat(parent, name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || held.Dev != named.Dev || held.Ino != named.Ino || named.Mode&unix.S_IFMT != unix.S_IFDIR || held.Nlink == 0 {
		return errors.New("legacy maintenance: retained directory name changed; custody incomplete")
	}
	return nil
}

func maintenancePreserveQueue(ctx context.Context, source, custody *os.File, lane []byte, queue, id string) error {
	// Snapshot bounded names before moving any: mutating while consuming a
	// directory cursor can skip an envelope. This is not an old-work drain;
	// confirmed retirement and the physical publication barrier precede it.
	names, readErr := source.Readdirnames(servicedrain.MaxCustodyEnvelopes + 1)
	if (readErr != nil && readErr != io.EOF) || len(names) > servicedrain.MaxCustodyEnvelopes {
		return errors.New("legacy maintenance: queue inventory incomplete")
	}
	for _, leaf := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		var before unix.Stat_t
		if err := unix.Fstatat(int(source.Fd()), leaf, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Size < 0 || before.Size > 8<<20 {
			return errors.New("legacy maintenance: unknown queue object retained in place; no activation")
		}
		audit, err := jsonMaintenance(maintenanceEnvelope{1, id, lane, queue, []byte(leaf), uint64(before.Dev), uint64(before.Ino), before.Size, "outcome_unknown_never_replay"})
		if err != nil || len(audit) > 8192 {
			return errors.New("legacy maintenance: bounded custody provenance required")
		}
		sum := sha256.Sum256(audit)
		key := hex.EncodeToString(sum[:])
		fd, err := unix.Openat(int(custody.Fd()), "audit-"+key, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
		if err == nil {
			f := os.NewFile(uintptr(fd), "maintenance-custody-audit")
			n, writeErr := f.Write(audit)
			if writeErr == nil && n != len(audit) {
				writeErr = io.ErrShortWrite
			}
			if writeErr == nil {
				writeErr = f.Sync()
			}
			err = errors.Join(writeErr, f.Close())
		} else if errors.Is(err, unix.EEXIST) {
			f, openErr := unix.Openat(int(custody.Fd()), "audit-"+key, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
			if openErr != nil {
				return openErr
			}
			file := os.NewFile(uintptr(f), "maintenance-custody-audit")
			var metadata unix.Stat_t
			if unix.Fstat(f, &metadata) != nil || metadata.Uid != uint32(os.Geteuid()) || metadata.Mode&unix.S_IFMT != unix.S_IFREG || metadata.Mode&07777 != 0600 || metadata.Nlink != 1 || metadata.Size != int64(len(audit)) {
				_ = file.Close()
				return errors.New("legacy maintenance: custody audit policy changed")
			}
			actual, readErr := io.ReadAll(io.LimitReader(file, 8193))
			err = errors.Join(readErr, file.Close())
			if !bytes.Equal(actual, audit) {
				return errors.New("legacy maintenance: custody intent changed")
			}
		}
		if err != nil {
			return err
		}
		if err := custody.Sync(); err != nil {
			return err
		}
		var named unix.Stat_t
		if unix.Fstatat(int(source.Fd()), leaf, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != before.Dev || named.Ino != before.Ino || named.Size != before.Size || named.Nlink != 1 {
			return errors.New("legacy maintenance: envelope changed before custody transfer")
		}
		if err := unix.RenameatxNp(int(source.Fd()), leaf, int(custody.Fd()), "envelope-"+key, unix.RENAME_EXCL); err != nil {
			return err
		}
		if unix.Fstatat(int(custody.Fd()), "envelope-"+key, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != before.Dev || named.Ino != before.Ino || named.Size != before.Size || named.Nlink != 1 {
			return errors.New("legacy maintenance: moved envelope readback unverified; custody retained")
		}
		if err := errors.Join(source.Sync(), custody.Sync()); err != nil {
			return err
		}
	}
	check, _, err := maintenanceChild(int(source.Fd()), ".")
	if err != nil {
		return err
	}
	defer check.Close()
	remaining, err := check.Readdirnames(1)
	if err != io.EOF || len(remaining) != 0 {
		return errors.New("legacy maintenance: late queue entries retained; re-inspect custody before activation")
	}
	return nil
}

// Fully prepare only a newly created directory before create-only publication.
// A crash leaves an unreferenced private stage, not a partially initialized
// deterministic custody name that recovery would have to chmod or adopt.
func maintenancePublishDirectory(parent *os.File, name string, uid, gid int, mode os.FileMode) (*os.File, error) {
	f, st, err := maintenanceChild(int(parent.Fd()), name)
	if errors.Is(err, unix.ENOENT) {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return nil, err
		}
		stage := ".maintenance-directory-" + hex.EncodeToString(nonce[:])
		if err := unix.Mkdirat(int(parent.Fd()), stage, 0700); err != nil {
			return nil, err
		}
		f, _, err = maintenanceChild(int(parent.Fd()), stage)
		if err != nil {
			return nil, err
		}
		if err := f.Chown(uid, gid); err != nil {
			_ = f.Close()
			return nil, err
		}
		if err := f.Chmod(mode); err != nil {
			_ = f.Close()
			return nil, err
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return nil, err
		}
		if err := unix.RenameatxNp(int(parent.Fd()), stage, int(parent.Fd()), name, unix.RENAME_EXCL); err != nil {
			_ = f.Close()
			return nil, err
		}
		if err := parent.Sync(); err != nil {
			_ = f.Close()
			return nil, err
		}
		err = unix.Fstat(int(f.Fd()), &st)
	}
	if err != nil {
		return nil, err
	}
	if st.Uid != uint32(uid) || st.Gid != uint32(gid) || st.Mode&07777 != uint16(mode.Perm()) {
		_ = f.Close()
		return nil, errors.New("legacy maintenance: existing custody directory policy differs; no chmod/adoption")
	}
	return f, nil
}
