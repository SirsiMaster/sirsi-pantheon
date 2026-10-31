//go:build darwin || linux

// Package packagedengine owns the exact installed-engine identity used by
// setup and Ma'at. It intentionally has no dependency on either package.
package packagedengine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const maxCodesignOutputBytes = 16 << 10

const (
	appEnginePath     = "/Applications/Pantheon.app/Contents/MacOS/sirsi"
	libexecEnginePath = "/usr/local/libexec/sirsi-pantheon/sirsi"
)

// IsCanonical reports whether path is one of the two exact shipped engine
// locations. It accepts no PATH, symlink, or loose-binary fallback.
func IsCanonical(path string) bool {
	clean, err := filepath.Abs(path)
	if err != nil || clean != path || filepath.Clean(path) != path {
		return false
	}
	return path == appEnginePath || path == libexecEnginePath
}

// Current returns the running executable only after resolving its path to an
// exact packaged location and validating the opened no-follow file identity.
func Current() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", errors.New("maat pod service: current executable path unavailable")
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil || !IsCanonical(resolved) {
		return "", errors.New("maat pod service: run prepare from the installed Pantheon app or canonical libexec sirsi")
	}
	if err := ValidateCurrent(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

// ValidateCurrent checks the canonical executable through an O_NOFOLLOW path
// walk, then confirms the name still identifies the same regular executable.
func ValidateCurrent(path string) error {
	if !IsCanonical(path) {
		return errors.New("maat pod service: current executable is not an accepted installed sirsi engine")
	}
	f, info, err := OpenCanonicalPath(path, false, false)
	if f != nil {
		defer f.Close()
	}
	if err != nil || info == nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("maat pod service: accepted installed sirsi engine is unavailable or not executable")
	}
	nameInfo, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, nameInfo) || !nameInfo.Mode().IsRegular() {
		return errors.New("maat pod service: accepted installed sirsi engine path changed during inspection")
	}
	return nil
}

// Verify checks the exact packaged-engine signature without executing the
// candidate payload. Output and runtime remain bounded.
func Verify(path string) error {
	if !IsCanonical(path) {
		return errors.New("maat pod service: executable is not an accepted installed sirsi engine")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := VerificationCommand(ctx, path)
	output := &boundedOutput{limit: maxCodesignOutputBytes}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Run(); err != nil || output.overflow {
		return errors.New("maat pod service: installed engine signature did not meet the Sirsi requirement")
	}
	return nil
}

// VerificationCommand returns the canonical fixed codesign invocation. The
// caller cannot select a binary, requirement, or verifier executable.
func VerificationCommand(ctx context.Context, path string) *exec.Cmd {
	const requirement = `=anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" and identifier "sirsi"`
	return exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--strict", "-R", requirement, path)
}

// OpenCanonicalPath opens every path component without following symlinks.
// It is shared with setup so the packaged-engine check and setup callers keep
// the same no-follow implementation.
func OpenCanonicalPath(path string, directory, allowMissing bool) (*os.File, os.FileInfo, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return nil, nil, errors.New("path must be canonical and absolute")
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	parent, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	for i, component := range parts {
		leaf := i == len(parts)-1
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if !leaf || directory {
			flags |= unix.O_DIRECTORY
		}
		fd, openErr := unix.Openat(parent, component, flags, 0)
		if openErr != nil {
			unix.Close(parent)
			if leaf && allowMissing && errors.Is(openErr, unix.ENOENT) {
				return nil, nil, nil
			}
			return nil, nil, openErr
		}
		unix.Close(parent)
		parent = fd
	}
	file := os.NewFile(uintptr(parent), path)
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

type boundedOutput struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = b.Buffer.Write(p[:remaining])
			b.overflow = true
		} else {
			_, _ = b.Buffer.Write(p)
		}
	} else if n > 0 {
		b.overflow = true
	}
	return n, nil
}
