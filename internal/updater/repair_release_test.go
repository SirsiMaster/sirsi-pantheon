package updater

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type repairReleaseTransport func(*http.Request) (*http.Response, error)

func (f repairReleaseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRepairReleaseSelectsOnlyExactRunningPublishedPair(t *testing.T) {
	for _, name := range []string{"exact", "unpublished", "wrong-tag", "incomplete", "oversize"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			client := &Client{ReleasesURL: "https://fixture.invalid/repos/owner/product/releases?per_page=10", HTTPClient: &http.Client{
				Transport: repairReleaseTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.Method != http.MethodGet || r.URL.Path != "/repos/owner/product/releases/tags/v0.24.200" || r.URL.RawQuery != "" {
						t.Fatal("not exact normal release selection", r.URL)
					}
					rel := commercialRelease("v0.24.200")
					if name == "wrong-tag" {
						rel.TagName = "v0.24.136"
					}
					if name == "incomplete" {
						rel.Assets = nil
					}
					raw, err := json.Marshal(rel)
					if err != nil {
						t.Fatal(err)
					}
					if name == "oversize" {
						raw = []byte(strings.Repeat(" ", (1<<20)+1))
					}
					status := http.StatusOK
					if name == "unpublished" {
						status = http.StatusNotFound
					}
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
				}),
			}}
			got, err := client.ReleaseForVersion("0.24.200")
			if calls != 1 || (err == nil) != (name == "exact") {
				t.Fatal(got, err, calls)
			}
		})
	}
}
