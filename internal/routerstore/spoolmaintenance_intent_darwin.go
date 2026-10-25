//go:build darwin

package routerstore

import (
	"errors"
	"io"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
	"golang.org/x/sys/unix"
)

type MaintenanceStartupIntent struct {
	Marker     servicedrain.MaintenanceMarker
	file       *os.File
	before     os.FileInfo
	leaf       string
	owner, gid uint32
}

// Reuse the qualified libSystem ACL probe; no direct syscall or permission
// modification. The no-cgo implementation remains explicitly fail-closed.
func MaintenanceMetadataNoACL(fd int) bool { return relayObservationNoACL(fd) }

func (i *MaintenanceStartupIntent) Close() error { return i.file.Close() }

// owner/gid are independent policy inputs, not values learned from JSON. The
// native relay always supplies root and its already-validated service group.
// This read-only retained-FD primitive is also used with owned fixture policy.
func OpenMaintenanceStartupIntent(root *os.Root, owner, gid uint32) (*MaintenanceStartupIntent, error) {
	leaf := servicedrain.MaintenanceMarkerName
	f, err := root.OpenFile(leaf, os.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		leaf = servicedrain.MaintenancePrepareName
		f, err = root.OpenFile(leaf, os.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW, 0)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
	}
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*MaintenanceStartupIntent, error) { _ = f.Close(); return nil, err }
	var st unix.Stat_t
	if unix.Fstat(int(f.Fd()), &st) != nil || st.Uid != owner || st.Gid != gid || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&07777 != 0640 || st.Nlink != 1 || st.Size <= 0 || st.Size > 8192 || !MaintenanceMetadataNoACL(int(f.Fd())) {
		return fail(errors.New("relay maintenance: occupied startup intent invalid/incomplete; stay closed"))
	}
	before, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	raw, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || int64(len(raw)) != st.Size {
		return fail(errors.New("relay maintenance: startup intent read changed"))
	}
	var marker servicedrain.MaintenanceMarker
	if leaf == servicedrain.MaintenancePrepareName {
		p, e := servicedrain.DecodeMaintenance[servicedrain.MaintenancePreparation](raw)
		marker, err = p.Marker, e
		if len(p.Intent) == 0 {
			return fail(errors.New("relay maintenance: prepare body absent; stay closed"))
		}
	} else {
		marker, err = servicedrain.DecodeMaintenance[servicedrain.MaintenanceMarker](raw)
	}
	if err != nil || marker.Validate() != nil {
		return fail(errors.New("relay maintenance: startup intent undecodable; stay closed"))
	}
	intent := &MaintenanceStartupIntent{Marker: marker, file: f, before: before, leaf: leaf, owner: owner, gid: gid}
	if err := intent.Verify(root); err != nil {
		return fail(err)
	}
	return intent, nil
}

func (i *MaintenanceStartupIntent) Verify(root *os.Root) error {
	current, err := i.file.Stat()
	if err != nil {
		return err
	}
	named, err := root.Lstat(i.leaf)
	if err != nil {
		return err
	}
	var st unix.Stat_t
	if unix.Fstat(int(i.file.Fd()), &st) != nil || st.Uid != i.owner || st.Gid != i.gid || st.Mode&07777 != 0640 || st.Nlink != 1 || !os.SameFile(i.before, current) || !os.SameFile(current, named) || i.before.Mode() != current.Mode() || i.before.Size() != current.Size() || !i.before.ModTime().Equal(current.ModTime()) {
		return errors.New("relay maintenance: retained startup intent changed")
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	var physical unix.Stat_t
	if unix.Fstat(int(dir.Fd()), &physical) != nil || uint64(physical.Dev) != i.Marker.RootDevice || physical.Ino != i.Marker.RootInode {
		return errors.New("relay maintenance: startup intent belongs to another root")
	}
	return nil
}
