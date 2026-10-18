package routerstore

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The service compresses only for clients that ask, and the body round-trips. Go's default transport asks
// and decodes on its own, so the whole remote suite also runs over this path.
func TestGzippedOnlyWhenAskedAndRoundTrips(t *testing.T) {
	body := strings.Repeat(`{"id":"x","instructions":"long body text"}`, 2000)
	h := gzipped(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, body) })

	req := httptest.NewRequest(http.MethodPost, "/v1/call/ListSince", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("a client asking for gzip got an uncompressed response")
	}
	if rec.Body.Len() >= len(body)/4 {
		t.Fatalf("compressed %d bytes from %d: not compressed", rec.Body.Len(), len(body))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if !strings.Contains(string(got), "long body text") {
		t.Fatal("decompressed body lost its content")
	}

	plain := httptest.NewRecorder()
	h(plain, httptest.NewRequest(http.MethodPost, "/v1/call/ListSince", nil))
	if plain.Header().Get("Content-Encoding") != "" || !strings.Contains(plain.Body.String(), "long body text") {
		t.Fatal("a client that did not ask must get plain JSON")
	}
}
