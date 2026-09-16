package routerstore

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A client with no SIRSI_RELAY_TRUST_GROUP on a group-trusted (setgid) spool
// must fail at once with the fix in the message — not wait 30 s for a relay
// that cannot enter its 0700 lane directory (SHA 2026-09-14 ctr hang).
func TestRefuseUntrustedClientOnTrustedSpool(t *testing.T) {
	spool := filepath.Join(t.TempDir(), "spool")
	if err := os.Mkdir(spool, 0o770); err != nil {
		t.Fatal(err)
	}
	if err := refuseUntrustedClientOnTrustedSpool(spool, -1); err != nil {
		t.Fatalf("single-uid spool (no setgid) must not be refused: %v", err)
	}
	if err := os.Chmod(spool, 0o770|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	err := refuseUntrustedClientOnTrustedSpool(spool, -1)
	if err == nil {
		t.Fatal("setgid spool + no trust group must be refused before publishing")
	}
	if !strings.Contains(err.Error(), spoolTrustGroupEnv+"=") {
		t.Fatalf("error must name the fix: %v", err)
	}
	if err := refuseUntrustedClientOnTrustedSpool(spool, 20); err != nil {
		t.Fatalf("a client WITH a trust group is never refused here: %v", err)
	}
	if err := refuseUntrustedClientOnTrustedSpool(filepath.Join(spool, "absent"), -1); err != nil {
		t.Fatalf("an absent spool is left to the existing path: %v", err)
	}
}

// A pre-existing 0700 lane directory that THIS uid owns is converged to the
// trust mode; a directory owned by someone else is still never chmod'd (rs-38).
func TestMkdirTrustedConvergesAnOwned0700Dir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "lane")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(dir)
	gid := int(st.Sys().(*syscall.Stat_t).Gid) // my own group: the trust check passes
	if err := mkdirTrusted(dir, 0o770, gid); err != nil {
		t.Fatalf("mkdirTrusted: %v", err)
	}
	st, _ = os.Stat(dir)
	if st.Mode().Perm() != 0o770 {
		t.Fatalf("owned 0700 lane dir not converged: got %o, want 0770", st.Mode().Perm())
	}
	// Negative control: no trust group configured → untouched, as before.
	other := filepath.Join(t.TempDir(), "lane2")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := mkdirTrusted(other, 0o770, -1); err != nil {
		t.Fatal(err)
	}
	st, _ = os.Stat(other)
	if st.Mode().Perm() != 0o700 {
		t.Fatalf("single-uid path must not widen a pre-existing dir: got %o", st.Mode().Perm())
	}
}
