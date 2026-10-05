// Package packageinventory verifies the final Pantheon.app payload without
// executing anything inside the bundle. It is deliberately descriptor-rooted:
// every directory and regular file is opened relative to a retained parent
// descriptor with O_NOFOLLOW, then revalidated before acceptance.
package packageinventory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const Schema = "pantheon.package-inventory/v1"

type Expectations struct {
	Version              string
	Build                string
	InfoPlist            []byte
	PkgInfo              []byte
	LaunchAgent          []byte
	RequireCodeSignature bool
}

type Entry struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Dev    uint64 `json:"dev"`
	Ino    uint64 `json:"ino"`
	Mode   uint32 `json:"mode"`
	Nlink  uint64 `json:"nlink"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
}

type Report struct {
	Schema          string  `json:"schema"`
	Version         string  `json:"version"`
	Build           string  `json:"build"`
	Entries         []Entry `json:"entries"`
	PythonFree      bool    `json:"python_free"`
	EngineCount     int     `json:"engine_count"`
	ExecutableCount int     `json:"executable_count"`
}

var allowed = map[string]string{
	"Contents":                                   "directory",
	"Contents/Info.plist":                        "regular",
	"Contents/PkgInfo":                           "regular",
	"Contents/MacOS":                             "directory",
	"Contents/MacOS/sirsi":                       "regular",
	"Contents/MacOS/sirsi-menubar":               "regular",
	"Contents/Resources":                         "directory",
	"Contents/Resources/ai.sirsi.pantheon.plist": "regular",
	"Contents/_CodeSignature":                    "directory",
	"Contents/_CodeSignature/CodeResources":      "regular",
}

// Verify returns a deterministic, non-executing inventory. The bundle root,
// all directories, and all leaves are opened through retained descriptors.
func Verify(appPath string, expected Expectations) (Report, error) {
	return VerifyWithFinalCheck(appPath, expected, nil)
}

// VerifyWithFinalCheck invokes finalCheck after payload validation and before
// the final app-path continuity check. It lets callers bind external expected
// inputs to the same acceptance boundary without reopening their pathnames.
func VerifyWithFinalCheck(appPath string, expected Expectations, finalCheck func() error) (Report, error) {
	return verifyWithHooks(appPath, expected, nil, finalCheck)
}

func verifyWithHook(appPath string, expected Expectations, beforeFinalPathCheck func()) (Report, error) {
	return verifyWithHooks(appPath, expected, beforeFinalPathCheck, nil)
}

func verifyWithHooks(appPath string, expected Expectations, beforeFinalPathCheck func(), finalCheck func() error) (Report, error) {
	authority, err := openAppPathAuthority(appPath)
	if err != nil {
		return Report{}, err
	}
	defer authority.close()
	rootFD := authority.rootFD

	snapshot := &scanSnapshot{entries: make(map[string]Entry), bytes: make(map[string][]byte)}
	if err := scanDir(rootFD, "", snapshot); err != nil {
		return Report{}, err
	}
	entries := make([]Entry, 0, len(snapshot.entries))
	for _, entry := range snapshot.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	report := Report{Schema: Schema, Version: expected.Version, Build: expected.Build, Entries: entries, PythonFree: true}
	for _, entry := range entries {
		switch entry.Path {
		case "Contents/MacOS/sirsi":
			report.EngineCount++
			report.ExecutableCount++
		case "Contents/MacOS/sirsi-menubar":
			report.ExecutableCount++
		}
	}
	if err := validateReport(report, snapshot.bytes, expected); err != nil {
		return Report{}, err
	}
	if beforeFinalPathCheck != nil {
		beforeFinalPathCheck()
	}
	if finalCheck != nil {
		if err := finalCheck(); err != nil {
			return Report{}, err
		}
	}
	// Keep the complete package namespace scan after every callback and
	// caller-supplied-input check. This is the last content-sensitive package
	// operation before checking that the retained app root is still named by
	// the same parent entry.
	if err := finalNamespaceRescan(rootFD, snapshot); err != nil {
		return Report{}, err
	}
	if err := authority.revalidate(); err != nil {
		return Report{}, err
	}
	return report, nil
}

type directoryEdge struct {
	parentIndex int
	name        string
	identity    unix.Stat_t
}

// appPathAuthority retains the complete no-follow parent chain and the opened
// bundle root. Inventory is accepted only while every parent name and the app
// leaf still identify those exact descriptors.
type appPathAuthority struct {
	fds         []int
	identities  []unix.Stat_t
	edges       []directoryEdge
	appName     string
	appIdentity unix.Stat_t
	rootFD      int
}

func openAppPathAuthority(appPath string) (*appPathAuthority, error) {
	if strings.TrimSpace(appPath) == "" || !path.IsAbs(appPath) {
		return nil, errors.New("package inventory: absolute app path is required")
	}
	clean := path.Clean(appPath)
	if clean != appPath {
		return nil, errors.New("package inventory: app path must be canonical")
	}
	if clean == "/" {
		return nil, errors.New("package inventory: app path must name a bundle")
	}
	parentPath, appName := path.Split(clean)
	parentPath = strings.TrimSuffix(parentPath, "/")
	if parentPath == "" {
		parentPath = "/"
	}
	if appName == "" || appName == "." || appName == ".." {
		return nil, errors.New("package inventory: invalid app leaf")
	}

	rootFD, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("package inventory: open filesystem root: %w", err)
	}
	var rootIdentity unix.Stat_t
	if err := unix.Fstat(rootFD, &rootIdentity); err != nil {
		unix.Close(rootFD)
		return nil, fmt.Errorf("package inventory: fstat filesystem root: %w", err)
	}
	authority := &appPathAuthority{fds: []int{rootFD}, identities: []unix.Stat_t{rootIdentity}, appName: appName, rootFD: -1}
	cleanup := func(err error) (*appPathAuthority, error) {
		authority.close()
		return nil, err
	}
	components := strings.Split(strings.TrimPrefix(parentPath, "/"), "/")
	if parentPath == "/" {
		components = nil
	}
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return cleanup(errors.New("package inventory: non-canonical parent path"))
		}
		parentIndex := len(authority.fds) - 1
		parentFD := authority.fds[parentIndex]
		var before unix.Stat_t
		if err := unix.Fstatat(parentFD, component, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return cleanup(fmt.Errorf("package inventory: stat app parent %q: %w", component, err))
		}
		if before.Mode&unix.S_IFMT != unix.S_IFDIR {
			return cleanup(fmt.Errorf("package inventory: app parent is not a real directory: %q", component))
		}
		fd, err := unix.Openat(parentFD, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return cleanup(fmt.Errorf("package inventory: open app parent %q: %w", component, err))
		}
		var opened unix.Stat_t
		if err := unix.Fstat(fd, &opened); err != nil || !sameDirectoryIdentity(before, opened) {
			unix.Close(fd)
			if err != nil {
				return cleanup(fmt.Errorf("package inventory: fstat app parent %q: %w", component, err))
			}
			return cleanup(fmt.Errorf("package inventory: app parent substitution at %q", component))
		}
		authority.edges = append(authority.edges, directoryEdge{parentIndex: parentIndex, name: component, identity: opened})
		authority.fds = append(authority.fds, fd)
		authority.identities = append(authority.identities, opened)
	}

	parentFD := authority.fds[len(authority.fds)-1]
	var before unix.Stat_t
	if err := unix.Fstatat(parentFD, appName, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return cleanup(fmt.Errorf("package inventory: stat app leaf: %w", err))
	}
	if before.Mode&unix.S_IFMT != unix.S_IFDIR {
		return cleanup(errors.New("package inventory: app root is not a real directory"))
	}
	rootFD, err = unix.Openat(parentFD, appName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return cleanup(fmt.Errorf("package inventory: open app root: %w", err))
	}
	var opened unix.Stat_t
	if err := unix.Fstat(rootFD, &opened); err != nil || !sameDirectoryIdentity(before, opened) {
		unix.Close(rootFD)
		if err != nil {
			return cleanup(fmt.Errorf("package inventory: fstat app root: %w", err))
		}
		return cleanup(errors.New("package inventory: app root substitution during open"))
	}
	authority.rootFD = rootFD
	authority.appIdentity = opened
	return authority, nil
}

func (a *appPathAuthority) revalidate() error {
	for i, fd := range a.fds {
		var opened unix.Stat_t
		if err := unix.Fstat(fd, &opened); err != nil || !sameDirectoryIdentity(a.identities[i], opened) {
			return fmt.Errorf("package inventory: retained parent identity changed before acceptance")
		}
	}
	for _, edge := range a.edges {
		parentFD := a.fds[edge.parentIndex]
		var named unix.Stat_t
		if err := unix.Fstatat(parentFD, edge.name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameDirectoryIdentity(edge.identity, named) {
			return fmt.Errorf("package inventory: parent path continuity failed before acceptance at %q", edge.name)
		}
	}
	var root, named unix.Stat_t
	parentFD := a.fds[len(a.fds)-1]
	if err := unix.Fstat(a.rootFD, &root); err != nil || !sameDirectoryIdentity(a.appIdentity, root) {
		return errors.New("package inventory: retained app root identity changed before acceptance")
	}
	if err := unix.Fstatat(parentFD, a.appName, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameDirectoryIdentity(a.appIdentity, named) {
		return errors.New("package inventory: app path identity changed before acceptance")
	}
	return nil
}

func (a *appPathAuthority) close() {
	for i := len(a.fds) - 1; i >= 0; i-- {
		_ = unix.Close(a.fds[i])
	}
	if a.rootFD >= 0 {
		_ = unix.Close(a.rootFD)
		a.rootFD = -1
	}
}

type scanSnapshot struct {
	entries map[string]Entry
	bytes   map[string][]byte
}

func scanDir(fd int, parent string, snapshot *scanSnapshot) error {
	return scanDirOwned(fd, parent, snapshot, false)
}

func scanDirOwned(fd int, parent string, snapshot *scanSnapshot, closeFD bool) error {
	if closeFD {
		defer unix.Close(fd)
	}
	names, err := directoryNames(fd, parent)
	if err != nil {
		return fmt.Errorf("package inventory: enumerate %q: %w", parent, err)
	}
	sort.Strings(names)
	for _, name := range names {
		rel := name
		if parent != "" {
			rel = parent + "/" + name
		}
		if _, ok := allowed[rel]; !ok {
			return fmt.Errorf("package inventory: unexpected entry %q", rel)
		}
		var before unix.Stat_t
		if err := unix.Fstatat(fd, name, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return fmt.Errorf("package inventory: stat %q: %w", rel, err)
		}
		if before.Mode&unix.S_IFMT == unix.S_IFLNK {
			return fmt.Errorf("package inventory: symlink rejected at %q", rel)
		}
		wantType, ok := allowed[rel]
		if !ok || statType(before.Mode) != wantType {
			return fmt.Errorf("package inventory: type mismatch at %q", rel)
		}
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC
		if wantType == "directory" {
			flags |= unix.O_DIRECTORY
		}
		childFD, err := unix.Openat(fd, name, flags, 0)
		if err != nil {
			return fmt.Errorf("package inventory: open %q: %w", rel, err)
		}
		var opened unix.Stat_t
		if err := unix.Fstat(childFD, &opened); err != nil {
			unix.Close(childFD)
			return fmt.Errorf("package inventory: fstat %q: %w", rel, err)
		}
		if !sameScannedEntryIdentity(wantType, before, opened) {
			unix.Close(childFD)
			return fmt.Errorf("package inventory: substitution detected before visit %q", rel)
		}
		if wantType == "regular" && opened.Nlink != 1 {
			unix.Close(childFD)
			return fmt.Errorf("package inventory: regular payload must have nlink=1 at %q", rel)
		}

		entry := Entry{Path: rel, Type: wantType, Dev: uint64(opened.Dev), Ino: uint64(opened.Ino), Mode: uint32(opened.Mode), Nlink: uint64(opened.Nlink), Size: opened.Size}
		if wantType == "directory" {
			snapshot.entries[rel] = entry
			if err := scanDirOwned(childFD, rel, snapshot, true); err != nil {
				return err
			}
		} else {
			content, digest, err := readStable(childFD, opened, rel)
			unix.Close(childFD)
			if err != nil {
				return err
			}
			entry.SHA256 = digest
			snapshot.entries[rel] = entry
			snapshot.bytes[rel] = content
		}
		var after unix.Stat_t
		if err := unix.Fstatat(fd, name, &after, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameScannedEntryIdentity(wantType, opened, after) {
			if err != nil {
				return fmt.Errorf("package inventory: parent continuity failed at %q: %w", rel, err)
			}
			return fmt.Errorf("package inventory: parent continuity failed at %q", rel)
		}
	}
	return nil
}

func finalNamespaceRescan(rootFD int, snapshot *scanSnapshot) error {
	current := &scanSnapshot{entries: make(map[string]Entry), bytes: make(map[string][]byte)}
	if err := rescanDir(rootFD, "", current); err != nil {
		return err
	}
	if len(current.entries) != len(snapshot.entries) {
		return fmt.Errorf("package inventory: namespace changed after file read")
	}
	for rel, expected := range snapshot.entries {
		actual, ok := current.entries[rel]
		if !ok {
			return fmt.Errorf("package inventory: namespace changed after file read at %q", rel)
		}
		if !sameEntryIdentity(expected, actual) {
			return fmt.Errorf("package inventory: entry identity changed after file read at %q", rel)
		}
		if expected.Type == "regular" && expected.SHA256 != actual.SHA256 {
			return fmt.Errorf("package inventory: content changed after file read at %q", rel)
		}
	}
	return nil
}

func rescanDir(fd int, parent string, snapshot *scanSnapshot) error {
	names, err := directoryNames(fd, parent)
	if err != nil {
		return fmt.Errorf("package inventory: rescan directory %q: %w", parent, err)
	}
	sort.Strings(names)
	for _, name := range names {
		rel := name
		if parent != "" {
			rel = parent + "/" + name
		}
		if _, ok := allowed[rel]; !ok {
			return fmt.Errorf("package inventory: late unexpected entry %q", rel)
		}
		var st unix.Stat_t
		if err := unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return fmt.Errorf("package inventory: rescan stat %q: %w", rel, err)
		}
		if st.Mode&unix.S_IFMT == unix.S_IFLNK {
			return fmt.Errorf("package inventory: late symlink at %q", rel)
		}
		wantType := statType(st.Mode)
		if wantType != allowed[rel] {
			return fmt.Errorf("package inventory: rescan type mismatch at %q", rel)
		}
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC
		if wantType == "directory" {
			flags |= unix.O_DIRECTORY
		}
		childFD, err := unix.Openat(fd, name, flags, 0)
		if err != nil {
			return fmt.Errorf("package inventory: rescan open %q: %w", rel, err)
		}
		var opened unix.Stat_t
		if err := unix.Fstat(childFD, &opened); err != nil {
			unix.Close(childFD)
			return fmt.Errorf("package inventory: rescan fstat %q: %w", rel, err)
		}
		if !sameScannedEntryIdentity(wantType, st, opened) {
			unix.Close(childFD)
			return fmt.Errorf("package inventory: rescan substitution at %q", rel)
		}
		if wantType == "regular" && opened.Nlink != 1 {
			unix.Close(childFD)
			return fmt.Errorf("package inventory: hard-linked regular payload at %q", rel)
		}
		entry := Entry{Path: rel, Type: wantType, Dev: uint64(opened.Dev), Ino: uint64(opened.Ino), Mode: uint32(opened.Mode), Nlink: uint64(opened.Nlink), Size: opened.Size}
		if wantType == "directory" {
			if err := rescanDir(childFD, rel, snapshot); err != nil {
				unix.Close(childFD)
				return err
			}
			unix.Close(childFD)
		} else {
			content, digest, err := readStable(childFD, opened, rel)
			unix.Close(childFD)
			if err != nil {
				return err
			}
			entry.SHA256 = digest
			snapshot.bytes[rel] = content
		}
		snapshot.entries[rel] = entry
		var after unix.Stat_t
		if err := unix.Fstatat(fd, name, &after, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameScannedEntryIdentity(wantType, opened, after) {
			if err != nil {
				return fmt.Errorf("package inventory: rescan parent continuity failed at %q: %w", rel, err)
			}
			return fmt.Errorf("package inventory: rescan parent continuity failed at %q", rel)
		}
	}
	return nil
}

func sameEntryIdentity(a, b Entry) bool {
	return a.Path == b.Path && a.Type == b.Type && a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Nlink == b.Nlink && a.Size == b.Size
}

func directoryNames(fd int, parent string) ([]string, error) {
	dupFD, err := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(dupFD), parent)
	if file == nil {
		unix.Close(dupFD)
		return nil, fmt.Errorf("open directory %q", parent)
	}
	names, readErr := file.Readdirnames(-1)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return names, nil
}

func readStable(fd int, opened unix.Stat_t, rel string) ([]byte, string, error) {
	first, err := readOnce(fd, opened.Size)
	if err != nil {
		return nil, "", fmt.Errorf("package inventory: read %q: %w", rel, err)
	}
	if _, err := unix.Seek(fd, 0, 0); err != nil {
		return nil, "", fmt.Errorf("package inventory: rewind %q: %w", rel, err)
	}
	second, err := readOnce(fd, opened.Size)
	if err != nil || string(first) != string(second) {
		return nil, "", fmt.Errorf("package inventory: content changed during read %q", rel)
	}
	var after unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil || !sameIdentity(opened, after) {
		return nil, "", fmt.Errorf("package inventory: identity changed during read %q", rel)
	}
	sum := sha256.Sum256(first)
	return first, hex.EncodeToString(sum[:]), nil
}

func readOnce(fd int, size int64) ([]byte, error) {
	if size < 0 || size > 32<<20 {
		return nil, fmt.Errorf("invalid file size %d", size)
	}
	data := make([]byte, int(size))
	for offset := 0; offset < len(data); {
		n, err := unix.Read(fd, data[offset:])
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			if err == io.EOF {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrUnexpectedEOF
		}
		offset += n
	}
	var extra [1]byte
	for {
		n, err := unix.Read(fd, extra[:])
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			if err == io.EOF {
				return data, nil
			}
			return nil, err
		}
		if n > 0 {
			return nil, errors.New("file grew beyond observed size")
		}
		return data, nil
	}
}

func validateReport(report Report, contents map[string][]byte, expected Expectations) error {
	if report.Schema != Schema || !report.PythonFree || report.EngineCount != 1 || report.ExecutableCount != 2 {
		return errors.New("package inventory: malformed report")
	}
	if !hasExact(report, "Contents/Info.plist", expected.InfoPlist) || !hasExact(report, "Contents/PkgInfo", expected.PkgInfo) || !hasExact(report, "Contents/Resources/ai.sirsi.pantheon.plist", expected.LaunchAgent) {
		return errors.New("package inventory: canonical payload bytes mismatch")
	}
	previous := ""
	seen := make(map[string]struct{}, len(report.Entries))
	engineCount := 0
	executableCount := 0
	for _, entry := range report.Entries {
		if entry.Path == "" || path.IsAbs(entry.Path) || strings.Contains(entry.Path, "..") || entry.Path <= previous {
			return errors.New("package inventory: entries are not a deterministic relative sequence")
		}
		previous = entry.Path
		if _, duplicate := seen[entry.Path]; duplicate {
			return fmt.Errorf("package inventory: duplicate entry %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}
		if expectedType, ok := allowed[entry.Path]; !ok || expectedType != entry.Type {
			return fmt.Errorf("package inventory: malformed entry %q", entry.Path)
		}
		if entry.Mode > 0xffff || statType(uint16(entry.Mode)) != entry.Type {
			return fmt.Errorf("package inventory: mode/type mismatch %q", entry.Path)
		}
		if entry.Path == "Contents/MacOS/sirsi" {
			engineCount++
		}
		if entry.Type == "regular" {
			if entry.Nlink != 1 || entry.Size < 0 || entry.Size > 32<<20 {
				return fmt.Errorf("package inventory: invalid regular-file metadata %q", entry.Path)
			}
			executable := entry.Mode&0o111 != 0
			switch entry.Path {
			case "Contents/MacOS/sirsi", "Contents/MacOS/sirsi-menubar":
				if !executable {
					return fmt.Errorf("package inventory: product executable payload is not executable %q", entry.Path)
				}
				executableCount++
			default:
				if executable {
					return fmt.Errorf("package inventory: resource payload has executable mode %q", entry.Path)
				}
			}
			digestBytes, err := hex.DecodeString(entry.SHA256)
			if err != nil || len(digestBytes) != sha256.Size {
				return fmt.Errorf("package inventory: invalid digest %q", entry.Path)
			}
			data, ok := contents[entry.Path]
			if !ok || digest(data) != entry.SHA256 {
				return fmt.Errorf("package inventory: digest does not bind scanned bytes %q", entry.Path)
			}
		} else if entry.SHA256 != "" || entry.Size < 0 {
			return fmt.Errorf("package inventory: invalid directory metadata %q", entry.Path)
		}
		if strings.Contains(strings.ToLower(entry.Path), "python") || strings.Contains(strings.ToLower(entry.Path), ".py") {
			return fmt.Errorf("package inventory: Python payload rejected at %q", entry.Path)
		}
		if data, ok := contents[entry.Path]; ok && (strings.Contains(strings.ToLower(string(data)), "libpython") || strings.Contains(strings.ToLower(string(data)), "python.framework")) {
			return fmt.Errorf("package inventory: Python linkage rejected at %q", entry.Path)
		}
	}
	if engineCount != report.EngineCount {
		return errors.New("package inventory: engine count does not match inventory")
	}
	if executableCount != report.ExecutableCount {
		return errors.New("package inventory: executable count does not match inventory")
	}
	for required := range allowed {
		if !expected.RequireCodeSignature && strings.HasPrefix(required, "Contents/_CodeSignature") {
			continue
		}
		if _, ok := seen[required]; !ok {
			return fmt.Errorf("package inventory: missing required entry %q", required)
		}
	}
	_, codeSignatureDirectory := seen["Contents/_CodeSignature"]
	_, codeResources := seen["Contents/_CodeSignature/CodeResources"]
	if codeSignatureDirectory != codeResources {
		return errors.New("package inventory: incomplete _CodeSignature payload")
	}
	return nil
}

func hasExact(report Report, rel string, want []byte) bool {
	for _, entry := range report.Entries {
		if entry.Path == rel {
			return entry.SHA256 == digest(want)
		}
	}
	return false
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func statType(mode uint16) string {
	switch mode & unix.S_IFMT {
	case unix.S_IFDIR:
		return "directory"
	case unix.S_IFREG:
		return "regular"
	default:
		return "other"
	}
}

func sameIdentity(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Nlink == b.Nlink && a.Size == b.Size
}

// Directory link count and size describe current children, not object
// identity; unrelated sibling changes must not invalidate a retained path.
// App-subtree completeness and metadata remain checked by the final rescan.
func sameDirectoryIdentity(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Mode&unix.S_IFMT == unix.S_IFDIR && b.Mode&unix.S_IFMT == unix.S_IFDIR
}

func sameScannedEntryIdentity(entryType string, a, b unix.Stat_t) bool {
	if entryType == "directory" {
		return sameDirectoryIdentity(a, b)
	}
	return sameIdentity(a, b)
}
