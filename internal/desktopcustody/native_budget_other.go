//go:build !unix

package desktopcustody

import "errors"

func readNativeBudgetFile(string) ([]byte, error) {
	return nil, errors.New("native execution fact unavailable")
}
