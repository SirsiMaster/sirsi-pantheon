package guard

import (
	"runtime"

	"github.com/SirsiMaster/sirsi-pantheon/internal/hapi"
)

// ResourceStats contains memory and system metrics for the Anubis dashboard.
type ResourceStats struct {
	UsedMemory    string
	TotalMemory   string
	PressureLevel string
}

// GetStats returns the current workstation resource utilization.
func GetStats() (*ResourceStats, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	profile, _ := hapi.DetectHardware()

	return &ResourceStats{
		UsedMemory:    hapi.FormatBytes(int64(m.Alloc)),
		TotalMemory:   hapi.FormatBytes(profile.TotalRAM),
		PressureLevel: "Normal", // Simplified for now
	}, nil
}
