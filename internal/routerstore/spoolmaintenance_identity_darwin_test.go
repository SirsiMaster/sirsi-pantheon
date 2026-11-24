//go:build darwin

package routerstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
	"golang.org/x/sys/unix"
)

func TestMaintenanceStartupUsesResolvedNamedGroupAndSkipsLookupWithoutIntent(t *testing.T) {
	const primaryGID, namedGID = uint32(20), uint32(501)
	stat := unix.Stat_t{Uid: 0, Gid: namedGID, Mode: unix.S_IFREG | 0640, Nlink: 1, Size: 64}
	identity := func() (int, int, error) { return 543, int(namedGID), nil }
	_, resolvedGID, err := identity()
	if err != nil || !maintenanceStartupIntentMetadataAllowed(stat, 0, uint32(resolvedGID)) {
		t.Fatal("root-authored startup intent with enrolled named group was refused")
	}
	if maintenanceStartupIntentMetadataAllowed(stat, 0, uint32(primaryGID)) {
		t.Fatal("startup intent was accepted under the process primary GID instead of the named group")
	}

	path := t.TempDir()
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	lookups := 0
	intent, err := openMaintenanceStartupIntentForRelay(root, 0, func() (int, int, error) {
		lookups++
		return identity()
	})
	if err != nil || intent != nil || lookups != 0 {
		t.Fatalf("empty startup intent = %v, %v, resolver calls=%d; want ordinary no-marker path with no identity lookup", intent, err, lookups)
	}
}

func TestMaintenanceStartupIntentOpensWithResolvedGroup(t *testing.T) {
	path := t.TempDir()
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(dir.Fd()), &st); err != nil {
		t.Fatal(err)
	}
	if err := dir.Close(); err != nil {
		t.Fatal(err)
	}
	marker := servicedrain.MaintenanceMarker{
		Schema: 1, ID: strings.Repeat("a", 32), PlanSHA256: strings.Repeat("b", 64),
		RootDevice: uint64(st.Dev), RootInode: st.Ino,
	}
	raw, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, servicedrain.MaintenanceMarkerName), append(raw, '\n'), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(path, servicedrain.MaintenanceMarkerName), 0640); err != nil {
		t.Fatal(err)
	}
	// Darwin temp children inherit the private directory's group, which need
	// not be the process primary group. This is owned fixture policy only;
	// production must still resolve and enforce its enrolled named group.
	fixtureGID := st.Gid
	file, err := root.OpenFile(servicedrain.MaintenanceMarkerName, os.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var markerStat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &markerStat); err != nil {
		t.Fatal(err)
	}
	owner := uint32(os.Getuid())
	if markerStat.Gid != fixtureGID || !maintenanceStartupIntentMetadataAllowed(markerStat, owner, fixtureGID) {
		t.Fatalf("fixture metadata uid=%d gid=%d mode=%#o; retained directory gid=%d, primary gid=%d", markerStat.Uid, markerStat.Gid, markerStat.Mode, fixtureGID, os.Getgid())
	}
	// Exercise the primary-group mismatch when present, and a different group
	// even on hosts where primary and inherited fixture groups happen to agree.
	wrongGID := uint32(os.Getgid())
	if wrongGID == fixtureGID {
		wrongGID = fixtureGID ^ 1
	}
	if maintenanceStartupIntentMetadataAllowed(markerStat, owner, wrongGID) {
		t.Fatalf("fixture marker accepted mismatched policy gid=%d, actual gid=%d", wrongGID, fixtureGID)
	}
	lookups := 0
	intent, err := openMaintenanceStartupIntentForRelay(root, owner, func() (int, int, error) {
		lookups++
		return os.Getuid(), int(fixtureGID), nil
	})
	if err != nil || intent == nil || lookups != 1 || intent.Marker != marker || intent.gid != fixtureGID || intent.owner != owner {
		t.Fatalf("resolved startup intent = %v, %v, identity calls=%d", intent, err, lookups)
	}
	if err := intent.Close(); err != nil {
		t.Fatal(err)
	}
}
