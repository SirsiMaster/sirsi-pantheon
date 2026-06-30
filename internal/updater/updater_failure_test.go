package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// fetchNewestRelease is the network entry point for the whole update flow; its
// failure branches (non-200, empty list, malformed JSON) were untested.

func clientFor(url string) *Client {
	c := NewClient()
	c.ReleasesURL = url
	c.AdvisoryURL = url
	return c
}

func TestFetchNewestRelease_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := clientFor(srv.URL).fetchNewestRelease(); err == nil {
		t.Fatal("a non-200 GitHub response must be an error")
	}
}

func TestFetchNewestRelease_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()
	if _, err := clientFor(srv.URL).fetchNewestRelease(); err == nil {
		t.Fatal("an empty release list must be an error")
	}
}

func TestFetchNewestRelease_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{not-json"))
	}))
	defer srv.Close()
	if _, err := clientFor(srv.URL).fetchNewestRelease(); err == nil {
		t.Fatal("malformed JSON must be an error")
	}
}

// TestCheck_DevVersionNeverUpdates replaces a placebo test that asserted nothing.
// A "dev" build must never report an update available, even when the server
// advertises a newer release.
func TestCheck_DevVersionNeverUpdates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || bytesContains(r.URL.Path, "releases") {
			_ = json.NewEncoder(w).Encode([]Release{
				{TagName: "v9.9.9", Assets: []Asset{{Name: "sirsi-pantheon_9.9.9_darwin_arm64.tar.gz", BrowserDownloadURL: "https://example/x"}}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(AdvisoryFile{})
	}))
	defer srv.Close()

	res := clientFor(srv.URL).Check("dev")
	if res.Error != nil {
		t.Fatalf("Check returned error: %v", res.Error)
	}
	if res.UpdateAvailable {
		t.Fatal("a 'dev' build must never report UpdateAvailable=true")
	}
}

// TestExtractSirsiBinary_NeutralizesPathTraversal proves the doc-comment claim:
// a tar entry whose name tries to escape destDir is neutralized by filepath.Base,
// so the binary lands INSIDE destDir and nothing is written to the parent.
func TestExtractSirsiBinary_NeutralizesPathTraversal(t *testing.T) {
	dir := t.TempDir()
	tarball := filepath.Join(dir, "rel.tar.gz")
	writeTarGz(t, tarball, "../sirsi", []byte("the-binary")) // ../ tries to escape

	destDir := filepath.Join(dir, "dest")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := ExtractSirsiBinary(tarball, destDir)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if out != filepath.Join(destDir, "sirsi") {
		t.Fatalf("binary must land inside destDir, got %q", out)
	}
	// The traversal target (destDir's parent) must NOT have been written.
	if _, err := os.Stat(filepath.Join(dir, "sirsi")); err == nil {
		t.Fatalf("path traversal escaped destDir to %s", filepath.Join(dir, "sirsi"))
	}
}

func writeTarGz(t *testing.T, path, entryName string, content []byte) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: entryName, Typeflag: tar.TypeReg, Size: int64(len(content)), Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func bytesContains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }
