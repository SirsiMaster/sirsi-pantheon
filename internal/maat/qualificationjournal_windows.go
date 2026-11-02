//go:build windows

package maat

import "fmt"

func readLifecycleJournal(string) ([]byte, error) {
	return nil, fmt.Errorf("bounded complete lifecycle replay unavailable on this platform")
}
