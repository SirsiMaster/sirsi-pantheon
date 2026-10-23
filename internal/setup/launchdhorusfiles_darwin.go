//go:build darwin

package setup

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// The lock coordinates maintainers. Retained descriptors and swap/readback
// detect substitution; this is not a kernel compare-and-swap against a hostile
// same-UID writer. Never delete a leaf on failure, including unknown replacements.
type horusFilePublication struct {
	dir                           int
	parent, leaf, backup, stage   string
	original, prepared, preserved *os.File
	before, after                 []byte
	published                     bool
}

func horusBackupLeaf(hash string) string {
	return "." + HorusServiceLabel + "." + hash + ".backup.plist"
}

func openHorusParent(path string) (int, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return -1, errors.New("Horus maintenance: canonical artifact path required")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(filepath.Dir(path), "/"), "/") {
		if part == "" {
			continue
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return -1, e
		}
		fd = next
	}
	return fd, nil
}

func (p *horusFilePublication) parentStable() error {
	var held, named unix.Stat_t
	if err := unix.Fstat(p.dir, &held); err != nil {
		return err
	}
	if err := unix.Lstat(p.parent, &named); err != nil {
		return err
	}
	physical, err := filepath.EvalSymlinks(p.parent)
	if err != nil || physical != p.parent || held.Dev != named.Dev || held.Ino != named.Ino || named.Mode&unix.S_IFMT != unix.S_IFDIR || held.Uid != uint32(os.Getuid()) || held.Mode&0022 != 0 {
		return errors.New("Horus maintenance: retained owner directory changed or unsafe")
	}
	return nil
}

func (p *horusFilePublication) verify(file *os.File, name string, want []byte, private bool) error {
	if err := p.parentStable(); err != nil {
		return err
	}
	var held, named, after unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &held); err != nil {
		return err
	}
	if err := unix.Fstatat(p.dir, name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if held.Mode&unix.S_IFMT != unix.S_IFREG || named.Mode&unix.S_IFMT != unix.S_IFREG || held.Dev != named.Dev || held.Ino != named.Ino || held.Nlink != 1 || held.Uid != uint32(os.Getuid()) || held.Mode&0022 != 0 || (private && held.Mode&07777 != 0600) || held.Size != int64(len(want)) {
		return errors.New("Horus maintenance: retained leaf identity/permissions changed")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	got, err := io.ReadAll(io.LimitReader(file, int64(len(want))+1))
	if err != nil || !bytes.Equal(got, want) {
		return errors.New("Horus maintenance: retained leaf byte/EOF readback failed")
	}
	if err := unix.Fstat(int(file.Fd()), &after); err != nil {
		return err
	}
	if held.Dev != after.Dev || held.Ino != after.Ino || held.Mode != after.Mode || held.Uid != after.Uid || held.Gid != after.Gid || held.Nlink != after.Nlink || held.Size != after.Size || held.Mtim != after.Mtim || held.Ctim != after.Ctim {
		return errors.New("Horus maintenance: retained leaf changed during readback")
	}
	if err := unix.Fstatat(p.dir, name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if named.Dev != held.Dev || named.Ino != held.Ino || named.Mode != held.Mode || named.Size != held.Size {
		return errors.New("Horus maintenance: leaf replaced during readback")
	}
	return p.parentStable()
}

func prepareHorusPublication(path string, before, after []byte) (horusPublication, error) {
	if filepath.Base(path) != HorusServiceLabel+".plist" || len(before) == 0 || len(before) > 1<<20 || len(after) == 0 || len(after) > 1<<20 {
		return nil, errors.New("Horus maintenance: exact bounded selected plist required")
	}
	dir, err := openHorusParent(path)
	if err != nil {
		return nil, err
	}
	p := &horusFilePublication{dir: dir, parent: filepath.Dir(path), leaf: filepath.Base(path), before: before, after: after}
	fail := func(err error) (horusPublication, error) {
		closeErr := p.Close()
		return nil, fmt.Errorf("Horus maintenance: staging failed; any created private backup/stage is retained as cleanup debt (backup %s, stage %s): %w", p.backup, p.stage, errors.Join(err, closeErr))
	}
	if err = p.parentStable(); err != nil {
		return fail(err)
	}
	fd, err := unix.Openat(dir, p.leaf, unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return fail(err)
	}
	p.original = os.NewFile(uintptr(fd), path)
	if err = p.verify(p.original, p.leaf, before, false); err != nil {
		return fail(err)
	}
	p.backup = horusBackupLeaf(horusDigest(before))
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return fail(err)
	}
	p.stage = "." + HorusServiceLabel + "." + hex.EncodeToString(nonce[:]) + ".stage.plist"
	create := func(name string, data []byte) (*os.File, error) {
		fd, err := unix.Openat(dir, name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
		if err != nil {
			return nil, errors.New("Horus maintenance: private backup/stage occupied or unavailable; nothing overwritten")
		}
		f := os.NewFile(uintptr(fd), filepath.Join(p.parent, name))
		if n, err := f.Write(data); err != nil || n != len(data) {
			f.Close()
			return nil, errors.New("Horus maintenance: private leaf write failed; retained cleanup debt")
		}
		if err = f.Sync(); err != nil {
			f.Close()
			return nil, err
		}
		if err = p.verify(f, name, data, true); err != nil {
			f.Close()
			return nil, err
		}
		return f, nil
	}
	p.preserved, err = create(p.backup, before)
	if err != nil {
		return fail(err)
	}
	p.prepared, err = create(p.stage, after)
	if err != nil {
		return fail(err)
	}
	if err = unix.Fsync(dir); err != nil {
		return fail(err)
	}
	return p, nil
}

func (p *horusFilePublication) Publish() error {
	if p.published {
		return errors.New("Horus maintenance: publication already performed")
	}
	if err := p.verify(p.original, p.leaf, p.before, false); err != nil {
		return err
	}
	if err := p.verify(p.preserved, p.backup, p.before, true); err != nil {
		return err
	}
	if err := p.verify(p.prepared, p.stage, p.after, true); err != nil {
		return err
	}
	if err := unix.RenameatxNp(p.dir, p.stage, p.dir, p.leaf, unix.RENAME_SWAP); err != nil {
		return err
	}
	p.published = true
	// The displaced original stays retained, never unlinked or exposed as a new
	// public backup. Refuse unknown identities rather than guessing a rollback.
	if err := p.verify(p.original, p.stage, p.before, false); err != nil {
		return err
	}
	if err := p.original.Chmod(0600); err != nil {
		return err
	}
	if err := p.original.Sync(); err != nil {
		return err
	}
	if err := p.verify(p.prepared, p.leaf, p.after, true); err != nil {
		return err
	}
	if err := unix.Fsync(p.dir); err != nil {
		return err
	}
	return p.parentStable()
}

func (p *horusFilePublication) Restore() error {
	if !p.published {
		return p.verify(p.original, p.leaf, p.before, false)
	}
	if err := p.verify(p.prepared, p.leaf, p.after, true); err != nil {
		return err
	}
	if err := p.verify(p.original, p.stage, p.before, false); err != nil {
		return err
	}
	if err := p.verify(p.preserved, p.backup, p.before, true); err != nil {
		return err
	}
	if err := unix.RenameatxNp(p.dir, p.stage, p.dir, p.leaf, unix.RENAME_SWAP); err != nil {
		return err
	}
	p.published = false
	if err := p.verify(p.original, p.leaf, p.before, false); err != nil {
		return err
	}
	if err := unix.Fsync(p.dir); err != nil {
		return err
	}
	return p.parentStable()
}

func (p *horusFilePublication) Close() error {
	var result error
	for _, f := range []*os.File{p.original, p.prepared, p.preserved} {
		if f != nil {
			result = errors.Join(result, f.Close())
		}
	}
	if p.dir >= 0 {
		result = errors.Join(result, unix.Close(p.dir))
		p.dir = -1
	}
	return result
}
