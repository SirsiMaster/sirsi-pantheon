//go:build darwin

package platform

import (
	"path/filepath"
	"testing"
)

func TestTryLockMenubarLeaseAllowsExactlyOneHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "menubar.instance.lock")
	first, err := tryLockMenubarLease(path)
	if err != nil {
		t.Fatalf("first menubar lease: %v", err)
	}
	if _, secondErr := tryLockMenubarLease(path); secondErr == nil {
		t.Fatal("second menubar lease unexpectedly acquired")
	}
	first()
	second, err := tryLockMenubarLease(path)
	if err != nil {
		t.Fatalf("menubar lease after release: %v", err)
	}
	second()
}
