//go:build darwin && cgo

package routerstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
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
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		t.Fatal(err)
	}
	policy := RelayObservationFilePolicy{UID: st.Uid, GID: st.Gid, Mode: 0600}
	if _, err := policy.validFile(int(replacement.Fd())); err == nil {
		t.Fatal("shared lifecycle file policy accepted an extra ACL")
	}
	if _, err := policy.validFile(int(f.Fd())); err != nil {
		t.Fatal("shared policy abandoned the retained no-ACL descriptor")
	}
}
