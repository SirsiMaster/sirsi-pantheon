//go:build !windows

package maat

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// journalRepairBeforeInstall is a narrow test seam for the last substitution
// boundary. Production leaves it empty; the repair retains the parent and
// source descriptors, then checks the governed name immediately before the
// descriptor-relative rename.
var journalRepairBeforeInstall = func() {}

// journalRepairBeforeSourceRevalidation lets the adversarial tests mutate the
// same open inode after the first read. The subsequent retained-descriptor
// reread must refuse that case even when size and pathname are unchanged.
var journalRepairBeforeSourceRevalidation = func() {}

type journalObjectIdentity struct {
	dev   uint64
	ino   uint64
	uid   uint32
	mode  uint32
	nlink uint64
	size  int64
}

func journalIdentity(stat *unix.Stat_t) journalObjectIdentity {
	return journalObjectIdentity{
		dev: uint64(stat.Dev), ino: stat.Ino, uid: stat.Uid, mode: uint32(stat.Mode),
		nlink: uint64(stat.Nlink), size: stat.Size,
	}
}

func (identity journalObjectIdentity) same(other journalObjectIdentity) bool {
	return identity.dev == other.dev && identity.ino == other.ino && identity.uid == other.uid && identity.mode == other.mode && identity.nlink == other.nlink && identity.size == other.size
}

func isPrivateRegular(identity journalObjectIdentity) bool {
	return identity.mode&unix.S_IFMT == unix.S_IFREG && identity.nlink == 1 && identity.mode&0o077 == 0
}

func isOwnedRegular(identity journalObjectIdentity) bool {
	return identity.mode&unix.S_IFMT == unix.S_IFREG && identity.nlink == 1 && identity.uid == uint32(os.Geteuid())
}

func withJournalMutationLock(path string, fn func() error) error {
	parentFD, _, base, err := openJournalParent(path)
	if err != nil {
		return err
	}
	defer unix.Close(parentFD)
	lockFD, err := unix.Openat(parentFD, "."+base+".lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return fmt.Errorf("maat decision journal: open mutation lock: %w", err)
	}
	defer unix.Close(lockFD)
	if err := unix.Flock(lockFD, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fmt.Errorf("maat decision journal: another local write or repair is in progress; retry after it finishes")
	}
	defer unix.Flock(lockFD, unix.LOCK_UN)
	return fn()
}

func repairInvalidRecords(j *FileDecisionJournal) (JournalRepairReceipt, error) {
	if j == nil || strings.TrimSpace(j.Path) == "" {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: empty path")
	}
	returnReceipt := JournalRepairReceipt{}
	err := withJournalMutationLock(j.Path, func() error {
		receipt, err := repairInvalidRecordsLocked(j)
		returnReceipt = receipt
		return err
	})
	return returnReceipt, err
}

func repairInvalidRecordsLocked(j *FileDecisionJournal) (JournalRepairReceipt, error) {
	parentFD, parentPath, base, err := openJournalParent(j.Path)
	if err != nil {
		return JournalRepairReceipt{}, err
	}
	defer unix.Close(parentFD)

	sourceFD, err := unix.Openat(parentFD, base, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if err == unix.ENOENT {
			return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: no journal exists to repair")
		}
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: open repair source: %w", err)
	}
	source := os.NewFile(uintptr(sourceFD), base)
	defer source.Close()
	beforeStat, err := fstatJournal(sourceFD)
	if err != nil {
		return JournalRepairReceipt{}, err
	}
	// A confirmed repair may tighten an owned, singly-linked regular legacy
	// journal through its retained descriptor. This is the only pre-rebuild
	// mutation: it never follows a pathname, never widens permissions, and
	// refuses a different owner, symlink, hard link, or non-regular object.
	if !isOwnedRegular(beforeStat) {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: repair source is not an owned singly-linked regular file")
	}
	if beforeStat.mode&0o077 != 0 {
		if err := unix.Fchmod(sourceFD, 0o600); err != nil {
			return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: tighten legacy journal permissions: %w", err)
		}
		if err := unix.Fsync(sourceFD); err != nil {
			return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: sync tightened legacy journal permissions: %w", err)
		}
		beforeStat, err = fstatJournal(sourceFD)
		if err != nil {
			return JournalRepairReceipt{}, err
		}
	}
	if !isPrivateRegular(beforeStat) {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: repair source is not a private singly-linked regular file")
	}
	raw, err := io.ReadAll(source)
	if err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: read repair source: %w", err)
	}
	valid, invalid, retained := partitionDecisionJournal(raw)
	if invalid == 0 {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: no invalid records need repair")
	}

	backupFD, backupName, err := createJournalLeaf(parentFD, base+".invalid-backup-")
	if err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: create preserved backup: %w", err)
	}
	backup := os.NewFile(uintptr(backupFD), backupName)
	backupKept := false
	defer func() {
		_ = backup.Close()
		if !backupKept {
			_ = unix.Unlinkat(parentFD, backupName, 0)
		}
	}()
	if err := writeAllFile(backup, raw); err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: write preserved backup: %w", err)
	}
	if err := backup.Sync(); err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: sync preserved backup: %w", err)
	}
	if err := unix.Fsync(parentFD); err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: sync backup directory: %w", err)
	}
	backupKept = true

	replacementFD, replacementName, err := createJournalLeaf(parentFD, base+".repair-")
	if err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: create replacement: %w", err)
	}
	replacement := os.NewFile(uintptr(replacementFD), replacementName)
	replacementInstalled := false
	defer func() {
		_ = replacement.Close()
		if !replacementInstalled {
			_ = unix.Unlinkat(parentFD, replacementName, 0)
		}
	}()
	if err := writeAllFile(replacement, valid); err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: write replacement: %w", err)
	}
	if err := replacement.Sync(); err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: sync replacement: %w", err)
	}
	if err := unix.Fsync(parentFD); err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: sync replacement directory: %w", err)
	}

	// Re-read through the retained source descriptor. This catches same-inode,
	// same-size content edits after the first read, while the mutation lock
	// serializes current Pantheon appenders through the final rename.
	journalRepairBeforeSourceRevalidation()
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: rewind source before install: %w", err)
	}
	currentBytes, err := io.ReadAll(source)
	if err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: reread source before install: %w", err)
	}
	afterStat, err := fstatJournal(sourceFD)
	if err != nil {
		return JournalRepairReceipt{}, err
	}
	if !beforeStat.same(afterStat) || !bytes.Equal(raw, currentBytes) {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: source changed during repair; original was preserved at %s and no replacement was installed", filepath.Join(parentPath, backupName))
	}

	journalRepairBeforeInstall()
	nameStat, err := fstatatJournal(parentFD, base)
	if err != nil || !beforeStat.same(nameStat) {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: governed journal name changed during repair; original was preserved at %s and no replacement was installed", filepath.Join(parentPath, backupName))
	}
	if err := unix.Renameat(parentFD, replacementName, parentFD, base); err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: install replacement: %w", err)
	}
	replacementInstalled = true
	if err := unix.Fsync(parentFD); err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: replacement installed but parent sync failed; preserve and inspect backup %s: %w", filepath.Join(parentPath, backupName), err)
	}
	installed, err := fstatatJournal(parentFD, base)
	if err != nil {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: stat installed replacement: %w", err)
	}
	if !isPrivateRegular(installed) {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: installed replacement identity is invalid; preserve and inspect backup %s", filepath.Join(parentPath, backupName))
	}
	if invalidAfter, retainedAfter := strictJournalBytes(valid); invalidAfter != 0 || retainedAfter != retained {
		return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: replacement did not pass strict verification; preserve and inspect backup %s", filepath.Join(parentPath, backupName))
	}
	digest := sha256.Sum256(raw)
	return JournalRepairReceipt{
		BackupPath: filepath.Join(parentPath, backupName), OriginalDigest: fmt.Sprintf("sha256:%x", digest), RemovedCount: invalid, RetainedCount: retained,
	}, nil
}

func openJournalParent(path string) (int, string, string, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return -1, "", "", fmt.Errorf("maat decision journal: path must be absolute")
	}
	parent, base := filepath.Dir(clean), filepath.Base(clean)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		return -1, "", "", fmt.Errorf("maat decision journal: invalid leaf")
	}
	fd, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, "", "", fmt.Errorf("maat decision journal: open retained parent: %w", err)
	}
	return fd, parent, base, nil
}

func createJournalLeaf(parentFD int, prefix string) (int, string, error) {
	for attempt := 0; attempt < 32; attempt++ {
		var random [12]byte
		if _, err := rand.Read(random[:]); err != nil {
			return -1, "", err
		}
		name := prefix + hex.EncodeToString(random[:])
		fd, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
		if err == unix.EEXIST {
			continue
		}
		if err != nil {
			return -1, "", err
		}
		identity, statErr := fstatJournal(fd)
		if statErr != nil || !isPrivateRegular(identity) {
			_ = unix.Close(fd)
			_ = unix.Unlinkat(parentFD, name, 0)
			if statErr != nil {
				return -1, "", statErr
			}
			return -1, "", fmt.Errorf("created object identity is invalid")
		}
		return fd, name, nil
	}
	return -1, "", fmt.Errorf("could not allocate create-only journal leaf")
}

func fstatJournal(fd int) (journalObjectIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return journalObjectIdentity{}, fmt.Errorf("maat decision journal: stat retained object: %w", err)
	}
	return journalIdentity(&stat), nil
}

func fstatatJournal(parentFD int, name string) (journalObjectIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return journalObjectIdentity{}, err
	}
	return journalIdentity(&stat), nil
}

func writeAllFile(file *os.File, data []byte) error {
	for len(data) > 0 {
		n, err := file.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func partitionDecisionJournal(raw []byte) (valid []byte, invalid, retained int) {
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var decision Decision
		if err := json.Unmarshal([]byte(trimmed), &decision); err != nil || validateDecision(decision) != nil {
			invalid++
			continue
		}
		valid = append(valid, line...)
		valid = append(valid, '\n')
		retained++
	}
	return valid, invalid, retained
}

func strictJournalBytes(raw []byte) (invalid, retained int) {
	_, invalid, retained = partitionDecisionJournal(raw)
	return invalid, retained
}
