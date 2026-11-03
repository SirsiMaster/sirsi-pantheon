//go:build darwin && !cgo

package main

import "errors"

const aclInspectionAvailable = false

// Without the libSystem bridge the ACL cannot be verified; refuse the operation.
func queryACL(_ int, _ []byte) error {
	return errors.New("ACL inspection requires cgo")
}
