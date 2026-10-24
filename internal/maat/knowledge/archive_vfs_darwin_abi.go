//go:build darwin && cgo

package knowledge

import (
	"runtime"
	"unsafe"

	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

const archiveABICopyLimit = uintptr(256)

func archiveABICopyLength(size uintptr) int {
	if size == 0 || size > archiveABICopyLimit {
		panic("archive ABI copy exceeds its fixed layout bound")
	}
	return int(size)
}

// Generated layouts below contain only fixed-width scalars and uintptr C
// addresses/function slots; they contain no Go pointers. The native shim
// interprets SQLite's numeric address only in C, where it may refer to
// modernc's generated Go-global module data. Go-owned buffers cross the cgo
// boundary as typed pointers and stay pinned only for the synchronous copy.
// Each wrapper supplies the exact generated struct/scalar size.
func archiveCopyFromC(_ *libc.TLS, destination unsafe.Pointer, source uintptr, size uintptr) {
	if destination == nil || source == 0 {
		panic("archive ABI copy requires nonnil addresses")
	}
	var pin runtime.Pinner
	pin.Pin(destination)
	defer pin.Unpin()
	length := archiveABICopyLength(size)
	archiveNativeCopyFromC(destination, source, uintptr(length))
	runtime.KeepAlive(destination)
}

func archiveCopyToC(_ *libc.TLS, destination uintptr, source unsafe.Pointer, size uintptr) {
	if destination == 0 || source == nil {
		panic("archive ABI copy requires nonnil addresses")
	}
	var pin runtime.Pinner
	pin.Pin(source)
	defer pin.Unpin()
	length := archiveABICopyLength(size)
	archiveNativeCopyToC(destination, source, uintptr(length))
	runtime.KeepAlive(source)
}

func archiveCopyCAddress(_ *libc.TLS, destination uintptr, source uintptr, size uintptr) {
	if destination == 0 || source == 0 {
		panic("archive ABI copy requires nonnil addresses")
	}
	length := archiveABICopyLength(size)
	archiveNativeCopyCAddress(destination, source, uintptr(length))
}

func archiveReadSQLiteVFS(tls *libc.TLS, address uintptr) sqlite3.Tsqlite3_vfs {
	var value sqlite3.Tsqlite3_vfs
	archiveCopyFromC(tls, unsafe.Pointer(&value), address, unsafe.Sizeof(value))
	return value
}

// SQLite protects its VFS registry links with its main mutex, but releases
// that mutex before returning from sqlite3_vfs_find. Copy only the stable
// configuration fields needed to delegate; FpNext, FzName, and FpAppData are
// registry-owned/mutable and intentionally remain zero in this snapshot.
func archiveReadSQLiteVFSConfiguration(tls *libc.TLS, address uintptr) sqlite3.Tsqlite3_vfs {
	var value sqlite3.Tsqlite3_vfs
	linkOffset := unsafe.Offsetof(sqlite3.Tsqlite3_vfs{}.FpNext)
	callbacksOffset := unsafe.Offsetof(sqlite3.Tsqlite3_vfs{}.FxOpen)
	archiveCopyFromC(tls, unsafe.Pointer(&value), address, linkOffset)
	archiveCopyFromC(tls, unsafe.Add(unsafe.Pointer(&value), callbacksOffset), archiveAddABIAddress(address, callbacksOffset), unsafe.Sizeof(value)-callbacksOffset)
	return value
}

func archiveAddABIAddress(base, offset uintptr) uintptr {
	if base == 0 || ^uintptr(0)-base < offset {
		panic("archive ABI address offset overflows")
	}
	return base + offset
}

func archiveReadUnixFile(tls *libc.TLS, address uintptr) sqlite3.TunixFile {
	var value sqlite3.TunixFile
	archiveCopyFromC(tls, unsafe.Pointer(&value), address, unsafe.Sizeof(value))
	return value
}

func archiveReadIOMethods(tls *libc.TLS, address uintptr) sqlite3.Tsqlite3_io_methods {
	var value sqlite3.Tsqlite3_io_methods
	archiveCopyFromC(tls, unsafe.Pointer(&value), address, unsafe.Sizeof(value))
	return value
}

func archiveReadCInt32(tls *libc.TLS, address uintptr) int32 {
	var value int32
	archiveCopyFromC(tls, unsafe.Pointer(&value), address, unsafe.Sizeof(value))
	return value
}

func archiveWriteCInt32(tls *libc.TLS, address uintptr, value int32) {
	archiveCopyToC(tls, address, unsafe.Pointer(&value), unsafe.Sizeof(value))
}

func archiveReadCUintptr(tls *libc.TLS, address uintptr) uintptr {
	var value uintptr
	archiveCopyFromC(tls, unsafe.Pointer(&value), address, unsafe.Sizeof(value))
	return value
}

func archiveWriteCUintptr(tls *libc.TLS, address uintptr, value uintptr) {
	archiveCopyToC(tls, address, unsafe.Pointer(&value), unsafe.Sizeof(value))
}

func archiveWriteCByte(tls *libc.TLS, address uintptr, value byte) {
	archiveCopyToC(tls, address, unsafe.Pointer(&value), unsafe.Sizeof(value))
}

func archiveReadCByte(tls *libc.TLS, address uintptr) byte {
	var value byte
	archiveCopyFromC(tls, unsafe.Pointer(&value), address, unsafe.Sizeof(value))
	return value
}
