//go:build linux

package routerstore

import (
	"errors"

	"golang.org/x/sys/unix"
)

// Only absence is accepted. Unsupported/unknown ACL inspection cannot widen
// the new summary's reader set. Existing objects are never chmod/chown'ed.
func relayObservationNoACL(fd int) bool {
	n, err := unix.Fgetxattr(fd, "system.posix_acl_access", nil)
	return (err == nil && n == 0) || errors.Is(err, unix.ENODATA)
}
