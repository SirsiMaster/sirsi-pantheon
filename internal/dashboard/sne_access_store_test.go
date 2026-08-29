package dashboard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateSNELocalAccessTokenIsDurableAndPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", sneLocalAccessTokenFilename)
	first, err := LoadOrCreateSNELocalAccessToken(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateSNELocalAccessToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("token was not restart-stable")
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %o, want 600", info.Mode().Perm())
	}
	directory, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if directory.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %o, want 700", directory.Mode().Perm())
	}
}

func TestLoadOrCreateSNELocalAccessTokenRejectsUnsafeExistingState(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("abcdefghijklmnopqrstuvwxyz123456\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, "token-link")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateSNELocalAccessToken(symlink); err == nil {
		t.Fatal("symlink capability was admitted")
	}
	wide := filepath.Join(root, "wide-token")
	if err := os.WriteFile(wide, []byte("abcdefghijklmnopqrstuvwxyz123456\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateSNELocalAccessToken(wide); err == nil {
		t.Fatal("world-readable capability was admitted")
	}
	malformed := filepath.Join(root, "malformed")
	if err := os.WriteFile(malformed, []byte("short\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateSNELocalAccessToken(malformed); err == nil {
		t.Fatal("malformed capability was admitted")
	}
}

func TestRotateSNELocalAccessTokenAtomicallyRevokesPriorValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", sneLocalAccessTokenFilename)
	first, err := LoadOrCreateSNELocalAccessToken(path)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := RotateSNELocalAccessToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if rotated == first || rotated == "" {
		t.Fatal("rotation did not replace the capability")
	}
	loaded, err := LoadOrCreateSNELocalAccessToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != rotated {
		t.Fatal("rotated capability was not durable")
	}
}
