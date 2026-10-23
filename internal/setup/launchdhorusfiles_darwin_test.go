//go:build darwin

package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func horusPublicationFixture(t *testing.T) (string, []byte, []byte) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, HorusServiceLabel+".plist")
	before, after := []byte("original public plist bytes"), []byte("converted private plist bytes")
	if err := os.WriteFile(path, before, 0644); err != nil {
		t.Fatal(err)
	}
	return path, before, after
}

func TestHorusPublicationPreservesOriginalAndRestoresExactIdentity(t *testing.T) {
	path, before, after := horusPublicationFixture(t)
	tx, err := prepareHorusPublication(path, before, after)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Close()
	p := tx.(*horusFilePublication)
	assertBytes := func(path string, want []byte) {
		t.Helper()
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("incorrect retained/public bytes: %v", err)
		}
	}
	assertBytes(path, before)
	backup := filepath.Join(filepath.Dir(path), p.backup)
	assertBytes(backup, before)
	info, err := os.Lstat(backup)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("backup not private")
	}
	if err := tx.Publish(); err != nil {
		t.Fatal(err)
	}
	assertBytes(path, after)
	assertBytes(backup, before)
	if err := tx.Restore(); err != nil {
		t.Fatal(err)
	}
	assertBytes(path, before)
	assertBytes(backup, before)
	// Close never unlinks the stage or backup; explicit cleanup debt remains.
	if _, err := os.Lstat(filepath.Join(filepath.Dir(path), p.stage)); err != nil {
		t.Fatal("stage unexpectedly removed")
	}
}

func TestHorusPublicationOccupiedBackupAndConcurrentReplacement(t *testing.T) {
	for _, name := range []string{"occupied-backup", "source-before-publish", "stage-before-publish", "backup-before-publish", "source-before-rollback", "parent-substitution"} {
		t.Run(name, func(t *testing.T) {
			path, before, after := horusPublicationFixture(t)
			backup := filepath.Join(filepath.Dir(path), "."+HorusServiceLabel+"."+horusDigest(before)+".backup.plist")
			unknown := []byte("unknown replacement must remain")
			if name == "occupied-backup" {
				if err := os.WriteFile(backup, unknown, 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := prepareHorusPublication(path, before, after); err == nil {
					t.Fatal("occupied backup overwritten")
				}
				got, _ := os.ReadFile(backup)
				source, _ := os.ReadFile(path)
				if !bytes.Equal(got, unknown) || !bytes.Equal(source, before) {
					t.Fatal("occupied backup/source changed")
				}
				return
			}
			tx, err := prepareHorusPublication(path, before, after)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Close()
			p := tx.(*horusFilePublication)
			replace := func(target string) {
				t.Helper()
				leaf := target + ".replacement"
				if err := os.WriteFile(leaf, unknown, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(leaf, target); err != nil {
					t.Fatal(err)
				}
			}
			target := path
			switch name {
			case "stage-before-publish":
				target = filepath.Join(p.parent, p.stage)
			case "backup-before-publish":
				target = backup
			case "source-before-rollback":
				if err := tx.Publish(); err != nil {
					t.Fatal(err)
				}
			case "parent-substitution":
				moved := p.parent + ".retained"
				if err := os.Rename(p.parent, moved); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := os.Remove(p.parent); err != nil {
						t.Error(err)
					}
					if err := os.Rename(moved, p.parent); err != nil {
						t.Error(err)
					}
				})
				if err := os.Symlink(moved, p.parent); err != nil {
					t.Fatal(err)
				}
				if err := tx.Publish(); err == nil {
					t.Fatal("substituted parent accepted")
				}
				return
			}
			replace(target)
			if name == "source-before-rollback" {
				err = tx.Restore()
			} else {
				err = tx.Publish()
			}
			if err == nil {
				t.Fatal("substituted identity accepted")
			}
			got, _ := os.ReadFile(target)
			if !bytes.Equal(got, unknown) {
				t.Fatal("unknown replacement overwritten or deleted")
			}
		})
	}
}

func TestHorusPublicationRefusesSymlinkAndUnexpectedSource(t *testing.T) {
	for _, name := range []string{"wrong-source", "wrong-leaf", "source-symlink"} {
		t.Run(name, func(t *testing.T) {
			path, before, after := horusPublicationFixture(t)
			switch name {
			case "wrong-source":
				before = []byte("different planned bytes")
			case "wrong-leaf":
				path = filepath.Join(filepath.Dir(path), "other.plist")
			case "source-symlink":
				other := path + ".retained"
				if err := os.Rename(path, other); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, path); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := prepareHorusPublication(path, before, after); err == nil {
				t.Fatal("unsafe source accepted")
			}
		})
	}
}
