//go:build darwin

package setup

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestMaintenanceSpoolUsesNamedGroupNotPrimaryGID(t *testing.T) {
	const serviceUID, primaryGID, namedGID = uint32(543), uint32(20), uint32(501)
	spool := unix.Stat_t{Uid: serviceUID, Gid: namedGID, Mode: unix.S_IFDIR | 0770}
	if !maintenanceSpoolIdentityAllowed(spool, int(serviceUID), int(namedGID), 0, 0) {
		t.Fatal("canonical spool with the enrolled service group was refused")
	}
	spool.Gid = primaryGID
	if maintenanceSpoolIdentityAllowed(spool, int(serviceUID), int(namedGID), 0, 0) {
		t.Fatal("canonical spool with only the service primary GID was accepted")
	}
}
