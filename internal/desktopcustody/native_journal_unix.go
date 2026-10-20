//go:build unix

package desktopcustody

func OpenNativeJournal(root string) (SleepJournal, func() error, error) {
	j, err := NewFileSleepJournal(root)
	if err != nil {
		return nil, nil, err
	}
	return j, j.Close, nil
}
