//go:build darwin

package routerstore

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
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
	intent, err := OpenMaintenanceStartupIntent(root, 0, uint32(os.Getgid()))
	if err != nil {
		return nil, err
	}
	if intent == nil {
		return io.NopCloser(strings.NewReader("")), nil
	}
	defer intent.Close()
	marker := intent.Marker
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	count, err := maintenanceCustodyCount(ctx, root, marker.ID)
	if err != nil {
		return nil, err
	}
	started, err := servicedrain.MaintenanceStart(os.Getpid())
	if err != nil {
		return nil, err
	}
	verify := func(expected servicedrain.MaintenanceReady) error {
		opened, e1 := dir.Stat()
		named, e2 := os.Lstat(rl.Spool)
		if e1 != nil || e2 != nil || !os.SameFile(opened, named) {
			return errors.New("relay maintenance: activation root continuity changed")
		}
		if err := intent.Verify(root); err != nil {
			return err
		}
		fresh, err := countMaintenanceCustody(ctx, root, marker.ID, true)
		if err != nil || fresh != expected.Preserved {
			return errors.Join(errors.New("relay maintenance: custody/queue changed; stay held"), err)
		}
		return nil
	}
	gate, err := servicedrain.NewStartupMaintenance(os.Getpid(), started, marker, count, verify, func() (int, error) { return maintenanceCustodyCount(ctx, root, marker.ID) })
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
	return CountMaintenanceCustody(ctx, root, id)
}

// CountMaintenanceCustody is the producer's bounded names-only inventory,
// shared with cross-path fixtures. It grants no authority and opens no payload.
func CountMaintenanceCustody(ctx context.Context, root *os.Root, id string) (int, error) {
	return countMaintenanceCustody(ctx, root, id, false)
}

func countMaintenanceCustody(ctx context.Context, root *os.Root, id string, requireEmpty bool) (int, error) {
	dir, err := root.Open(".")
	if err != nil {
		return 0, err
	}
	defer dir.Close()
	count := 0
	budget := new(servicedrain.CustodyBudget)
	for lanes := 0; ; lanes++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		entries, err := dir.ReadDir(1)
		if err == io.EOF {
			return count, nil
		}
		if err != nil || lanes >= servicedrain.MaxCustodyLanes {
			return 0, errors.New("relay maintenance: bounded lane enumeration unavailable")
		}
		e := entries[0]
		if e.Name() == RelayObservationMetadataName {
			if err := ReadMaintenanceRelayMetadata(ctx, int(dir.Fd()), "/private/var/sirsipantheon/relay"); err != nil {
				return 0, err
			}
			continue
		}
		if (e.Name() == servicedrain.MaintenanceMarkerName || e.Name() == servicedrain.MaintenancePrepareName) && e.Type().IsRegular() {
			continue
		}
		if !e.IsDir() || !validSpoolAgentName(e.Name()) {
			return 0, errors.New("relay maintenance: unknown lane name")
		}
		if requireEmpty {
			if err := maintenanceNoForwardSources(int(dir.Fd()), e.Name()); err != nil {
				return 0, err
			}
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
			for _, name := range names {
				budgetErr := budget.Observe(name)
				var st unix.Stat_t
				if budgetErr != nil || unix.Fstatat(int(custody.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 {
					_ = custody.Close()
					return 0, errors.New("relay maintenance: unknown custody entry retained; stay held")
				}
				if strings.HasPrefix(name, "envelope-") {
					count++
				}
			}
			if readErr != nil && readErr != io.EOF {
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

// This adapter consumes Nash's one existing metadata schema and policy. It
// does not create another summary, producer identity or metadata allowlist.
func ReadMaintenanceRelayMetadata(ctx context.Context, root int, path string) error {
	parent, err := openAbsoluteDirectoryNoFollow(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	var rst, pst unix.Stat_t
	if err := unix.Fstat(root, &rst); err != nil {
		return err
	}
	if err := unix.Fstat(parent, &pst); err != nil {
		return err
	}
	policy, err := RelayObservationPolicyForRoot(path, rst, pst)
	if err != nil {
		return err
	}
	_, err = ReadRelayObservationMetadataAt(ctx, root, policy)
	return err
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
