//go:build darwin && !cgo

package knowledge

import "errors"

// Keep archive path types available to the Unix path layer while refusing
// Darwin archive setup before any SQLite VFS or file operation is attempted.
type archiveAuthority struct{}
type archiveVFS struct{}

func prepareDarwinSecureArchiveLocation(string) (*secureArchiveLocation, error) {
	return nil, errors.New("Ma'at knowledge archive is unavailable: Darwin's retained-root SQLite VFS requires cgo")
}

func (v *archiveVFS) Close() error { return errors.New("Darwin archive VFS requires cgo") }
func (a *archiveAuthority) release() error {
	return errors.New("Darwin archive authority requires cgo")
}
