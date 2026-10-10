package router

import (
	"errors"
	"math"
	"reflect"
	"runtime"
	"testing"
)

func TestIntervalIdle(t *testing.T) {
	for _, tc := range []struct {
		name, out string
		idle      float64
		ok        bool
	}{
		{"interval not boot", "cpu\nus sy id\n1 1 98\n80 15 5\n", 5, true},
		{"repeated header", "cpu\nus sy id\n1 1 98\ncpu\nus sy id\n80 15 5\n", 5, true},
		{"third row rejected", "cpu\nus sy id\n1 1 98\n80 15 5\n1 1 98\n", 0, false},
		{"idle", "cpu\nus sy id\n80 15 5\n0 0 100\n", 100, true},
		{"one row", "cpu\nus sy id\n1 1 98\n", 0, false},
		{"wrong header", "cpu\nid us sy\n1 1 98\n1 1 98\n", 0, false},
		{"nan", "cpu\nus sy id\n1 1 98\nNaN 0 100\n", 0, false},
		{"inf", "cpu\nus sy id\n1 1 98\n0 0 +Inf\n", 0, false},
		{"negative", "cpu\nus sy id\n1 1 98\n-1 1 100\n", 0, false},
		{"over100", "cpu\nus sy id\n1 1 98\n0 0 101\n", 0, false},
		{"bad sum", "cpu\nus sy id\n1 1 98\n10 10 10\n", 0, false},
		{"extra columns", "cpu\nus sy id\n1 1 98\n1 1 98 0\n", 0, false},
		{"empty", "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, ok := parseIntervalIdle(tc.out)
			if ok != tc.ok || (ok && v != tc.idle) {
				t.Fatalf("got %v %v", v, ok)
			}
		})
	}
}

func TestProbeFallbackAndAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, sample, fallback string
		fail                   bool
		want                   float64
		ok                     bool
		calls                  int
	}{
		{"saturated", "cpu\nus sy id\n0 0 100\n90 5 5\n", "", false, 9.5, true, 1},
		{"free", "cpu\nus sy id\n90 5 5\n0 0 100\n", "", false, 0, true, 1},
		{"threshold", "cpu\nus sy id\n0 0 100\n90 0 10\n", "", false, 9, true, 1},
		{"command failure fallback", "", "{ 20 10 5 }", true, 10, true, 2},
		{"truncated fallback", "cpu\nus sy id\n0 0 100\n", "{ 4 3 2 }", false, 2, true, 2},
		{"unknown", "", "bad", true, 0, false, 2},
		{"nan fallback", "", "{ NaN 1 1 }", true, 0, false, 2},
		{"negative fallback", "", "{ -2 1 1 }", true, 0, false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			v, ok := probeLoadWith(func(name string, args ...string) ([]byte, error) {
				calls++
				if calls == 1 {
					if name != "/usr/sbin/iostat" || !reflect.DeepEqual(args, []string{"-d", "-C", "-n", "0", "-c", "2", "-w", "1"}) {
						t.Fatal("wrong native invocation")
					}
					if tc.fail {
						return nil, errors.New("denied")
					}
					return []byte(tc.sample), nil
				}
				if name != "/usr/sbin/sysctl" {
					t.Fatal(name)
				}
				return []byte(tc.fallback), nil
			}, 10)
			if ok != tc.ok || math.Abs(v-tc.want) > 1e-9 || calls != tc.calls {
				t.Fatalf("got %v %v calls%d", v, ok, calls)
			}
			if ok {
				SetLoadAvgFn(func() (float64, bool) { return v * float64(runtime.NumCPU()) / 10, true })
				defer SetLoadAvgFn(nil)
				hold, load, cores := shouldDeferDispatch()
				if hold != (tc.want >= 9) || cores != runtime.NumCPU() || math.Abs(load-v*float64(cores)/10) > 1e-9 {
					t.Fatal("pressure gate changed")
				}
			}
		})
	}
}
