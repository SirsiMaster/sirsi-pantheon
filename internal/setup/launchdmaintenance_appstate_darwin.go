//go:build darwin

package setup

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"golang.org/x/sys/unix"
)

const maintenanceAppStateLeaf = ".ai.sirsi.router.relay.app-admission.json"
const maintenanceAppStateLimit = 32 * 1024

// Failed writes retain their own staging leaf. Bound that explicit debt rather
// than deleting an unknown replacement or accumulating an unbounded history.
func maintenanceAppStageBudget(parent int) error {
	fd, err := unix.Openat(parent, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	dir := os.NewFile(uintptr(fd), "maintenance-app-parent")
	defer dir.Close()
	entries, stages := 0, 0
	for {
		batch, err := dir.ReadDir(64)
		entries += len(batch)
		if entries > 4096 {
			return errors.New("maintenance app: parent namespace exceeds inspection bound")
		}
		for _, entry := range batch {
			if strings.HasPrefix(entry.Name(), maintenanceAppStateLeaf+".") && strings.HasSuffix(entry.Name(), ".stage") {
				stages++
			}
		}
		if stages >= 32 {
			return errors.New("maintenance app: retained staging debt limit reached; no publication attempted")
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (j maintenanceJournal) appState() (MaintenanceAppAdmission, error) {
	var result MaintenanceAppAdmission
	parent, err := j.parent()
	if err != nil {
		return result, err
	}
	defer unix.Close(parent)
	fd, err := unix.Openat(parent, maintenanceAppStateLeaf, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	f := os.NewFile(uintptr(fd), maintenanceAppStateLeaf)
	defer f.Close()
	var before, after, named unix.Stat_t
	if unix.Fstat(fd, &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Mode&07777 != 0600 || before.Uid != j.owner || before.Nlink != 1 || before.Size <= 0 || before.Size > maintenanceAppStateLimit || !routerstore.MaintenanceMetadataNoACL(fd) {
		return result, errors.New("maintenance app: private admission state invalid; no reset")
	}
	raw, err := io.ReadAll(io.LimitReader(f, maintenanceAppStateLimit+1))
	if err != nil {
		return result, err
	}
	if err := decodeMaintenanceAppState(raw, &result); err != nil {
		return result, err
	}
	if unix.Fstat(fd, &after) != nil || unix.Fstatat(parent, maintenanceAppStateLeaf, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Size != after.Size || before.Mtim != after.Mtim || before.Mode != after.Mode || before.Uid != after.Uid || after.Dev != named.Dev || after.Ino != named.Ino || after.Mode != named.Mode || after.Nlink != 1 || !routerstore.MaintenanceMetadataNoACL(fd) {
		return result, errors.New("maintenance app: admission state continuity changed")
	}
	check, err := j.parent()
	if err != nil {
		return result, err
	}
	var heldParent, namedParent unix.Stat_t
	checkErr := unix.Fstat(check, &namedParent)
	_ = unix.Close(check)
	if checkErr != nil || unix.Fstat(parent, &heldParent) != nil || heldParent.Dev != namedParent.Dev || heldParent.Ino != namedParent.Ino {
		return result, errors.New("maintenance app: admission parent continuity changed")
	}
	return result, nil
}
func (j maintenanceJournal) persistAppState(expected, next MaintenanceAppAdmission) error {
	current, err := j.appState()
	if err != nil || current != expected {
		return errors.Join(errors.New("maintenance app: admission CAS failed"), err)
	}
	if err := validAppStateTransition(expected, next); err != nil {
		return err
	}
	data, err := jsonMaintenance(next)
	if err != nil || len(data) > maintenanceAppStateLimit {
		return errors.New("maintenance app: admission bound exhausted; retained state cannot be evicted")
	}
	parent, err := j.parent()
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	if err := maintenanceAppStageBudget(parent); err != nil {
		return err
	}
	nonce, err := appRandom()
	if err != nil {
		return err
	}
	stage := maintenanceAppStateLeaf + "." + nonce + ".stage"
	fd, err := unix.Openat(parent, stage, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), stage)
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if n, err := f.Write(data); err != nil || n != len(data) {
		return errors.Join(io.ErrShortWrite, err)
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	readback, err := io.ReadAll(io.LimitReader(f, maintenanceAppStateLimit+1))
	if err != nil || !bytes.Equal(data, readback) {
		return errors.New("maintenance app: staged admission readback unverified")
	}
	var held, named unix.Stat_t
	if unix.Fstat(fd, &held) != nil || held.Uid != j.owner || held.Mode&07777 != 0600 || held.Nlink != 1 || !routerstore.MaintenanceMetadataNoACL(fd) ||
		unix.Fstatat(parent, stage, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || held.Dev != named.Dev || held.Ino != named.Ino {
		return errors.New("maintenance app: staged admission identity changed; retained debt")
	}
	// The existing service lock serializes all callers. Reinspect the protected
	// parent and old leaf immediately before replacing; no unknown cleanup.
	check, err := j.parent()
	if err != nil {
		return err
	}
	var a, b unix.Stat_t
	checkErr := unix.Fstat(check, &a)
	_ = unix.Close(check)
	if checkErr != nil || unix.Fstat(parent, &b) != nil || a.Dev != b.Dev || a.Ino != b.Ino {
		return errors.New("maintenance app: admission parent changed")
	}
	latest, err := j.appState()
	if err != nil || latest != expected {
		return errors.New("maintenance app: admission changed before publication")
	}
	if expected.Schema == 0 {
		err = unix.RenameatxNp(parent, stage, parent, maintenanceAppStateLeaf, unix.RENAME_EXCL)
	} else {
		err = unix.Renameat(parent, stage, parent, maintenanceAppStateLeaf)
	}
	if err != nil {
		return err
	}
	if err := unix.Fsync(parent); err != nil {
		return err
	}
	observed, err := j.appState()
	if err != nil || observed != next {
		return errors.Join(errors.New("maintenance app: generation fence durability unverified; no safe retry"), err)
	}
	return nil
}

// A previous publication may have reached rename but failed its final fsync or
// readback. Reading that leaf alone is not a durable fence acknowledgement.
// Reconcile the exact state under the caller's existing service lock, without
// replacing it or replaying any lifecycle operation.
func (j maintenanceJournal) syncAppState(expected MaintenanceAppAdmission) error {
	observed, err := j.appState()
	if err != nil || observed != expected {
		return errors.Join(errors.New("maintenance app: durability binding changed"), err)
	}
	parent, err := j.parent()
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	fd, err := unix.Openat(parent, maintenanceAppStateLeaf, unix.O_RDWR|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var held, named unix.Stat_t
	if unix.Fstat(fd, &held) != nil || held.Mode&unix.S_IFMT != unix.S_IFREG || held.Uid != j.owner || held.Mode&07777 != 0600 || held.Nlink != 1 || !routerstore.MaintenanceMetadataNoACL(fd) {
		return errors.New("maintenance app: untrusted durability descriptor")
	}
	if err := unix.Fsync(fd); err != nil {
		return err
	}
	if err := unix.Fsync(parent); err != nil {
		return err
	}
	if unix.Fstatat(parent, maintenanceAppStateLeaf, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || held.Dev != named.Dev || held.Ino != named.Ino {
		return errors.New("maintenance app: durability leaf changed")
	}
	observed, err = j.appState()
	if err != nil || observed != expected {
		return errors.Join(errors.New("maintenance app: durability readback changed"), err)
	}
	return nil
}
