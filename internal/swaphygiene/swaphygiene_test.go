package swaphygiene

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func fake(swap, vm, mp string) Runner {
	return func(name string, args ...string) (string, error) {
		switch name {
		case "sysctl":
			if swap == "" {
				return "", errors.New("denied")
			}
			return swap, nil
		case "vm_stat":
			return vm, nil
		case "memory_pressure":
			return mp, nil
		}
		return "", errors.New("unexpected " + name)
	}
}

func vm(in, out int) string {
	return fmt.Sprintf("Mach Virtual Memory Statistics: (page size of 16384 bytes)\nSwapins:                                       %d.\nSwapouts:                                      %d.\n", in, out)
}

const (
	swapIdle = "total = 6144.00M  used = 55.19M  free = 6088.81M  (encrypted)"
	swapNone = "total = 6144.00M  used = 0.00M  free = 6144.00M  (encrypted)"
	mp80     = "System-wide memory free percentage: 80%"
	mp10     = "System-wide memory free percentage: 10%"
)

func assess(swap, vmA, vmB, mpB string) Receipt {
	t0 := time.Unix(1000, 0)
	a := Take(fake(swap, vmA, mp80), t0)
	b := Take(fake(swap, vmB, mpB), t0.Add(time.Hour))
	return Assess(&a, b)
}

// The verdict comes from paging movement, never from a nonzero allocation (both directions).
func TestAssessDistinguishesIdleAllocationFromPaging(t *testing.T) {
	if r := assess(swapIdle, vm(100, 100), vm(100, 100), mp80); r.Verdict != VerdictIdleAllocation || r.ReleaseTimingOK || !r.CorrectnessOnlyOK {
		t.Fatalf("55 MiB allocated, no movement: %+v", r)
	}
	if r := assess(swapIdle, vm(100, 100), vm(900, 400), mp80); r.Verdict != VerdictActivePaging || r.RestartProposed {
		t.Fatalf("paging with plenty of free memory: %+v", r)
	}
	if r := assess(swapIdle, vm(100, 100), vm(900, 400), mp10); r.Verdict != VerdictPressure || !r.RestartProposed || r.CorrectnessOnlyOK {
		t.Fatalf("paging under pressure must propose (not trigger) a restart: %+v", r)
	}
	if r := assess(swapNone, vm(1, 1), vm(1, 1), mp80); r.Verdict != VerdictClean || !r.ReleaseTimingOK {
		t.Fatalf("clean host: %+v", r)
	}
}

// Missing telemetry is reported as unknown; a single sample cannot claim paging;
// a counter reset is not movement.
func TestAssessNeverInventsAHealthyOrSickReading(t *testing.T) {
	s := Take(fake("", vm(1, 1), mp80), time.Unix(1, 0))
	if r := Assess(nil, s); r.Verdict != VerdictUnknown || r.CorrectnessOnlyOK {
		t.Fatalf("no swap telemetry: %+v", r)
	}
	one := Take(fake(swapIdle, vm(500, 500), mp80), time.Unix(1, 0))
	if r := Assess(nil, one); r.Verdict != VerdictIdleAllocation || r.DeltaSwapPages != -1 {
		t.Fatalf("one sample must not claim paging: %+v", r)
	}
	later := Take(fake(swapIdle, vm(5, 5), mp80), time.Unix(9, 0)) // counters went down: reboot
	if r := Assess(&one, later); r.DeltaSwapPages != -1 || r.Verdict == VerdictActivePaging {
		t.Fatalf("counter reset read as paging: %+v", r)
	}
}

func TestRecordPersistsReceiptsAndUsesThePreviousSample(t *testing.T) {
	home := t.TempDir()
	if _, err := Record(home, fake(swapIdle, vm(100, 100), mp80), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	r, err := Record(home, fake(swapIdle, vm(1000, 100), mp80), time.Unix(2, 0))
	if err != nil || r.Verdict != VerdictActivePaging || r.DeltaSwapPages != 900 {
		t.Fatalf("second record: %+v err=%v", r, err)
	}
	if last, err := Last(home); err != nil || last.Verdict != VerdictActivePaging {
		t.Fatalf("last.json: %+v err=%v", last, err)
	}
}
