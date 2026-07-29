//go:build darwin

package vitals

/*
#include <libproc.h>
#include <sys/resource.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// PhysFootprint returns a process's PHYSICAL FOOTPRINT in bytes — the number
// macOS itself uses when it decides who to Jetsam, and the number Activity
// Monitor shows as "Memory".
//
// WHY THIS EXISTS, AND WHY RSS IS A LIE HERE.
//
// Every health check in this codebase sampled `ps -o rss`. RSS counts only
// RESIDENT pages. On Apple Silicon the compressor holds the rest, and it is not
// a rounding error — measured on this workstation 2026-07-27, on the gemma
// broker, minutes after a clean restart:
//
//	ps rss:                    2.77 GB   <- what every check saw
//	phys_footprint:           19.2  GB   <- what the kernel saw
//	phys_footprint (peak):    40.5  GB   <- on a 48 GB machine
//
// A 7x live divergence, 15x at peak. The Jetsam reports for the three OOM
// events that day name this process with a 43.94 GB footprint of which
// 39.87 GB was compressed and only 0.68 GB resident — a 65x error. Any watcher
// sampling RSS reports an innocent process while the machine dies around it.
//
// This is the same class as the free-vs-available bug fixed in #316: the metric
// was legible, cheap, and measuring the wrong thing.
//
// proc_pid_rusage is a syscall, not a subprocess, so this is cheap enough to
// call for every process in a census — unlike `footprint(1)` or `vmmap(1)`,
// which are correct but far too slow to sample.
func PhysFootprint(pid int) (uint64, error) {
	var ri C.struct_rusage_info_v4
	rc := C.proc_pid_rusage(C.int(pid), C.RUSAGE_INFO_V4,
		(*C.rusage_info_t)(unsafe.Pointer(&ri)))
	if rc != 0 {
		// Non-fatal by design: a process can exit between enumeration and
		// measurement, and callers must degrade to RSS rather than fail a whole
		// census over one dead pid.
		return 0, fmt.Errorf("proc_pid_rusage(%d): rc=%d", pid, int(rc))
	}
	return uint64(ri.ri_phys_footprint), nil
}

// PeakPhysFootprint returns the high-water mark of a process's physical
// footprint. The peak is what explains a Jetsam AFTER the fact: a process can
// sit at a modest footprint when you look at it and still have been the largest
// consumer at the moment the kernel chose a victim. The broker above read
// 19.2 GB live with a 40.5 GB peak.
func PeakPhysFootprint(pid int) (uint64, error) {
	var ri C.struct_rusage_info_v4
	rc := C.proc_pid_rusage(C.int(pid), C.RUSAGE_INFO_V4,
		(*C.rusage_info_t)(unsafe.Pointer(&ri)))
	if rc != 0 {
		return 0, fmt.Errorf("proc_pid_rusage(%d): rc=%d", pid, int(rc))
	}
	return uint64(ri.ri_lifetime_max_phys_footprint), nil
}
