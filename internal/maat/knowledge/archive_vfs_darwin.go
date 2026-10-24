//go:build darwin && cgo

package knowledge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"
	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

// The archive VFS is a per-archive, non-default wrapper around the pinned
// modernc.org/sqlite v1.44.0 Darwin unix-posix VFS. SQLite's Unix VFS owns
// locking, deferred closes, and file I/O; this layer owns only the path
// namespace, retained descriptors, and directory-entry durability.
type archiveAuthority struct {
	mu            sync.Mutex
	home          string
	path          string
	fds           [5]int
	identities    [5]fileIdentity
	refs          int
	database      int
	retiring      bool
	retirementErr error
	files         map[int]fileIdentity
	names         map[string]fileIdentity
	active        map[int]int
}

type archiveVFS struct {
	mu        sync.Mutex
	name      string
	logical   string
	authority *archiveAuthority
	tls       *libc.TLS
	namePtr   uintptr
	vfsPtr    uintptr
	id        uintptr
	files     map[uintptr]archiveVFSFile
	bridges   []uintptr
	closed    bool
}

type archiveVFSFile struct {
	vfs       *archiveVFS
	authority *archiveAuthority
	seedFD    int
	name      string
	journal   bool
	delete    bool
	methods   uintptr
	original  sqlite3.Tsqlite3_io_methods
}

var (
	archiveAuthorityMu         sync.Mutex
	archiveAuthorities         = map[string]*archiveAuthority{}
	archiveAuthorityByDatabase = map[fileIdentity]*archiveAuthority{}
	archiveVFSID               atomic.Uint64
	archiveVFSStatesMu         sync.Mutex
	archiveVFSStates           = map[uintptr]*archiveVFS{}
)

func prepareDarwinSecureArchiveLocation(home string) (*secureArchiveLocation, error) {
	if !filepath.IsAbs(home) {
		return nil, errors.New("Ma'at knowledge home must be absolute")
	}
	home = filepath.Clean(home)
	root, err := openAbsoluteDirectory(home)
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
	dirs := [4]int{root, sirsi, maat, knowledgeDir}
	var identities [5]fileIdentity
	for index, fd := range dirs {
		identities[index], err = descriptorIdentity(fd)
		if err != nil {
			closeArchiveFDs(dirs[:])
			return nil, errors.New("Ma'at knowledge archive path identity is unavailable")
		}
	}
	key := archiveAuthorityKey(home, identities)

	archiveAuthorityMu.Lock()
	authority := archiveAuthorities[key]
	if authority != nil {
		if authority.retiring {
			archiveAuthorityMu.Unlock()
			closeArchiveFDs(dirs[:])
			return nil, errors.New("Ma'at knowledge archive authority is retiring")
		}
		if err := verifyArchiveFileAt(knowledgeDir, "archive.sqlite3"); err != nil {
			archiveAuthorityMu.Unlock()
			closeArchiveFDs(dirs[:])
			return nil, err
		}
		leaf, statErr := fileIdentityAt(knowledgeDir, "archive.sqlite3")
		if statErr != nil || leaf != authority.identities[4] {
			archiveAuthorityMu.Unlock()
			closeArchiveFDs(dirs[:])
			return nil, errors.New("Ma'at knowledge archive leaf identity changed")
		}
		authority.refs++
		archiveAuthorityMu.Unlock()
		closeArchiveFDs(dirs[:])
	} else {
		var st unix.Stat_t
		created := false
		var existingIdentity fileIdentity
		statErr := unix.Fstatat(knowledgeDir, "archive.sqlite3", &st, unix.AT_SYMLINK_NOFOLLOW)
		if statErr != nil && statErr != unix.ENOENT {
			archiveAuthorityMu.Unlock()
			closeArchiveFDs(dirs[:])
			return nil, errors.New("Ma'at knowledge archive file is not safe")
		}
		if statErr == nil {
			if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || st.Mode&0o077 != 0 {
				archiveAuthorityMu.Unlock()
				closeArchiveFDs(dirs[:])
				return nil, errors.New("Ma'at knowledge archive file is not safe")
			}
			identity := fileIdentity{device: uint64(st.Dev), inode: st.Ino}
			existingIdentity = identity
			if registered := archiveAuthorityByDatabase[identity]; registered != nil {
				archiveAuthorityMu.Unlock()
				closeArchiveFDs(dirs[:])
				return nil, errors.New("Ma'at knowledge database inode already has a process authority")
			}
		}
		flags := unix.O_RDWR | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if statErr == unix.ENOENT {
			flags |= unix.O_CREAT | unix.O_EXCL
			created = true
		}
		database, openErr := unix.Openat(knowledgeDir, "archive.sqlite3", flags, 0o600)
		if openErr != nil {
			archiveAuthorityMu.Unlock()
			closeArchiveFDs(dirs[:])
			return nil, errors.New("Ma'at knowledge archive file is not safe")
		}
		if err := verifyArchiveFile(database); err != nil {
			_ = unix.Close(database)
			archiveAuthorityMu.Unlock()
			closeArchiveFDs(dirs[:])
			return nil, err
		}
		identities[4], err = descriptorIdentity(database)
		if err != nil {
			_ = unix.Close(database)
			archiveAuthorityMu.Unlock()
			closeArchiveFDs(dirs[:])
			return nil, errors.New("Ma'at knowledge archive file identity is unavailable")
		}
		if existingIdentity != (fileIdentity{}) && identities[4] != existingIdentity {
			_ = unix.Close(database)
			archiveAuthorityMu.Unlock()
			closeArchiveFDs(dirs[:])
			return nil, errors.New("Ma'at knowledge archive leaf changed during open")
		}
		if registered := archiveAuthorityByDatabase[identities[4]]; registered != nil {
			_ = unix.Close(database)
			archiveAuthorityMu.Unlock()
			closeArchiveFDs(dirs[:])
			return nil, errors.New("Ma'at knowledge database inode already has a process authority")
		}
		if created {
			if err := unix.Fsync(knowledgeDir); err != nil {
				_ = unix.Close(database)
				_ = unix.Unlinkat(knowledgeDir, "archive.sqlite3", 0)
				archiveAuthorityMu.Unlock()
				closeArchiveFDs(dirs[:])
				return nil, errors.New("Ma'at knowledge archive directory entry could not be synced")
			}
		}
		allFDs := [5]int{root, sirsi, maat, knowledgeDir, database}
		authority = &archiveAuthority{
			home: home, path: filepath.Join(home, ".sirsi", "maat", "knowledge", "archive.sqlite3"),
			fds: allFDs, identities: identities, refs: 1, database: database,
			files:  map[int]fileIdentity{database: identities[4]},
			names:  map[string]fileIdentity{"archive.sqlite3": identities[4]},
			active: make(map[int]int),
		}
		if err := verifySidecars(knowledgeDir); err != nil {
			_ = unix.Close(database)
			closeArchiveFDs(dirs[:])
			archiveAuthorityMu.Unlock()
			return nil, err
		}
		archiveAuthorities[key] = authority
		archiveAuthorityByDatabase[identities[4]] = authority
		archiveAuthorityMu.Unlock()
	}
	if err := verifySidecars(authority.fds[3]); err != nil {
		_ = authority.release()
		return nil, err
	}
	location := &secureArchiveLocation{
		home: home, path: authority.path, rootFD: authority.fds[0], sirsiFD: authority.fds[1],
		maatFD: authority.fds[2], archiveFD: authority.fds[3], database: authority.database,
		identities: authority.identities, authority: authority,
	}
	vfs, err := registerArchiveVFS(authority)
	if err != nil {
		_ = authority.release()
		return nil, err
	}
	location.vfs = vfs
	location.vfsName = vfs.name
	return location, nil
}

func archiveAuthorityKey(home string, ids [5]fileIdentity) string {
	var b strings.Builder
	b.WriteString(home)
	for _, id := range ids[:4] {
		fmt.Fprintf(&b, ":%x:%x", id.device, id.inode)
	}
	return b.String()
}

func closeArchiveFDs(fds []int) {
	for _, fd := range fds {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
	}
}

func (a *archiveAuthority) release() error {
	archiveAuthorityMu.Lock()
	defer archiveAuthorityMu.Unlock()
	if a == nil {
		return errors.New("Ma'at knowledge archive authority is unavailable")
	}
	if a.retiring {
		a.mu.Lock()
		defer a.mu.Unlock()
		if err := a.retireDescriptors(unix.Close); err != nil {
			a.refs = 1
			return errors.New("Ma'at knowledge archive authority retirement is incomplete")
		}
		a.refs = 0
		deleteArchiveAuthority(a)
		return nil
	}
	if a.refs <= 0 {
		return errors.New("Ma'at knowledge archive authority is already retired")
	}
	a.refs--
	if a.refs != 0 {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.active) != 0 {
		a.refs = 1
		return errors.New("Ma'at knowledge SQLite handles remain open")
	}
	a.retiring = true
	if err := a.retireDescriptors(unix.Close); err != nil {
		a.refs = 1
		return errors.New("Ma'at knowledge archive authority retirement is incomplete")
	}
	a.refs = 0
	deleteArchiveAuthority(a)
	return nil
}

func (a *archiveAuthority) retireDescriptors(closeFD func(int) error) error {
	// A close error is ambiguous: the OS may already have consumed the number.
	// Consume ownership before calling close, retain the error as permanent
	// retirement debt, and continue retiring independent descriptors. Never
	// retry a number whose close outcome was ambiguous.
	for fd := range a.files {
		delete(a.files, fd)
		if fd == a.database {
			a.database = -1
			a.fds[4] = -1
		}
		if err := closeFD(fd); err != nil {
			a.recordRetirementError(fmt.Errorf("retire archive descriptor %d: %w", fd, err))
		}
	}
	for index := 3; index >= 0; index-- {
		fd := a.fds[index]
		if fd < 0 {
			continue
		}
		a.fds[index] = -1
		if err := closeFD(fd); err != nil {
			a.recordRetirementError(fmt.Errorf("retire archive directory descriptor %d: %w", fd, err))
		}
	}
	return a.retirementErr
}

func (a *archiveAuthority) recordRetirementError(err error) {
	if err != nil {
		a.retirementErr = errors.Join(a.retirementErr, err)
	}
}

func deleteArchiveAuthority(authority *archiveAuthority) {
	for key, registered := range archiveAuthorities {
		if registered == authority {
			delete(archiveAuthorities, key)
			break
		}
	}
	if authority.identities[4] != (fileIdentity{}) && archiveAuthorityByDatabase[authority.identities[4]] == authority {
		delete(archiveAuthorityByDatabase, authority.identities[4])
	}
}

func (a *archiveAuthority) verifyNamespace() error {
	p := &secureArchiveLocation{home: a.home, path: a.path, rootFD: a.fds[0], sirsiFD: a.fds[1], maatFD: a.fds[2], archiveFD: a.fds[3], database: a.database, identities: a.identities}
	return p.verify()
}

func (a *archiveAuthority) openRetainedFile(name string, flags int32) (int, bool, error) {
	if err := a.verifyNamespace(); err != nil {
		return -1, false, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.retirementErr != nil {
		return -1, false, errors.Join(errors.New("Ma'at knowledge archive authority has unresolved descriptor-retirement debt"), a.retirementErr)
	}
	var st unix.Stat_t
	statErr := unix.Fstatat(a.fds[3], name, &st, unix.AT_SYMLINK_NOFOLLOW)
	if id, ok := a.names[name]; ok {
		observed := fileIdentity{device: uint64(st.Dev), inode: st.Ino}
		if statErr != nil || observed != id || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || st.Mode&0o077 != 0 {
			return -1, false, errors.New("Ma'at knowledge archive entry changed")
		}
		fd, exists := findRetainedIdentity(a.files, id)
		if exists {
			if flags&sqlite3.SQLITE_OPEN_EXCLUSIVE != 0 {
				return -1, false, unix.EEXIST
			}
			if err := verifyArchiveFile(fd); err != nil {
				return -1, false, err
			}
			a.active[fd]++
			return fd, false, nil
		}
	}
	err := statErr
	created := false
	if err == unix.ENOENT {
		if flags&sqlite3.SQLITE_OPEN_CREATE == 0 {
			return -1, false, err
		}
		created = true
	} else if err != nil {
		return -1, false, err
	} else if flags&sqlite3.SQLITE_OPEN_EXCLUSIVE != 0 {
		return -1, false, unix.EEXIST
	}
	openFlags := unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
	if flags&sqlite3.SQLITE_OPEN_READONLY != 0 {
		openFlags |= unix.O_RDONLY
	} else {
		openFlags |= unix.O_RDWR
	}
	if created {
		openFlags |= unix.O_CREAT | unix.O_EXCL
	}
	fd, err := unix.Openat(a.fds[3], name, openFlags, 0o600)
	if err != nil {
		return -1, false, err
	}
	if err := verifyArchiveFile(fd); err != nil {
		_ = unix.Close(fd)
		return -1, false, err
	}
	id, err := descriptorIdentity(fd)
	if err != nil {
		_ = unix.Close(fd)
		return -1, false, err
	}
	if prior, ok := findRetainedIdentity(a.files, id); ok {
		_ = unix.Close(fd)
		fd = prior
	} else {
		a.files[fd] = id
	}
	a.names[name] = id
	if created {
		if err := unix.Fsync(a.fds[3]); err != nil {
			if fd != a.database {
				_ = unix.Close(fd)
				delete(a.files, fd)
			}
			_ = unix.Unlinkat(a.fds[3], name, 0)
			delete(a.names, name)
			return -1, false, err
		}
	}
	a.active[fd]++
	return fd, created, nil
}

func findRetainedIdentity(files map[int]fileIdentity, id fileIdentity) (int, bool) {
	for fd, existing := range files {
		if existing == id {
			return fd, true
		}
	}
	return -1, false
}

func (a *archiveAuthority) innerClosed(fd int) {
	a.innerClosedWith(fd, unix.Close)
}

func (a *archiveAuthority) innerClosedWith(fd int, closeFD func(int) error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active[fd] > 1 {
		a.active[fd]--
		return
	}
	delete(a.active, fd)
	if fd == a.database {
		return
	}
	if identity, ok := a.files[fd]; ok {
		// Ownership is consumed before close: an error does not establish that
		// the descriptor remains open, and fstat/retry could target a reused FD.
		delete(a.files, fd)
		if err := closeFD(fd); err != nil {
			a.recordRetirementError(fmt.Errorf("close archive sidecar descriptor %d (%x:%x): %w", fd, identity.device, identity.inode, err))
		}
	}
}

func (a *archiveAuthority) deleteFile(name string) error {
	if err := a.verifyNamespace(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	current, err := fileIdentityAt(a.fds[3], name)
	if err == unix.ENOENT {
		return nil
	}
	if err != nil {
		return err
	}
	expected, ok := a.names[name]
	if !ok {
		return errors.New("Ma'at knowledge archive entry has no retained identity")
	}
	if current != expected {
		return errors.New("Ma'at knowledge archive entry identity changed before deletion")
	}
	if err := verifyArchiveFileAt(a.fds[3], name); err != nil {
		return err
	}
	if err := unix.Unlinkat(a.fds[3], name, 0); err != nil {
		return err
	}
	if err := unix.Fsync(a.fds[3]); err != nil {
		return err
	}
	delete(a.names, name)
	return nil
}

func (a *archiveAuthority) accessFile(tls *libc.TLS, name string, result uintptr) int32 {
	if err := a.verifyNamespace(); err != nil {
		return sqlite3.SQLITE_IOERR_ACCESS
	}
	var st unix.Stat_t
	if err := unix.Fstatat(a.fds[3], name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if err == unix.ENOENT {
			archiveWriteCInt32(tls, result, 0)
			return sqlite3.SQLITE_OK
		}
		return sqlite3.SQLITE_IOERR_ACCESS
	}
	// This archive uses durable DELETE journaling. A sidecar may be queried by
	// SQLite while deciding whether WAL is present, but it is never opened or
	// accepted as an existing archive file.
	if name == "archive.sqlite3-wal" || name == "archive.sqlite3-shm" {
		return sqlite3.SQLITE_IOERR_ACCESS
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || st.Mode&0o077 != 0 {
		return sqlite3.SQLITE_IOERR_ACCESS
	}
	archiveWriteCInt32(tls, result, 1)
	return sqlite3.SQLITE_OK
}

func (a *archiveAuthority) validName(name string) (string, bool) {
	switch name {
	case "archive.sqlite3":
		return name, false
	case "archive.sqlite3-journal":
		return name, true
	default:
		const tempPrefix = "archive.sqlite3-tmp-"
		if strings.HasPrefix(name, tempPrefix) && !strings.ContainsAny(strings.TrimPrefix(name, tempPrefix), "/\\") {
			return name, strings.HasSuffix(name, "-journal")
		}
		return "", false
	}
}

func registerArchiveVFS(authority *archiveAuthority) (*archiveVFS, error) {
	tls := libc.NewTLS()
	baseName, err := libc.CString("unix-posix")
	if err != nil {
		tls.Close()
		return nil, errors.New("Ma'at knowledge SQLite POSIX VFS is unavailable")
	}
	basePtr := sqlite3.Xsqlite3_vfs_find(tls, baseName)
	libc.Xfree(tls, baseName)
	if basePtr == 0 {
		tls.Close()
		return nil, errors.New("Ma'at knowledge SQLite POSIX VFS is unavailable")
	}
	base := archiveReadSQLiteVFSConfiguration(tls, basePtr)
	if base.FszOsFile < int32(unsafe.Sizeof(sqlite3.TunixFile{})) {
		tls.Close()
		return nil, errors.New("Ma'at knowledge SQLite POSIX file ABI is unsupported")
	}
	id := uintptr(archiveVFSID.Add(1))
	name := fmt.Sprintf("maat-archive-%d-%s", os.Getpid(), strconv.FormatUint(uint64(id), 16))
	namePtr, err := libc.CString(name)
	if err != nil {
		tls.Close()
		return nil, errors.New("Ma'at knowledge SQLite VFS could not be allocated")
	}
	vfsPtr := libc.Xmalloc(tls, libc.Tsize_t(unsafe.Sizeof(sqlite3.Tsqlite3_vfs{})))
	if vfsPtr == 0 {
		libc.Xfree(tls, namePtr)
		tls.Close()
		return nil, errors.New("Ma'at knowledge SQLite VFS could not be allocated")
	}
	wrapped := base
	wrapped.FpNext, wrapped.FzName, wrapped.FpAppData = 0, namePtr, id
	*(*func(*libc.TLS, uintptr, uintptr, uintptr, int32, uintptr) int32)(unsafe.Pointer(&wrapped.FxOpen)) = archiveVFSOpen
	*(*func(*libc.TLS, uintptr, uintptr, int32) int32)(unsafe.Pointer(&wrapped.FxDelete)) = archiveVFSDelete
	*(*func(*libc.TLS, uintptr, uintptr, int32, uintptr) int32)(unsafe.Pointer(&wrapped.FxAccess)) = archiveVFSAccess
	*(*func(*libc.TLS, uintptr, uintptr, int32, uintptr) int32)(unsafe.Pointer(&wrapped.FxFullPathname)) = archiveVFSFullPathname
	archiveCopyToC(tls, vfsPtr, unsafe.Pointer(&wrapped), unsafe.Sizeof(wrapped))
	vfs := &archiveVFS{name: name, logical: "/maat-archive/" + name + "/archive.sqlite3", authority: authority, tls: tls, namePtr: namePtr, vfsPtr: vfsPtr, id: id, files: make(map[uintptr]archiveVFSFile)}
	archiveVFSStatesMu.Lock()
	archiveVFSStates[id] = vfs
	archiveVFSStatesMu.Unlock()
	if rc := sqlite3.Xsqlite3_vfs_register(tls, vfsPtr, 0); rc != sqlite3.SQLITE_OK {
		archiveVFSStatesMu.Lock()
		delete(archiveVFSStates, id)
		archiveVFSStatesMu.Unlock()
		libc.Xfree(tls, vfsPtr)
		libc.Xfree(tls, namePtr)
		tls.Close()
		return nil, errors.New("Ma'at knowledge SQLite VFS could not be registered")
	}
	return vfs, nil
}

func (v *archiveVFS) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return nil
	}
	if len(v.files) != 0 {
		return errors.New("Ma'at knowledge SQLite VFS still has open files")
	}
	if rc := sqlite3.Xsqlite3_vfs_unregister(v.tls, v.vfsPtr); rc != sqlite3.SQLITE_OK {
		return errors.New("Ma'at knowledge SQLite VFS could not be unregistered")
	}
	archiveVFSStatesMu.Lock()
	delete(archiveVFSStates, v.id)
	archiveVFSStatesMu.Unlock()
	for _, ptr := range v.bridges {
		libc.Xfree(v.tls, ptr)
	}
	libc.Xfree(v.tls, v.vfsPtr)
	libc.Xfree(v.tls, v.namePtr)
	v.tls.Close()
	v.closed = true
	return nil
}

func archiveVFSFor(tls *libc.TLS, pVfs uintptr) *archiveVFS {
	if pVfs == 0 {
		return nil
	}
	id := archiveReadCUintptr(tls, pVfs+unsafe.Offsetof(sqlite3.Tsqlite3_vfs{}.FpAppData))
	archiveVFSStatesMu.Lock()
	v := archiveVFSStates[id]
	archiveVFSStatesMu.Unlock()
	return v
}

func archiveVFSOpen(tls *libc.TLS, pVfs, zName, pFile uintptr, flags int32, pOutFlags uintptr) int32 {
	v := archiveVFSFor(tls, pVfs)
	if v == nil || pFile == 0 {
		return sqlite3.SQLITE_CANTOPEN
	}
	outFlags := pOutFlags
	if outFlags == 0 {
		outFlags = libc.Xmalloc(tls, libc.Tsize_t(unsafe.Sizeof(int32(0))))
		if outFlags == 0 {
			return sqlite3.SQLITE_NOMEM
		}
		defer libc.Xfree(tls, outFlags)
	}
	name := ""
	deleteOnClose := flags&sqlite3.SQLITE_OPEN_DELETEONCLOSE != 0
	if zName == 0 {
		if !deleteOnClose {
			return sqlite3.SQLITE_CANTOPEN
		}
		name = fmt.Sprintf("archive.sqlite3-tmp-%d-%x", os.Getpid(), archiveVFSID.Add(1))
		flags |= sqlite3.SQLITE_OPEN_CREATE | sqlite3.SQLITE_OPEN_EXCLUSIVE | sqlite3.SQLITE_OPEN_DELETEONCLOSE
	} else {
		name = libc.GoString(zName)
		if !strings.HasPrefix(name, v.logical) {
			return sqlite3.SQLITE_CANTOPEN
		}
		name = strings.TrimPrefix(name, "/maat-archive/"+v.name+"/")
	}
	name, journal := v.authority.validName(name)
	if name == "" || flags&sqlite3.SQLITE_OPEN_WAL != 0 || flags&sqlite3.SQLITE_OPEN_SUPER_JOURNAL != 0 {
		return sqlite3.SQLITE_CANTOPEN
	}
	fd, _, err := v.authority.openRetainedFile(name, flags)
	if err != nil {
		return sqlite3.SQLITE_CANTOPEN
	}
	// The v1.44.0 Darwin unix-posix VFS recognizes this regular-file descriptor
	// path and duplicates the already-open descriptor. Keep the C filename and
	// seed descriptor alive until its delegated unixFile closes so SQLite's
	// inode lock table and deferred-close rules continue to own that lifetime.
	bridgePath, err := libc.CString("/dev/fd/" + strconv.Itoa(fd))
	if err != nil {
		v.authority.innerClosed(fd)
		return sqlite3.SQLITE_NOMEM
	}
	baseName, err := libc.CString("unix-posix")
	if err != nil {
		libc.Xfree(tls, bridgePath)
		v.authority.innerClosed(fd)
		return sqlite3.SQLITE_NOMEM
	}
	basePtr := sqlite3.Xsqlite3_vfs_find(tls, baseName)
	libc.Xfree(tls, baseName)
	if basePtr == 0 {
		libc.Xfree(tls, bridgePath)
		v.authority.innerClosed(fd)
		return sqlite3.SQLITE_CANTOPEN
	}
	base := archiveReadSQLiteVFSConfiguration(tls, basePtr)
	baseOpen := *(*func(*libc.TLS, uintptr, uintptr, uintptr, int32, uintptr) int32)(unsafe.Pointer(&base.FxOpen))
	baseFlags := flags &^ (sqlite3.SQLITE_OPEN_CREATE | sqlite3.SQLITE_OPEN_EXCLUSIVE | sqlite3.SQLITE_OPEN_DELETEONCLOSE | sqlite3.SQLITE_OPEN_URI)
	rc := baseOpen(tls, basePtr, bridgePath, pFile, baseFlags, outFlags)
	if rc != sqlite3.SQLITE_OK {
		libc.Xfree(tls, bridgePath)
		v.authority.innerClosed(fd)
		return rc
	}
	openedFile := archiveReadUnixFile(tls, pFile)
	innerFD := int(openedFile.Fh)
	var delegated unix.Stat_t
	var retained unix.Stat_t
	if innerFD < 0 || unix.Fstat(innerFD, &delegated) != nil || unix.Fstat(fd, &retained) != nil || delegated.Dev != retained.Dev || delegated.Ino != retained.Ino {
		closeOpenedBaseFile(tls, pFile)
		libc.Xfree(tls, bridgePath)
		v.authority.innerClosed(fd)
		return sqlite3.SQLITE_CANTOPEN
	}
	if archiveReadCInt32(tls, outFlags)&sqlite3.SQLITE_OPEN_READONLY != 0 {
		closeOpenedBaseFile(tls, pFile)
		libc.Xfree(tls, bridgePath)
		v.authority.innerClosed(fd)
		return sqlite3.SQLITE_CANTOPEN
	}
	if err := archiveSetCloseOnExec(innerFD); err != nil {
		closeOpenedBaseFile(tls, pFile)
		libc.Xfree(tls, bridgePath)
		v.authority.innerClosed(fd)
		return sqlite3.SQLITE_CANTOPEN
	}
	methodsAddress := openedFile.FpMethod
	if methodsAddress == 0 {
		closeOpenedBaseFile(tls, pFile)
		libc.Xfree(tls, bridgePath)
		v.authority.innerClosed(fd)
		return sqlite3.SQLITE_CANTOPEN
	}
	methods := archiveReadIOMethods(tls, methodsAddress)
	if methods.FxClose == 0 || methods.FxWrite == 0 || methods.FxSync == 0 || methods.FxFileControl == 0 {
		closeOpenedBaseFile(tls, pFile)
		libc.Xfree(tls, bridgePath)
		v.authority.innerClosed(fd)
		return sqlite3.SQLITE_CANTOPEN
	}
	methodCopy := libc.Xmalloc(tls, libc.Tsize_t(unsafe.Sizeof(sqlite3.Tsqlite3_io_methods{})))
	if methodCopy == 0 {
		closeOpenedBaseFile(tls, pFile)
		libc.Xfree(tls, bridgePath)
		v.authority.innerClosed(fd)
		return sqlite3.SQLITE_NOMEM
	}
	wrappedMethods := methods
	// The archive uses durable DELETE rollback journals and deliberately has no
	// shared-memory namespace. SQLite decides whether WAL is supported from the
	// main database file's I/O-method version and xShmMap callback before it
	// attempts the journal-mode transition. The delegated unix-posix methods
	// advertise SHM and mmap support, so exposing them here would let SQLite
	// switch its in-memory pager mode to WAL before our later header-write guard
	// refuses the durable header change. Keep the wrapped method table at v1 and
	// hide every v2/v3 optional capability; the retained-root VFS remains the
	// authority for files, while unix-posix continues to own rollback locking,
	// I/O, and sync for the v1 interface.
	wrappedMethods.FiVersion = 1
	wrappedMethods.FxShmMap = 0
	wrappedMethods.FxShmLock = 0
	wrappedMethods.FxShmBarrier = 0
	wrappedMethods.FxShmUnmap = 0
	wrappedMethods.FxFetch = 0
	wrappedMethods.FxUnfetch = 0
	original := wrappedMethods
	*(*func(*libc.TLS, uintptr) int32)(unsafe.Pointer(&wrappedMethods.FxClose)) = archiveVFSFileClose
	*(*func(*libc.TLS, uintptr, uintptr, int32, sqlite3.Tsqlite3_int64) int32)(unsafe.Pointer(&wrappedMethods.FxWrite)) = archiveVFSFileWrite
	*(*func(*libc.TLS, uintptr, int32) int32)(unsafe.Pointer(&wrappedMethods.FxSync)) = archiveVFSFileSync
	*(*func(*libc.TLS, uintptr, int32, uintptr) int32)(unsafe.Pointer(&wrappedMethods.FxFileControl)) = archiveVFSFileControl
	archiveCopyToC(tls, methodCopy, unsafe.Pointer(&wrappedMethods), unsafe.Sizeof(wrappedMethods))
	archiveWriteCUintptr(tls, pFile+unsafe.Offsetof(sqlite3.TunixFile{}.FpMethod), methodCopy)
	bridge := archiveVFSFile{vfs: v, authority: v.authority, seedFD: fd, name: name, journal: journal, delete: deleteOnClose, methods: methodCopy, original: original}
	v.mu.Lock()
	v.files[pFile] = bridge
	v.bridges = append(v.bridges, bridgePath)
	v.mu.Unlock()
	vfsFileMu.Lock()
	archiveVFSFiles[pFile] = bridge
	vfsFileMu.Unlock()
	return sqlite3.SQLITE_OK
}

func archiveSetCloseOnExec(fd int) error {
	return archiveSetCloseOnExecWith(fd, unix.FcntlInt)
}

func archiveSetCloseOnExecWith(fd int, fcntl func(uintptr, int, int) (int, error)) error {
	flags, err := fcntl(uintptr(fd), unix.F_GETFD, 0)
	if err != nil {
		return err
	}
	if flags&unix.FD_CLOEXEC == 0 {
		if _, err := fcntl(uintptr(fd), unix.F_SETFD, flags|unix.FD_CLOEXEC); err != nil {
			return err
		}
	}
	flags, err = fcntl(uintptr(fd), unix.F_GETFD, 0)
	if err != nil {
		return err
	}
	if flags&unix.FD_CLOEXEC == 0 {
		return errors.New("SQLite descriptor did not retain FD_CLOEXEC")
	}
	return nil
}

func closeOpenedBaseFile(tls *libc.TLS, pFile uintptr) {
	file := archiveReadUnixFile(tls, pFile)
	if file.FpMethod == 0 {
		return
	}
	methods := archiveReadIOMethods(tls, file.FpMethod)
	if methods.FxClose != 0 {
		closeFn := *(*func(*libc.TLS, uintptr) int32)(unsafe.Pointer(&methods.FxClose))
		_ = closeFn(tls, pFile)
	}
}

func archiveVFSFileClose(tls *libc.TLS, pFile uintptr) int32 {
	vfsFileMu.Lock()
	bridge, ok := archiveVFSFiles[pFile]
	delete(archiveVFSFiles, pFile)
	vfsFileMu.Unlock()
	if !ok {
		return sqlite3.SQLITE_IOERR_CLOSE
	}
	closeFn := *(*func(*libc.TLS, uintptr) int32)(unsafe.Pointer(&bridge.original.FxClose))
	rc := closeFn(tls, pFile)
	libc.Xfree(tls, bridge.methods)
	bridge.authority.innerClosed(bridge.seedFD)
	if bridge.delete {
		if err := bridge.authority.deleteFile(bridge.name); err != nil && rc == sqlite3.SQLITE_OK {
			rc = sqlite3.SQLITE_IOERR_DELETE
		}
	}
	bridge.vfs.mu.Lock()
	delete(bridge.vfs.files, pFile)
	bridge.vfs.mu.Unlock()
	return rc
}

func archiveVFSFileWrite(tls *libc.TLS, pFile, pBuffer uintptr, amount int32, offset sqlite3.Tsqlite3_int64) int32 {
	vfsFileMu.Lock()
	bridge, ok := archiveVFSFiles[pFile]
	vfsFileMu.Unlock()
	if !ok || bridge.original.FxWrite == 0 || amount < 0 || (amount > 0 && pBuffer == 0) {
		return sqlite3.SQLITE_IOERR_WRITE
	}
	if bridge.name == "archive.sqlite3" && archiveWriteEnablesWALHeader(tls, pBuffer, amount, int64(offset)) {
		// SQLite stores the rollback/WAL read and write format bytes at header
		// offsets 18 and 19. Refuse the transition at the only write boundary
		// before either durable header byte can leave rollback-journal mode.
		return sqlite3.SQLITE_READONLY
	}
	writeFn := *(*func(*libc.TLS, uintptr, uintptr, int32, sqlite3.Tsqlite3_int64) int32)(unsafe.Pointer(&bridge.original.FxWrite))
	return writeFn(tls, pFile, pBuffer, amount, offset)
}

func archiveWriteEnablesWALHeader(tls *libc.TLS, pBuffer uintptr, amount int32, offset int64) bool {
	if pBuffer == 0 || amount <= 0 || offset < 0 {
		return false
	}
	end := offset + int64(amount)
	if end < offset {
		return false
	}
	for _, headerOffset := range [...]int64{18, 19} {
		if headerOffset >= offset && headerOffset < end {
			if archiveReadCByte(tls, pBuffer+uintptr(headerOffset-offset)) == 2 {
				return true
			}
		}
	}
	return false
}

func archiveVFSFileSync(tls *libc.TLS, pFile uintptr, flags int32) int32 {
	vfsFileMu.Lock()
	bridge, ok := archiveVFSFiles[pFile]
	vfsFileMu.Unlock()
	if !ok {
		return sqlite3.SQLITE_IOERR_FSYNC
	}
	syncFn := *(*func(*libc.TLS, uintptr, int32) int32)(unsafe.Pointer(&bridge.original.FxSync))
	if rc := syncFn(tls, pFile, flags); rc != sqlite3.SQLITE_OK {
		return rc
	}
	if bridge.journal && unix.Fsync(bridge.authority.fds[3]) != nil {
		return sqlite3.SQLITE_IOERR_DIR_FSYNC
	}
	return sqlite3.SQLITE_OK
}

func archiveVFSFileControl(tls *libc.TLS, pFile uintptr, op int32, pArg uintptr) int32 {
	vfsFileMu.Lock()
	bridge, ok := archiveVFSFiles[pFile]
	vfsFileMu.Unlock()
	if !ok {
		return sqlite3.SQLITE_NOTFOUND
	}
	switch op {
	case int32(sqlite3.SQLITE_FCNTL_HAS_MOVED):
		if pArg == 0 {
			return sqlite3.SQLITE_MISUSE
		}
		archiveWriteCInt32(tls, pArg, archiveFileHasMoved(&bridge))
		return sqlite3.SQLITE_OK
	case int32(sqlite3.SQLITE_FCNTL_TEMPFILENAME):
		if pArg == 0 {
			return sqlite3.SQLITE_MISUSE
		}
		name := fmt.Sprintf("%s-tmp-%d-%x", bridge.vfs.logical, os.Getpid(), archiveVFSID.Add(1))
		cName, err := libc.CString(name)
		if err != nil {
			return sqlite3.SQLITE_NOMEM
		}
		defer libc.Xfree(tls, cName)
		allocated := sqlite3.Xsqlite3_malloc64(tls, libc.Uint64FromInt64(int64(len(name)+1)))
		if allocated == 0 {
			return sqlite3.SQLITE_NOMEM
		}
		archiveCopyCAddress(tls, allocated, cName, uintptr(len(name)+1))
		archiveWriteCUintptr(tls, pArg, allocated)
		return sqlite3.SQLITE_OK
	case int32(sqlite3.SQLITE_FCNTL_GET_LOCKPROXYFILE):
		if pArg == 0 {
			return sqlite3.SQLITE_MISUSE
		}
		archiveWriteCUintptr(tls, pArg, 0)
		return sqlite3.SQLITE_OK
	case int32(sqlite3.SQLITE_FCNTL_SET_LOCKPROXYFILE):
		return sqlite3.SQLITE_NOTFOUND
	case int32(sqlite3.SQLITE_FCNTL_VFSNAME):
		if pArg == 0 {
			return sqlite3.SQLITE_MISUSE
		}
		cName, err := libc.CString(bridge.vfs.name)
		if err != nil {
			return sqlite3.SQLITE_NOMEM
		}
		defer libc.Xfree(tls, cName)
		allocated := sqlite3.Xsqlite3_malloc64(tls, libc.Uint64FromInt64(int64(len(bridge.vfs.name)+1)))
		if allocated == 0 {
			return sqlite3.SQLITE_NOMEM
		}
		archiveCopyCAddress(tls, allocated, cName, uintptr(len(bridge.vfs.name)+1))
		archiveWriteCUintptr(tls, pArg, allocated)
		return sqlite3.SQLITE_OK
	case int32(sqlite3.SQLITE_FCNTL_LOCKSTATE), int32(sqlite3.SQLITE_FCNTL_LAST_ERRNO),
		int32(sqlite3.SQLITE_FCNTL_CHUNK_SIZE), int32(sqlite3.SQLITE_FCNTL_SIZE_HINT),
		int32(sqlite3.SQLITE_FCNTL_MMAP_SIZE):
		if pArg == 0 {
			return sqlite3.SQLITE_MISUSE
		}
		if bridge.original.FxFileControl == 0 {
			return sqlite3.SQLITE_NOTFOUND
		}
		control := *(*func(*libc.TLS, uintptr, int32, uintptr) int32)(unsafe.Pointer(&bridge.original.FxFileControl))
		return control(tls, pFile, op, pArg)
	default:
		// Do not let inherited controls reinterpret the /dev/fd bridge as a
		// writable pathname, create a lock proxy, or bypass the retained root.
		return sqlite3.SQLITE_NOTFOUND
	}
}

func archiveFileHasMoved(bridge *archiveVFSFile) int32 {
	if bridge == nil || bridge.authority == nil || bridge.seedFD < 0 {
		return 1
	}
	a := bridge.authority
	if a.verifyNamespace() != nil {
		return 1
	}
	a.mu.Lock()
	expected, known := a.names[bridge.name]
	a.mu.Unlock()
	if !known {
		return 1
	}
	var opened, current unix.Stat_t
	if unix.Fstat(bridge.seedFD, &opened) != nil || unix.Fstatat(a.fds[3], bridge.name, &current, unix.AT_SYMLINK_NOFOLLOW) != nil {
		return 1
	}
	openedID := fileIdentity{device: uint64(opened.Dev), inode: opened.Ino}
	currentID := fileIdentity{device: uint64(current.Dev), inode: current.Ino}
	if openedID != expected || currentID != expected || current.Mode&unix.S_IFMT != unix.S_IFREG || current.Uid != uint32(os.Geteuid()) || current.Nlink != 1 || current.Mode&0o077 != 0 {
		return 1
	}
	return 0
}

var (
	vfsFileMu       sync.Mutex
	archiveVFSFiles = map[uintptr]archiveVFSFile{}
)

func archiveVFSDelete(tls *libc.TLS, pVfs, zPath uintptr, _ int32) int32 {
	v := archiveVFSFor(tls, pVfs)
	if v == nil || zPath == 0 {
		return sqlite3.SQLITE_IOERR_DELETE
	}
	name, ok := archiveVFSName(v, libc.GoString(zPath))
	if !ok || name == "archive.sqlite3" {
		return sqlite3.SQLITE_IOERR_DELETE
	}
	if err := v.authority.deleteFile(name); err != nil && err != unix.ENOENT {
		return sqlite3.SQLITE_IOERR_DELETE
	}
	return sqlite3.SQLITE_OK
}

func archiveVFSAccess(tls *libc.TLS, pVfs, zPath uintptr, _ int32, result uintptr) int32 {
	v := archiveVFSFor(tls, pVfs)
	if v == nil || zPath == 0 || result == 0 {
		return sqlite3.SQLITE_IOERR_ACCESS
	}
	name, ok := archiveVFSName(v, libc.GoString(zPath))
	if !ok {
		name, ok = archiveVFSWALProbeName(v, libc.GoString(zPath))
	}
	if !ok {
		return sqlite3.SQLITE_IOERR_ACCESS
	}
	return v.authority.accessFile(tls, name, result)
}

func archiveVFSFullPathname(tls *libc.TLS, pVfs, zPath uintptr, nOut int32, zOut uintptr) int32 {
	v := archiveVFSFor(tls, pVfs)
	if v == nil || zPath == 0 || zOut == 0 || nOut <= 0 {
		return sqlite3.SQLITE_CANTOPEN_FULLPATH
	}
	input := libc.GoString(zPath)
	if input != v.logical {
		if _, ok := archiveVFSName(v, input); !ok {
			return sqlite3.SQLITE_CANTOPEN_FULLPATH
		}
	}
	if len(input)+1 > int(nOut) {
		return sqlite3.SQLITE_CANTOPEN_FULLPATH
	}
	libc.Xstrncpy(tls, zOut, zPath, libc.Tsize_t(nOut))
	return sqlite3.SQLITE_OK
}

func archiveVFSName(v *archiveVFS, input string) (string, bool) {
	prefix := "/maat-archive/" + v.name + "/"
	if !strings.HasPrefix(input, prefix) {
		return "", false
	}
	name, _ := v.authority.validName(strings.TrimPrefix(input, prefix))
	return name, name != ""
}

func archiveVFSWALProbeName(v *archiveVFS, input string) (string, bool) {
	prefix := "/maat-archive/" + v.name + "/"
	if !strings.HasPrefix(input, prefix) {
		return "", false
	}
	name := strings.TrimPrefix(input, prefix)
	if name != "archive.sqlite3-wal" && name != "archive.sqlite3-shm" {
		return "", false
	}
	return name, true
}
