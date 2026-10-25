//go:build darwin && cgo

package setup

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

// Obtain the retained payload's static CDHash only after canonical signature
// validation. For a running image, ask the kernel-root code host to resolve
// BOTH PID and that hash. Never use CopySigningInformation(dynamic) as the
// loaded hash: Apple's API explicitly says that information can come from disk.
static int sirsi_maintenance_image(const char *path, int pid, unsigned char out[20]) {
    SecStaticCodeRef code = NULL;
    SecRequirementRef requirement = NULL;
    CFDictionaryRef info = NULL, attributes = NULL;
    CFNumberRef number = NULL;
    SecCodeRef guest = NULL;
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
    if (pid > 1) {
        number = CFNumberCreate(NULL, kCFNumberIntType, &pid);
        if (!number) { result = -3; goto done; }
        const void *keys[] = { kSecGuestAttributePid, kSecGuestAttributeHash };
        const void *values[] = { number, hash };
        attributes = CFDictionaryCreate(NULL, keys, values, 2, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
        if (!number || !attributes) { result = -3; goto done; }
        result = SecCodeCopyGuestWithAttributes(NULL, attributes, kSecCSDefaultFlags, &guest);
        if (result) goto done;
        // Refuse hosts that accept but ignore the hash selector. A successful
        // PID lookup alone must never masquerade as loaded-image identity.
        unsigned char wrongBytes[20];
        memcpy(wrongBytes, out, 20);
        wrongBytes[0] ^= 0xff;
        CFDataRef wrongHash = CFDataCreate(NULL, wrongBytes, 20);
        if (!wrongHash) { result = -3; goto done; }
        const void *wrongValues[] = { number, wrongHash };
        CFDictionaryRef wrongAttributes = CFDictionaryCreate(NULL, keys, wrongValues, 2, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
        CFRelease(wrongHash);
        if (!wrongAttributes) { result = -3; goto done; }
        SecCodeRef wrongGuest = NULL;
        OSStatus wrongResult = SecCodeCopyGuestWithAttributes(NULL, wrongAttributes, kSecCSDefaultFlags, &wrongGuest);
        CFRelease(wrongAttributes);
        if (wrongGuest) CFRelease(wrongGuest);
        if (wrongResult != errSecCSNoSuchCode) { result = -4; goto done; }
        // Unsupported/ambiguous guest selectors are explicit refusal. Do not
        // retry using just PID or the current executable pathname.
    }
done:
    if (guest) CFRelease(guest);
    if (attributes) CFRelease(attributes);
    if (number) CFRelease(number);
    if (info) CFRelease(info);
    if (requirement) CFRelease(requirement);
    if (code) CFRelease(code);
    return (int)result;
}
*/
import "C"

import (
	"encoding/hex"
	"fmt"
	"unsafe"
)

func maintenanceSignedImage(path string, pid int) (string, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var hash [20]byte
	if status := C.sirsi_maintenance_image(cpath, C.int(pid), (*C.uchar)(unsafe.Pointer(&hash[0]))); status != 0 {
		return "", fmt.Errorf("legacy maintenance: signed payload/kernel PID+CDHash guest binding unavailable (status %d); no pathname fallback", int(status))
	}
	return hex.EncodeToString(hash[:]), nil
}
