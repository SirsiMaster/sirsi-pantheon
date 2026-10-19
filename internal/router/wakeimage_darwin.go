//go:build darwin && cgo

package router

/*
#include <libproc.h>
#include <sys/proc_info.h>
#include <stdint.h>
#include <unistd.h>
#include <errno.h>
#include <string.h>

// Ask the kernel about this function's loaded executable mapping, not whatever
// inode happens to occupy its pathname after an installer rename.
static int sirsi_loaded_wake_image(uint64_t *device, uint64_t *inode) {
    struct proc_regionwithpathinfo region;
    memset(&region, 0, sizeof(region));
    int n = proc_pidinfo(getpid(), PROC_PIDREGIONPATHINFO,
        (uint64_t)(uintptr_t)&sirsi_loaded_wake_image, &region, sizeof(region));
    if (n != sizeof(region)) return errno ? errno : EIO;
    if (!region.prp_vip.vip_vi.vi_stat.vst_ino) return EIO;
    *device = region.prp_vip.vip_vi.vi_stat.vst_dev;
    *inode = region.prp_vip.vip_vi.vi_stat.vst_ino;
    return 0;
}
*/
import "C"

import "syscall"

func nativeWakeImageIdentity() (wakeImageIdentity, error) {
	var device, inode C.uint64_t
	if code := C.sirsi_loaded_wake_image(&device, &inode); code != 0 {
		return wakeImageIdentity{}, syscall.Errno(code)
	}
	return wakeImageIdentity{uint64(device), uint64(inode)}, nil
}
