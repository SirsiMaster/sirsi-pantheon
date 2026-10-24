//go:build darwin && cgo

package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

func TestArchiveDarwinVFSRetainsSharedAuthorityAndRootsTemporaryFiles(t *testing.T) {
	home := t.TempDir()
	first, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(home)
	if err != nil {
		_ = first.Close()
		t.Fatal(err)
	}
	if first.location.authority != second.location.authority {
		t.Fatal("same-process archive handles did not share retained authority descriptors")
	}
	if first.location.vfsName == "" || second.location.vfsName == "" || first.location.vfsName == second.location.vfsName {
		t.Fatal("archive handles did not receive distinct non-default VFS registrations")
	}
	conn, err := first.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `BEGIN IMMEDIATE`); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `UPDATE archive_meta SET value=value WHERE key='schema'`); err != nil {
		_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		_ = conn.Close()
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `CREATE TEMP TABLE archive_vfs_temp (value TEXT)`); err != nil {
		_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		_ = conn.Close()
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		_ = conn.Close()
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `COMMIT`); err != nil {
		_ = conn.Close()
		t.Fatal("closing a sibling handle disturbed the active SQLite transaction:", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(first.path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "archive.sqlite3-tmp-") || entry.Name() == "archive.sqlite3-journal" {
			t.Fatalf("SQLite temporary or rollback file remained after close: %q", entry.Name())
		}
	}
}

func TestArchiveDarwinVFSRefusesWALWithoutChangingJournalPolicy(t *testing.T) {
	archive, _ := openTestArchive(t)
	var requested string
	walErr := archive.db.QueryRowContext(context.Background(), `PRAGMA journal_mode=WAL`).Scan(&requested)
	var actual string
	if err := archive.db.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if walErr != nil || !strings.EqualFold(requested, "delete") || !strings.EqualFold(actual, "delete") {
		t.Fatalf("WAL request escaped the archive VFS policy: requested=%q error=%v actual=%q", requested, walErr, actual)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(archive.path + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("WAL sidecar exists after refusal: %v", err)
	}
}

func TestArchiveWALHeaderGuardRejectsEitherFormatByte(t *testing.T) {
	tls := libc.NewTLS()
	t.Cleanup(func() { tls.Close() })
	baseName, err := libc.CString("unix-posix")
	if err != nil {
		t.Fatal(err)
	}
	baseVFS := sqlite3.Xsqlite3_vfs_find(tls, baseName)
	libc.Xfree(tls, baseName)
	if baseVFS == 0 {
		t.Fatal("could not locate generated unix-posix VFS")
	}
	initialVFS := archiveReadSQLiteVFSConfiguration(tls, baseVFS)
	if initialVFS.FiVersion <= 0 || initialVFS.FszOsFile <= 0 || initialVFS.FxOpen == 0 {
		t.Fatal("generated unix-posix VFS snapshot is incomplete")
	}
	buffer := archiveTestCBuffer(t, tls, 32)
	intCell := archiveTestCInt32(t, tls, 0)
	for offset := uintptr(0); offset < 32; offset++ {
		archiveWriteCByte(tls, buffer+offset, 0xa5)
	}
	archiveWriteCInt32(tls, intCell, 0x12345678)
	if got := archiveReadCInt32(tls, intCell); got != 0x12345678 {
		t.Fatalf("C-owned int32 ABI round trip = %#x, want %#x", got, int32(0x12345678))
	}
	wordCell := archiveTestCUintptr(t, tls, 0)
	const wordSentinel = uintptr(0x12345678)
	archiveWriteCUintptr(tls, wordCell, wordSentinel)
	if got := archiveReadCUintptr(tls, wordCell); got != wordSentinel {
		t.Fatalf("C-owned uintptr ABI round trip = %#x, want %#x", got, wordSentinel)
	}
	archiveWriteCInt32(tls, buffer+4, 0x10203040)
	for _, offset := range []uintptr{0, 1, 2, 3, 8, 9, 10, 11, 31} {
		if got := archiveReadCByte(tls, buffer+offset); got != 0xa5 {
			t.Fatalf("ABI write changed neighboring C byte at offset %d: %#x", offset, got)
		}
	}
	for iteration := int32(0); iteration < 3; iteration++ {
		_ = archiveTestGrowStack(16)
		runtime.GC()
		globalSnapshot := archiveReadSQLiteVFSConfiguration(tls, baseVFS)
		if globalSnapshot.FiVersion != initialVFS.FiVersion || globalSnapshot.FszOsFile != initialVFS.FszOsFile || globalSnapshot.FxOpen != initialVFS.FxOpen {
			t.Fatalf("generated VFS global copy changed across stack growth/GC: version=%d fileSize=%d open=%#x", globalSnapshot.FiVersion, globalSnapshot.FszOsFile, globalSnapshot.FxOpen)
		}
		want := int32(0x12345678) + iteration
		archiveWriteCInt32(tls, intCell, want)
		if got := archiveReadCInt32(tls, intCell); got != want {
			t.Fatalf("C-owned int32 round trip after stack growth/GC = %#x, want %#x", got, want)
		}
		wantWord := wordSentinel + uintptr(iteration)
		archiveWriteCUintptr(tls, wordCell, wantWord)
		if got := archiveReadCUintptr(tls, wordCell); got != wantWord {
			t.Fatalf("C-owned uintptr round trip after stack growth/GC = %#x, want %#x", got, wantWord)
		}
		wantByte := byte(0x5a + iteration)
		archiveWriteCByte(tls, buffer+30, wantByte)
		if got := archiveReadCByte(tls, buffer+30); got != wantByte {
			t.Fatalf("C-owned byte round trip after stack growth/GC = %#x, want %#x", got, wantByte)
		}
	}
	for _, headerOffset := range []int{18, 19} {
		t.Run(string(rune('a'+headerOffset-18)), func(t *testing.T) {
			archiveWriteCByte(tls, buffer+uintptr(headerOffset), 2)
			if !archiveWriteEnablesWALHeader(tls, buffer, 32, 0) {
				t.Fatalf("WAL header byte at offset %d was not rejected", headerOffset)
			}
			archiveWriteCByte(tls, buffer+uintptr(headerOffset), 1)
			if archiveWriteEnablesWALHeader(tls, buffer, 32, 0) {
				t.Fatalf("rollback-journal header byte at offset %d was rejected", headerOffset)
			}
		})
	}
	partial := archiveTestCBuffer(t, tls, 1)
	archiveWriteCByte(tls, partial, 2)
	if !archiveWriteEnablesWALHeader(tls, partial, 1, 19) {
		t.Fatal("partial write to the second WAL format byte was not rejected")
	}
}

func archiveTestGrowStack(depth int) byte {
	var frame [1024]byte
	frame[0] = byte(depth)
	frame[len(frame)-1] = byte(depth ^ 0xff)
	result := frame[0] ^ frame[len(frame)-1]
	if depth > 0 {
		result ^= archiveTestGrowStack(depth - 1)
	}
	runtime.KeepAlive(&frame)
	return result
}

func TestArchiveDarwinVFSRejectsHardlinkedRollbackJournal(t *testing.T) {
	home := t.TempDir()
	archive, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(t.TempDir(), "journal-sentinel")
	if err := os.WriteFile(sentinel, []byte("must remain untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := archive.path + "-journal"
	if err := os.Link(sentinel, journal); err != nil {
		t.Fatal(err)
	}
	before := fileDigest(t, sentinel)
	if _, err := Open(home); err == nil {
		t.Fatal("archive accepted a rollback journal with another link")
	}
	if got := fileDigest(t, sentinel); got != before {
		t.Fatal("refusing a rollback journal with another link changed its target")
	}
}

func TestArchiveDarwinVFSRefusesSubstitutedJournalDeletion(t *testing.T) {
	archive, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	authority := archive.location.authority
	journalName := "archive.sqlite3-journal"
	journalPath := archive.path + "-journal"
	if err := authority.deleteFile(journalName); err != nil {
		t.Fatalf("deleting an already absent journal should remain idempotent: %v", err)
	}
	original := []byte("retained journal identity")
	replacement := []byte("replacement must not be unlinked")
	if err := os.WriteFile(journalPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	fd, _, err := authority.openRetainedFile(journalName, sqlite3.SQLITE_OPEN_READWRITE)
	if err != nil {
		t.Fatal(err)
	}
	authority.innerClosed(fd)
	movedPath := journalPath + ".moved"
	if err := os.Rename(journalPath, movedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journalPath, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := authority.deleteFile(journalName); err == nil {
		t.Fatal("journal replacement was unlinked without matching the retained identity")
	}
	if got, err := os.ReadFile(journalPath); err != nil || string(got) != string(replacement) {
		t.Fatalf("replacement journal changed: data=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(movedPath); err != nil || string(got) != string(original) {
		t.Fatalf("moved original journal changed: data=%q err=%v", got, err)
	}
	if err := os.Remove(journalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(movedPath); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveDarwinVFSNullOutFlagsAndContainedFileControls(t *testing.T) {
	home := t.TempDir()
	archive, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = archive.Close() })

	pFile := openArchiveVFSFileWithoutOutFlags(t, archive)
	fileOpen := true
	defer func() {
		if fileOpen {
			_ = archiveVFSFileClose(archive.location.vfs.tls, pFile)
		}
		libc.Xfree(archive.location.vfs.tls, pFile)
	}()

	moved := archiveTestCInt32(t, archive.location.vfs.tls, 0)
	if rc := archiveVFSFileControl(archive.location.vfs.tls, pFile, sqlite3.SQLITE_FCNTL_HAS_MOVED, moved); rc != sqlite3.SQLITE_OK || archiveReadCInt32(archive.location.vfs.tls, moved) != 0 {
		t.Fatalf("HAS_MOVED on retained archive file: rc=%d moved=%d", rc, archiveReadCInt32(archive.location.vfs.tls, moved))
	}
	tempNameOut := archiveTestCUintptr(t, archive.location.vfs.tls, 0)
	if rc := archiveVFSFileControl(archive.location.vfs.tls, pFile, sqlite3.SQLITE_FCNTL_TEMPFILENAME, tempNameOut); rc != sqlite3.SQLITE_OK {
		t.Fatalf("TEMPFILENAME failed: rc=%d", rc)
	}
	tempName := archiveReadCUintptr(archive.location.vfs.tls, tempNameOut)
	if tempName == 0 || !strings.HasPrefix(libc.GoString(tempName), archive.location.vfs.logical+"-tmp-") {
		t.Fatalf("temporary filename escaped the retained VFS namespace: %q", libc.GoString(tempName))
	}
	if _, ok := archiveVFSName(archive.location.vfs, libc.GoString(tempName)); !ok {
		t.Fatal("generated temporary filename is not accepted inside the retained namespace")
	}
	sqlite3.Xsqlite3_free(archive.location.vfs.tls, tempName)
	proxy := archiveTestCUintptr(t, archive.location.vfs.tls, 1)
	if rc := archiveVFSFileControl(archive.location.vfs.tls, pFile, sqlite3.SQLITE_FCNTL_GET_LOCKPROXYFILE, proxy); rc != sqlite3.SQLITE_OK || archiveReadCUintptr(archive.location.vfs.tls, proxy) != 0 {
		t.Fatalf("GET_LOCKPROXYFILE did not report disabled proxying: rc=%d path=%d", rc, archiveReadCUintptr(archive.location.vfs.tls, proxy))
	}
	if rc := archiveVFSFileControl(archive.location.vfs.tls, pFile, sqlite3.SQLITE_FCNTL_SET_LOCKPROXYFILE, 0); rc != sqlite3.SQLITE_NOTFOUND {
		t.Fatalf("SET_LOCKPROXYFILE was not refused: rc=%d", rc)
	}
	if err := archiveVFSFileClose(archive.location.vfs.tls, pFile); err != sqlite3.SQLITE_OK {
		t.Fatalf("raw VFS file close failed: %d", err)
	}
	fileOpen = false
}

func TestArchiveDarwinVFSHasMovedAndWalAccessStayRooted(t *testing.T) {
	archive, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = archive.Close() })
	pFile := openArchiveVFSFileWithoutOutFlags(t, archive)
	fileOpen := true
	defer func() {
		if fileOpen {
			_ = archiveVFSFileClose(archive.location.vfs.tls, pFile)
		}
		libc.Xfree(archive.location.vfs.tls, pFile)
	}()

	walName, err := libc.CString(archive.location.vfs.logical + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	defer libc.Xfree(archive.location.vfs.tls, walName)
	exists := archiveTestCInt32(t, archive.location.vfs.tls, -1)
	if rc := archiveVFSAccess(archive.location.vfs.tls, archive.location.vfs.vfsPtr, walName, 0, exists); rc != sqlite3.SQLITE_OK || archiveReadCInt32(archive.location.vfs.tls, exists) != 0 {
		t.Fatalf("absent WAL probe should succeed as absent: rc=%d exists=%d", rc, archiveReadCInt32(archive.location.vfs.tls, exists))
	}
	walPath := archive.path + "-wal"
	if err := os.WriteFile(walPath, []byte("not a usable WAL"), 0o600); err != nil {
		t.Fatal(err)
	}
	if rc := archiveVFSAccess(archive.location.vfs.tls, archive.location.vfs.vfsPtr, walName, 0, exists); rc == sqlite3.SQLITE_OK {
		t.Fatal("existing WAL sidecar was accepted by the DELETE-only archive VFS")
	}
	if err := os.Remove(walPath); err != nil {
		t.Fatal(err)
	}

	// A namespace substitution after open must be visible through HAS_MOVED.
	movedPath := archive.path + ".moved"
	if err := os.Rename(archive.path, movedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive.path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	moved := archiveTestCInt32(t, archive.location.vfs.tls, 0)
	if rc := archiveVFSFileControl(archive.location.vfs.tls, pFile, sqlite3.SQLITE_FCNTL_HAS_MOVED, moved); rc != sqlite3.SQLITE_OK || archiveReadCInt32(archive.location.vfs.tls, moved) != 1 {
		t.Fatalf("HAS_MOVED missed archive leaf substitution: rc=%d moved=%d", rc, archiveReadCInt32(archive.location.vfs.tls, moved))
	}
	_ = archiveVFSFileClose(archive.location.vfs.tls, pFile)
	fileOpen = false
}

func TestArchiveDarwinVFSRefusesSecondAuthorityForRenamedDatabaseInode(t *testing.T) {
	homeA, homeB := t.TempDir(), t.TempDir()
	archive, err := Open(homeA)
	if err != nil {
		t.Fatal(err)
	}
	oldKnowledge := filepath.Dir(archive.path)
	newKnowledge := filepath.Join(homeB, ".sirsi", "maat", "knowledge")
	if err := os.MkdirAll(filepath.Dir(newKnowledge), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldKnowledge, newKnowledge); err != nil {
		t.Fatal(err)
	}
	if second, err := Open(homeB); err == nil {
		_ = second.Close()
		t.Fatal("renamed database inode acquired a second retained authority")
	}
	archiveAuthorityMu.Lock()
	retainedAuthority := archiveAuthorityByDatabase[archive.location.authority.identities[4]]
	retainedRefs := archive.location.authority.refs
	archiveAuthorityMu.Unlock()
	if retainedAuthority != archive.location.authority || retainedRefs != 1 {
		t.Fatalf("alias refusal disturbed the original authority: authority=%p refs=%d", retainedAuthority, retainedRefs)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveAuthorityRetirementConsumesAmbiguousCloseAndKeepsDebt(t *testing.T) {
	directoryFD, err := unix.Open(t.TempDir(), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	databaseFD, err := unix.Open(os.DevNull, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = unix.Close(directoryFD)
		t.Fatal(err)
	}
	identity := fileIdentity{device: 1, inode: 7}
	authority := &archiveAuthority{
		database: databaseFD,
		fds:      [5]int{-1, -1, -1, directoryFD, databaseFD},
		files:    map[int]fileIdentity{databaseFD: identity},
	}
	replacementFD := -1
	t.Cleanup(func() {
		if authority.files[databaseFD] != (fileIdentity{}) {
			_ = unix.Close(databaseFD)
		}
		if authority.fds[3] >= 0 {
			_ = unix.Close(authority.fds[3])
		}
		if replacementFD >= 0 {
			_ = unix.Close(replacementFD)
		}
	})

	var calls []int
	closer := func(fd int) error {
		calls = append(calls, fd)
		if fd != databaseFD {
			return unix.Close(fd)
		}
		if err := unix.Close(fd); err != nil {
			return err
		}
		replacementFD, err = unix.Open(os.DevNull, unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		if replacementFD != fd {
			t.Fatalf("replacement descriptor=%d, want consumed number %d", replacementFD, fd)
		}
		return unix.EIO
	}
	if err := authority.retireDescriptors(closer); !errors.Is(err, unix.EIO) {
		t.Fatalf("retirement error=%v, want retained EIO debt", err)
	}
	if authority.database != -1 || authority.fds[4] != -1 || len(authority.files) != 0 || authority.fds[3] != -1 {
		t.Fatalf("descriptor ownership was not monotonically consumed: %#v", authority)
	}
	if len(calls) != 2 {
		t.Fatalf("retirement stopped after an ambiguous close instead of processing remaining descriptors: %v", calls)
	}

	if err := authority.retireDescriptors(closer); !errors.Is(err, unix.EIO) {
		t.Fatalf("retry error=%v, want original retirement debt", err)
	}
	if len(calls) != 2 {
		t.Fatalf("retry attempted to close a possibly reused descriptor: %v", calls)
	}
	var st unix.Stat_t
	if err := unix.Fstat(replacementFD, &st); err != nil {
		t.Fatalf("replacement descriptor was closed by retirement retry: %v", err)
	}
}

func TestArchiveInnerClosedConsumesDescriptorWhenCloseOutcomeIsAmbiguous(t *testing.T) {
	fd, err := unix.Open(os.DevNull, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	replacementFD := -1
	authority := &archiveAuthority{files: map[int]fileIdentity{fd: {device: 1, inode: 2}}, active: map[int]int{fd: 1}}
	t.Cleanup(func() {
		if replacementFD >= 0 {
			_ = unix.Close(replacementFD)
		} else {
			_ = unix.Close(fd)
		}
	})
	authority.innerClosedWith(fd, func(closing int) error {
		if err := unix.Close(closing); err != nil {
			return err
		}
		replacementFD, err = unix.Open(os.DevNull, unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		if replacementFD != fd {
			t.Fatalf("replacement descriptor=%d, want consumed number %d", replacementFD, fd)
		}
		return unix.EIO
	})
	if _, ok := authority.files[fd]; ok || authority.active[fd] != 0 || !errors.Is(authority.retirementErr, unix.EIO) {
		t.Fatalf("ambiguous close ownership/debt not recorded: files=%v active=%v err=%v", authority.files, authority.active, authority.retirementErr)
	}
	authority.innerClosedWith(fd, func(closing int) error {
		t.Fatalf("consumed descriptor %d was closed a second time", closing)
		return nil
	})
	var st unix.Stat_t
	if err := unix.Fstat(replacementFD, &st); err != nil {
		t.Fatalf("replacement descriptor did not survive later close: %v", err)
	}
}

func TestArchiveSetCloseOnExecChecksAndPropagatesDescriptorFlags(t *testing.T) {
	tests := []struct {
		name string
		fn   func(uintptr, int, int) (int, error)
		want error
	}{
		{name: "get failure", fn: func(uintptr, int, int) (int, error) { return 0, unix.EBADF }, want: unix.EBADF},
		{name: "set failure", fn: func(_ uintptr, cmd, _ int) (int, error) {
			if cmd == unix.F_GETFD {
				return 0, nil
			}
			return 0, unix.EPERM
		}, want: unix.EPERM},
		{name: "verification failure", fn: func(uintptr, int, int) (int, error) { return 0, nil }, want: errors.New("FD_CLOEXEC")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := archiveSetCloseOnExecWith(9, tt.fn)
			if err == nil || (tt.name != "verification failure" && !errors.Is(err, tt.want)) || (tt.name == "verification failure" && !strings.Contains(err.Error(), tt.want.Error())) {
				t.Fatalf("close-on-exec error=%v, want %v", err, tt.want)
			}
		})
	}
	actual, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer actual.Close()
	if err := archiveSetCloseOnExec(int(actual.Fd())); err != nil {
		t.Fatal(err)
	}
	flags, err := unix.FcntlInt(actual.Fd(), unix.F_GETFD, 0)
	if err != nil || flags&unix.FD_CLOEXEC == 0 {
		t.Fatalf("FD_CLOEXEC not retained: flags=%#x err=%v", flags, err)
	}
}

func openArchiveVFSFileWithoutOutFlags(t *testing.T, archive *Archive) uintptr {
	t.Helper()
	vfs := archive.location.vfs
	tls := vfs.tls
	fileSize := archiveReadSQLiteVFS(tls, vfs.vfsPtr).FszOsFile
	if fileSize < int32(unsafe.Sizeof(sqlite3.TunixFile{})) {
		t.Fatalf("VFS os-file allocation size %d is smaller than unixFile", fileSize)
	}
	pFile := libc.Xmalloc(tls, libc.Tsize_t(fileSize))
	if pFile == 0 {
		t.Fatal("could not allocate SQLite test file")
	}
	libc.Xmemset(tls, pFile, 0, libc.Tsize_t(fileSize))
	zName, err := libc.CString(vfs.logical)
	if err != nil {
		libc.Xfree(tls, pFile)
		t.Fatal(err)
	}
	rc := archiveVFSOpen(tls, vfs.vfsPtr, zName, pFile, sqlite3.SQLITE_OPEN_READWRITE|sqlite3.SQLITE_OPEN_MAIN_DB, 0)
	libc.Xfree(tls, zName)
	if rc != sqlite3.SQLITE_OK {
		libc.Xfree(tls, pFile)
		t.Fatalf("archive VFS rejected NULL output flags: rc=%d", rc)
	}
	file := archiveReadUnixFile(tls, pFile)
	if file.FpMethod == 0 {
		_ = archiveVFSFileClose(tls, pFile)
		libc.Xfree(tls, pFile)
		t.Fatal("archive wrapper returned a NULL I/O-method table")
	}
	methods := archiveReadIOMethods(tls, file.FpMethod)
	if methods.FiVersion != 1 || methods.FxShmMap != 0 || methods.FxShmLock != 0 || methods.FxShmBarrier != 0 || methods.FxShmUnmap != 0 || methods.FxFetch != 0 || methods.FxUnfetch != 0 {
		_ = archiveVFSFileClose(tls, pFile)
		libc.Xfree(tls, pFile)
		t.Fatal("archive wrapper advertises SQLite I/O capabilities beyond rollback-only version 1")
	}
	return pFile
}

func archiveTestCBuffer(t *testing.T, tls *libc.TLS, size uintptr) uintptr {
	t.Helper()
	address := libc.Xmalloc(tls, libc.Tsize_t(size))
	if address == 0 {
		t.Fatal("could not allocate test-owned C memory")
	}
	libc.Xmemset(tls, address, 0, libc.Tsize_t(size))
	t.Cleanup(func() { libc.Xfree(tls, address) })
	return address
}

func archiveTestCInt32(t *testing.T, tls *libc.TLS, value int32) uintptr {
	address := archiveTestCBuffer(t, tls, unsafe.Sizeof(value))
	archiveWriteCInt32(tls, address, value)
	return address
}

func archiveTestCUintptr(t *testing.T, tls *libc.TLS, value uintptr) uintptr {
	address := archiveTestCBuffer(t, tls, unsafe.Sizeof(value))
	archiveWriteCUintptr(tls, address, value)
	return address
}
