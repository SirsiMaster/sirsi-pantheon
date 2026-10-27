//go:build darwin

package setup

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
	"golang.org/x/sys/unix"
)

func maintenancePrepare(s MaintenanceStatus) error {
	root, _, err := maintenanceSpool(s.Plan.RootDevice, s.Plan.RootInode)
	if err != nil {
		return err
	}
	defer root.Close()
	_, gid, err := maintenanceRelayUser()
	if err != nil {
		return err
	}
	return maintenancePrepareAt(root, s, 0, gid, nil)
}

// Native callers fix owner=0 and the independently resolved named relay-group
// GID. The per-call beforeCommit seam interrupts private staging before any durable
// transaction authority exists. Public prepare publication is atomic and precedes
// the private journal; committed preparation is always a complete startup hold.
func maintenancePrepareAt(root *os.Root, s MaintenanceStatus, owner, gid int, afterName func() error) error {
	prepared := s
	prepared.Revision++
	prepared.Phase = "prepared"
	prepared.NextAction = "Resume or abort this exact durable preparation"
	intent, err := jsonMaintenance(prepared)
	if err != nil {
		return err
	}
	raw, err := jsonMaintenance(servicedrain.MaintenancePreparation{Marker: maintenanceMarkerValue(s), Intent: intent})
	if err != nil || len(raw) > 8192 {
		return errors.New("legacy maintenance: preparation exceeds bound")
	}
	if _, err := root.Lstat(servicedrain.MaintenancePrepareName); err == nil {
		old, err := maintenanceReadPreparationAt(root, uint32(owner), uint32(gid))
		if err != nil {
			return err
		}
		if old.ID != s.ID || old.Plan != s.Plan {
			return errors.New("legacy maintenance: occupied preparation requires exact recovery")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := maintenancePublishIntentAt(root, servicedrain.MaintenancePrepareName, s.ID, raw, owner, gid, afterName); err != nil {
		return err
	}
	old, err := maintenanceReadPreparationAt(root, uint32(owner), uint32(gid))
	if err != nil || old != prepared {
		return errors.Join(errors.New("legacy maintenance: preparation readback unverified; hold retained"), err)
	}
	return nil
}

// Only the two closed startup-control leaves may be published here. Staging is
// outside the queue namespace: an interrupted pre-commit write is private debt,
// never a torn authoritative prepare or an unknown lane in the spool.
func maintenancePublishIntentAt(root *os.Root, leaf, id string, raw []byte, owner, gid int, beforeCommit func() error) error {
	decodedID, idErr := hex.DecodeString(id)
	if (leaf != servicedrain.MaintenancePrepareName && leaf != servicedrain.MaintenanceMarkerName) || idErr != nil || len(decodedID) != 16 || hex.EncodeToString(decodedID) != id || len(raw) == 0 || len(raw) > 8192 {
		return errors.New("legacy maintenance: invalid startup-control publication")
	}
	parentPath := filepath.Dir(root.Name())
	parentFD, err := maintenanceOpenDirectory(parentPath)
	if err != nil {
		return err
	}
	parent := os.NewFile(uintptr(parentFD), parentPath)
	defer parent.Close()
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	var ps, rs, named unix.Stat_t
	if unix.Fstat(parentFD, &ps) != nil || unix.Fstat(int(dir.Fd()), &rs) != nil || ps.Dev != rs.Dev || (ps.Uid != uint32(owner) && ps.Uid != rs.Uid) || ps.Mode&0022 != 0 ||
		unix.Fstatat(parentFD, filepath.Base(root.Name()), &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != rs.Dev || named.Ino != rs.Ino {
		return errors.New("legacy maintenance: protected same-filesystem staging parent/root required")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	stage := ".sirsi-maintenance-" + id + "-" + hex.EncodeToString(nonce[:]) + ".stage"
	fd, err := unix.Openat(parentFD, stage, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), stage)
	defer f.Close()
	// No failure path unlinks a possibly replaced name. The precise stage path
	// is retained in the returned debt, including failed writes or publication.
	returnErr := func() error {
		if err := f.Chown(owner, gid); err != nil {
			return err
		}
		if n, err := f.Write(raw); err != nil || n != len(raw) {
			return errors.Join(io.ErrShortWrite, err)
		}
		if err := f.Chmod(0640); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}
		if !routerstore.MaintenanceMetadataNoACL(fd) {
			return errors.New("unsupported startup-control ACL")
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		actual, err := io.ReadAll(io.LimitReader(f, 8193))
		var held unix.Stat_t
		if err != nil || !bytes.Equal(raw, actual) || unix.Fstat(fd, &held) != nil ||
			held.Mode&unix.S_IFMT != unix.S_IFREG || held.Mode&07777 != 0640 || held.Uid != uint32(owner) || held.Gid != uint32(gid) || held.Nlink != 1 ||
			unix.Fstatat(parentFD, stage, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != held.Dev || named.Ino != held.Ino {
			return errors.New("startup-control staging readback/identity unverified")
		}
		if err := parent.Sync(); err != nil {
			return err
		}
		if beforeCommit != nil {
			if err := beforeCommit(); err != nil {
				return err
			}
		}
		// Cooperative point-in-time continuity; this is not an atomicity claim
		// against a hostile root/service writer.
		checkFD, err := maintenanceOpenDirectory(parentPath)
		if err != nil {
			return err
		}
		var check unix.Stat_t
		probeErr := unix.Fstat(checkFD, &check)
		_ = unix.Close(checkFD)
		if probeErr != nil || check.Dev != ps.Dev || check.Ino != ps.Ino ||
			unix.Fstatat(parentFD, filepath.Base(root.Name()), &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != rs.Dev || named.Ino != rs.Ino ||
			unix.Fstatat(parentFD, stage, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != held.Dev || named.Ino != held.Ino {
			return errors.New("startup-control parent/root/stage continuity changed")
		}
		if err := unix.RenameatxNp(parentFD, stage, int(dir.Fd()), leaf, unix.RENAME_EXCL); err != nil {
			return err
		}
		return errors.Join(dir.Sync(), parent.Sync())
	}()
	if returnErr != nil {
		return fmt.Errorf("legacy maintenance: startup-control publication unverified; retained stage %s: %w", filepath.Join(parentPath, stage), returnErr)
	}
	return nil
}

func maintenanceReadPreparation() (MaintenanceStatus, error) {
	root, _, err := maintenanceSpool(0, 0)
	if err != nil {
		return MaintenanceStatus{}, err
	}
	defer root.Close()
	_, gid, err := maintenanceRelayUser()
	if err != nil {
		return MaintenanceStatus{}, err
	}
	return maintenanceReadPreparationAt(root, 0, uint32(gid))
}

func maintenanceReadPreparationAt(root *os.Root, owner, gid uint32) (MaintenanceStatus, error) {
	f, status, err := maintenanceOpenPreparationAt(root, owner, gid)
	if f != nil {
		_ = f.Close()
	}
	return status, err
}

func maintenanceOpenPreparationAt(root *os.Root, owner, gid uint32) (*os.File, MaintenanceStatus, error) {
	var status MaintenanceStatus
	f, err := root.OpenFile(servicedrain.MaintenancePrepareName, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, status, err
	}
	fail := func(err error) (*os.File, MaintenanceStatus, error) { _ = f.Close(); return nil, status, err }
	var before, after unix.Stat_t
	if unix.Fstat(int(f.Fd()), &before) != nil || before.Uid != owner || before.Gid != gid || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Mode&07777 != 0640 || before.Nlink != 1 || before.Size <= 0 || before.Size > 8192 || !routerstore.MaintenanceMetadataNoACL(int(f.Fd())) {
		return fail(errors.New("legacy maintenance: preparation incomplete/unknown; startup hold retained"))
	}
	beforeInfo, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	raw, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil {
		return fail(err)
	}
	prepare, err := servicedrain.DecodeMaintenance[servicedrain.MaintenancePreparation](raw)
	if err != nil {
		return fail(err)
	}
	status, err = servicedrain.DecodeMaintenance[MaintenanceStatus](append(bytes.TrimSuffix(prepare.Intent, []byte{'\n'}), '\n'))
	if err != nil || status.Schema != 1 || status.Revision == 0 || status.Plan.validate() != nil || status.Phase != "prepared" || prepare.Marker.Validate() != nil || prepare.Marker != maintenanceMarkerValue(status) {
		return fail(errors.New("legacy maintenance: preparation binding invalid"))
	}
	dir, err := root.Open(".")
	if err != nil {
		return fail(err)
	}
	var st unix.Stat_t
	err = unix.Fstat(int(dir.Fd()), &st)
	_ = dir.Close()
	if err != nil || status.Plan.RootDevice != uint64(st.Dev) || status.Plan.RootInode != st.Ino {
		return fail(errors.New("legacy maintenance: preparation physical root changed"))
	}
	named, err := root.Lstat(servicedrain.MaintenancePrepareName)
	if err != nil || unix.Fstat(int(f.Fd()), &after) != nil || !os.SameFile(beforeInfo, named) || before.Dev != after.Dev || before.Ino != after.Ino || before.Size != after.Size || before.Mtim != after.Mtim || before.Mode != after.Mode || before.Uid != after.Uid || before.Gid != after.Gid || after.Nlink != 1 {
		return fail(errors.New("legacy maintenance: preparation continuity changed"))
	}
	return f, status, nil
}

func maintenanceClearPreparation(s MaintenanceStatus) error {
	root, _, err := maintenanceSpool(s.Plan.RootDevice, s.Plan.RootInode)
	if err != nil {
		return err
	}
	defer root.Close()
	_, gid, err := maintenanceRelayUser()
	if err != nil {
		return err
	}
	return maintenanceClearPreparationAt(root, s, 0, uint32(gid))
}

func maintenanceClearPreparationAt(root *os.Root, s MaintenanceStatus, owner, gid uint32) error {
	if s.Phase != "completed" && s.Phase != "aborted" {
		return errors.New("legacy maintenance: pending intent cannot be cleared")
	}
	f, prepared, err := maintenanceOpenPreparationAt(root, owner, gid)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	if prepared.ID != s.ID || prepared.Plan != s.Plan {
		return errors.New("legacy maintenance: preparation cleanup transaction mismatch")
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	var held, named unix.Stat_t
	if unix.Fstat(int(f.Fd()), &held) != nil || unix.Fstatat(int(dir.Fd()), servicedrain.MaintenancePrepareName, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || held.Dev != named.Dev || held.Ino != named.Ino || held.Uid != owner || held.Nlink != 1 {
		return errors.New("legacy maintenance: cleanup identity changed; unknown replacement retained")
	}
	if err := unix.Unlinkat(int(dir.Fd()), servicedrain.MaintenancePrepareName, 0); err != nil {
		return err
	}
	return dir.Sync()
}
