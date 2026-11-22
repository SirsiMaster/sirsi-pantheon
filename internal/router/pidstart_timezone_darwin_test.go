//go:build darwin

package router

import (
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// The child belongs to this test; only it is stopped by cleanup. No live
// service or shared registry is modified. Changing time.Local is test-process
// local and must not be run in parallel with another time.Local test.
func TestRealOwnedProcessLegacyTimezoneAndExit(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command("/bin/sleep", "10")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	info, err := unix.SysctlKinfoProc("kern.proc.pid", child.Process.Pid)
	if err != nil || info == nil || int(info.Proc.P_pid) != child.Process.Pid {
		t.Fatalf("owned PID start unavailable: %v", err)
	}
	recorded := time.Unix(info.Proc.P_starttime.Sec, 0).In(chicago).Format(time.ANSIC)
	savedLocal := time.Local
	time.Local = time.UTC
	defer func() { time.Local = savedLocal }()
	if got := PIDStateOf(child.Process.Pid, recorded); got != PIDUnknown {
		t.Fatalf("live owned process with ambiguous timezone: got %s, want unknown", got)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	if got := PIDStateOf(child.Process.Pid, recorded); got != PIDGone {
		t.Fatalf("exited owned process: got %s, want gone", got)
	}
}
