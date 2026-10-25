//go:build darwin

package localreceipt

import (
	"golang.org/x/sys/unix"
	"time"
)

func parentStart(pid int) string {
	k, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || k == nil || int(k.Proc.P_pid) != pid || k.Proc.P_starttime.Sec <= 0 {
		return ""
	}
	return time.Unix(k.Proc.P_starttime.Sec, 0).Local().Format(time.ANSIC)
}
