package routerstore

import (
	"fmt"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
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
//
// This is the single-UID path: exactly the process's own uid, or root on the
// parent only. Use CheckSpoolDirTrustingGroup when the relay legitimately runs
// under a different uid than its lane clients (a dedicated service account).
func CheckSpoolDir(spool string) (string, error) {
	return checkSpoolDir(spool, "")
}

// CheckSpoolDirTrustingGroup is CheckSpoolDir plus one additional, explicit
// acceptance path: a parent or spool directory owned by a DIFFERENT uid is
// still accepted if its group is exactly trustGroup and it is group-writable
// (never other-writable — that stays refused unconditionally). This is for a
// relay deployed under a dedicated, non-interactive service account that must
// still serve lane clients running as the interactive user: same-uid trust
// (the doc comment on CheckSpoolDir, "file modes do not isolate same-uid
// processes from each other") is widened to "same uid OR a named group",
// never to "anyone."
//
// trustGroup must resolve via os/user.LookupGroup; an unresolvable name fails
// closed rather than silently falling back to single-UID behavior — a typo'd
// group name must not look like a passing, more permissive configuration.
//
// The auto-tightening step widens correspondingly: a spool directory whose
// group is trustGroup is tightened to 0770 (group keeps rwx), not 0700 — the
// plain CheckSpoolDir path is completely unaffected and still tightens to
// 0700, so calling this with an empty trustGroup is NOT equivalent to calling
// CheckSpoolDir (call CheckSpoolDir directly for that).
func CheckSpoolDirTrustingGroup(spool, trustGroup string) (string, error) {
	if strings.TrimSpace(trustGroup) == "" {
		return "", fmt.Errorf("spool: trust group name is empty")
	}
	return checkSpoolDir(spool, trustGroup)
}

// spoolOwnerTrusted decides whether a directory owned by (uid, gid) is
// acceptable to a process running as selfUID, given the configured trust
// group's gid (trustGID, or -1 when no trust group is configured) and whether
// group-write is actually set on the directory's mode. Pulled out as a pure
// function so the group-trust decision is unit-testable without needing real
// multi-user file ownership (chown requires root) — tests drive this directly
// with synthetic uid/gid values.
func spoolOwnerTrusted(uid, gid uint32, selfUID, trustGID int, groupWritable bool) bool {
	if int(uid) == selfUID {
		return true
	}
	if trustGID >= 0 && int(gid) == trustGID && groupWritable {
		return true
	}
	return false
}

func checkSpoolDir(spool, trustGroup string) (string, error) {
	if !filepath.IsAbs(spool) || filepath.Clean(spool) != spool {
		return "", fmt.Errorf("spool: %q must be an absolute, clean path", spool)
	}
	trustGID := -1
	if trustGroup != "" {
		g, err := user.LookupGroup(trustGroup)
		if err != nil {
			return "", fmt.Errorf("spool: trust group %q: %w", trustGroup, err)
		}
		gid, err := strconv.Atoi(g.Gid)
		if err != nil {
			return "", fmt.Errorf("spool: trust group %q: bad gid %q: %w", trustGroup, g.Gid, err)
		}
		trustGID = gid
	}
	selfUID := os.Getuid()
	parent, err := filepath.EvalSymlinks(filepath.Dir(spool))
	if err != nil {
		return "", fmt.Errorf("spool: parent %s: %w", filepath.Dir(spool), err)
	}
	pst, err := os.Stat(parent)
	if err != nil {
		return "", fmt.Errorf("spool: parent %s: %w", parent, err)
	}
	parentGroupTrusted := false
	if sys, ok := pst.Sys().(*syscall.Stat_t); ok {
		parentGroupTrusted = trustGID >= 0 && int(sys.Gid) == trustGID
		trusted := sys.Uid == 0 || spoolOwnerTrusted(sys.Uid, sys.Gid, selfUID, trustGID, pst.Mode().Perm()&0o020 != 0)
		if !trusted {
			return "", fmt.Errorf("spool: parent %s is owned by uid %d, not %d or root; refusing", parent, sys.Uid, selfUID)
		}
	}
	// Default: refuse if EITHER group or other can write (0o022) — unchanged
	// from before this function grew a trust group. A trusted group's write bit
	// is the whole point of CheckSpoolDirTrustingGroup, so it alone is excused;
	// other-write is refused unconditionally either way.
	parentWriteMask := os.FileMode(0o022)
	if parentGroupTrusted {
		parentWriteMask = 0o002
	}
	if pst.Mode().Perm()&parentWriteMask != 0 && pst.Mode()&os.ModeSticky == 0 {
		return "", fmt.Errorf("spool: parent %s is writable by others (%o); refusing", parent, pst.Mode().Perm())
	}
	canon := filepath.Join(parent, filepath.Base(spool))
	st, err := os.Lstat(canon)
	switch {
	case os.IsNotExist(err):
		// A brand-new spool always starts single-owner (0700), even under
		// CheckSpoolDirTrustingGroup: this process's own gid at creation time
		// need not be trustGID (group MEMBERSHIP does not imply it is your
		// PRIMARY gid), so there is nothing correct to widen to yet. Migrating
		// an EXISTING spool to a trust group is the supported path — the OWNER
		// (whoever already has the directory) chgrp's it to the trust group
		// once, out of band, and the tightening step below converges it to 0770
		// on this and every future call. This must be run by the owning uid: a
		// DIFFERENT uid can never chmod or chgrp a directory it does not own
		// (that is exactly the ownership check above), so a service account
		// discovering someone else's 0700 spool cannot self-migrate it — the
		// migration is an owner-run, one-time step, not something this
		// function, or the service account, can perform on its own.
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
	groupTrusted := false
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		groupTrusted = trustGID >= 0 && int(sys.Gid) == trustGID
		if !spoolOwnerTrusted(sys.Uid, sys.Gid, selfUID, trustGID, st.Mode().Perm()&0o020 != 0) {
			return "", fmt.Errorf("spool: %s is owned by uid %d, not %d; refusing", canon, sys.Uid, selfUID)
		}
	}
	// No explicit other-write refusal here, matching the original: any looseness
	// on the spool dir itself (group OR other bits) is silently corrected below,
	// never refused.
	//
	// groupTrusted CONVERGES unconditionally to 0770 — both directions, not
	// just tightening. SSA review, PR #753: the first version only stripped
	// disallowed bits, so a spool already correctly chgrp'd to the trust group
	// but still at the plain 0700 it was created with (chgrp alone, no chmod)
	// passed this check UNCHANGED at 0700 — group-write was never actually
	// granted, silently defeating the migration path the doc comment claimed
	// worked. A verified-correct group is exactly the case where widening is
	// safe and intended, unlike the default path below.
	//
	// The DEFAULT (non-trusted) path is UNCHANGED from before this function
	// grew a trust group: only chmod when a disallowed bit (0o077) is actually
	// present, so an already-tighter mode (e.g. 0600) is left alone — widening
	// a single-uid spool that nobody configured for sharing is never
	// appropriate, and there is no verified group to justify it.
	if groupTrusted {
		if st.Mode().Perm() != 0o770 {
			if err := os.Chmod(canon, 0o770); err != nil {
				return "", fmt.Errorf("spool: converge mode: %w", err)
			}
		}
		return canon, nil
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
