//go:build darwin

package main

import (
	"os"
	"testing"
)

// The ps-free lookup must agree with the kernel about our own parent, and an
// invalid pid must fail (both directions).
func TestKinfoAnchorProcessNeedsNoPS(t *testing.T) {
	t.Setenv("PATH", "") // no ps reachable: only the sysctl path can answer
	p, err := kinfoAnchorProcess(os.Getpid())
	if err != nil || p.parentPID != os.Getppid() || p.command == "" {
		t.Fatalf("kinfo self lookup: %+v err=%v (want ppid %d)", p, err, os.Getppid())
	}
	if _, err := kinfoAnchorProcess(2147483000); err == nil {
		t.Fatal("a nonexistent pid must not resolve")
	}
}

// With ps denied, the production lookup falls back to the kernel instead of
// failing registration.
func TestLookupAnchorProcessFallsBackWhenPSDenied(t *testing.T) {
	t.Setenv("PATH", "")
	p, err := lookupAnchorProcess(os.Getpid())
	if err != nil || p.parentPID != os.Getppid() {
		t.Fatalf("fallback lookup with no ps: %+v err=%v", p, err)
	}
}
