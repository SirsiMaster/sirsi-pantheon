//go:build windows

// Package router — admission_windows.go
//
// Stub for platforms without flock(2) — this repo is Mac-first (ADR-032) and
// RunWakeLoop/the informer are not built for Windows yet. Returns a no-op
// unlock so admitConsumer still compiles; concurrent admission on Windows is
// not serialized by this mechanism. Add a real implementation (LockFileEx)
// if Windows support becomes a priority.
package router

func lockConsumerAdmission(path string) (unlock func(), err error) {
	return func() {}, nil
}
