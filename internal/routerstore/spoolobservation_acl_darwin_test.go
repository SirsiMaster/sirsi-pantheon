//go:build darwin && cgo

package routerstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRelayObservationDarwinACLRetainedDescriptor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned-empty-summary")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if !relayObservationNoACL(int(f.Fd())) {
		t.Fatal("owned file without extended ACL was refused")
	}
	if relayObservationNoACL(-1) {
		t.Fatal("invalid descriptor accepted")
	}
	if err := os.Rename(path, path+"-retained"); err != nil {
		t.Fatal(err)
	}
	replacement, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = replacement.Close() })
	// Only this empty owned fixture receives an ACL; no existing spool, user
	// file or production permission is touched. Parent remains private.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "/bin/chmod", "+a", "everyone allow read", path).Run(); err != nil {
		t.Fatalf("set owned fixture ACL: %v", err)
	}
	if relayObservationNoACL(int(replacement.Fd())) {
		t.Fatal("nonempty extended ACL accepted")
	}
	if !relayObservationNoACL(int(f.Fd())) {
		t.Fatal("probe followed replacement instead of retained descriptor")
	}
}
