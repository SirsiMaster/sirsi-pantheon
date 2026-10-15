//go:build darwin

package router

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPIDStartWithoutPS(t *testing.T) {
	t.Setenv("PATH", "")
	got := defaultPIDStart(os.Getpid())
	if got == "" || got != nativePIDStart(os.Getpid()) {
		t.Fatalf("kernel-only self identity unavailable: %q", got)
	}
	for _, pid := range []int{0, 1, -1, 2147483000} {
		if got := PIDStartTimeOf(pid); got != "" {
			t.Fatalf("invalid or absent pid %d acquired identity %q", pid, got)
		}
	}
}

func TestNativePIDStartPreservesExistingPSIdentity(t *testing.T) {
	cmd := exec.Command("/bin/ps", "-o", "lstart=", "-p", strconv.Itoa(os.Getpid()))
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("native compatibility comparator failed: %v", err)
	}
	if got, want := nativePIDStart(os.Getpid()), strings.TrimSpace(string(out)); got == "" || got != want {
		t.Fatalf("kernel start %q differs from existing ps start %q", got, want)
	}
}

func TestNativePIDStartRejectsUnavailableOrWrongIdentity(t *testing.T) {
	for _, name := range []string{"error", "nil", "wrong-pid", "zero-time"} {
		t.Run(name, func(t *testing.T) {
			lookup := func(key string, pid int) (*unix.KinfoProc, error) {
				if key != "kern.proc.pid" || pid != 42 {
					t.Fatal("unexpected kernel query")
				}
				k := &unix.KinfoProc{}
				k.Proc.P_pid = 42
				k.Proc.P_starttime.Sec = 1234
				switch name {
				case "error":
					return k, errors.New("denied")
				case "nil":
					return nil, nil
				case "wrong-pid":
					k.Proc.P_pid = 43
				case "zero-time":
					k.Proc.P_starttime.Sec = 0
				}
				return k, nil
			}
			if got := nativePIDStartWith(42, lookup); got != "" {
				t.Fatalf("unverified identity %q", got)
			}
		})
	}
}

func TestNativePIDStartChangesWithProcessGeneration(t *testing.T) {
	probe := func(sec int64) string {
		return nativePIDStartWith(42, func(string, int) (*unix.KinfoProc, error) {
			k := &unix.KinfoProc{}
			k.Proc.P_pid = 42
			k.Proc.P_starttime.Sec = sec
			return k, nil
		})
	}
	if a, b := probe(1234), probe(1235); a == "" || b == "" || a == b {
		t.Fatal("changed kernel start must not retain old generation")
	}
	if got := probe(time.Date(2026, time.October, 8, 12, 0, 0, 0, time.Local).Unix()); !strings.Contains(got, "Oct  8") {
		t.Fatalf("single-digit day lost ps-compatible padding: %q", got)
	}
}
