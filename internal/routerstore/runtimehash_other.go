//go:build !darwin

package routerstore

import "os"

// fileIdentity is unavailable off darwin: no key, so the hash is never cached
// (fail safe to hashing every process). ponytail: add linux Ctim when needed.
func fileIdentity(os.FileInfo) (ino, dev uint64, ctimeNs int64, ok bool) {
	return 0, 0, 0, false
}
