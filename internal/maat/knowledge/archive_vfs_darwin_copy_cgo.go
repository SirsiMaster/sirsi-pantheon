//go:build darwin && cgo

package knowledge

/*
#include <stddef.h>
#include <stdint.h>
#include <string.h>

// The numeric source/destination is a modernc SQLite ABI address. It can
// identify generated Go module data, so only C performs this integer-to-C
// pointer conversion. The Go buffer is passed separately as a typed pointer.
static void sirsi_archive_copy_from_c(void *destination, uintptr_t source, size_t size) {
	memcpy(destination, (const void *)source, size);
}

static void sirsi_archive_copy_to_c(uintptr_t destination, const void *source, size_t size) {
	memcpy((void *)destination, source, size);
}

static void sirsi_archive_copy_c_address(uintptr_t destination, uintptr_t source, size_t size) {
	memcpy((void *)destination, (const void *)source, size);
}
*/
import "C"

import "unsafe"

func archiveNativeCopyFromC(destination unsafe.Pointer, source uintptr, size uintptr) {
	C.sirsi_archive_copy_from_c(destination, C.uintptr_t(source), C.size_t(size))
}

func archiveNativeCopyToC(destination uintptr, source unsafe.Pointer, size uintptr) {
	C.sirsi_archive_copy_to_c(C.uintptr_t(destination), source, C.size_t(size))
}

func archiveNativeCopyCAddress(destination uintptr, source uintptr, size uintptr) {
	C.sirsi_archive_copy_c_address(C.uintptr_t(destination), C.uintptr_t(source), C.size_t(size))
}
