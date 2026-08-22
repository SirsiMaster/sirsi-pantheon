package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func TestGenerateSignAndRefuseIdentityOverwrite(t *testing.T) {
	root := t.TempDir()
	privatePath := filepath.Join(root, "private", "catalog.pem")
	publicPath := filepath.Join(root, "trust", "catalog.pub")
	if err := generateKeyPair(privatePath, publicPath); err != nil {
		t.Fatal(err)
	}
	privateInfo, err := os.Stat(privatePath)
	if err != nil {
		t.Fatal(err)
	}
	if privateInfo.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode=%o", privateInfo.Mode().Perm())
	}
	if err := generateKeyPair(privatePath, publicPath); err == nil {
		t.Fatal("signing identity overwrite was accepted")
	}
	privateKey, _, err := loadPrivateKey(privatePath)
	if err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "runtime-packages.json")
	signaturePath := catalogPath + ".sig"
	catalog := []byte(`{"schema_version":"pantheon.sne-runtime-packages.v2","catalog_id":"test","entries":[]}`)
	if err := os.WriteFile(catalogPath, catalog, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(signaturePath, []byte(signatureText(privateKey, catalog)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sne.VerifyRuntimeCatalogSignature(catalogPath, signaturePath, publicPath); err != nil {
		t.Fatal(err)
	}
}
