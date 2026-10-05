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
// child path after inspection: the mutation is made relative to the retained
// agent descriptor, then the repaired child is reopened and identity-checked.
// A service-owned or ambiguous object is refused rather than guessed at.
func RepairSpoolOutbox(spoolRoot, agent string) (SpoolOutboxRepair, error) {
	if !validSpoolAgentName(agent) {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: invalid agent name %q", agent)
	}
	if strings.TrimSpace(spoolRoot) == "" || !filepath.IsAbs(spoolRoot) || filepath.Clean(spoolRoot) != spoolRoot {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: spool root must be an absolute, clean path")
	}
	// Unlike the relay bootstrap path, a repair must not create a missing spool
	// root as a side effect of a stale board observation.
	if _, err := os.Lstat(spoolRoot); err != nil {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: inspect spool root: %w", err)
	}
	canonical, err := CheckSpoolDir(spoolRoot)
	if err != nil {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: %w", err)
	}

	rootFD, err := unix.Open(canonical, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: open checked spool root: %w", err)
	}
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
