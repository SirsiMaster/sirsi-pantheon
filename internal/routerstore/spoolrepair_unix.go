//go:build !windows

package routerstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// RepairSpoolOutbox restores one unreadable, locally-owned relay outbox to
// mode 0700. It never follows a symlink, deletes a file, or names an absolute
// child path after inspection: the mutation is made relative to a retained
// parent/root/agent descriptor chain, then the repaired child is reopened and
// identity-checked. A service-owned or ambiguous object is refused rather
// than guessed at.
func RepairSpoolOutbox(spoolRoot, agent string) (SpoolOutboxRepair, error) {
	return repairSpoolOutbox(spoolRoot, agent, nil)
}

// repairSpoolOutbox has a narrowly-scoped test seam immediately before the
// retained root is opened. Production always passes nil. The seam proves that
// the root actually opened is itself validated through the retained parent;
// a path checked earlier can never bless a replacement at that namespace leaf.
func repairSpoolOutbox(spoolRoot, agent string, beforeRootOpen func()) (SpoolOutboxRepair, error) {
	if !validSpoolAgentName(agent) {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: invalid agent name %q", agent)
	}
	if strings.TrimSpace(spoolRoot) == "" || !filepath.IsAbs(spoolRoot) || filepath.Clean(spoolRoot) != spoolRoot {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: spool root must be an absolute, clean path")
	}
	canonical, parentFD, rootFD, err := openRepairSpoolRoot(spoolRoot, beforeRootOpen)
	if err != nil {
		return SpoolOutboxRepair{}, err
	}
	defer unix.Close(parentFD)
	defer unix.Close(rootFD)
	agentFD, err := unix.Openat(rootFD, agent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: open agent directory: %w", err)
	}
	defer unix.Close(agentFD)
	var st unix.Stat_t
	// A mode-000 outbox cannot itself be opened for reading. fstatat/fchmodat
	// keep the operation anchored to the retained parent descriptor and require
	// the leaf itself not be a symlink; only after its mode is restored can it be
	// opened and bound as a usable directory descriptor.
	if err := unix.Fstatat(agentFD, "outbox", &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: inspect outbox through retained agent descriptor: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: retained leaf is not a directory")
	}
	if int(st.Uid) != os.Getuid() {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: retained outbox is owned by uid %d, not this local user; refusing", st.Uid)
	}
	before := os.FileMode(st.Mode).Perm()
	if err := unix.Fchmodat(agentFD, "outbox", 0o700, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: set private owner mode through retained agent descriptor: %w", err)
	}
	outboxFD, err := unix.Openat(agentFD, "outbox", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: reopen repaired outbox without following links: %w", err)
	}
	defer unix.Close(outboxFD)
	var repaired unix.Stat_t
	if err := unix.Fstat(outboxFD, &repaired); err != nil {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: stat repaired outbox descriptor: %w", err)
	}
	if repaired.Dev != st.Dev || repaired.Ino != st.Ino || repaired.Mode&unix.S_IFMT != unix.S_IFDIR {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: outbox identity changed during repair; refusing success")
	}
	// The changed directory and its namespace parent must reach stable storage
	// before Pantheon says the repair is complete. A sync failure is surfaced as
	// unresolved; the caller rechecks the board instead of assuming success.
	if err := unix.Fsync(outboxFD); err != nil {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: persist outbox mode: %w", err)
	}
	if err := unix.Fsync(agentFD); err != nil {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: persist agent directory: %w", err)
	}
	return SpoolOutboxRepair{
		Agent:          agent,
		CanonicalSpool: canonical,
		PreviousMode:   fmt.Sprintf("%04o", before),
		RepairedMode:   "0700",
	}, nil
}

// openRepairSpoolRoot binds repair to a root opened through an already-retained
// no-follow parent descriptor. It deliberately does not call CheckSpoolDir:
// that helper is correct for bootstrap convergence, but it validates by path
// and may create or chmod a root. A user-requested outbox repair must neither
// create nor bless a root by a separate pathname check. The exact descriptor
// that subsequent Openat/Fstatat operations use is validated here instead.
func openRepairSpoolRoot(spoolRoot string, beforeRootOpen func()) (string, int, int, error) {
	parentPath, err := filepath.EvalSymlinks(filepath.Dir(spoolRoot))
	if err != nil {
		return "", -1, -1, fmt.Errorf("outbox repair: resolve spool parent: %w", err)
	}
	if !filepath.IsAbs(parentPath) || filepath.Clean(parentPath) != parentPath {
		return "", -1, -1, fmt.Errorf("outbox repair: canonical spool parent is not an absolute, clean path")
	}
	parentFD, err := unix.Open(parentPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", -1, -1, fmt.Errorf("outbox repair: open spool parent without following links: %w", err)
	}
	closeParent := true
	defer func() {
		if closeParent {
			_ = unix.Close(parentFD)
		}
	}()

	var parent unix.Stat_t
	if err := unix.Fstat(parentFD, &parent); err != nil {
		return "", -1, -1, fmt.Errorf("outbox repair: stat retained spool parent: %w", err)
	}
	if parent.Mode&unix.S_IFMT != unix.S_IFDIR {
		return "", -1, -1, fmt.Errorf("outbox repair: retained spool parent is not a directory")
	}
	if int(parent.Uid) != os.Getuid() && parent.Uid != 0 {
		return "", -1, -1, fmt.Errorf("outbox repair: retained spool parent is owned by uid %d, not this user or root; refusing", parent.Uid)
	}
	if parent.Mode&0o002 != 0 && parent.Mode&unix.S_ISVTX == 0 {
		return "", -1, -1, fmt.Errorf("outbox repair: retained spool parent is writable by others; refusing")
	}

	if beforeRootOpen != nil {
		beforeRootOpen()
	}
	rootName := filepath.Base(spoolRoot)
	rootFD, err := unix.Openat(parentFD, rootName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", -1, -1, fmt.Errorf("outbox repair: open spool root without following links: %w", err)
	}
	closeRoot := true
	defer func() {
		if closeRoot {
			_ = unix.Close(rootFD)
		}
	}()
	var root unix.Stat_t
	if err := unix.Fstat(rootFD, &root); err != nil {
		return "", -1, -1, fmt.Errorf("outbox repair: stat retained spool root: %w", err)
	}
	if root.Mode&unix.S_IFMT != unix.S_IFDIR {
		return "", -1, -1, fmt.Errorf("outbox repair: retained spool root is not a directory")
	}
	if int(root.Uid) != os.Getuid() {
		return "", -1, -1, fmt.Errorf("outbox repair: retained spool root is owned by uid %d, not this local user; refusing", root.Uid)
	}
	if root.Mode&0o077 != 0 {
		return "", -1, -1, fmt.Errorf("outbox repair: retained spool root is not private to this local user; refusing")
	}
	var namedRoot unix.Stat_t
	if err := unix.Fstatat(parentFD, rootName, &namedRoot, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return "", -1, -1, fmt.Errorf("outbox repair: revalidate spool root name through retained parent: %w", err)
	}
	if namedRoot.Dev != root.Dev || namedRoot.Ino != root.Ino || namedRoot.Mode&unix.S_IFMT != unix.S_IFDIR {
		return "", -1, -1, fmt.Errorf("outbox repair: spool root changed during retained open; refusing")
	}

	closeParent = false
	closeRoot = false
	return filepath.Join(parentPath, rootName), parentFD, rootFD, nil
}

func validSpoolAgentName(agent string) bool {
	if agent == "" || len(agent) > 128 || filepath.Base(agent) != agent || agent == "." || agent == ".." {
		return false
	}
	for _, r := range agent {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}
