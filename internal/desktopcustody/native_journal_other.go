//go:build !unix

package desktopcustody

import "errors"

func OpenNativeJournal(string) (SleepJournal, func() error, error) {
	return nil, nil, errors.New("sleep custody: secure native journal unavailable on this platform")
}
