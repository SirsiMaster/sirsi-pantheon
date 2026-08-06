//go:build windows

// Package selfupdate — lock_other.go
//
// Stub for platforms without flock(2). Returns nil; concurrent
// self-updates on Windows are not serialized by this mechanism. Add a real
// implementation (LockFileEx) if Windows support becomes a priority.
package selfupdate

// AcquireInstallLock is a no-op on non-unix platforms.
func AcquireInstallLock() (*InstallLock, error) { return nil, nil }
