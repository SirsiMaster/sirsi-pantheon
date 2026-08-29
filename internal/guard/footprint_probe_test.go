package guard

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// TestFootprintProbesAreCapturedAsOneCensusPair protects the A21 test seam:
// synthetic process rows must use injected measurements rather than querying
// a real PID that happens to share the fixture identifier.
func TestFootprintProbesAreCapturedAsOneCensusPair(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	physicalCalls := 0
	peakCalls := 0
	originalPhysical, originalPeak := getFootprintProbes()
	setFootprintProbes(
		func(pid int) (uint64, error) {
			physicalCalls++
			if pid != 42 {
				t.Fatalf("physical probe called for unexpected pid %d", pid)
			}
			return 123, nil
		},
		func(pid int) (uint64, error) {
			peakCalls++
			if pid != 42 {
				t.Fatalf("peak probe called for unexpected pid %d", pid)
			}
			return 456, nil
		},
	)
	t.Cleanup(func() { setFootprintProbes(originalPhysical, originalPeak) })

	mock := &platform.Mock{CommandResults: map[string]string{
		"ps -axo pid,rss,vsz,%cpu,user,comm": "PID RSS VSZ %CPU USER COMM\n42 100 200 3.5 me /usr/bin/demo\n",
	}}
	processes, err := getProcessListWith(mock)
	if err != nil {
		t.Fatal(err)
	}
	if len(processes) != 1 {
		t.Fatalf("got %d processes, want one", len(processes))
	}
	if processes[0].Footprint != 123 || processes[0].PeakFootprint != 456 {
		t.Fatalf("got footprint=%d peak=%d, want 123/456", processes[0].Footprint, processes[0].PeakFootprint)
	}
	if physicalCalls != 1 || peakCalls != 1 {
		t.Fatalf("probe calls=%d/%d, want one call per probe", physicalCalls, peakCalls)
	}
}
