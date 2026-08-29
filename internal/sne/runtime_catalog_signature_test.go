package sne

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignedRuntimeCatalogFailsClosedOnMutationAndWrongKey(t *testing.T) {
	root := t.TempDir()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	entry := RuntimePackage{
		ModelID: "gemma-test", PackageRoot: filepath.Join(root, "package"),
		RuntimeSHA256: strings.Repeat("a", 64), NativeRuntimeSHA256: strings.Repeat("b", 64),
		MLXDylibSHA256: strings.Repeat("c", 64), MetallibSHA256: strings.Repeat("d", 64),
		JACCLSHA256: strings.Repeat("e", 64),
	}
	catalogBytes, err := json.Marshal(RuntimePackageCatalog{SchemaVersion: RuntimePackageCatalogSchema, CatalogID: "signed", Entries: []RuntimePackage{entry}})
	if err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "runtime-packages.json")
	signaturePath := catalogPath + ".sig"
	publicKeyPath := catalogPath + ".pub"
	if err = os.WriteFile(catalogPath, catalogBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(signaturePath, []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, catalogBytes))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(publicKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadSignedRuntimePackageCatalog(catalogPath, signaturePath, publicKeyPath); err != nil {
		t.Fatal(err)
	}

	mutated := append(append([]byte(nil), catalogBytes...), '\n')
	if err = os.WriteFile(catalogPath, mutated, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadSignedRuntimePackageCatalog(catalogPath, signaturePath, publicKeyPath); err == nil || !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("mutated catalog was admitted: %v", err)
	}
	if err = os.WriteFile(catalogPath, catalogBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	wrongPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	wrongDER, _ := x509.MarshalPKIXPublicKey(wrongPublic)
	if err := os.WriteFile(publicKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: wrongDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSignedRuntimePackageCatalog(catalogPath, signaturePath, publicKeyPath); err == nil || !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("wrong trust root was admitted: %v", err)
	}
}

func TestRealSignedRuntimePackageCatalog(t *testing.T) {
	catalogPath := os.Getenv("SNE_REAL_SIGNED_RUNTIME_CATALOG")
	signaturePath := os.Getenv("SNE_REAL_SIGNED_RUNTIME_CATALOG_SIGNATURE")
	publicKeyPath := os.Getenv("SNE_REAL_SIGNED_RUNTIME_CATALOG_PUBLIC_KEY")
	if catalogPath == "" || signaturePath == "" || publicKeyPath == "" {
		t.Skip("real signed runtime catalog inputs not supplied")
	}
	for label, path := range map[string]string{"catalog": catalogPath, "signature": signaturePath, "public key": publicKeyPath} {
		if !filepath.IsAbs(path) {
			t.Fatalf("real signed runtime %s path must be absolute: %q", label, path)
		}
	}
	catalog, err := LoadSignedRuntimePackageCatalog(catalogPath, signaturePath, publicKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if packagesRoot := os.Getenv("SNE_REAL_SIGNED_RUNTIME_PACKAGES_ROOT"); packagesRoot != "" {
		catalog, err = catalog.MaterializePackageRoots(packagesRoot)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range catalog.Entries {
			if entry.PackageID == "" || entry.PackageRoot == "" {
				t.Fatalf("portable entry was not materialized: %+v", entry)
			}
		}
	}
	t.Logf("pantheon_sne_signed_catalog accepted=true catalog_id=%s entries=%d", catalog.CatalogID, len(catalog.Entries))
}
