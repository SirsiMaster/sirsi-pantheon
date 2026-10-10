package router

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestPressureChild(t *testing.T) {
	p := os.Getenv("PRESSURE_TEST_PATH")
	if p == "" {
		return
	}
	mode := os.Getenv("PRESSURE_TEST_MODE")
	if mode == "crash" {
		f, err := os.OpenFile(p+".lock", os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p+".ready", []byte("locked"), 0600); err != nil {
			t.Fatal(err)
		}
		os.Exit(23) // intentionally no unlock: kernel must release on process death
	}
	v, ok := coordinatedHostLoad(p, func() (float64, bool) {
		f, err := os.OpenFile(p+".calls", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("probe\n"); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		return 9.5, mode != "unknown"
	}, 2*time.Second)
	if mode == "unknown" {
		if ok || v != 0 {
			t.Fatal(v, ok)
		}
	} else if !ok || v != 9.5 {
		t.Fatal(v, ok)
	}
}

func TestCrossProcessPressure(t *testing.T) {
	for _, mode := range []string{"cold", "stale", "unknown", "crash"} {
		t.Run(mode, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "cache")
			if mode == "stale" {
				if err := os.WriteFile(p, []byte("v2 "+strconv.FormatInt(time.Now().Add(-time.Minute).UnixMilli(), 10)+" true 1\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			command := func(m string) *exec.Cmd {
				c := exec.Command(os.Args[0], "-test.run=^TestPressureChild$")
				c.Env = append(os.Environ(), "PRESSURE_TEST_PATH="+p, "PRESSURE_TEST_MODE="+m)
				return c
			}
			if mode == "crash" {
				err := command("crash").Run()
				if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 23 {
					t.Fatalf("crash control %v", err)
				}
				if _, err := os.Stat(p + ".ready"); err != nil {
					t.Fatal(err)
				}
			}
			var wg sync.WaitGroup
			for i := 0; i < 9; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					childMode := mode
					if childMode == "crash" {
						childMode = "cold"
					}
					out, err := command(childMode).CombinedOutput()
					if err != nil {
						t.Errorf("child %v: %s", err, out)
					}
				}()
			}
			wg.Wait()
			b, err := os.ReadFile(p + ".calls")
			if err != nil {
				t.Fatal(err)
			}
			if n := strings.Count(string(b), "probe\n"); n != 1 {
				t.Fatalf("spawned %d probes across 9 processes", n)
			}
		})
	}
}

func TestCoordinationFailure(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cache")
	f, err := os.OpenFile(p+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	calls := 0
	probe := func() (float64, bool) { calls++; return 10, true }
	v, ok := coordinatedHostLoad(p, probe, 40*time.Millisecond)
	if ok || v != 0 || calls != 0 {
		t.Fatal("contender probed", v, ok, calls)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(p, 0700); err != nil {
		t.Fatal(err)
	} // rename failure after actual probe
	v, ok = coordinatedHostLoad(p, probe, time.Second)
	if !ok || v != 10 || calls != 1 {
		t.Fatal("publish failure lost measured pressure", v, ok, calls)
	}
	v, ok = coordinatedHostLoad(filepath.Join(p, "absent", "cache"), probe, time.Second)
	if ok || v != 0 || calls != 1 {
		t.Fatal("lock failure probed", v, ok, calls)
	}
}
