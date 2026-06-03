//go:build !darwin

package platform

// Disk-access tiers are macOS-specific (TCC). Elsewhere there is no equivalent
// gate, so report full visibility (access is governed by the OS/user directly).

// CheckDiskAccess returns full access on non-macOS platforms.
func CheckDiskAccess() DiskAccess { return DiskAccess{Level: AccessFull} }

// HasFullDiskAccess returns true on non-macOS platforms.
func HasFullDiskAccess() bool { return true }
