//go:build darwin && !cgo

package main

import "testing"

func TestACLQueryWithoutCGORefuses(t *testing.T) {
	var buf [8192]byte
	if err := validateACLResponse(buf[:], queryACL(-1, buf[:])); err == nil {
		t.Fatal("no-cgo ACL query accepted unverifiable metadata")
	}
}
