//go:build !darwin && !linux

package completionproof

import (
	"errors"
	"os"
)

func openCompletionProof(string) (*os.File, error) {
	return nil, errors.New("safe completion proof capture is unsupported on this platform")
}
