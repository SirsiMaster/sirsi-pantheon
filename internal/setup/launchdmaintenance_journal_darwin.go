//go:build darwin

package setup

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
	"golang.org/x/sys/unix"
)

const maintenanceJournalLeaf = ".ai.sirsi.router.relay.maintenance.jsonl"

// This is the existing selected-service filesystem custody boundary, not a
// router database. owner is instance-local for owned inert fixtures; native
// construction always uses root and /Library/LaunchDaemons.
type maintenanceJournal struct {
	directory string
	owner     uint32
}

func maintenanceOpenDirectory(path string) (int, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return -1, errors.New("legacy maintenance: canonical directory required")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if part == "" {
			continue
		}
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if err != nil {
			return -1, err
		}
		fd = next
	}
	return fd, nil
}

func (j maintenanceJournal) parent() (int, error) {
	fd, err := maintenanceOpenDirectory(j.directory)
	if err != nil {
		return -1, err
	}
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Uid != j.owner || st.Mode&0022 != 0 {
		_ = unix.Close(fd)
		return -1, errors.New("legacy maintenance: protected lifecycle parent required")
	}
	return fd, nil
}

func (j maintenanceJournal) load() (MaintenanceStatus, error) {
	var last MaintenanceStatus
	parent, err := j.parent()
	if err != nil {
		return last, err
	}
	defer unix.Close(parent)
	fd, err := unix.Openat(parent, maintenanceJournalLeaf, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return last, nil
	}
	if err != nil {
		return last, err
	}
	f := os.NewFile(uintptr(fd), maintenanceJournalLeaf)
	defer f.Close()
	var before unix.Stat_t
	if unix.Fstat(fd, &before) != nil || before.Uid != j.owner || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Mode&07777 != 0600 || before.Nlink != 1 || before.Size <= 0 || before.Size > 1<<20 || !routerstore.MaintenanceMetadataNoACL(fd) {
		return last, errors.New("legacy maintenance: private bounded transaction journal required; no reset")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || int64(len(raw)) != before.Size || raw[len(raw)-1] != '\n' {
		return last, errors.New("legacy maintenance: incomplete journal retained; no replay/reset")
	}
	reader := bufio.NewReader(bytes.NewReader(raw))
	seen := map[string]bool{}
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return last, err
		}
		next, err := servicedrain.DecodeMaintenance[MaintenanceStatus](line)
		id, idErr := hex.DecodeString(next.ID)
		if err != nil || idErr != nil || len(id) != 16 || hex.EncodeToString(id) != next.ID || next.Schema != 1 || next.Plan.validate() != nil || next.Revision != last.Revision+1 || next.Preserved < 0 || next.OldOutcomes != "unknown_not_certified_drained" || !maintenanceTransition(last, next) || !maintenanceSuccessorRecord(next) {
			return last, errors.New("legacy maintenance: malformed transaction history retained")
		}
		switch next.Phase {
		case "prepared", "retirement_requested", "retirement_observed", "retired", "custody_preserved", "bootstrap_requested", "activation_requested", "activated", "completed", "abort_requested", "aborted":
		default:
			return last, errors.New("legacy maintenance: unknown transaction phase")
		}
		if last.ID != "" && next.ID != last.ID {
			if seen[next.ID] || next.Phase != "prepared" || (last.Phase != "completed" && last.Phase != "aborted") {
				return last, errors.New("legacy maintenance: overlapping/reused transaction")
			}
		} else if last.ID != "" && next.Plan != last.Plan {
			return last, errors.New("legacy maintenance: transaction plan changed")
		}
		seen[next.ID] = true
		last = next
	}
	var after, named unix.Stat_t
	if unix.Fstat(fd, &after) != nil || unix.Fstatat(parent, maintenanceJournalLeaf, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Size != after.Size || after.Dev != named.Dev || after.Ino != named.Ino || after.Size != named.Size || after.Nlink != 1 {
		return last, errors.New("legacy maintenance: journal changed during read")
	}
	return last, nil
}

func (j maintenanceJournal) persist(expected uint64, next MaintenanceStatus) error {
	current, err := j.load()
	if err != nil || current.Revision != expected || next.Revision != expected+1 || !maintenanceTransition(current, next) || !maintenanceSuccessorRecord(next) {
		return errors.Join(errors.New("legacy maintenance: journal revision CAS refused"), err)
	}
	parent, err := j.parent()
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	flags := unix.O_WRONLY | unix.O_APPEND | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
	if expected == 0 {
		flags |= unix.O_CREAT | unix.O_EXCL
	}
	fd, err := unix.Openat(parent, maintenanceJournalLeaf, flags, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), maintenanceJournalLeaf)
	defer f.Close()
	var before unix.Stat_t
	if unix.Fstat(fd, &before) != nil || before.Uid != j.owner || before.Nlink != 1 || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Mode&07777 != 0600 || !routerstore.MaintenanceMetadataNoACL(fd) {
		return errors.New("legacy maintenance: unsafe journal append refused")
	}
	b, err := jsonMaintenance(next)
	if err != nil || before.Size+int64(len(b)) > 1<<20 {
		return errors.New("legacy maintenance: bounded phase record required")
	}
	if n, err := f.Write(b); err != nil || n != len(b) {
		return errors.Join(io.ErrShortWrite, err)
	}
	if err := f.Sync(); err != nil {
		return err
	}
	var named unix.Stat_t
	if unix.Fstatat(parent, maintenanceJournalLeaf, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != before.Dev || named.Ino != before.Ino || named.Nlink != 1 {
		return errors.New("legacy maintenance: phase append name changed; retained debt")
	}
	if err := unix.Fsync(parent); err != nil {
		return err
	}
	readback, err := j.load()
	if err != nil || readback != next {
		return errors.Join(errors.New("legacy maintenance: phase readback unverified"), err)
	}
	return nil
}

func maintenanceSuccessorRecord(s MaintenanceStatus) bool {
	if s.Phase != "activation_requested" && s.Phase != "activated" && s.Phase != "completed" {
		return true
	}
	gen, err := hex.DecodeString(s.Generation)
	p := s.Successor
	return err == nil && len(gen) == 16 && hex.EncodeToString(gen) == s.Generation && validateServiceInstance(p) == nil && sameRootRelayArtifacts(p, s.Plan.Original) && !(p.PID == s.Plan.Original.PID && p.Started == s.Plan.Original.Started)
}
