//go:build darwin || linux

package completionproof

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func openDirectory(path string) (int, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return -1, errors.New("directory must be normalized and absolute")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if part == "" {
			continue
		}
		next, openErr := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			return -1, openErr
		}
		fd = next
	}
	return fd, nil
}

func openCompletionProof(root, relative string) (*os.File, error) {
	if filepath.IsAbs(relative) || filepath.Clean(relative) != relative || relative == "." {
		return nil, errors.New("unsafe relative proof path")
	}
	for _, part := range strings.Split(relative, "/") {
		if part == ".." || part == "" {
			return nil, errors.New("unsafe proof path component")
		}
	}
	parent, err := openDirectory(root)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(relative, "/")
	for _, part := range parts[:len(parts)-1] {
		next, openErr := unix.Openat(parent, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(parent)
		if openErr != nil {
			return nil, openErr
		}
		parent = next
	}
	fd, err := unix.Openat(parent, parts[len(parts)-1], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	_ = unix.Close(parent)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), filepath.Join(root, relative))
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("adopt completion proof descriptor")
	}
	return file, nil
}

func exportValidator(path string, contents []byte) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("explicit export path required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve export path: %w", err)
	}
	parent, err := openDirectory(filepath.Dir(absolute))
	if err != nil {
		return fmt.Errorf("open export directory (symlinks refused): %w", err)
	}
	defer unix.Close(parent)
	fd, err := unix.Openat(parent, filepath.Base(absolute), unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return fmt.Errorf("create absent validator export: %w", err)
	}
	f := os.NewFile(uintptr(fd), absolute)
	if f == nil {
		_ = unix.Close(fd)
		return errors.New("adopt export descriptor")
	}
	_, writeErr := f.Write(contents)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	// On I/O failure leave the newly-created partial file for explicit operator
	// inspection, never unlink a pathname that another writer may have replaced.
	if writeErr != nil {
		return fmt.Errorf("validator export incomplete; inspect destination: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("validator export close failed: %w", closeErr)
	}
	return nil
}
