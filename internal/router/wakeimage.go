package router

import (
	"os"
	"path/filepath"
)

type wakeImageIdentity struct{ device, inode uint64 }

// wakeExecutable watches the installer-owned executable name, not a version
// string or a renamed old image. Installation must replace that regular file
// atomically; the installer remains responsible for signature verification.
type wakeExecutable struct {
	path   string
	loaded wakeImageIdentity
}

func watchWakeExecutable(path string, loaded wakeImageIdentity) (*wakeExecutable, error) {
	if !filepath.IsAbs(path) || loaded.inode == 0 {
		return nil, os.ErrInvalid
	}
	return &wakeExecutable{path: path, loaded: loaded}, nil
}

func (w *wakeExecutable) replaced() bool {
	if w == nil {
		return false
	}
	info, err := os.Lstat(w.path)
	// A partial/failed installation must not evict the running generation.
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return false
	}
	identity, err := wakeFileIdentity(info)
	return err == nil && identity != w.loaded
}

func wakeUpgradeReady(replaced, consumerRunning bool) bool {
	return replaced && !consumerRunning
}
