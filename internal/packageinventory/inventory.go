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
	Schema      string  `json:"schema"`
	Version     string  `json:"version"`
	Build       string  `json:"build"`
	Entries     []Entry `json:"entries"`
	PythonFree  bool    `json:"python_free"`
	EngineCount int     `json:"engine_count"`
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
	if strings.TrimSpace(appPath) == "" {
		return Report{}, errors.New("package inventory: app path is required")
	}
	rootFD, err := unix.Open(appPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return Report{}, fmt.Errorf("package inventory: open app root: %w", err)
	}
	defer unix.Close(rootFD)

	snapshot := &scanSnapshot{entries: make(map[string]Entry), bytes: make(map[string][]byte)}
	if err := scanDir(rootFD, "", snapshot); err != nil {
		return Report{}, err
	}
	if err := finalNamespaceRescan(rootFD, snapshot); err != nil {
		return Report{}, err
	}

	entries := make([]Entry, 0, len(snapshot.entries))
	for _, entry := range snapshot.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	report := Report{Schema: Schema, Version: expected.Version, Build: expected.Build, Entries: entries, PythonFree: true}
	for _, entry := range entries {
		if entry.Path == "Contents/MacOS/sirsi" || entry.Path == "Contents/MacOS/sirsi-menubar" {
			report.EngineCount++
		}
	}
	if err := validateReport(report, snapshot.bytes, expected); err != nil {
		return Report{}, err
	}
	return report, nil
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
		if !sameIdentity(before, opened) {
			unix.Close(childFD)
			return fmt.Errorf("package inventory: substitution detected before visit %q", rel)
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
		if err := unix.Fstatat(fd, name, &after, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameIdentity(opened, after) {
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
		if !sameIdentity(st, opened) {
			unix.Close(childFD)
			return fmt.Errorf("package inventory: rescan substitution at %q", rel)
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
		if err := unix.Fstatat(fd, name, &after, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameIdentity(opened, after) {
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
	data := make([]byte, 0, size)
	buf := make([]byte, 32<<10)
	for {
		n, err := unix.Read(fd, buf)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if err == io.EOF || n == 0 {
			return data, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func validateReport(report Report, contents map[string][]byte, expected Expectations) error {
	if report.Schema != Schema || !report.PythonFree || report.EngineCount != 2 {
		return errors.New("package inventory: malformed report")
	}
	if !hasExact(report, "Contents/Info.plist", expected.InfoPlist) || !hasExact(report, "Contents/PkgInfo", expected.PkgInfo) || !hasExact(report, "Contents/Resources/ai.sirsi.pantheon.plist", expected.LaunchAgent) {
		return errors.New("package inventory: canonical payload bytes mismatch")
	}
	previous := ""
	seen := make(map[string]struct{}, len(report.Entries))
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
		if entry.Type == "regular" && len(entry.SHA256) != 64 {
			return fmt.Errorf("package inventory: missing digest %q", entry.Path)
		}
		if strings.Contains(strings.ToLower(entry.Path), "python") || strings.Contains(strings.ToLower(entry.Path), ".py") {
			return fmt.Errorf("package inventory: Python payload rejected at %q", entry.Path)
		}
		if data, ok := contents[entry.Path]; ok && (strings.Contains(strings.ToLower(string(data)), "libpython") || strings.Contains(strings.ToLower(string(data)), "python.framework")) {
			return fmt.Errorf("package inventory: Python linkage rejected at %q", entry.Path)
		}
	}
	for required := range allowed {
		if !expected.RequireCodeSignature && strings.HasPrefix(required, "Contents/_CodeSignature") {
			continue
		}
		if _, ok := seen[required]; !ok {
			return fmt.Errorf("package inventory: missing required entry %q", required)
		}
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
