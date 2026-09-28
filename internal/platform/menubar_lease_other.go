//go:build !darwin

package platform

// TryLockMenubarSurface preserves the previous singleton behavior for the
// historical Go surface on non-macOS platforms. The shipped native menubar is
// macOS-only, so there is no Swift lease to coordinate there.
func TryLockMenubarSurface() (func(), error) {
	return TryLock("menubar")
}
