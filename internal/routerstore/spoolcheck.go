package routerstore

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// CheckSpoolDir fails closed unless spool names a real directory owned by the
// current user, reached through a parent that is itself owned by the current
// user or root and not writable by others, with no symlink at the final
// element. The parent is canonicalized first (macOS mounts /var through a
// symlink), so callers must use the returned canonical path everywhere — that
// is what the LaunchAgent and the relay bind to. The directory is created
// (0700) when absent; a loose mode is tightened. Both the installer and the
// relay itself call it, so a substituted or re-pointed spool is refused at
// install and again at every relay start (ADR-062 20a.4, SSA 2026-09-10).
func CheckSpoolDir(spool string) (string, error) {
	if !filepath.IsAbs(spool) || filepath.Clean(spool) != spool {
		return "", fmt.Errorf("spool: %q must be an absolute, clean path", spool)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(spool))
	if err != nil {
		return "", fmt.Errorf("spool: parent %s: %w", filepath.Dir(spool), err)
	}
	pst, err := os.Stat(parent)
	if err != nil {
		return "", fmt.Errorf("spool: parent %s: %w", parent, err)
	}
	if sys, ok := pst.Sys().(*syscall.Stat_t); ok && int(sys.Uid) != os.Getuid() && sys.Uid != 0 {
		return "", fmt.Errorf("spool: parent %s is owned by uid %d, not %d or root; refusing", parent, sys.Uid, os.Getuid())
	}
	if pst.Mode().Perm()&0o022 != 0 && pst.Mode()&os.ModeSticky == 0 {
		return "", fmt.Errorf("spool: parent %s is writable by others (%o); refusing", parent, pst.Mode().Perm())
	}
	canon := filepath.Join(parent, filepath.Base(spool))
	st, err := os.Lstat(canon)
	switch {
	case os.IsNotExist(err):
		if merr := os.Mkdir(canon, 0o700); merr != nil {
			return "", fmt.Errorf("spool: create: %w", merr)
		}
		st, err = os.Lstat(canon)
		if err != nil {
			return "", err
		}
	case err != nil:
		return "", fmt.Errorf("spool: %w", err)
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("spool: %s is a symlink; refusing", canon)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("spool: %s is not a directory; refusing", canon)
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok && int(sys.Uid) != os.Getuid() {
		return "", fmt.Errorf("spool: %s is owned by uid %d, not %d; refusing", canon, sys.Uid, os.Getuid())
	}
	if st.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(canon, 0o700); err != nil {
			return "", fmt.Errorf("spool: tighten mode: %w", err)
		}
	}
	return canon, nil
}

// CheckServiceURL enforces the service contract for any process that will hold
// the host token: an https URL with a host and nothing else. Plaintext http is
// refused outright — a bearer token must never travel unencrypted — and there
// is no development exception here (tests exercise the relay through the Go
// struct with httptest, never through this gate).
func CheckServiceURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("service URL %q: %w", raw, err)
	}
	if u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("service URL %q: must be https://host[:port][/path] with no credentials (the host token must never travel in plaintext)", raw)
	}
	return strings.TrimRight(u.String(), "/"), nil
}
