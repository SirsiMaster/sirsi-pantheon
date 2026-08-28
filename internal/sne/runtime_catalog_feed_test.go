package sne

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignedRuntimeCatalogFeedDownloadsAndInstallsExactVersion(t *testing.T) {
	root := t.TempDir()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, _ := x509.MarshalPKIXPublicKey(publicKey)
	publicKeyPath := filepath.Join(root, "catalog.pub")
	if err = os.WriteFile(publicKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	catalogBytes, _ := json.Marshal(RuntimePackageCatalog{SchemaVersion: RuntimePackageCatalogSchema, CatalogID: "remote-v1", Entries: []RuntimePackage{{
		ModelID: "gemma-test", PackageID: "package-v1", RuntimeSHA256: strings.Repeat("a", 64), NativeRuntimeSHA256: strings.Repeat("b", 64), MLXDylibSHA256: strings.Repeat("c", 64), MetallibSHA256: strings.Repeat("d", 64), JACCLSHA256: strings.Repeat("e", 64),
	}}})
	catalogDigest := sha256.Sum256(catalogBytes)
	version := hex.EncodeToString(catalogDigest[:])
	catalogSignature := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, catalogBytes)) + "\n")
	var feedBytes, feedSignature []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/feed.json":
			_, _ = writer.Write(feedBytes)
		case "/feed.json.sig":
			_, _ = writer.Write(feedSignature)
		case "/catalog.json":
			_, _ = writer.Write(catalogBytes)
		case "/catalog.json.sig":
			_, _ = writer.Write(catalogSignature)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	feedBytes, _ = json.Marshal(RuntimeCatalogFeed{SchemaVersion: RuntimeCatalogFeedSchema, FeedID: "stable", Entries: []RuntimeCatalogFeedEntry{{VersionSHA256: version, CatalogURL: server.URL + "/catalog.json", SignatureURL: server.URL + "/catalog.json.sig"}}})
	feedSignature = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, feedBytes)) + "\n")
	result, err := FetchAndInstallRuntimeCatalogUpdate(context.Background(), server.Client(), server.URL+"/feed.json", server.URL+"/feed.json.sig", publicKeyPath, filepath.Join(root, "store"), filepath.Join(root, "packages"), version)
	if err != nil || result.VersionSHA256 != version {
		t.Fatalf("install result=%+v err=%v", result, err)
	}
	if current, err := CurrentRuntimeCatalogVersion(filepath.Join(root, "store")); err != nil || current != version {
		t.Fatalf("current=%s err=%v", current, err)
	}

	feedBytes = append(feedBytes, '\n')
	if _, err := FetchAndInstallRuntimeCatalogUpdate(context.Background(), server.Client(), server.URL+"/feed.json", server.URL+"/feed.json.sig", publicKeyPath, filepath.Join(root, "store"), filepath.Join(root, "packages"), version); err == nil || !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("mutated feed was admitted: %v", err)
	}
}

func TestRealSignedRuntimeCatalogFeed(t *testing.T) {
	feedPath := os.Getenv("SNE_REAL_SIGNED_RUNTIME_CATALOG_FEED")
	signaturePath := os.Getenv("SNE_REAL_SIGNED_RUNTIME_CATALOG_FEED_SIGNATURE")
	publicKeyPath := os.Getenv("SNE_REAL_SIGNED_RUNTIME_CATALOG_PUBLIC_KEY")
	if feedPath == "" || signaturePath == "" || publicKeyPath == "" {
		t.Skip("real signed runtime catalog feed inputs not supplied")
	}
	for label, path := range map[string]string{"feed": feedPath, "signature": signaturePath, "public key": publicKeyPath} {
		if !filepath.IsAbs(path) {
			t.Fatalf("real signed runtime catalog %s path must be absolute: %q", label, path)
		}
	}
	feed, err := LoadSignedRuntimeCatalogFeed(feedPath, signaturePath, publicKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pantheon_sne_signed_feed accepted=true feed_id=%s entries=%d", feed.FeedID, len(feed.Entries))
}
