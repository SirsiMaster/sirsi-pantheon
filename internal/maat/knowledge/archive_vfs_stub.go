//go:build !darwin && !windows

package knowledge

import "errors"

type archiveAuthority struct{}
type archiveVFS struct{}

func prepareDarwinSecureArchiveLocation(string) (*secureArchiveLocation, error) {
	return nil, errors.New("Darwin archive VFS is unavailable")
}

func (v *archiveVFS) Close() error { return errors.New("archive VFS is unavailable") }
func (a *archiveAuthority) release() error {
	return errors.New("archive authority is unavailable")
}
