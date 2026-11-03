//go:build darwin && cgo

package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestACLQueryInvalidDescriptorFailsClosed(t *testing.T) {
	var buf [8192]byte
	err := queryACL(-1, buf[:])
	if !errors.Is(err, syscall.EBADF) {
		t.Fatalf("invalid descriptor: got %v, want EBADF", err)
	}
	if validateACLResponse(buf[:], err) == nil {
		t.Fatal("failed ACL query was accepted")
	}
	if queryACL(-1, nil) == nil {
		t.Fatal("empty buffer was accepted")
	}
}

func TestACLQueryInertFile(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "fixture"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var buf [8192]byte
	if err := validateACLResponse(buf[:], queryACL(int(f.Fd()), buf[:])); err != nil {
		t.Fatalf("ACL query for inert fixture: %v", err)
	}
}
