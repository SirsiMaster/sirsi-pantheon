//go:build darwin

package router

import (
	"time"

	"golang.org/x/sys/unix"
)

// Use the same kernel interface as the CLI's ps-free anchor resolver. Preserve
// lstart's local-time, second-resolution representation for existing threads;
// this is not a migration to a new PID-generation format.
func nativePIDStart(pid int) string {
	return nativePIDStartWith(pid, func(name string, pid int) (*unix.KinfoProc, error) {
		return unix.SysctlKinfoProc(name, pid)
	})
}

func nativePIDStartWith(pid int, lookup func(string, int) (*unix.KinfoProc, error)) string {
	if pid < minAgentPID {
		return ""
	}
	k, err := lookup("kern.proc.pid", pid)
	if err != nil || k == nil || int(k.Proc.P_pid) != pid || k.Proc.P_starttime.Sec <= 0 {
		return ""
	}
	return time.Unix(k.Proc.P_starttime.Sec, 0).Local().Format(time.ANSIC)
}
