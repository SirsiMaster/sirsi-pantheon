//go:build darwin

package routerstore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
	"golang.org/x/sys/unix"
)

// No marker means no additional requirement on the ordinary/producer-covered
// route. A root-authored pending transaction, however, is consumed BEFORE any
// recoverInflight/discovery/retry/housekeeping call, including after reboot.
func (rl *Relay) awaitMaintenanceStartup(ctx context.Context) (io.Closer, error) {
	if rl.Spool != "/private/var/sirsipantheon/relay" {
		return io.NopCloser(strings.NewReader("")), nil
	}
	root, err := os.OpenRoot(rl.Spool)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.OpenFile(servicedrain.MaintenanceMarkerName, os.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		return io.NopCloser(strings.NewReader("")), nil
	}
	if err != nil {
		return nil, fmt.Errorf("relay maintenance: startup marker unavailable; producer stays closed: %w", err)
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if unix.Fstat(int(f.Fd()), &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != 0 || st.Gid != uint32(os.Getgid()) || st.Mode&07777 != 0640 || st.Nlink != 1 || st.Size <= 0 || st.Size > 8192 {
		return nil, errors.New("relay maintenance: root-owned exact service-group private startup marker required")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || int64(len(raw)) != st.Size {
		return nil, errors.New("relay maintenance: startup marker read changed")
	}
	marker, err := servicedrain.DecodeMaintenance[servicedrain.MaintenanceMarker](raw)
	if err != nil || marker.Validate() != nil {
		return nil, errors.New("relay maintenance: invalid pending transaction; no startup recovery")
	}
	after, e1 := f.Stat()
	named, e2 := root.Lstat(servicedrain.MaintenanceMarkerName)
	if e1 != nil || e2 != nil || !os.SameFile(before, after) || !os.SameFile(after, named) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, errors.New("relay maintenance: startup marker identity changed")
	}
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	var physical unix.Stat_t
	if unix.Fstat(int(dir.Fd()), &physical) != nil || uint64(physical.Dev) != marker.RootDevice || uint64(physical.Ino) != marker.RootInode {
		return nil, errors.New("relay maintenance: pending transaction belongs to another physical spool")
	}
	count, err := maintenanceCustodyCount(ctx, root, marker.ID)
	if err != nil {
		return nil, err
	}
	started, err := servicedrain.MaintenanceStart(os.Getpid())
	if err != nil {
		return nil, err
	}
	verify := func() error {
		opened, e1 := dir.Stat()
		named, e2 := os.Lstat(rl.Spool)
		current, e3 := f.Stat()
		markerName, e4 := root.Lstat(servicedrain.MaintenanceMarkerName)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || !os.SameFile(opened, named) || !os.SameFile(before, current) || !os.SameFile(current, markerName) || before.Mode() != current.Mode() || before.Size() != current.Size() || !before.ModTime().Equal(current.ModTime()) {
			return errors.New("relay maintenance: activation root/marker continuity changed; stay held")
		}
		fresh, err := maintenanceCustodyCount(ctx, root, marker.ID)
		if err != nil || fresh != count {
			return errors.Join(errors.New("relay maintenance: custody/queue changed; stay held"), err)
		}
		return nil
	}
	gate, err := servicedrain.NewStartupMaintenance(os.Getpid(), started, marker, count, verify)
	if err != nil {
		return nil, err
	}
	server, err := servicedrain.StartMaintenance(gate)
	if err != nil {
		return nil, err
	}
	if err := gate.Wait(ctx); err != nil {
		return nil, errors.Join(err, server.Close())
	}
	return server, nil
}

// Names and metadata only. Payloads retained under failed/.maintenance-ID are
// not replayed and are not swept as ordinary failed/*.json leaves.
func maintenanceCustodyCount(ctx context.Context, root *os.Root, id string) (int, error) {
	dir, err := root.Open(".")
	if err != nil {
		return 0, err
	}
	defer dir.Close()
	count, inspected := 0, 0
	for lanes := 0; ; lanes++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		entries, err := dir.ReadDir(1)
		if err == io.EOF {
			return count, nil
		}
		if err != nil || lanes >= 1024 {
			return 0, errors.New("relay maintenance: bounded lane enumeration unavailable")
		}
		e := entries[0]
		if e.Name() == servicedrain.MaintenanceMarkerName && e.Type().IsRegular() {
			continue
		}
		if !e.IsDir() || !validSpoolAgentName(e.Name()) {
			return 0, errors.New("relay maintenance: unknown lane name")
		}
		if err := maintenanceNoForwardSources(int(dir.Fd()), e.Name()); err != nil {
			return 0, err
		}
		custody, err := maintenanceOpenCustody(int(dir.Fd()), e.Name(), id)
		if err != nil {
			return 0, err
		}
		if custody == nil {
			continue
		}
		for {
			if err := ctx.Err(); err != nil {
				_ = custody.Close()
				return 0, err
			}
			names, readErr := custody.Readdirnames(1)
			inspected += len(names)
			for _, name := range names {
				prefix := "audit-"
				if strings.HasPrefix(name, "envelope-") {
					prefix = "envelope-"
				}
				key := strings.TrimPrefix(name, prefix)
				digest, decodeErr := hex.DecodeString(key)
				var st unix.Stat_t
				if !strings.HasPrefix(name, prefix) || decodeErr != nil || len(digest) != 32 || hex.EncodeToString(digest) != key || unix.Fstatat(int(custody.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 {
					_ = custody.Close()
					return 0, errors.New("relay maintenance: unknown custody entry retained; stay held")
				}
				if strings.HasPrefix(name, "envelope-") {
					count++
				}
			}
			if inspected > 65536 || (readErr != nil && readErr != io.EOF) {
				_ = custody.Close()
				return 0, errors.New("relay maintenance: bounded custody enumeration unavailable")
			}
			if readErr == io.EOF {
				break
			}
		}
		if err := custody.Close(); err != nil {
			return 0, err
		}
	}
}

func maintenanceNoForwardSources(root int, lane string) error {
	parent, _, err := relayOpenDirectory(root, lane)
	if err != nil {
		return err
	}
	defer parent.Close()
	for _, name := range []string{"req", "inflight", "outbox"} {
		queue, _, err := relayOpenDirectory(int(parent.Fd()), name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		entries, readErr := queue.Readdirnames(1)
		closeErr := queue.Close()
		if readErr != io.EOF || len(entries) != 0 || closeErr != nil {
			return errors.New("relay maintenance: late/unknown forwarding sources retained; stay held")
		}
	}
	return nil
}

func maintenanceOpenCustody(root int, lane, id string) (*os.File, error) {
	parent, _, err := relayOpenDirectory(root, lane)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	failed, _, err := relayOpenDirectory(int(parent.Fd()), "failed")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer failed.Close()
	result, _, err := relayOpenDirectory(int(failed.Fd()), ".maintenance-"+id)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return result, err
}
