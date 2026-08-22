package sne

import (
	"fmt"
	"math"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

const minimumSwapHeadroomBytes = uint64(2) << 30

// ResourceAdmission is the measured, per-launch decision Pantheon makes before
// SNE is allowed to allocate unified memory. It is operational evidence, not a
// model-performance claim.
type ResourceAdmission struct {
	RequiredBytes     uint64 `json:"required_bytes"`
	TotalRAMBytes     uint64 `json:"total_ram_bytes"`
	AvailableRAMBytes uint64 `json:"available_ram_bytes"`
	DynamicReserve    uint64 `json:"dynamic_reserve_bytes"`
	LifecycleReserve  uint64 `json:"lifecycle_reserve_bytes"`
	SwapUsedBytes     uint64 `json:"swap_used_bytes"`
	SwapLimitBytes    uint64 `json:"swap_limit_bytes"`
	Pressure          string `json:"pressure"`
	PressureSource    string `json:"pressure_source"`
	YieldToForeground bool   `json:"yield_to_foreground"`
}

// ResourceAdmissionError is safe for CLI, UI, Nexus, and support diagnostics.
// It contains no process arguments, paths, prompts, credentials, or model data.
type ResourceAdmissionError struct {
	Code     string
	Message  string
	Recovery string
}

func (e *ResourceAdmissionError) Error() string { return e.Message }

func resourceAdmissionError(code, message, recovery string) error {
	return &ResourceAdmissionError{Code: code, Message: message, Recovery: recovery}
}

type resourceSample struct {
	capacity guard.NodeCapacity
	death    guard.MemoryDeath
}

// SampleResourceState returns Pantheon's current caretaker measurements without
// making a launch decision. It is intended for diagnostics and operator UI;
// launch admission must still call EvaluateResourceAdmission with the exact
// measured model footprint.
func SampleResourceState() ResourceAdmission {
	sample := sampleSNEResourcesFn()
	capacity := sample.capacity
	death := sample.death
	liveReserve := capacity.DynamicReserve() - capacity.AgentRSS
	if liveReserve < 0 {
		liveReserve = 0
	}
	result := ResourceAdmission{
		TotalRAMBytes:     positiveUint64(capacity.TotalRAM),
		AvailableRAMBytes: positiveUint64(capacity.FreeRAM),
		DynamicReserve:    positiveUint64(liveReserve),
		Pressure:          capacity.Pressure.String(),
		PressureSource:    capacity.PressureSource,
	}
	if death.SwapReadable {
		result.SwapUsedBytes = uint64(math.Round(death.SwapUsedGB * float64(uint64(1)<<30)))
	}
	result.SwapLimitBytes = result.TotalRAMBytes / 16
	if result.SwapLimitBytes < minimumSwapHeadroomBytes {
		result.SwapLimitBytes = minimumSwapHeadroomBytes
	}
	return result
}

var sampleSNEResourcesFn = func() resourceSample {
	return resourceSample{capacity: guard.SampleNodeCapacity(), death: guard.SampleMemoryDeath()}
}

// EvaluateResourceAdmission samples this Mac and returns Pantheon's current
// launch decision without starting SNE. Surfaces use it for a truthful preview;
// Supervisor resamples again at the actual launch boundary.
func EvaluateResourceAdmission(required uint64, yieldToForeground bool) (ResourceAdmission, error) {
	return assessResourceAdmission(sampleSNEResourcesFn(), required, yieldToForeground)
}

// ValidateHostMemoryCeiling rejects stale or copied profiles whose process
// ceiling is physically impossible on this Mac. Live launch headroom remains a
// separate decision because available RAM, reserves, pressure, and swap vary.
func ValidateHostMemoryCeiling(ceiling uint64, admission ResourceAdmission) error {
	if ceiling == 0 {
		return nil
	}
	if admission.TotalRAMBytes == 0 {
		return resourceAdmissionError("ram_measurement_unavailable", "SNE memory-ceiling validation cannot read live RAM capacity", "Retry after Pantheon regains access to macOS memory telemetry.")
	}
	if ceiling > admission.TotalRAMBytes {
		return resourceAdmissionError("memory_ceiling_exceeds_host_capacity", fmt.Sprintf("SNE profile memory ceiling %d exceeds host physical RAM %d", ceiling, admission.TotalRAMBytes), "Select a device-qualified SNE profile whose memory ceiling fits this Mac, then retry.")
	}
	return nil
}

func assessResourceAdmission(sample resourceSample, required uint64, yieldToForeground bool) (ResourceAdmission, error) {
	return assessResourceAdmissionWithLifecycleReserve(sample, required, 0, yieldToForeground)
}

// EvaluateRestartResourceAdmission adds node-relative headroom for the transient
// working-set churn of a fresh SNE process after crash, reload, or supervised
// restart. It does not charge a second model copy because Pantheon reaps the old
// PID before launch; the reserve protects foreground pages from being displaced
// while the replacement process faults the model back into unified memory.
func EvaluateRestartResourceAdmission(required uint64, yieldToForeground bool) (ResourceAdmission, error) {
	sample := sampleSNEResourcesFn()
	lifecycleReserve := positiveUint64(sample.capacity.TotalRAM / 12)
	return assessResourceAdmissionWithLifecycleReserve(sample, required, lifecycleReserve, yieldToForeground)
}

func assessResourceAdmissionWithLifecycleReserve(sample resourceSample, required, lifecycleReserve uint64, yieldToForeground bool) (ResourceAdmission, error) {
	capacity := sample.capacity
	death := sample.death
	// FreeRAM is sampled after live agents have consumed their resident memory.
	// DynamicReserve includes AgentRSS for older capacity callers, so subtract it
	// here to avoid charging the same resident pages twice. The OS baseline and
	// node-scaled margin remain reserved for foreground growth and reclamation.
	liveReserve := capacity.DynamicReserve() - capacity.AgentRSS
	if liveReserve < 0 {
		liveReserve = 0
	}
	result := ResourceAdmission{
		RequiredBytes:     required,
		TotalRAMBytes:     positiveUint64(capacity.TotalRAM),
		AvailableRAMBytes: positiveUint64(capacity.FreeRAM),
		DynamicReserve:    positiveUint64(liveReserve),
		LifecycleReserve:  lifecycleReserve,
		Pressure:          capacity.Pressure.String(),
		PressureSource:    capacity.PressureSource,
		YieldToForeground: yieldToForeground,
	}
	if death.SwapReadable {
		result.SwapUsedBytes = uint64(math.Round(death.SwapUsedGB * float64(uint64(1)<<30)))
	}
	result.SwapLimitBytes = result.TotalRAMBytes / 16
	if result.SwapLimitBytes < minimumSwapHeadroomBytes {
		result.SwapLimitBytes = minimumSwapHeadroomBytes
	}

	if required == 0 {
		return result, resourceAdmissionError("measured_footprint_required", "SNE resource admission requires a measured model footprint", "Repair or reinstall the signed model package; Pantheon will not guess its memory requirement.")
	}
	if result.TotalRAMBytes == 0 || result.AvailableRAMBytes == 0 {
		return result, resourceAdmissionError("ram_measurement_unavailable", "SNE resource admission cannot read live RAM capacity", "Retry after Pantheon regains access to macOS memory telemetry.")
	}
	if !death.SwapReadable {
		return result, resourceAdmissionError("swap_measurement_unavailable", "SNE resource admission cannot read live swap state", "Retry after Pantheon regains access to macOS swap telemetry.")
	}
	if death.Dying {
		return result, resourceAdmissionError("memory_emergency", fmt.Sprintf("SNE resource admission refused a memory-death spiral: swap %.2f GiB, available %.2f GiB, load %.1f/%d cores", death.SwapUsedGB, death.AvailableGB, death.Load1, death.Cores), "Stop unnecessary memory-heavy work or restart the Mac, then retry after pressure and swap recover.")
	}
	if capacity.Pressure == guard.PressureCritical || (yieldToForeground && capacity.Pressure == guard.PressureWarn) {
		return result, resourceAdmissionError("memory_pressure", fmt.Sprintf("SNE resource admission yielded to %s memory pressure (%s)", capacity.Pressure.String(), capacity.PressureSource), "Let foreground work finish or close an unnecessary memory-heavy app, then retry.")
	}
	if result.SwapUsedBytes > result.SwapLimitBytes {
		return result, resourceAdmissionError("swap_cleanup_required", fmt.Sprintf("SNE resource admission requires swap cleanup: used %d bytes exceeds node limit %d", result.SwapUsedBytes, result.SwapLimitBytes), "Do not force another model load. Let Pantheon stop inactive model services; if swap remains high, restart the Mac and retry.")
	}
	if required > ^uint64(0)-result.DynamicReserve || required+result.DynamicReserve > ^uint64(0)-result.LifecycleReserve || required+result.DynamicReserve+result.LifecycleReserve > result.AvailableRAMBytes {
		return result, resourceAdmissionError("memory_headroom_insufficient", fmt.Sprintf("SNE resource admission lacks headroom: model %d + reserve %d + lifecycle reserve %d exceeds available %d", required, result.DynamicReserve, result.LifecycleReserve, result.AvailableRAMBytes), "Finish foreground work, select a smaller admitted model, or retry after more memory becomes available.")
	}
	return result, nil
}

func positiveUint64(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}
