package desktoprecovery

import (
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestClaimDirectoryIdentityAndStatFailure(t *testing.T) {
	statErr := errors.New("stat denied")
	for _, tc := range []struct {
		name    string
		stat    unix.Stat_t
		statErr error
		wantErr bool
	}{
		{"private operator", unix.Stat_t{Mode: unix.S_IFDIR | 0o700, Uid: 501}, nil, false},
		{"different owner", unix.Stat_t{Mode: unix.S_IFDIR | 0o700, Uid: 502}, nil, true},
		{"group writable", unix.Stat_t{Mode: unix.S_IFDIR | 0o770, Uid: 501}, nil, true},
		{"world readable", unix.Stat_t{Mode: unix.S_IFDIR | 0o755, Uid: 501}, nil, true},
		{"setgid", unix.Stat_t{Mode: unix.S_IFDIR | 0o2700, Uid: 501}, nil, true},
		{"regular file", unix.Stat_t{Mode: unix.S_IFREG | 0o700, Uid: 501}, nil, true},
		{"stat failure", unix.Stat_t{}, statErr, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateClaimDirectory(42, func(fd int, stat *unix.Stat_t) error {
				if fd != 42 {
					t.Fatal("wrong descriptor")
				}
				*stat = tc.stat
				return tc.statErr
			}, 501)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error %v", err, tc.wantErr)
			}
			if tc.statErr != nil && !errors.Is(err, statErr) {
				t.Fatalf("stat error not wrapped: %v", err)
			}
		})
	}
}

func TestFileAdmissionStoreRejectsPermissionDriftWithoutClaim(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	store, openErr := NewFileAdmissionStore(root)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer store.Close()
	if err := os.Chmod(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileAdmissionStore(root); err == nil {
		t.Fatal("startup accepted shared directory")
	}
	id := sha256.Sum256([]byte("permission-drift"))
	if err := store.Claim(id, time.Now().Add(time.Minute)); err == nil {
		t.Fatal("admission accepted permission drift")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("denial created a claim: %v %v", entries, err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.Claim(id, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.Claim(id, time.Now().Add(time.Minute)); !errors.Is(err, ErrAdmissionAlreadyUsed) {
		t.Fatalf("replay accepted: %v", err)
	}
}
