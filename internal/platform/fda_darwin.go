//go:build darwin

package platform

import "os"

type diskProbe struct{ name, path string }

// fullGates require Full Disk Access specifically — readability of ANY one means
// AccessFull.
var fullGates = []diskProbe{
	{"TCC database", "/Library/Application Support/com.apple.TCC/TCC.db"},
	{"Mail", "/Library/Mail"},
	{"Safari", "/Library/Safari"},
}

// folderGates are per-folder TCC locations — granted piecemeal. Readability of
// one (without a full gate) means AccessSome.
var folderGates = []diskProbe{
	{"Desktop", "/Desktop"},
	{"Documents", "/Documents"},
	{"Downloads", "/Downloads"},
}

// canRead reports whether path is readable by THIS binary, and whether it exists
// at all (an absent path is inconclusive — neither granted nor denied).
func canRead(path string) (readable, exists bool) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false
		}
		return false, true // exists but stat denied → blocked
	}
	if info.IsDir() {
		f, derr := os.Open(path)
		if derr != nil {
			return false, true
		}
		defer f.Close()
		_, rerr := f.Readdirnames(1)
		// io.EOF means an empty but READABLE dir — still access.
		return rerr == nil || rerr.Error() == "EOF", true
	}
	f, oerr := os.Open(path)
	if oerr != nil {
		return false, true
	}
	defer f.Close()
	_, rerr := f.Read(make([]byte, 1))
	return rerr == nil || rerr.Error() == "EOF", true
}

// CheckDiskAccess resolves the current visibility spectrum by probing the full-
// and folder-gate sets.
func CheckDiskAccess() DiskAccess {
	da := DiskAccess{Level: AccessNone}
	full := false
	someFolder := false
	for _, g := range fullGates {
		readable, exists := canRead(os.Getenv("HOME") + g.path)
		if readable {
			full = true
			da.Accessible = append(da.Accessible, g.name)
		} else if exists {
			da.Blocked = append(da.Blocked, g.name)
		}
	}
	for _, g := range folderGates {
		readable, exists := canRead(os.Getenv("HOME") + g.path)
		if readable {
			someFolder = true
			da.Accessible = append(da.Accessible, g.name)
		} else if exists {
			da.Blocked = append(da.Blocked, g.name)
		}
	}
	switch {
	case full:
		da.Level = AccessFull
	case someFolder:
		da.Level = AccessSome
	default:
		da.Level = AccessNone
	}
	return da
}

// HasFullDiskAccess is the convenience boolean (AccessFull) for callers that only
// care whether Sirsi can see everything.
func HasFullDiskAccess() bool { return CheckDiskAccess().Level == AccessFull }
