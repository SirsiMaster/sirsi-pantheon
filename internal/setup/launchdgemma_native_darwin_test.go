//go:build darwin

package setup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestGemmaNativeKernelChildrenDoesNotCreateObserverChild(t *testing.T) {
	const helper = "PANTHEON_GEMMA_KERNEL_CHILD_FIXTURE"
	if os.Getenv(helper) != "1" {
		// Other package tests can own children concurrently. Exercise the real
		// observer in an isolated test process whose child inventory we control.
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestGemmaNativeKernelChildrenDoesNotCreateObserverChild$", "-test.count=1")
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, helper+"=") {
				cmd.Env = append(cmd.Env, value)
			}
		}
		cmd.Env = append(cmd.Env, helper+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated native child observer: %v: %s", err, output)
		}
		return
	}
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
