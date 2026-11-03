//go:build darwin && cgo

package main

/*
#include <errno.h>
#include <stddef.h>
#include <sys/attr.h>
#include <unistd.h>

// Keep the SDK-defined request and errno on the calling thread. The buffer is
// borrowed only for this call; libSystem does not retain it.
static int sirsi_installer_query_acl(int fd, void *buf, size_t size) {
	struct attrlist attrs = {0};
	attrs.bitmapcount = ATTR_BIT_MAP_COUNT;
	attrs.commonattr = ATTR_CMN_EXTENDED_SECURITY;
	if (fgetattrlist(fd, &attrs, buf, size, FSOPT_REPORT_FULLSIZE) != 0) {
		return errno != 0 ? errno : EIO;
	}
	return 0;
}
*/
import "C"

import (
	"errors"
	"syscall"
	"unsafe"
)

const aclInspectionAvailable = true

func queryACL(fd int, buf []byte) error {
	if len(buf) == 0 {
		return errors.New("empty ACL query buffer")
	}
	if errno := C.sirsi_installer_query_acl(C.int(fd), unsafe.Pointer(&buf[0]), C.size_t(len(buf))); errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}
