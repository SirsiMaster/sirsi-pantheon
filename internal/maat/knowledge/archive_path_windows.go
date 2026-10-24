//go:build windows

package knowledge

import (
	"errors"
)

type secureArchiveLocation struct {
	path    string
	vfsName string
}

func (p *secureArchiveLocation) sqlitePath() string { return p.path }

func userHomeDir() (string, error) { return "", errors.New("secure archive paths are unsupported") }

func prepareSecureArchiveLocation(string) (*secureArchiveLocation, error) {
	return nil, errors.New("Ma'at knowledge archive is unavailable: secure directory verification is unsupported on this platform")
}

func (*secureArchiveLocation) verify() error {
	return errors.New("Ma'at knowledge archive is unavailable: secure directory verification is unsupported on this platform")
}

func (p *secureArchiveLocation) verifyForSQLiteUse() error { return p.verify() }

func (*secureArchiveLocation) Close() error { return nil }

func readLegacyArchiveFile(string, int64) ([]byte, error) {
	return nil, errors.New("secure legacy reads are unsupported on this platform")
}
