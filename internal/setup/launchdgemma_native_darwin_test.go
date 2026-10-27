//go:build darwin

package setup

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/unix"
)

func TestGemmaNativeKernelChildrenDoesNotCreateObserverChild(t *testing.T) {
	if e := gemmaKernelChildrenClear(os.Getpid()); e != nil {
		t.Fatal(e)
	}
	child := exec.Command("/bin/sleep", "5")
	if e := child.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	if gemmaKernelChildrenClear(os.Getpid()) == nil {
		t.Fatal("actual child ignored")
	}
}
func TestGemmaNativeAbsentPIDNeedsIndependentProof(t *testing.T) {
	child := exec.Command("/usr/bin/true")
	if e := child.Start(); e != nil {
		t.Fatal(e)
	}
	pid := child.Process.Pid
	if e := child.Wait(); e != nil {
		t.Fatal(e)
	}
	if e := unix.Kill(pid, 0); !errors.Is(e, unix.ESRCH) {
		t.Fatal(e)
	}
	if e := gemmaKernelAbsent(pid); e != nil {
		t.Fatal(e)
	}
	if gemmaKernelAbsent(os.Getpid()) == nil {
		t.Fatal("live self became absent")
	}
}
