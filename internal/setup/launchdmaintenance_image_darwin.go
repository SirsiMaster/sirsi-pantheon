//go:build darwin && cgo

package setup

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <errno.h>
#include <stdlib.h>
#include <string.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <unistd.h>

// Apple XNU bsd/sys/codesign.h defines CS_OPS_CDHASH as the read-only
// csops operation that returns the selected process's kernel-loaded CDHash.
// The macOS SDK exports SYS_csops but omits the operation constant.
#define SIRSI_CS_OPS_CDHASH 5u

// Obtain the retained payload's static CDHash only after canonical signature
// validation. Loaded identity is read independently from the kernel below.
static int sirsi_maintenance_static_image(const char *path, unsigned char out[20]) {
    SecStaticCodeRef code = NULL;
    SecRequirementRef requirement = NULL;
    CFDictionaryRef info = NULL;
    CFURLRef url = CFURLCreateFromFileSystemRepresentation(NULL, (const UInt8 *)path, strlen(path), false);
    if (!url) return -1;
    OSStatus result = SecStaticCodeCreateWithPath(url, kSecCSDefaultFlags, &code);
    CFRelease(url);
    if (result) goto done;
    result = SecRequirementCreateWithString(CFSTR("anchor apple generic and certificate leaf[subject.OU] = \"9D382WV988\" and identifier \"sirsi\""), kSecCSDefaultFlags, &requirement);
    if (result) goto done;
    result = SecStaticCodeCheckValidity(code, kSecCSStrictValidate, requirement);
    if (result) goto done;
    result = SecCodeCopySigningInformation(code, kSecCSSigningInformation, &info);
    if (result) goto done;
    CFDataRef hash = CFDictionaryGetValue(info, kSecCodeInfoUnique);
    if (!hash || CFGetTypeID(hash) != CFDataGetTypeID() || CFDataGetLength(hash) != 20) { result = -2; goto done; }
    memcpy(out, CFDataGetBytePtr(hash), 20);
done:
    if (info) CFRelease(info);
    if (requirement) CFRelease(requirement);
    if (code) CFRelease(code);
    return (int)result;
}

// This is a read-only query of the kernel's code-signing state for pid. A
// failure is returned to the caller; there is no Security/PID/path fallback.
static int sirsi_maintenance_loaded_cdhash(int pid, unsigned char out[20]) {
    if (pid <= 1) return EINVAL;
    long result = syscall(SYS_csops, (pid_t)pid, SIRSI_CS_OPS_CDHASH, out, 20);
    if (result == -1) return errno ? errno : EIO;
    if (result != 0) return EIO;
    return 0;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func maintenanceSignedImage(path string, pid int) (string, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	return maintenanceImageCDHash(path, pid,
		func(string) ([]byte, error) {
			var hash [20]byte
			if status := C.sirsi_maintenance_static_image(cpath, (*C.uchar)(unsafe.Pointer(&hash[0]))); status != 0 {
				return nil, fmt.Errorf("strict retained signature inspection failed (status %d)", int(status))
			}
			return hash[:], nil
		},
		maintenanceKernelCDHash)
}

func maintenanceKernelCDHash(pid int) ([]byte, error) {
	if pid <= 1 {
		return nil, fmt.Errorf("invalid selected kernel PID %d", pid)
	}
	var hash [20]byte
	if status := C.sirsi_maintenance_loaded_cdhash(C.int(pid), (*C.uchar)(unsafe.Pointer(&hash[0]))); status != 0 {
		return nil, fmt.Errorf("csops(CS_OPS_CDHASH) failed (errno %d)", int(status))
	}
	return hash[:], nil
}
