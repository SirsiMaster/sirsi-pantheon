// Package selfupdate — lock.go
//
// InstallLock is the cross-platform token returned by AcquireInstallLock.
package selfupdate

// InstallLock is the token returned by AcquireInstallLock. Close releases the
// underlying primitive and, on Unix, truncates the PID so shell's acquire_lock()
// empty-PID reap branch cleans up the file on the next acquire.
type InstallLock struct {
	closer func() error
}

// Close releases the install lock.
func (l *InstallLock) Close() error {
	if l.closer != nil {
		return l.closer()
	}
	return nil
}
