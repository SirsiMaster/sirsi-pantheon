//go:build darwin && cgo

package routerstore

/*
#include <stdint.h>
#include <sys/attr.h>
#include <unistd.h>

// Call the supported libSystem entry point using the SDK's ABI, rather than a
// direct kernel syscall. All storage is bounded, C-owned and call-local.
static int sirsi_relay_observation_no_acl(int fd) {
	struct attrlist attrs = {0};
	attrs.bitmapcount = ATTR_BIT_MAP_COUNT;
	attrs.commonattr = ATTR_CMN_EXTENDED_SECURITY;
	struct {
		uint32_t length;
		attrreference_t acl;
	} result = {0};
	// Report the full required size: a nonempty ACL must not look empty when
	// its variable bytes do not fit in this fixed-size result buffer.
	if (fgetattrlist(fd, &attrs, &result, sizeof(result), FSOPT_REPORT_FULLSIZE) != 0) {
		return 0;
	}
	return result.length == sizeof(result) && result.acl.attr_length == 0;
}
*/
import "C"

// Exact gid/mode must not accidentally include inherited ACL readers. Darwin
// protects com.apple.system.Security from ordinary fgetxattr; instead inspect
// ATTR_CMN_EXTENDED_SECURITY on the retained descriptor through libSystem.
// See SDK sys/attr.h and Apple xnu/bsd/vfs/vfs_attrlist.c. No ACL is changed.
func relayObservationNoACL(fd int) bool {
	return C.sirsi_relay_observation_no_acl(C.int(fd)) == 1
}
