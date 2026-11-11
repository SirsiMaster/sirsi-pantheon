package routerstore

import (
	"os"
	"syscall"
)

// fileIdentity returns inode, device and ctime (ns) from the stat record.
func fileIdentity(fi os.FileInfo) (ino, dev uint64, ctimeNs int64, ok bool) {
	st, isStat := fi.Sys().(*syscall.Stat_t)
	if !isStat {
		return 0, 0, 0, false
	}
	return st.Ino, uint64(st.Dev), st.Ctimespec.Nano(), true
}
