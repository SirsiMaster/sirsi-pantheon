//go:build !windows

package knowledge

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func userHomeDir() (string, error) { return os.UserHomeDir() }

type fileIdentity struct {
	device uint64
	inode  uint64
}

type secureArchiveLocation struct {
	home       string
	path       string
	rootFD     int
	sirsiFD    int
	maatFD     int
	archiveFD  int
	database   int
	identities [5]fileIdentity
	vfsName    string
	vfs        *archiveVFS
	authority  *archiveAuthority
}

// sqlitePath is a logical SQLite filename. Darwin resolves it through the
// per-archive retained-root VFS; other Unix systems retain their existing
// descriptor-path implementation. Neither path claims atomicity against a
// hostile same-UID process racing an operation-boundary check.
func (p *secureArchiveLocation) sqlitePath() string {
	if p != nil && p.vfsName != "" {
		return "/maat-archive/" + p.vfsName + "/archive.sqlite3"
	}
	return "/dev/fd/" + strconv.Itoa(p.archiveFD) + "/archive.sqlite3"
}

func prepareSecureArchiveLocation(home string) (*secureArchiveLocation, error) {
	if runtime.GOOS == "darwin" {
		return prepareDarwinSecureArchiveLocation(home)
	}
	if !filepath.IsAbs(home) {
		return nil, errors.New("Ma'at knowledge home must be absolute")
	}
	root, err := openAbsoluteDirectory(filepath.Clean(home))
	if err != nil {
		return nil, errors.New("Ma'at knowledge home is not a safe directory")
	}
	if err := ownedDirectory(root, false); err != nil {
		_ = unix.Close(root)
		return nil, err
	}
	sirsi, err := openChildDirectory(root, ".sirsi", false)
	if err != nil {
		_ = unix.Close(root)
		return nil, err
	}
	maat, err := openChildDirectory(sirsi, "maat", false)
	if err != nil {
		_ = unix.Close(sirsi)
		_ = unix.Close(root)
		return nil, err
	}
	knowledgeDir, err := openChildDirectory(maat, "knowledge", true)
	if err != nil {
		_ = unix.Close(maat)
		_ = unix.Close(sirsi)
		_ = unix.Close(root)
		return nil, err
	}
	fd, err := unix.Openat(knowledgeDir, "archive.sqlite3", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0o600)
	if err != nil {
		_ = unix.Close(knowledgeDir)
		_ = unix.Close(maat)
		_ = unix.Close(sirsi)
		_ = unix.Close(root)
		return nil, errors.New("Ma'at knowledge archive file is not safe")
	}
	location := &secureArchiveLocation{
		home: filepath.Clean(home), path: filepath.Join(home, ".sirsi", "maat", "knowledge", "archive.sqlite3"),
		rootFD: root, sirsiFD: sirsi, maatFD: maat, archiveFD: knowledgeDir, database: fd,
	}
	fds := []int{root, sirsi, maat, knowledgeDir, fd}
	for index, descriptor := range fds {
		identity, err := descriptorIdentity(descriptor)
		if err != nil {
			_ = location.Close()
			return nil, errors.New("Ma'at knowledge archive path identity is unavailable")
		}
		location.identities[index] = identity
	}
	if err := verifyArchiveFile(fd); err != nil {
		_ = location.Close()
		return nil, err
	}
	if err := verifySidecars(knowledgeDir); err != nil {
		_ = location.Close()
		return nil, err
	}
	return location, nil
}

func (p *secureArchiveLocation) Close() error {
	if p == nil {
		return nil
	}
	if p.vfs != nil {
		if err := p.vfs.Close(); err != nil {
			return err
		}
		p.vfs = nil
	}
	if p.authority != nil {
		err := p.authority.release()
		if err == nil {
			p.authority = nil
			p.database, p.archiveFD, p.maatFD, p.sirsiFD, p.rootFD = -1, -1, -1, -1, -1
		}
		return err
	}
	var first error
	for _, fd := range []int{p.database, p.archiveFD, p.maatFD, p.sirsiFD, p.rootFD} {
		if fd >= 0 {
			if err := unix.Close(fd); err != nil && first == nil {
				first = err
			}
		}
	}
	p.database, p.archiveFD, p.maatFD, p.sirsiFD, p.rootFD = -1, -1, -1, -1, -1
	return first
}

func (p *secureArchiveLocation) verify() error {
	if p == nil || p.rootFD < 0 || p.database < 0 {
		return errors.New("archive path authority is unavailable")
	}
	root, err := openAbsoluteDirectory(p.home)
	if err != nil {
		return errors.New("Ma'at knowledge home changed")
	}
	defer unix.Close(root)
	current := []int{root}
	for _, name := range []string{".sirsi", "maat", "knowledge"} {
		next, openErr := openExistingChildDirectory(current[len(current)-1], name)
		if openErr != nil {
			for _, fd := range current[1:] {
				_ = unix.Close(fd)
			}
			return errors.New("Ma'at knowledge archive parent changed")
		}
		current = append(current, next)
	}
	defer func() {
		for _, fd := range current[1:] {
			_ = unix.Close(fd)
		}
	}()
	for index, fd := range current {
		identity, statErr := descriptorIdentity(fd)
		if statErr != nil || identity != p.identities[index] {
			return errors.New("Ma'at knowledge archive parent identity changed")
		}
		if err := ownedDirectory(fd, index == 3); err != nil {
			return err
		}
	}
	identity, err := fileIdentityAt(current[3], "archive.sqlite3")
	if err != nil || identity != p.identities[4] || verifyArchiveFileAt(current[3], "archive.sqlite3") != nil {
		return errors.New("Ma'at knowledge archive leaf identity changed")
	}
	return verifySidecars(current[3])
}

func (p *secureArchiveLocation) verifyForSQLiteUse() error {
	if err := p.verify(); err != nil {
		return err
	}
	identity, err := descriptorIdentity(p.database)
	if err != nil || identity != p.identities[4] {
		return errors.New("Ma'at knowledge SQLite inode identity changed")
	}
	return nil
}

func fileIdentityAt(directory int, name string) (fileIdentity, error) {
	var st unix.Stat_t
	if err := unix.Fstatat(directory, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fileIdentity{}, err
	}
	return fileIdentity{device: uint64(st.Dev), inode: st.Ino}, nil
}

func verifyArchiveFileAt(directory int, name string) error {
	var st unix.Stat_t
	if err := unix.Fstatat(directory, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || st.Mode&0o077 != 0 {
		return errors.New("Ma'at knowledge archive file identity or permissions are unsafe")
	}
	return nil
}

func descriptorIdentity(fd int) (fileIdentity, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fileIdentity{}, err
	}
	return fileIdentity{device: uint64(st.Dev), inode: st.Ino}, nil
}

func verifyArchiveFile(fd int) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || st.Mode&0o077 != 0 {
		return errors.New("Ma'at knowledge archive file identity or permissions are unsafe")
	}
	return nil
}

func verifySidecars(directory int) error {
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		var st unix.Stat_t
		err := unix.Fstatat(directory, "archive.sqlite3"+suffix, &st, unix.AT_SYMLINK_NOFOLLOW)
		if err == unix.ENOENT {
			continue
		}
		if err != nil {
			return errors.New("Ma'at knowledge archive sidecar is not safe")
		}
		if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || st.Mode&0o077 != 0 {
			return errors.New("Ma'at knowledge archive sidecar identity or permissions are unsafe")
		}
	}
	return nil
}

func readLegacyArchiveFile(path string, limit int64) ([]byte, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.New("legacy knowledge path is invalid")
	}
	parent, err := openAbsoluteDirectory(filepath.Dir(absolute))
	if err != nil {
		return nil, errors.New("legacy knowledge parent is not safe")
	}
	defer unix.Close(parent)
	fd, err := unix.Openat(parent, filepath.Base(absolute), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("legacy knowledge file could not be opened safely")
	}
	file := os.NewFile(uintptr(fd), "legacy knowledge")
	defer file.Close()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Uid != uint32(os.Geteuid()) || before.Nlink != 1 || before.Size < 0 || before.Size > limit {
		return nil, errors.New("legacy knowledge file identity or size is unsafe")
	}
	beforeInfo, err := file.Stat()
	if err != nil {
		return nil, errors.New("legacy knowledge file could not be verified")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("legacy knowledge file could not be read within its bound")
	}
	var after unix.Stat_t
	afterInfo, statErr := file.Stat()
	if err := unix.Fstat(fd, &after); err != nil || statErr != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Size != after.Size || !os.SameFile(beforeInfo, afterInfo) || !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		return nil, errors.New("legacy knowledge file changed during read")
	}
	return data, nil
}

func openAbsoluteDirectory(path string) (int, error) {
	if !filepath.IsAbs(path) {
		return -1, errors.New("directory path must be absolute")
	}
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, component := range strings.Split(strings.Trim(filepath.Clean(path), string(filepath.Separator)), string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		next, openErr := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			return -1, openErr
		}
		fd = next
	}
	return fd, nil
}

func openChildDirectory(parent int, name string, private bool) (int, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		if mkErr := unix.Mkdirat(parent, name, 0o700); mkErr != nil && mkErr != unix.EEXIST {
			return -1, errors.New("Ma'at knowledge directory could not be created")
		}
		fd, err = unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	}
	if err != nil {
		return -1, errors.New("Ma'at knowledge directory is not a safe directory")
	}
	if err := ownedDirectory(fd, private); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func openExistingChildDirectory(parent int, name string) (int, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, errors.New("Ma'at knowledge directory is not a safe directory")
	}
	return fd, nil
}

func ownedDirectory(fd int, private bool) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != uint32(os.Geteuid()) {
		return errors.New("Ma'at knowledge directory owner or type is unsafe")
	}
	if private && st.Mode&0o077 != 0 {
		return errors.New("Ma'at knowledge directory is not private")
	}
	if !private && st.Mode&0o022 != 0 {
		return errors.New("Ma'at knowledge parent is writable by another account")
	}
	return nil
}
