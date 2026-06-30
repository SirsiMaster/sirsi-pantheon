package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestVerifyChecksum covers the CLI self-update authenticity gate: a matching
// digest passes, a mismatch is refused, and an asset absent from the manifest
// fails closed (never installed unverified).
func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	art := filepath.Join(dir, "sirsi-pantheon_x_darwin_arm64.tar.gz")
	if err := os.WriteFile(art, []byte("the-real-artifact-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("the-real-artifact-bytes"))
	good := hex.EncodeToString(sum[:])
	manifest := []byte(good + "  sirsi-pantheon_x_darwin_arm64.tar.gz\n" +
		"00deadbeef  other_asset.tar.gz\n")

	if err := VerifyChecksum(art, "sirsi-pantheon_x_darwin_arm64.tar.gz", manifest); err != nil {
		t.Fatalf("matching checksum should pass, got %v", err)
	}
	bad := []byte("0000000000000000000000000000000000000000000000000000000000000000  sirsi-pantheon_x_darwin_arm64.tar.gz\n")
	if err := VerifyChecksum(art, "sirsi-pantheon_x_darwin_arm64.tar.gz", bad); err == nil {
		t.Fatal("checksum mismatch must be refused")
	}
	if err := VerifyChecksum(art, "not-in-manifest.tar.gz", manifest); err == nil {
		t.Fatal("asset absent from manifest must fail closed")
	}
}

// TestChecksumsAsset finds the goreleaser checksums.txt asset and returns nil
// when it is absent (so the caller can fail closed).
func TestChecksumsAsset(t *testing.T) {
	rel := &Release{Assets: []Asset{
		{Name: "sirsi-pantheon_x_darwin_arm64.tar.gz"},
		{Name: "checksums.txt", BrowserDownloadURL: "https://example/checksums.txt"},
	}}
	if a := ChecksumsAsset(rel); a == nil || a.Name != "checksums.txt" {
		t.Fatalf("expected checksums.txt asset, got %+v", a)
	}
	if a := ChecksumsAsset(&Release{Assets: []Asset{{Name: "only.tar.gz"}}}); a != nil {
		t.Fatalf("expected nil when checksums.txt absent, got %+v", a)
	}
}

// TestDownload_SizeCap proves the download is bounded: a body over the cap is an
// error and leaves no partial file behind (disk-exhaustion defense).
func TestDownload_SizeCap(t *testing.T) {
	old := maxDownloadSize
	maxDownloadSize = 16
	defer func() { maxDownloadSize = old }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 1024)) // far over the 16-byte cap
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "artifact")
	if _, err := Download(srv.URL, dest); err == nil {
		t.Fatal("download over the size cap must error")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("an over-cap download must not leave a partial file")
	}
}

// TestDownload_UnderCap confirms a normal-sized body still succeeds.
func TestDownload_UnderCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("small-ok"))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "artifact")
	n, err := Download(srv.URL, dest)
	if err != nil || n != int64(len("small-ok")) {
		t.Fatalf("normal download should succeed: n=%d err=%v", n, err)
	}
}
