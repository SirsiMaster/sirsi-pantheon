//go:build darwin

package main

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// kinfoAnchorProcess reads a process's parent and name from the kernel with
// sysctl, no fork. A sandboxed worker (codex: fork/exec /bin/ps denied) can
// still call it, which lets thread registration resolve its anchor. The name is
// the kernel's p_comm, truncated to 16 bytes, so it is only the fallback for
// when `ps` is unavailable.
func kinfoAnchorProcess(pid int) (anchorProcess, error) {
	k, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return anchorProcess{}, fmt.Errorf("inspect pid %d: %w", pid, err)
	}
	return anchorProcess{parentPID: int(k.Eproc.Ppid), command: unix.ByteSliceToString(k.Proc.P_comm[:])}, nil
}
