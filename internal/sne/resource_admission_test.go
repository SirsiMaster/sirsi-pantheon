package sne

import (
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

func safeResourceSample() resourceSample {
	return resourceSample{
		capacity: guard.NodeCapacity{
			TotalRAM: 48 << 30, FreeRAM: 32 << 30, OSBaseline: 1 << 30,
			Pressure: guard.PressureNormal, PressureSource: "kernel-dispatch",
		},
		death: guard.MemoryDeath{SwapReadable: true, Cores: 10, AvailableGB: 32},
	}
}

func TestResourceAdmissionErrorsCarryStableRecovery(t *testing.T) {
	sample := safeResourceSample()
	sample.death.SwapUsedGB = 4
	_, err := assessResourceAdmission(sample, 14<<30, true)
	var admissionErr *ResourceAdmissionError
	if !errors.As(err, &admissionErr) {
		t.Fatalf("error type = %T, want ResourceAdmissionError", err)
	}
	if admissionErr.Code != "swap_cleanup_required" || admissionErr.Recovery == "" {
		t.Fatalf("admission error = %+v", admissionErr)
	}
}

func TestResourceAdmissionAcceptsMeasuredSNEV2Footprint(t *testing.T) {
	got, err := assessResourceAdmission(safeResourceSample(), 14<<30, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequiredBytes != 14<<30 || got.DynamicReserve != 9<<30 {
		t.Fatalf("resource admission = %+v", got)
	}
}

func TestHostMemoryCeilingRejectsImpossibleCopiedProfile(t *testing.T) {
	admission := ResourceAdmission{TotalRAMBytes: 16 << 30}
	err := ValidateHostMemoryCeiling(40<<30, admission)
	var admissionErr *ResourceAdmissionError
	if !errors.As(err, &admissionErr) {
		t.Fatalf("error type = %T, want ResourceAdmissionError", err)
	}
	if admissionErr.Code != "memory_ceiling_exceeds_host_capacity" || admissionErr.Recovery == "" {
		t.Fatalf("admission error = %+v", admissionErr)
	}
}

func TestHostMemoryCeilingAcceptsHostBoundAndOptionalPolicy(t *testing.T) {
	admission := ResourceAdmission{TotalRAMBytes: 16 << 30}
	for _, ceiling := range []uint64{0, 12 << 30, 16 << 30} {
		if err := ValidateHostMemoryCeiling(ceiling, admission); err != nil {
			t.Fatalf("ceiling %d rejected: %v", ceiling, err)
		}
	}
}

func TestResourceAdmissionRejectsSwapPressureAndMissingHeadroom(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*resourceSample)
		want   string
	}{
		{"unreadable-swap", func(s *resourceSample) { s.death.SwapReadable = false }, "cannot read live swap"},
		{"swap", func(s *resourceSample) { s.death.SwapUsedGB = 4 }, "requires swap cleanup"},
		{"pressure", func(s *resourceSample) { s.capacity.Pressure = guard.PressureWarn }, "yielded to warn"},
		{"headroom", func(s *resourceSample) { s.capacity.FreeRAM = 20 << 30 }, "lacks headroom"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sample := safeResourceSample()
			test.mutate(&sample)
			if _, err := assessResourceAdmission(sample, 14<<30, true); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResourceAdmissionAllowsWarnWhenForegroundYieldDisabled(t *testing.T) {
	sample := safeResourceSample()
	sample.capacity.Pressure = guard.PressureWarn
	if _, err := assessResourceAdmission(sample, 14<<30, false); err != nil {
		t.Fatal(err)
	}
}

func TestResourceAdmissionDoesNotDoubleCountResidentAgents(t *testing.T) {
	sample := safeResourceSample()
	sample.capacity.AgentRSS = 4 << 30
	got, err := assessResourceAdmission(sample, 14<<30, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.DynamicReserve != 9<<30 {
		t.Fatalf("dynamic reserve = %d, want agent-independent live reserve %d", got.DynamicReserve, int64(9)<<30)
	}
}

func TestRestartResourceAdmissionProtectsForegroundFromReloadChurn(t *testing.T) {
	sample := safeResourceSample()
	sample.capacity.FreeRAM = 26 << 30
	if _, err := assessResourceAdmission(sample, 14<<30, true); err != nil {
		t.Fatalf("ordinary launch should fit: %v", err)
	}
	restartReserve := uint64(sample.capacity.TotalRAM / 12)
	got, err := assessResourceAdmissionWithLifecycleReserve(sample, 14<<30, restartReserve, true)
	if err == nil || !strings.Contains(err.Error(), "lifecycle reserve") {
		t.Fatalf("restart admission error = %v, want lifecycle reserve rejection", err)
	}
	if got.LifecycleReserve != 4<<30 {
		t.Fatalf("lifecycle reserve = %d, want %d", got.LifecycleReserve, uint64(4)<<30)
	}
}
