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
	lookups := 0
	intent, err := openMaintenanceStartupIntentForRelay(root, uint32(os.Getuid()), func() (int, int, error) {
		lookups++
		return os.Getuid(), os.Getgid(), nil
	})
	if err != nil || intent == nil || lookups != 1 || intent.Marker != marker {
		t.Fatalf("resolved startup intent = %v, %v, identity calls=%d", intent, err, lookups)
	}
	if err := intent.Close(); err != nil {
		t.Fatal(err)
	}
}
