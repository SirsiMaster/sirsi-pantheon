package guard

import (
	"errors"
	"testing"
)

// These fixtures replace the existing raw-probe seams and are not parallel.
func TestSampleMemorySummaryNeverRanksProcesses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	total, vm, ps := hapiTotalRAMFn, hapiVMStatFn, hapiPsFn
	footprint := getHapiFootprintFn()
	ranked := hapiSampleFn
	t.Cleanup(func() {
		hapiTotalRAMFn, hapiVMStatFn, hapiPsFn = total, vm, ps
		setHapiFootprintFn(footprint)
		setHapiSampleFn(ranked)
	})
	hapiTotalRAMFn = func() int64 { return 48 << 30 }
	hapiVMStatFn = func() ([]byte, error) {
		return []byte("Mach Virtual Memory Statistics: (page size of 4096 bytes)\nPages free: 100.\nPages inactive: 200.\nPages speculative: 50.\n"), nil
	}
	hapiPsFn = func() ([]byte, error) { t.Fatal("summary enumerated processes"); return nil, nil }
	setHapiFootprintFn(func(int) (uint64, error) { t.Fatal("summary read a process footprint"); return 0, nil })
	setHapiSampleFn(func() (MemSample, error) { t.Fatal("summary invoked the ranked sampler"); return MemSample{}, nil })
	sample, err := SampleMemorySummary()
	if err != nil || sample.TotalRAM != 48<<30 || sample.FreeBytes != 350*4096 {
		t.Fatalf("summary=%+v err=%v", sample, err)
	}
	if sample.Top == nil || len(sample.Top) != 0 || sample.FreePercent != float64(sample.FreeBytes)/float64(sample.TotalRAM)*100 {
		t.Fatalf("summary must retain capacity math and empty top: %+v", sample)
	}
}

func TestSampleMemorySummaryPreservesFreeProbeFailureSemantics(t *testing.T) {
	total, vm := hapiTotalRAMFn, hapiVMStatFn
	t.Cleanup(func() { hapiTotalRAMFn, hapiVMStatFn = total, vm })
	hapiTotalRAMFn = func() int64 { return 48 << 30 }
	hapiVMStatFn = func() ([]byte, error) { return nil, errors.New("inert probe failure") }
	sample, err := SampleMemorySummary()
	if err != nil || sample.TotalRAM != 48<<30 || sample.FreeBytes != 0 || len(sample.Top) != 0 {
		t.Fatalf("existing unavailable-free semantics changed: %+v %v", sample, err)
	}
}
