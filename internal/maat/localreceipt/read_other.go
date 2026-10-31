//go:build !unix

package localreceipt

import "errors"

func readFact(string) ([]byte, error) {
	return nil, errors.New("local admission fact unavailable on this platform")
}
