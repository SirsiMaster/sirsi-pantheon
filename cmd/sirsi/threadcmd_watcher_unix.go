//go:build !windows

package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type privateRouterWatcherFiles struct {
	homePath         string
	home, sirsi, dir *os.File
}

// Pin directory descriptors; never follow a .sirsi/router-watchers symlink or
// repair a pre-existing insecure directory in place. HOME is injectable but
// must be an existing actor-owned directory, not writable by group/other.
func openRouterWatcherFiles(home func() (string, error), create bool) (routerWatcherFiles, error) {
	path, err := home()
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || os.Getuid() != os.Geteuid() {
		return nil, errors.New("watcher custody requires absolute clean HOME and matching real/effective user")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open watcher HOME: %w", err)
	}
	f := &privateRouterWatcherFiles{homePath: path, home: os.NewFile(uintptr(fd), path)}
	fail := func(err error) (routerWatcherFiles, error) {
		f.Close()
		return nil, err
	}
	parent := f.home
	for _, leaf := range []string{".sirsi", "router-watchers"} {
		fd := int(parent.Fd())
		if err := validateRouterWatcherDirectory(fd, false); err != nil {
			return fail(err)
		}
		if create {
			if err := unix.Mkdirat(fd, leaf, 0700); err != nil && !errors.Is(err, unix.EEXIST) {
				return fail(err)
			}
		}
		next, openErr := unix.Openat(fd, leaf, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if openErr != nil {
			return fail(fmt.Errorf("open watcher custody directory %s: %w", leaf, openErr))
		}
		parent = os.NewFile(uintptr(next), leaf)
		if leaf == ".sirsi" {
			f.sirsi = parent
		} else {
			f.dir = parent
		}
	}
	if err := f.revalidate(); err != nil {
		return fail(err)
	}
	return f, nil
}

func validateRouterWatcherDirectory(fd int, private bool) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0022 != 0 ||
		(private && stat.Mode&0777 != 0700) {
		return errors.New("watcher directory must be actor-owned, nonsymlink and not group/other writable; custody leaf must be mode0700")
	}
	return nil
}

func (f *privateRouterWatcherFiles) Close() error {
	var err error
	for _, file := range []*os.File{f.dir, f.sirsi, f.home} {
		if file != nil {
			err = errors.Join(err, file.Close())
		}
	}
	return err
}

// Ownership of a detached directory is not custody of the canonical pathname.
// Reopen HOME and compare its actual identity, then check both named child links
// against the retained descriptors without following symlinks. A renamed old
// watcher must self-retire, not keep reading its detached publication forever.
func (f *privateRouterWatcherFiles) revalidate() error {
	if os.Getuid() != os.Geteuid() {
		return errors.New("watcher user identity changed; custody lost")
	}
	fd, err := unix.Open(f.homePath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("reopen canonical watcher HOME: %w", err)
	}
	defer unix.Close(fd)
	if err := validateRouterWatcherDirectory(fd, false); err != nil {
		return err
	}
	if err := validateRouterWatcherDirectory(int(f.home.Fd()), false); err != nil {
		return err
	}
	var retained, current unix.Stat_t
	if err := unix.Fstat(int(f.home.Fd()), &retained); err != nil {
		return err
	}
	if err := unix.Fstat(fd, &current); err != nil {
		return err
	}
	if retained.Dev != current.Dev || retained.Ino != current.Ino {
		return errors.New("canonical watcher HOME replaced; custody lost")
	}
	if err := validateLinkedRouterWatcherDirectory(f.home, ".sirsi", f.sirsi, false); err != nil {
		return err
	}
	return validateLinkedRouterWatcherDirectory(f.sirsi, "router-watchers", f.dir, true)
}

func validateLinkedRouterWatcherDirectory(parent *os.File, leaf string, retained *os.File, private bool) error {
	if err := validateRouterWatcherDirectory(int(retained.Fd()), private); err != nil {
		return err
	}
	var opened, named unix.Stat_t
	if err := unix.Fstat(int(retained.Fd()), &opened); err != nil {
		return err
	}
	if err := unix.Fstatat(int(parent.Fd()), leaf, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("revalidate watcher directory link %s: %w", leaf, err)
	}
	if named.Mode&unix.S_IFMT != unix.S_IFDIR || opened.Dev != named.Dev || opened.Ino != named.Ino {
		return fmt.Errorf("watcher directory link %s replaced; custody lost", leaf)
	}
	return nil
}

func validRouterWatcherLeaf(leaf string) bool {
	return leaf != "" && leaf != "." && leaf != ".." && filepath.Base(leaf) == leaf
}

func (f *privateRouterWatcherFiles) read(leaf string) ([]byte, error) {
	if !validRouterWatcherLeaf(leaf) {
		return nil, errors.New("invalid watcher custody leaf")
	}
	if err := f.revalidate(); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(int(f.dir.Fd()), leaf, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), leaf)
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0777 != 0600 || stat.Nlink != 1 {
		return nil, errors.New("watcher record must be actor-owned, regular mode0600 with one link; retained without signaling")
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return nil, err
	}
	if len(data) > 4096 {
		return nil, errors.New("watcher record exceeds bounded format")
	}
	if err := f.revalidate(); err != nil {
		return nil, err
	}
	return data, nil
}

func (f *privateRouterWatcherFiles) write(leaf string, data []byte) error {
	if !validRouterWatcherLeaf(leaf) || len(data) > 4096 {
		return errors.New("invalid watcher publication")
	}
	if err := f.revalidate(); err != nil {
		return err
	}
	// Refuse an existing symlink, hardlink, device, FIFO or insecure record.
	// Rename below never follows its destination even if the name changes.
	if _, err := f.read(leaf); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var nonce [16]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return err
	}
	tmp := ".publish-" + hex.EncodeToString(nonce[:])
	dirfd := int(f.dir.Fd())
	fd, err := unix.Openat(dirfd, tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), tmp)
	defer file.Close()
	defer unix.Unlinkat(dirfd, tmp, 0) // only our unique temporary name, never a lease
	n, err := file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := f.revalidate(); err != nil {
		return err
	}
	if err := unix.Renameat(dirfd, tmp, dirfd, leaf); err != nil {
		return err
	}
	if err := f.revalidate(); err != nil {
		return err
	}
	return f.dir.Sync()
}
