// Package selfupdate — lock.go
//
// InstallLock is the cross-platform token returned by AcquireInstallLock.
package selfupdate

// InstallLock is the token returned by AcquireInstallLock. Close releases the
// underlying primitive and, on Unix, removes the lock file so a subsequent
// shell-side acquire_lock() does not find a stale file-shaped lock and wedge.
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
