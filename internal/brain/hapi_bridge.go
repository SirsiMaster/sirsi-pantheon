package brain

import (
	"github.com/SirsiMaster/sirsi-pantheon/internal/hapi"
	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
)

// HapiBridge connects accelerator detection (Hapi) to the inference layer (Brain).
type HapiBridge struct {
	profile *hapi.HardwareProfile
}

// NewHapiBridge initializes hardware detection and returns a bridge instance.
func NewHapiBridge() (*HapiBridge, error) {
	profile, err := hapi.DetectHardware()
	if err != nil {
		logging.Error("🧠 Brain: Hapi detection failed", "error", err)
		return nil, err
	}

	logging.Info("🧠 Brain: Accelerator detected",
		"type", profile.GPU.Type,
		"vram", profile.GPU.VRAM,
		"cores", profile.CPUCores)

	return &HapiBridge{profile: profile}, nil
}

// BackendPreference returns the optimal inference backend based on hardware.
func (b *HapiBridge) BackendPreference() string {
	if b.profile == nil {
		return "stub"
	}

	switch b.profile.GPU.Type {
	case hapi.GPUAppleMetal:
		return "coreml"
	case hapi.GPUNVIDIA:
		return "onnx-cuda"
	case hapi.GPUAMD:
		return "onnx-rocm"
	default:
		return "onnx-cpu"
	}
}

// Profile returns the underlying hardware profile.
func (b *HapiBridge) Profile() *hapi.HardwareProfile {
	return b.profile
}
