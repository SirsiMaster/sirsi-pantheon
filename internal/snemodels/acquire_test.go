package snemodels

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireResumesAndVerifiesExactSource(t *testing.T) {
	content := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	digest := sha256.Sum256(content)
	var sawRange string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		sawRange = request.Header.Get("Range")
		if sawRange != "" {
			var offset int
			_, _ = fmt.Sscanf(sawRange, "bytes=%d-", &offset)
			writer.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(content)-1, len(content)))
			writer.WriteHeader(http.StatusPartialContent)
			_, _ = writer.Write(content[offset:])
			return
		}
		_, _ = writer.Write(content)
	}))
	defer server.Close()
	entry := SourceEntry{CatalogEntry: "test", Provider: "huggingface", Repository: "owner/repo", Revision: "revision", LicenseID: "terms", Files: []SourceFile{{Path: "model.bin", SHA256: fmt.Sprintf("%x", digest), SizeBytes: int64(len(content))}}}
	destination := t.TempDir()
	partial := filepath.Join(destination, "model.bin.partial")
	if err := os.WriteFile(partial, content[:10], 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Acquire(context.Background(), entry, AcquireOptions{Destination: destination, BaseURL: server.URL, Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if sawRange != "bytes=10-" || result.Bytes != int64(len(content)) {
		t.Fatalf("range=%q result=%+v", sawRange, result)
	}
	got, err := os.ReadFile(filepath.Join(destination, "model.bin"))
	if err != nil || string(got) != string(content) {
		t.Fatalf("download=%q error=%v", got, err)
	}
}

func TestAcquireRejectsCorruptionAndUnsafeTransport(t *testing.T) {
	content := []byte("wrong")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { _, _ = writer.Write(content) }))
	defer server.Close()
	entry := SourceEntry{CatalogEntry: "test", Provider: "huggingface", Repository: "owner/repo", Revision: "revision", LicenseID: "terms", Files: []SourceFile{{Path: "model.bin", SHA256: strings.Repeat("a", 64), SizeBytes: int64(len(content))}}}
	if _, err := Acquire(context.Background(), entry, AcquireOptions{Destination: t.TempDir(), BaseURL: server.URL, Client: server.Client()}); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("corruption error = %v", err)
	}
	if _, err := Acquire(context.Background(), entry, AcquireOptions{Destination: t.TempDir(), BaseURL: "http://example.com"}); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("transport error = %v", err)
	}
}
