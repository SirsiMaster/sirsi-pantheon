package router

import "os"

// wakeExecutable watches the installer-owned executable name, not a version
// string or a renamed old image. Installation must replace that regular file
// atomically; the installer remains responsible for signature verification.
type wakeExecutable struct {
	path    string
	initial os.FileInfo
}

func watchWakeExecutable(path string) (*wakeExecutable, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return nil, os.ErrInvalid
	}
	return &wakeExecutable{path: path, initial: info}, nil
}

func (w *wakeExecutable) replaced() bool {
	if w == nil {
		return false
	}
	info, err := os.Lstat(w.path)
	// A partial/failed installation must not evict the running generation.
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 &&
		!os.SameFile(w.initial, info)
}

func wakeUpgradeReady(replaced, consumerRunning bool) bool {
	return replaced && !consumerRunning
}
