//go:build !darwin && !linux

package completionproof

import (
	"errors"
	"os"
)

func openCompletionProof(string, string) (*os.File, error) {
	return nil, errors.New("safe completion proof capture is unsupported on this platform")
}

func exportValidator(string, []byte) error {
	return errors.New("safe validator export is unsupported on this platform")
}
