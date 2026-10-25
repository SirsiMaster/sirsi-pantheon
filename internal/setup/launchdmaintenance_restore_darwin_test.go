//go:build darwin && cgo

package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMaintenanceRestorationRetainsBackupAndRefusesTargetSubstitution(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "verified", true: "changed_before_publish"}[changed], func(t *testing.T) {
			path, parent := maintenanceOwnedDirectory(t)
			target := filepath.Join(path, "sirsi")
			old, newBytes := []byte("owned previous replacement"), []byte("owned approved replacement")
			if err := os.WriteFile(target, old, 0600); err != nil {
				t.Fatal(err)
			}
			current, _, err := maintenanceFileAt(parent, "sirsi", uint32(os.Getuid()))
			if err != nil {
				t.Fatal(err)
			}
			var st unix.Stat_t
			if err := unix.Fstat(int(parent.Fd()), &st); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(newBytes)
			p := MaintenanceRestorePlan{Transaction: strings.Repeat("a", 32), Nonce: strings.Repeat("b", 32), ParentDevice: uint64(st.Dev), ParentInode: st.Ino,
				ArtifactSHA256: hex.EncodeToString(hash[:]), TargetExists: current.Exists, TargetDevice: current.Device, TargetInode: current.Inode,
				TargetSize: current.Size, TargetMode: current.Mode, TargetUID: current.UID, TargetGID: current.GID, TargetSHA256: current.SHA256}
			verifications := 0
			verify := func(name string) error {
				verifications++
				if filepath.Dir(name) != path {
					t.Fatal("escaped target")
				}
				return nil
			}
			pub, err := prepareMaintenanceArtifactAt(context.Background(), path, uint32(os.Getuid()), uint32(os.Getgid()), p, newBytes, verify, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer pub.Close()
			record := pub.Prepared()
			backup, err := os.ReadFile(record.BackupPath)
			if err != nil || string(backup) != string(old) {
				t.Fatal("original backup lost", err)
			}
			if changed {
				replacement := filepath.Join(path, "unexpected")
				if err := os.WriteFile(replacement, []byte("unrelated newer replacement"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, target); err != nil {
					t.Fatal(err)
				}
			}
			err = pub.Publish(context.Background())
			if (err != nil) != changed {
				t.Fatal(err)
			}
			got, readErr := os.ReadFile(target)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if changed {
				if string(got) != "unrelated newer replacement" {
					t.Fatal("changed target overwritten")
				}
				if _, err := os.Stat(record.StagePath); err != nil {
					t.Fatal("stage debt lost", err)
				}
				if verifications != 1 {
					t.Fatal("verified uncommitted changed target", verifications)
				}
			} else {
				if string(got) != string(newBytes) || verifications != 2 {
					t.Fatal(string(got), verifications)
				}
				if _, err := os.Stat(record.StagePath); !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
			}
			if _, err := os.Stat(record.BackupPath); err != nil {
				t.Fatal("backup lost", err)
			}
		})
	}
}
