package sne

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignedRuntimeCatalogStoreUpdateRollbackAndRemoval(t *testing.T) {
	root := t.TempDir()
	storeRoot := filepath.Join(root, "store")
	packagesRoot := filepath.Join(root, "packages")
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, _ := x509.MarshalPKIXPublicKey(publicKey)
	publicKeyPath := filepath.Join(root, "catalog.pub")
	if err := os.WriteFile(publicKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	writeCatalog := func(id, packageID string) (string, string, string) {
		t.Helper()
		catalog := RuntimePackageCatalog{
			SchemaVersion: RuntimePackageCatalogSchema, CatalogID: id,
			Entries: []RuntimePackage{{
				ModelID: "gemma-test", PackageID: packageID,
				RuntimeSHA256: strings.Repeat("a", 64), NativeRuntimeSHA256: strings.Repeat("b", 64),
				MLXDylibSHA256: strings.Repeat("c", 64), MetallibSHA256: strings.Repeat("d", 64), JACCLSHA256: strings.Repeat("e", 64),
			}},
		}
		data, _ := json.Marshal(catalog)
		digest := sha256.Sum256(data)
		version := hex.EncodeToString(digest[:])
		catalogPath := filepath.Join(root, id+".json")
		signaturePath := catalogPath + ".sig"
		if err := os.WriteFile(catalogPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
		signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, data)) + "\n"
		if err := os.WriteFile(signaturePath, []byte(signature), 0o600); err != nil {
			t.Fatal(err)
		}
		return catalogPath, signaturePath, version
	}

	catalog1, signature1, version1 := writeCatalog("v1", "package-v1")
	installed1, err := InstallSignedRuntimeCatalog(storeRoot, catalog1, signature1, publicKeyPath, packagesRoot)
	if err != nil || installed1.VersionSHA256 != version1 || installed1.PreviousSHA256 != "" {
		t.Fatalf("install v1=%+v err=%v", installed1, err)
	}
	catalog2, signature2, version2 := writeCatalog("v2", "package-v2")
	installed2, err := InstallSignedRuntimeCatalog(storeRoot, catalog2, signature2, publicKeyPath, packagesRoot)
	if err != nil || installed2.VersionSHA256 != version2 || installed2.PreviousSHA256 != version1 {
		t.Fatalf("install v2=%+v err=%v", installed2, err)
	}
	if current, err := CurrentRuntimeCatalogVersion(storeRoot); err != nil || current != version2 {
		t.Fatalf("current=%s err=%v", current, err)
	}
	if versions, err := ListRuntimeCatalogVersions(storeRoot); err != nil || len(versions) != 2 {
		t.Fatalf("versions=%v err=%v", versions, err)
	}
	if err := RollbackSignedRuntimeCatalog(storeRoot, version1, publicKeyPath, packagesRoot); err != nil {
		t.Fatal(err)
	}
	if current, _ := CurrentRuntimeCatalogVersion(storeRoot); current != version1 {
		t.Fatalf("rollback current=%s", current)
	}
	if err := RemoveInactiveRuntimeCatalog(storeRoot, version1); err == nil {
		t.Fatal("active catalog removal was accepted")
	}
	if err := RemoveInactiveRuntimeCatalog(storeRoot, version2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(storeRoot, runtimeCatalogVersionsDir, version2)); !os.IsNotExist(err) {
		t.Fatalf("inactive version survived removal: %v", err)
	}
	if versions, err := ListRuntimeCatalogVersions(storeRoot); err != nil || len(versions) != 1 || versions[0] != version1 {
		t.Fatalf("versions after removal=%v err=%v", versions, err)
	}

	data, _ := os.ReadFile(catalog2)
	data = append(data, '\n')
	if err := os.WriteFile(catalog2, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallSignedRuntimeCatalog(storeRoot, catalog2, signature2, publicKeyPath, packagesRoot); err == nil {
		t.Fatal("tampered catalog update was admitted")
	}
	if current, _ := CurrentRuntimeCatalogVersion(storeRoot); current != version1 {
		t.Fatalf("failed update changed current=%s", current)
	}
}
