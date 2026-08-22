package sne

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

func TestSampleResourceStateDoesNotPretendToAdmitAModel(t *testing.T) {
	original := sampleSNEResourcesFn
	t.Cleanup(func() { sampleSNEResourcesFn = original })
	sampleSNEResourcesFn = func() resourceSample {
		return resourceSample{
			capacity: guard.NodeCapacity{TotalRAM: 48 << 30, FreeRAM: 20 << 30, AgentRSS: 2 << 30, PressureSource: "test"},
			death:    guard.MemoryDeath{SwapReadable: true, SwapUsedGB: 1.25},
		}
	}
	state := SampleResourceState()
	if state.RequiredBytes != 0 || state.TotalRAMBytes != 48<<30 || state.AvailableRAMBytes != 20<<30 || state.SwapUsedBytes == 0 || state.PressureSource != "test" {
		t.Fatalf("unexpected resource snapshot: %+v", state)
	}
}
