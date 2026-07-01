package guard

import (
	"github.com/SirsiMaster/sirsi-pantheon/internal/seba"
)

// ResourceStats contains memory and system metrics for the Anubis dashboard.
type ResourceStats struct {
	UsedMemory    string
	TotalMemory   string
	PressureLevel string
}

// GetStats returns the current workstation resource utilization. It reports the
// SYSTEM's used RAM and live pressure — NOT this CLI's own Go heap. (The old
// implementation used runtime.MemStats.Alloc, which is sirsi's heap (~MB), so the
// dashboard read "RAM: 2.8 MB / 48.0 GB" — a meaningless self-measurement — and
// hardcoded "Normal" pressure regardless of reality.)
func GetStats() (*ResourceStats, error) {
	nc := SampleNodeCapacity()
	used := nc.TotalRAM - nc.FreeRAM
	if used < 0 {
		used = 0
	}
	pressure := "Normal"
	switch nc.Pressure {
	case PressureWarn:
		pressure = "Warning"
	case PressureCritical:
		pressure = "Critical"
	}
	return &ResourceStats{
		UsedMemory:    seba.FormatBytes(used),
		TotalMemory:   seba.FormatBytes(nc.TotalRAM),
		PressureLevel: pressure,
	}, nil
}
